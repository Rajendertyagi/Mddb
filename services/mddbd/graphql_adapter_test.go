package main

import (
	"context"
	"os"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// gqlAdapterTestServer creates a Server with an AuthManager for GraphQL adapter tests.
func gqlAdapterTestServer(t *testing.T) (*Server, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "gql_adapter_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}

	s := &Server{
		DB:   db,
		Path: f.Name(),
		Mode: ModeRW,
		BucketNames: BucketNames{
			Docs:    []byte("docs"),
			IdxMeta: []byte("idxmeta"),
			Rev:     []byte("rev"),
			ByKey:   []byte("bykey"),
		},
	}

	if err := s.ensureBuckets(); err != nil {
		_ = db.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}

	cleanup := func() {
		_ = db.Close()
		_ = os.Remove(f.Name())
	}
	return s, cleanup
}

// ---------------------------------------------------------------------------
// Test: NewGraphQLServerAdapter
// ---------------------------------------------------------------------------

func TestNewGraphQLServerAdapter(t *testing.T) {
	s, cleanup := gqlAdapterTestServer(t)
	defer cleanup()

	adapter := NewGraphQLServerAdapter(s)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}

	// adapter is already gql.ServerInterface — compile-time guarantee from NewGraphQLServerAdapter.
	_ = adapter
}

// ---------------------------------------------------------------------------
// Test: GetClaimsFromContext - no claims
// ---------------------------------------------------------------------------

func TestGQLAdapter_GetClaimsFromContext_NoClaims(t *testing.T) {
	s, cleanup := gqlAdapterTestServer(t)
	defer cleanup()

	adapter := NewGraphQLServerAdapter(s)
	claims, ok := adapter.GetClaimsFromContext(context.Background())
	if ok {
		t.Error("expected ok=false when no claims in context")
	}
	if claims.Username != "" {
		t.Errorf("expected empty username, got %s", claims.Username)
	}
}

// ---------------------------------------------------------------------------
// Test: GetClaimsFromContext - with claims
// ---------------------------------------------------------------------------

func TestGQLAdapter_GetClaimsFromContext_WithClaims(t *testing.T) {
	s, cleanup := gqlAdapterTestServer(t)
	defer cleanup()

	adapter := NewGraphQLServerAdapter(s)

	// Set claims in context
	ctx := context.WithValue(context.Background(), authContextKey, &JWTClaims{
		Username: "alice",
		Admin:    true,
	})

	claims, ok := adapter.GetClaimsFromContext(ctx)
	if !ok {
		t.Error("expected ok=true when claims in context")
	}
	if claims.Username != "alice" {
		t.Errorf("expected username=alice, got %s", claims.Username)
	}
	if !claims.Admin {
		t.Error("expected admin=true")
	}
}

// ---------------------------------------------------------------------------
// Test: CheckPermission - no AuthManager
// ---------------------------------------------------------------------------

func TestGQLAdapter_CheckPermission_NoAuth(t *testing.T) {
	s, cleanup := gqlAdapterTestServer(t)
	defer cleanup()

	s.AuthManager = nil
	adapter := NewGraphQLServerAdapter(s)

	err := adapter.CheckPermission(context.Background(), "blog", 0)
	if err != nil {
		t.Errorf("expected nil error when AuthManager is nil, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test: Authenticate - no AuthManager
// ---------------------------------------------------------------------------

func TestGQLAdapter_Authenticate_NoAuth(t *testing.T) {
	s, cleanup := gqlAdapterTestServer(t)
	defer cleanup()

	s.AuthManager = nil
	adapter := NewGraphQLServerAdapter(s)

	_, err := adapter.Authenticate("user", "pass")
	if err == nil {
		t.Error("expected error when AuthManager is nil")
	}
	if err != ErrAuthNotEnabled {
		t.Errorf("expected ErrAuthNotEnabled, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test: Authenticate - with AuthManager
// ---------------------------------------------------------------------------

func TestGQLAdapter_Authenticate_WithAuth(t *testing.T) {
	s, cleanup := gqlAdapterTestServer(t)
	defer cleanup()

	am := NewAuthManager(s.DB, AuthConfig{
		JWTSecret:     "testsecret",
		JWTExpiry:     time.Hour,
		AdminUsername: "admin",
		AdminPassword: "adminpass",
	})
	_ = am.EnsureBuckets()
	_ = am.BootstrapAdmin()
	_ = am.LoadAll()
	s.AuthManager = am

	adapter := NewGraphQLServerAdapter(s)

	info, err := adapter.Authenticate("admin", "adminpass")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if info.Username != "admin" {
		t.Errorf("expected username=admin, got %s", info.Username)
	}
}

func TestGQLAdapter_Authenticate_BadCredentials(t *testing.T) {
	s, cleanup := gqlAdapterTestServer(t)
	defer cleanup()

	am := NewAuthManager(s.DB, AuthConfig{
		JWTSecret:     "testsecret",
		JWTExpiry:     time.Hour,
		AdminUsername: "admin",
		AdminPassword: "adminpass",
	})
	_ = am.EnsureBuckets()
	_ = am.BootstrapAdmin()
	_ = am.LoadAll()
	s.AuthManager = am

	adapter := NewGraphQLServerAdapter(s)

	_, err := adapter.Authenticate("admin", "wrongpass")
	if err == nil {
		t.Error("expected error for bad credentials")
	}
}

// ---------------------------------------------------------------------------
// Test: GenerateJWT - no AuthManager
// ---------------------------------------------------------------------------

func TestGQLAdapter_GenerateJWT_NoAuth(t *testing.T) {
	s, cleanup := gqlAdapterTestServer(t)
	defer cleanup()

	s.AuthManager = nil
	adapter := NewGraphQLServerAdapter(s)

	_, _, err := adapter.GenerateJWT("user", false)
	if err == nil {
		t.Error("expected error when AuthManager is nil")
	}
	if err != ErrAuthNotEnabled {
		t.Errorf("expected ErrAuthNotEnabled, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test: GenerateJWT - with AuthManager
// ---------------------------------------------------------------------------

func TestGQLAdapter_GenerateJWT_WithAuth(t *testing.T) {
	s, cleanup := gqlAdapterTestServer(t)
	defer cleanup()

	am := NewAuthManager(s.DB, AuthConfig{
		JWTSecret:     "testsecret",
		JWTExpiry:     time.Hour,
		AdminUsername: "admin",
		AdminPassword: "adminpass",
	})
	_ = am.EnsureBuckets()
	_ = am.BootstrapAdmin()
	_ = am.LoadAll()
	s.AuthManager = am

	adapter := NewGraphQLServerAdapter(s)

	token, expiresAt, err := adapter.GenerateJWT("admin", true)
	if err != nil {
		t.Fatalf("GenerateJWT: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
	if expiresAt <= time.Now().Unix() {
		t.Error("expected expiresAt to be in the future")
	}
}

// ---------------------------------------------------------------------------
// Test: Stub methods return "not yet implemented"
// ---------------------------------------------------------------------------

func TestGQLAdapter_GetDocument_NotImplemented(t *testing.T) {
	s, cleanup := gqlAdapterTestServer(t)
	defer cleanup()

	adapter := NewGraphQLServerAdapter(s)
	_, err := adapter.GetDocument(context.Background(), "blog", "key", "en", nil)
	if err == nil {
		t.Error("expected error")
	}
}

func TestGQLAdapter_SearchDocuments_NotImplemented(t *testing.T) {
	s, cleanup := gqlAdapterTestServer(t)
	defer cleanup()

	adapter := NewGraphQLServerAdapter(s)
	_, err := adapter.SearchDocuments(context.Background(), "blog", nil, "key", true, 10, 0)
	if err == nil {
		t.Error("expected error")
	}
}

func TestGQLAdapter_AddDocument_NotImplemented(t *testing.T) {
	s, cleanup := gqlAdapterTestServer(t)
	defer cleanup()

	adapter := NewGraphQLServerAdapter(s)
	_, _, err := adapter.AddDocument(context.Background(), "blog", "key", "en", nil, "content", 0)
	if err == nil {
		t.Error("expected error")
	}
}

func TestGQLAdapter_DeleteDocument_NotImplemented(t *testing.T) {
	s, cleanup := gqlAdapterTestServer(t)
	defer cleanup()

	adapter := NewGraphQLServerAdapter(s)
	err := adapter.DeleteDocument(context.Background(), "blog", "key", "en")
	if err == nil {
		t.Error("expected error")
	}
}

func TestGQLAdapter_GetStats_NotImplemented(t *testing.T) {
	s, cleanup := gqlAdapterTestServer(t)
	defer cleanup()

	adapter := NewGraphQLServerAdapter(s)
	_, err := adapter.GetStats(context.Background())
	if err == nil {
		t.Error("expected error")
	}
}

func TestGQLAdapter_VectorSearch_NotImplemented(t *testing.T) {
	s, cleanup := gqlAdapterTestServer(t)
	defer cleanup()

	adapter := NewGraphQLServerAdapter(s)
	_, err := adapter.VectorSearch(context.Background(), "blog", "query", nil, 5, 0.5, nil, false)
	if err == nil {
		t.Error("expected error")
	}
}
