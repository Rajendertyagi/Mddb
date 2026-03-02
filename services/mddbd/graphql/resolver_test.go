package graphql

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Mock ServerInterface for testing
type mockServer struct {
	authenticateFn    func(username, password string) (UserInfo, error)
	generateJWTFn     func(username string, isAdmin bool) (string, int64, error)
	getClaimsFn       func(ctx context.Context) (Claims, bool)
	checkPermissionFn func(ctx context.Context, collection string, perm int) error
	getDocumentFn     func(ctx context.Context, collection, key, lang string, env map[string]string) (interface{}, error)
	deleteDocumentFn  func(ctx context.Context, collection, key, lang string) error
}

func (m *mockServer) Authenticate(username, password string) (UserInfo, error) {
	if m.authenticateFn != nil {
		return m.authenticateFn(username, password)
	}
	return UserInfo{}, errors.New("not implemented")
}

func (m *mockServer) GenerateJWT(username string, isAdmin bool) (string, int64, error) {
	if m.generateJWTFn != nil {
		return m.generateJWTFn(username, isAdmin)
	}
	return "", 0, errors.New("not implemented")
}

func (m *mockServer) GetClaimsFromContext(ctx context.Context) (Claims, bool) {
	if m.getClaimsFn != nil {
		return m.getClaimsFn(ctx)
	}
	return Claims{}, false
}

func (m *mockServer) CheckPermission(ctx context.Context, collection string, perm int) error {
	if m.checkPermissionFn != nil {
		return m.checkPermissionFn(ctx, collection, perm)
	}
	return nil
}

func (m *mockServer) GetDocument(ctx context.Context, collection, key, lang string, env map[string]string) (interface{}, error) {
	if m.getDocumentFn != nil {
		return m.getDocumentFn(ctx, collection, key, lang, env)
	}
	return nil, errors.New("not implemented")
}

func (m *mockServer) SearchDocuments(ctx context.Context, collection string, filterMeta map[string][]string, sort string, asc bool, limit, offset int) (interface{}, error) {
	return nil, errors.New("not yet implemented - use REST API")
}

func (m *mockServer) AddDocument(ctx context.Context, collection, key, lang string, meta map[string][]string, contentMd string, ttl int64) (interface{}, bool, error) {
	return nil, false, errors.New("not yet implemented - use REST API")
}

func (m *mockServer) DeleteDocument(ctx context.Context, collection, key, lang string) error {
	if m.deleteDocumentFn != nil {
		return m.deleteDocumentFn(ctx, collection, key, lang)
	}
	return errors.New("not implemented")
}

func (m *mockServer) GetStats(ctx context.Context) (interface{}, error) {
	return nil, errors.New("not yet implemented - use REST API")
}

func (m *mockServer) VectorSearch(ctx context.Context, collection, query string, queryVector []float32, topK int, threshold float64, filterMeta map[string][]string, includeContent bool) (interface{}, error) {
	return nil, errors.New("not yet implemented - use REST API")
}

// Tests

func TestLogin_Success(t *testing.T) {
	mock := &mockServer{
		authenticateFn: func(username, password string) (UserInfo, error) {
			if username == "admin" && password == "secret" {
				return UserInfo{
					Username:  "admin",
					Admin:     true,
					CreatedAt: time.Now().Unix(),
				}, nil
			}
			return UserInfo{}, errors.New("invalid credentials")
		},
		generateJWTFn: func(username string, isAdmin bool) (string, int64, error) {
			return "mock-jwt-token", time.Now().Add(24 * time.Hour).Unix(), nil
		},
	}

	resolver := &Resolver{server: mock}
	mutResolver := &mutationResolver{resolver}

	result, err := mutResolver.Login(context.Background(), "admin", "secret")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.Token != "mock-jwt-token" {
		t.Errorf("Expected token 'mock-jwt-token', got %s", result.Token)
	}

	if result.ExpiresAt <= time.Now().Unix() {
		t.Errorf("Expected future expiration time, got %d", result.ExpiresAt)
	}
}

func TestLogin_InvalidCredentials(t *testing.T) {
	mock := &mockServer{
		authenticateFn: func(username, password string) (UserInfo, error) {
			return UserInfo{}, errors.New("invalid credentials")
		},
	}

	resolver := &Resolver{server: mock}
	mutResolver := &mutationResolver{resolver}

	_, err := mutResolver.Login(context.Background(), "admin", "wrongpass")
	if err == nil {
		t.Fatal("Expected error for invalid credentials, got nil")
	}

	if err.Error() != "authentication failed: invalid credentials" {
		t.Errorf("Expected 'authentication failed: invalid credentials', got %s", err.Error())
	}
}

func TestDeleteDocument_Success(t *testing.T) {
	mock := &mockServer{
		checkPermissionFn: func(ctx context.Context, collection string, perm int) error {
			// Allow delete (write permission = 1)
			if perm == 1 {
				return nil
			}
			return errors.New("permission denied")
		},
		deleteDocumentFn: func(ctx context.Context, collection, key, lang string) error {
			if collection == "blog" && key == "post1" && lang == "en" {
				return nil
			}
			return errors.New("not found")
		},
	}

	resolver := &Resolver{server: mock}
	mutResolver := &mutationResolver{resolver}

	result, err := mutResolver.DeleteDocument(context.Background(), "blog", "post1", "en")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !result {
		t.Error("Expected true, got false")
	}
}

func TestDeleteDocument_PermissionDenied(t *testing.T) {
	mock := &mockServer{
		checkPermissionFn: func(ctx context.Context, collection string, perm int) error {
			return errors.New("insufficient permissions")
		},
	}

	resolver := &Resolver{server: mock}
	mutResolver := &mutationResolver{resolver}

	_, err := mutResolver.DeleteDocument(context.Background(), "blog", "post1", "en")
	if err == nil {
		t.Fatal("Expected permission error, got nil")
	}

	if err.Error() != "permission denied: insufficient permissions" {
		t.Errorf("Expected permission error, got %s", err.Error())
	}
}

func TestDeleteDocument_NotFound(t *testing.T) {
	mock := &mockServer{
		checkPermissionFn: func(ctx context.Context, collection string, perm int) error {
			return nil
		},
		deleteDocumentFn: func(ctx context.Context, collection, key, lang string) error {
			return errors.New("document not found")
		},
	}

	resolver := &Resolver{server: mock}
	mutResolver := &mutationResolver{resolver}

	result, err := mutResolver.DeleteDocument(context.Background(), "blog", "nonexistent", "en")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if result {
		t.Error("Expected false when error occurs, got true")
	}
}

func TestMapMetaInputToInternal(t *testing.T) {
	tests := []struct {
		name     string
		input    []*MetaInput
		expected map[string][]string
	}{
		{
			name: "single meta",
			input: []*MetaInput{
				{Key: "author", Values: []string{"John"}},
			},
			expected: map[string][]string{
				"author": {"John"},
			},
		},
		{
			name: "multiple meta",
			input: []*MetaInput{
				{Key: "author", Values: []string{"John"}},
				{Key: "tags", Values: []string{"tutorial", "graphql"}},
			},
			expected: map[string][]string{
				"author": {"John"},
				"tags":   {"tutorial", "graphql"},
			},
		},
		{
			name:     "empty input",
			input:    []*MetaInput{},
			expected: map[string][]string{},
		},
		{
			name: "nil value in slice",
			input: []*MetaInput{
				{Key: "author", Values: []string{"John"}},
				nil,
			},
			expected: map[string][]string{
				"author": {"John"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapMetaInputToInternal(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d keys, got %d", len(tt.expected), len(result))
			}

			for key, expectedVals := range tt.expected {
				actualVals, ok := result[key]
				if !ok {
					t.Errorf("Missing key %s in result", key)
					continue
				}

				if len(actualVals) != len(expectedVals) {
					t.Errorf("For key %s: expected %d values, got %d", key, len(expectedVals), len(actualVals))
					continue
				}

				for i, expectedVal := range expectedVals {
					if actualVals[i] != expectedVal {
						t.Errorf("For key %s at index %d: expected %s, got %s", key, i, expectedVal, actualVals[i])
					}
				}
			}
		})
	}
}
