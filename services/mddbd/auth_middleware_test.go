package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// authMwSetup creates an AuthManager with a test DB for middleware tests.
func authMwSetup(t *testing.T) (*AuthManager, func()) {
	t.Helper()
	dbPath := "/tmp/test_auth_mw_" + t.Name() + ".db"
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("bolt.Open: %v", err)
	}

	config := AuthConfig{
		JWTSecret:     "mw-test-secret-key-12345",
		JWTExpiry:     24 * time.Hour,
		AdminUsername: "admin",
		AdminPassword: "adminpass",
	}

	am := NewAuthManager(db, config)
	if err := am.EnsureBuckets(); err != nil {
		_ = db.Close()
		_ = os.Remove(dbPath)
		t.Fatalf("EnsureBuckets: %v", err)
	}
	if err := am.BootstrapAdmin(); err != nil {
		_ = db.Close()
		_ = os.Remove(dbPath)
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	if err := am.LoadAll(); err != nil {
		_ = db.Close()
		_ = os.Remove(dbPath)
		t.Fatalf("LoadAll: %v", err)
	}

	cleanup := func() {
		_ = db.Close()
		_ = os.Remove(dbPath)
	}
	return am, cleanup
}

// authMwDummyHandler returns a simple handler that responds 200 OK.
func authMwDummyHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
}

func TestAuthMiddleware_Disabled(t *testing.T) {
	am, cleanup := authMwSetup(t)
	defer cleanup()

	// Disable auth
	am.enabled = false

	handler := am.HTTPMiddleware(authMwDummyHandler())

	req := httptest.NewRequest("GET", "/v1/docs/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 when auth disabled, got %d", w.Code)
	}
}

func TestAuthMiddleware_PublicEndpoints(t *testing.T) {
	am, cleanup := authMwSetup(t)
	defer cleanup()

	handler := am.HTTPMiddleware(authMwDummyHandler())

	endpoints := []string{"/health", "/v1/health", "/v1/auth/login", "/metrics"}
	for _, ep := range endpoints {
		req := httptest.NewRequest("GET", ep, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("endpoint %q: expected 200, got %d", ep, w.Code)
		}
	}
}

func TestAuthMiddleware_NoAuth(t *testing.T) {
	am, cleanup := authMwSetup(t)
	defer cleanup()

	handler := am.HTTPMiddleware(authMwDummyHandler())

	req := httptest.NewRequest("GET", "/v1/docs/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with no auth, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthMiddleware_ValidBearerToken(t *testing.T) {
	am, cleanup := authMwSetup(t)
	defer cleanup()

	token, err := GenerateJWT("admin", true, am.config.JWTSecret, am.config.JWTExpiry)
	if err != nil {
		t.Fatalf("GenerateJWT: %v", err)
	}

	handler := am.HTTPMiddleware(authMwDummyHandler())

	req := httptest.NewRequest("GET", "/v1/docs/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	am, cleanup := authMwSetup(t)
	defer cleanup()

	handler := am.HTTPMiddleware(authMwDummyHandler())

	req := httptest.NewRequest("GET", "/v1/docs/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-value")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", w.Code)
	}
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	am, cleanup := authMwSetup(t)
	defer cleanup()

	// Generate an already-expired token
	token, err := GenerateJWT("admin", true, am.config.JWTSecret, -1*time.Hour)
	if err != nil {
		t.Fatalf("GenerateJWT: %v", err)
	}

	handler := am.HTTPMiddleware(authMwDummyHandler())

	req := httptest.NewRequest("GET", "/v1/docs/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", w.Code)
	}
}

func TestAuthMiddleware_ValidAPIKey(t *testing.T) {
	am, cleanup := authMwSetup(t)
	defer cleanup()

	// Create an API key for admin
	apiKey, err := am.CreateAPIKey("admin", "test key", 0)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	handler := am.HTTPMiddleware(authMwDummyHandler())

	req := httptest.NewRequest("GET", "/v1/docs/test", nil)
	req.Header.Set("X-API-Key", apiKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid API key, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthMiddleware_InvalidAPIKey(t *testing.T) {
	am, cleanup := authMwSetup(t)
	defer cleanup()

	handler := am.HTTPMiddleware(authMwDummyHandler())

	req := httptest.NewRequest("GET", "/v1/docs/test", nil)
	req.Header.Set("X-API-Key", "invalid-api-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid API key, got %d", w.Code)
	}
}

func TestAuthMiddleware_DisabledUser(t *testing.T) {
	am, cleanup := authMwSetup(t)
	defer cleanup()

	// Create and disable a user
	_, err := am.CreateUser("disabled-user", "pass123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := am.DeleteUser("disabled-user"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	token, err := GenerateJWT("disabled-user", false, am.config.JWTSecret, am.config.JWTExpiry)
	if err != nil {
		t.Fatalf("GenerateJWT: %v", err)
	}

	handler := am.HTTPMiddleware(authMwDummyHandler())

	req := httptest.NewRequest("GET", "/v1/docs/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for disabled user, got %d", w.Code)
	}
}

func TestAuthMiddleware_NonexistentUser(t *testing.T) {
	am, cleanup := authMwSetup(t)
	defer cleanup()

	// Token for a user that doesn't exist in the database
	token, err := GenerateJWT("ghost", false, am.config.JWTSecret, am.config.JWTExpiry)
	if err != nil {
		t.Fatalf("GenerateJWT: %v", err)
	}

	handler := am.HTTPMiddleware(authMwDummyHandler())

	req := httptest.NewRequest("GET", "/v1/docs/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for nonexistent user, got %d", w.Code)
	}
}

func TestAuthMiddleware_WrongSecret(t *testing.T) {
	am, cleanup := authMwSetup(t)
	defer cleanup()

	// Token signed with different secret
	token, err := GenerateJWT("admin", true, "different-secret-key", am.config.JWTExpiry)
	if err != nil {
		t.Fatalf("GenerateJWT: %v", err)
	}

	handler := am.HTTPMiddleware(authMwDummyHandler())

	req := httptest.NewRequest("GET", "/v1/docs/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong secret, got %d", w.Code)
	}
}

func TestAuthMiddleware_MalformedAuthHeader(t *testing.T) {
	am, cleanup := authMwSetup(t)
	defer cleanup()

	handler := am.HTTPMiddleware(authMwDummyHandler())

	tests := []struct {
		name string
		auth string
	}{
		{"just-token", "some-token-without-bearer"},
		{"empty-bearer", "Bearer "},
		{"basic-auth", "Basic dXNlcjpwYXNz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/v1/docs/test", nil)
			req.Header.Set("Authorization", tt.auth)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", w.Code)
			}
		})
	}
}

func TestAuthMiddleware_ContextContainsClaims(t *testing.T) {
	am, cleanup := authMwSetup(t)
	defer cleanup()

	token, err := GenerateJWT("admin", true, am.config.JWTSecret, am.config.JWTExpiry)
	if err != nil {
		t.Fatalf("GenerateJWT: %v", err)
	}

	var capturedClaims *JWTClaims
	handler := am.HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaimsFromContext(r.Context())
		if ok {
			capturedClaims = claims
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/v1/docs/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if capturedClaims == nil {
		t.Fatal("expected claims in context")
	}
	if capturedClaims.Username != "admin" {
		t.Errorf("Username = %q, want admin", capturedClaims.Username)
	}
	if !capturedClaims.Admin {
		t.Error("expected Admin to be true")
	}
}

// ---- Helper function tests ----

func TestExtractTokenFromRequest_Bearer(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer my-token-123")
	token := extractTokenFromRequest(req)
	if token != "my-token-123" {
		t.Errorf("got %q, want my-token-123", token)
	}
}

func TestExtractTokenFromRequest_BearerCaseInsensitive(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "bearer my-token-123")
	token := extractTokenFromRequest(req)
	if token != "my-token-123" {
		t.Errorf("got %q, want my-token-123", token)
	}
}

func TestExtractTokenFromRequest_Empty(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	token := extractTokenFromRequest(req)
	if token != "" {
		t.Errorf("got %q, want empty", token)
	}
}

func TestExtractTokenFromRequest_NoBearerPrefix(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "just-a-token")
	token := extractTokenFromRequest(req)
	if token != "" {
		t.Errorf("got %q, want empty (no bearer prefix)", token)
	}
}

func TestExtractTokenFromRequest_BasicAuth(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	token := extractTokenFromRequest(req)
	if token != "" {
		t.Errorf("got %q, want empty (Basic auth)", token)
	}
}

func TestIsPublicEndpoint(t *testing.T) {
	tests := []struct {
		path   string
		public bool
	}{
		{"/health", true},
		{"/v1/health", true},
		{"/v1/auth/login", true},
		{"/metrics", true},
		{"/v1/docs/test", false},
		{"/v1/collections", false},
		{"/v1/auth/register", false},
		{"/unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isPublicEndpoint(tt.path)
			if got != tt.public {
				t.Errorf("isPublicEndpoint(%q) = %v, want %v", tt.path, got, tt.public)
			}
		})
	}
}
