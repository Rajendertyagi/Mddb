package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidTenantName(t *testing.T) {
	valid := []string{"", "acme", "acme-corp", "Tenant_1", "a", "0-9_AZ"}
	for _, name := range valid {
		if !ValidTenantName(name) {
			t.Errorf("ValidTenantName(%q) = false, want true", name)
		}
	}
	invalid := []string{"acme/corp", "a b", "ten|ant", "a*", ".", "acme!",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} // 65 chars
	for _, name := range invalid {
		if ValidTenantName(name) {
			t.Errorf("ValidTenantName(%q) = true, want false", name)
		}
	}
}

func TestCollectionInTenant(t *testing.T) {
	cases := []struct {
		tenant, collection string
		want               bool
	}{
		{"", "anything", true},       // global scope owns everything
		{"acme", "acme/notes", true}, // inside namespace
		{"acme", "acme/a/b", true},   // nested names allowed
		{"acme", "acme", false},      // bare tenant name is not a collection
		{"acme", "acme/", false},     // empty collection part
		{"acme", "acmeX/notes", false},
		{"acme", "other/notes", false},
		{"acme", "notes", false},
	}
	for _, c := range cases {
		if got := CollectionInTenant(c.tenant, c.collection); got != c.want {
			t.Errorf("CollectionInTenant(%q, %q) = %v, want %v", c.tenant, c.collection, got, c.want)
		}
	}
}

func TestGenerateTenantJWTStripsAdmin(t *testing.T) {
	secret := "test-secret-key-12345678901234567890"

	// Tenant user: admin flag must be stripped even when passed as true
	token, err := GenerateTenantJWT("alice", "acme", true, secret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateTenantJWT: %v", err)
	}
	claims, err := ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("ValidateJWT: %v", err)
	}
	if claims.Admin {
		t.Error("tenant claims must never carry the global admin flag")
	}
	if claims.Tenant != "acme" {
		t.Errorf("Tenant = %q, want acme", claims.Tenant)
	}

	// Global user: admin flag preserved
	token, err = GenerateJWT("root", true, secret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateJWT: %v", err)
	}
	claims, err = ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("ValidateJWT: %v", err)
	}
	if !claims.Admin || claims.Tenant != "" {
		t.Errorf("global admin claims corrupted: admin=%v tenant=%q", claims.Admin, claims.Tenant)
	}
}

func TestCreateTenantUser(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	user, err := am.CreateTenantUser("alice", "password123", "acme")
	if err != nil {
		t.Fatalf("CreateTenantUser: %v", err)
	}
	if user.Tenant != "acme" {
		t.Errorf("Tenant = %q, want acme", user.Tenant)
	}
	if got := am.UserTenant("alice"); got != "acme" {
		t.Errorf("UserTenant(alice) = %q, want acme", got)
	}
	if got := am.UserTenant("nonexistent"); got != "" {
		t.Errorf("UserTenant(nonexistent) = %q, want empty", got)
	}

	if _, err := am.CreateTenantUser("bob", "password123", "bad/tenant"); err != ErrInvalidTenant {
		t.Errorf("invalid tenant accepted, err = %v", err)
	}

	// Tenant survives reload from disk
	if err := am.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if got := am.UserTenant("alice"); got != "acme" {
		t.Errorf("UserTenant(alice) after reload = %q, want acme", got)
	}
}

// tenantCtx builds a context carrying claims for the given user/tenant.
func tenantCtx(username, tenant string, admin bool) context.Context {
	claims := &JWTClaims{Username: username, Admin: admin, Tenant: tenant}
	return context.WithValue(context.Background(), authContextKey, claims)
}

func TestCheckPermissionTenantIsolation(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	if _, err := am.CreateTenantUser("alice", "password123", "acme"); err != nil {
		t.Fatalf("CreateTenantUser: %v", err)
	}
	// Tenant-wide wildcard: alice is a "tenant admin" of acme
	if err := am.SetPermission(&Permission{Username: "alice", Collection: "*", Read: true, Write: true}); err != nil {
		t.Fatalf("SetPermission: %v", err)
	}

	ctx := tenantCtx("alice", "acme", false)

	// Own namespace: allowed
	if err := am.CheckPermission(ctx, "acme/notes", PermRead); err != nil {
		t.Errorf("read own tenant collection: %v", err)
	}
	if err := am.CheckPermission(ctx, "acme/notes", PermWrite); err != nil {
		t.Errorf("write own tenant collection: %v", err)
	}

	// Foreign namespace and global collections: denied
	for _, coll := range []string{"other/notes", "notes", "acme"} {
		if err := am.CheckPermission(ctx, coll, PermRead); err != ErrForbidden {
			t.Errorf("read %q: err = %v, want ErrForbidden", coll, err)
		}
	}

	// "*" scope: read allowed (listing, handler filters), write/admin denied
	if err := am.CheckPermission(ctx, "*", PermRead); err != nil {
		t.Errorf("wildcard read scope: %v", err)
	}
	if err := am.CheckPermission(ctx, "*", PermWrite); err != ErrForbidden {
		t.Errorf("wildcard write: err = %v, want ErrForbidden", err)
	}
	if err := am.CheckPermission(ctx, "*", PermAdmin); err != ErrForbidden {
		t.Errorf("wildcard admin: err = %v, want ErrForbidden", err)
	}

	// A forged admin flag on tenant claims must not cross the namespace gate
	forged := tenantCtx("alice", "acme", true)
	if err := am.CheckPermission(forged, "other/notes", PermRead); err != ErrForbidden {
		t.Errorf("forged admin crossed tenant gate: err = %v, want ErrForbidden", err)
	}

	// Global admin remains unrestricted
	root := tenantCtx("root", "", true)
	if err := am.CheckPermission(root, "acme/notes", PermWrite); err != nil {
		t.Errorf("global admin denied: %v", err)
	}
	if err := am.CheckPermission(root, "anything", PermAdmin); err != nil {
		t.Errorf("global admin denied: %v", err)
	}
}

func TestCheckPermissionTenantSpecificGrant(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	if _, err := am.CreateTenantUser("bob", "password123", "acme"); err != nil {
		t.Fatalf("CreateTenantUser: %v", err)
	}
	// Read-only grant on a single collection inside the tenant
	if err := am.SetPermission(&Permission{Username: "bob", Collection: "acme/docs", Read: true}); err != nil {
		t.Fatalf("SetPermission: %v", err)
	}

	ctx := tenantCtx("bob", "acme", false)

	if err := am.CheckPermission(ctx, "acme/docs", PermRead); err != nil {
		t.Errorf("granted read denied: %v", err)
	}
	if err := am.CheckPermission(ctx, "acme/docs", PermWrite); err != ErrForbidden {
		t.Errorf("ungranted write allowed: err = %v", err)
	}
	if err := am.CheckPermission(ctx, "acme/other", PermRead); err != ErrForbidden {
		t.Errorf("ungranted collection allowed: err = %v", err)
	}
}

func TestHTTPMiddlewareCarriesTenant(t *testing.T) {
	am, cleanup := authMwSetup(t)
	defer cleanup()

	if _, err := am.CreateTenantUser("carol", "password123", "acme"); err != nil {
		t.Fatalf("CreateTenantUser: %v", err)
	}
	key, err := am.CreateAPIKey("carol", "test", 0)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	var got *JWTClaims
	handler := am.HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = GetClaimsFromContext(r.Context())
	}))

	req := httptest.NewRequest("POST", "/v1/get", nil)
	req.Header.Set("X-API-Key", key)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got == nil {
		t.Fatal("no claims injected")
	}
	if got.Tenant != "acme" {
		t.Errorf("claims.Tenant = %q, want acme", got.Tenant)
	}
	if got.Admin {
		t.Error("tenant API key must not yield admin claims")
	}
}
