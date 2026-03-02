package main

import (
	"context"
	"os"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func setupTestAuthManager(t *testing.T) (*AuthManager, *bolt.DB, func()) {
	// Create temp database
	dbPath := "/tmp/test_auth_" + t.Name() + ".db"
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	config := AuthConfig{
		JWTSecret:     "test-secret-key-12345678901234567890",
		JWTExpiry:     24 * time.Hour,
		AdminUsername: "admin",
		AdminPassword: "changeme",
	}

	am := NewAuthManager(db, config)

	if err := am.EnsureBuckets(); err != nil {
		t.Fatalf("Failed to ensure buckets: %v", err)
	}

	cleanup := func() {
		_ = db.Close()
		_ = os.Remove(dbPath)
	}

	return am, db, cleanup
}

func TestNewAuthManager(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	if am == nil {
		t.Fatal("NewAuthManager returned nil")
	}

	if !am.IsEnabled() {
		t.Error("AuthManager should be enabled")
	}
}

func TestAuthManager_EnsureBuckets(t *testing.T) {
	_, db, cleanup := setupTestAuthManager(t)
	defer cleanup()

	// Verify buckets exist
	err := db.View(func(tx *bolt.Tx) error {
		if tx.Bucket([]byte("auth_users")) == nil {
			t.Error("auth_users bucket not created")
		}
		if tx.Bucket([]byte("auth_apikeys")) == nil {
			t.Error("auth_apikeys bucket not created")
		}
		if tx.Bucket([]byte("auth_permissions")) == nil {
			t.Error("auth_permissions bucket not created")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Failed to verify buckets: %v", err)
	}
}

func TestAuthManager_CreateUser(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	username := "testuser"
	password := "password123"

	// Create user
	user, err := am.CreateUser(username, password)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if user.Username != username {
		t.Errorf("Username = %s, want %s", user.Username, username)
	}

	if user.PasswordHash == "" {
		t.Error("PasswordHash should not be empty")
	}

	if user.PasswordHash == password {
		t.Error("Password should be hashed")
	}

	if user.Disabled {
		t.Error("User should not be disabled by default")
	}

	// Try to create duplicate user
	_, err = am.CreateUser(username, password)
	if err != ErrUserExists {
		t.Fatalf("CreateUser should fail with ErrUserExists for duplicate, got: %v", err)
	}
}

func TestAuthManager_CreateUser_EmptyFields(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	tests := []struct {
		name     string
		username string
		password string
	}{
		{"empty username", "", "password123"},
		{"empty password", "testuser", ""},
		{"both empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := am.CreateUser(tt.username, tt.password)
			if err == nil {
				t.Error("CreateUser should fail with empty fields")
			}
		})
	}
}

func TestAuthManager_Authenticate(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	username := "testuser"
	password := "password123"

	// Create user
	_, err := am.CreateUser(username, password)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Test correct password
	user, err := am.Authenticate(username, password)
	if err != nil {
		t.Fatalf("Authenticate failed with correct password: %v", err)
	}

	if user.Username != username {
		t.Errorf("Authenticated user = %s, want %s", user.Username, username)
	}

	// Test wrong password
	_, err = am.Authenticate(username, "wrongpassword")
	if err != ErrInvalidCredentials {
		t.Fatalf("Authenticate should fail with ErrInvalidCredentials, got: %v", err)
	}

	// Test non-existent user
	_, err = am.Authenticate("nonexistent", password)
	if err != ErrInvalidCredentials {
		t.Fatalf("Authenticate should fail for non-existent user, got: %v", err)
	}
}

func TestAuthManager_Authenticate_DisabledUser(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	username := "testuser"
	password := "password123"

	// Create and disable user
	_, err := am.CreateUser(username, password)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	err = am.DeleteUser(username)
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	// Should fail authentication
	_, err = am.Authenticate(username, password)
	if err != ErrInvalidCredentials {
		t.Fatalf("Authenticate should fail for disabled user, got: %v", err)
	}
}

func TestAuthManager_CreateAPIKey(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	username := "testuser"
	description := "Test API key"

	// Create user first
	_, err := am.CreateUser(username, "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create API key
	key, err := am.CreateAPIKey(username, description, 0)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	if key == "" {
		t.Error("CreateAPIKey should return a key")
	}

	if key[:10] != "mddb_live_" {
		t.Errorf("API key has wrong prefix: got %s, want mddb_live_", key[:10])
	}

	// Verify key can be validated
	keyUsername, err := am.ValidateAPIKey(key)
	if err != nil {
		t.Fatalf("ValidateAPIKey failed: %v", err)
	}

	if keyUsername != username {
		t.Errorf("ValidateAPIKey username = %s, want %s", keyUsername, username)
	}
}

func TestAuthManager_CreateAPIKey_NonExistentUser(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	_, err := am.CreateAPIKey("nonexistent", "Test", 0)
	if err == nil {
		t.Fatal("CreateAPIKey should fail for non-existent user")
	}
}

func TestAuthManager_ValidateAPIKey(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	username := "testuser"

	// Create user
	_, err := am.CreateUser(username, "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create API key
	key, err := am.CreateAPIKey(username, "Test", 0)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	// Test valid key
	resultUsername, err := am.ValidateAPIKey(key)
	if err != nil {
		t.Fatalf("ValidateAPIKey failed for valid key: %v", err)
	}

	if resultUsername != username {
		t.Errorf("ValidateAPIKey username = %s, want %s", resultUsername, username)
	}

	// Test invalid key
	_, err = am.ValidateAPIKey("mddb_live_invalid")
	if err != ErrAPIKeyNotFound {
		t.Fatalf("ValidateAPIKey should fail with ErrAPIKeyNotFound, got: %v", err)
	}

	// Test empty key
	_, err = am.ValidateAPIKey("")
	if err != ErrAPIKeyNotFound {
		t.Fatalf("ValidateAPIKey should fail for empty key, got: %v", err)
	}
}

func TestAuthManager_ValidateAPIKey_Expired(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	username := "testuser"

	// Create user
	_, err := am.CreateUser(username, "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create expired API key
	expiresAt := time.Now().Add(-1 * time.Second).Unix()
	key, err := am.CreateAPIKey(username, "Expired key", expiresAt)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	// Should fail validation
	_, err = am.ValidateAPIKey(key)
	if err != ErrAPIKeyExpired {
		t.Fatalf("ValidateAPIKey should fail with ErrAPIKeyExpired, got: %v", err)
	}
}

func TestAuthManager_SetPermission(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	username := "testuser"
	collection := "blog"

	// Create user
	_, err := am.CreateUser(username, "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Set permission
	perm := &Permission{
		Username:   username,
		Collection: collection,
		Read:       true,
		Write:      false,
		Admin:      false,
	}

	err = am.SetPermission(perm)
	if err != nil {
		t.Fatalf("SetPermission failed: %v", err)
	}

	// Verify permission
	perms := am.GetPermissions(username)
	if len(perms) != 1 {
		t.Fatalf("Expected 1 permission, got %d", len(perms))
	}

	if perms[0].Collection != collection {
		t.Errorf("Permission collection = %s, want %s", perms[0].Collection, collection)
	}

	if !perms[0].Read {
		t.Error("Permission should have read access")
	}

	if perms[0].Write {
		t.Error("Permission should not have write access")
	}
}

func TestAuthManager_SetPermission_Update(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	username := "testuser"
	collection := "blog"

	// Create user
	_, err := am.CreateUser(username, "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Set initial permission
	perm := &Permission{
		Username:   username,
		Collection: collection,
		Read:       true,
		Write:      false,
		Admin:      false,
	}
	if err := am.SetPermission(perm); err != nil {
		t.Fatalf("SetPermission failed: %v", err)
	}

	// Update permission
	perm.Write = true
	err = am.SetPermission(perm)
	if err != nil {
		t.Fatalf("SetPermission (update) failed: %v", err)
	}

	perms := am.GetPermissions(username)
	if !perms[0].Write {
		t.Error("Permission should have write access after update")
	}
}

func TestAuthManager_CheckPermission(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	username := "testuser"
	password := "password123"
	collection := "blog"

	// Create user
	_, err := am.CreateUser(username, password)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create context with claims
	claims := &JWTClaims{
		Username: username,
		Admin:    false,
	}
	ctx := context.WithValue(context.Background(), authContextKey, claims)

	// No permissions yet - should fail
	err = am.CheckPermission(ctx, collection, PermRead)
	if err != ErrForbidden {
		t.Fatalf("CheckPermission should fail with ErrForbidden, got: %v", err)
	}

	// Grant read permission
	perm := &Permission{
		Username:   username,
		Collection: collection,
		Read:       true,
		Write:      false,
		Admin:      false,
	}
	if err := am.SetPermission(perm); err != nil {
		t.Fatalf("SetPermission failed: %v", err)
	}

	// Load permissions into cache
	if err := am.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	// Should succeed for read
	err = am.CheckPermission(ctx, collection, PermRead)
	if err != nil {
		t.Errorf("CheckPermission should succeed for read: %v", err)
	}

	// Should fail for write
	err = am.CheckPermission(ctx, collection, PermWrite)
	if err != ErrForbidden {
		t.Fatalf("CheckPermission should fail for write, got: %v", err)
	}
}

func TestAuthManager_CheckPermission_Wildcard(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	username := "testuser"

	// Create user
	_, err := am.CreateUser(username, "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Grant wildcard read permission
	perm := &Permission{
		Username:   username,
		Collection: "*",
		Read:       true,
		Write:      false,
		Admin:      false,
	}
	if err := am.SetPermission(perm); err != nil {
		t.Fatalf("SetPermission failed: %v", err)
	}
	if err := am.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	claims := &JWTClaims{
		Username: username,
		Admin:    false,
	}
	ctx := context.WithValue(context.Background(), authContextKey, claims)

	// Should succeed for any collection
	collections := []string{"blog", "docs", "products"}
	for _, col := range collections {
		err := am.CheckPermission(ctx, col, PermRead)
		if err != nil {
			t.Errorf("CheckPermission should succeed for collection %s with wildcard: %v", col, err)
		}
	}
}

func TestAuthManager_CheckPermission_Admin(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	username := "admin"

	// Create user
	_, err := am.CreateUser(username, "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Grant admin permission
	perm := &Permission{
		Username:   username,
		Collection: "*",
		Read:       false,
		Write:      false,
		Admin:      true,
	}
	if err := am.SetPermission(perm); err != nil {
		t.Fatalf("SetPermission failed: %v", err)
	}
	if err := am.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	claims := &JWTClaims{
		Username: username,
		Admin:    true, // Admin flag in claims
	}
	ctx := context.WithValue(context.Background(), authContextKey, claims)

	// Admin should bypass all checks
	operations := []PermissionType{PermRead, PermWrite, PermAdmin}
	for _, op := range operations {
		err := am.CheckPermission(ctx, "blog", op)
		if err != nil {
			t.Errorf("CheckPermission should succeed for admin with operation %v: %v", op, err)
		}
	}
}

func TestAuthManager_IsAdmin(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	username := "testuser"

	// Create user
	_, err := am.CreateUser(username, "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Initially not admin
	if am.IsAdmin(username) {
		t.Error("User should not be admin initially")
	}

	// Grant admin permission
	perm := &Permission{
		Username:   username,
		Collection: "*",
		Read:       false,
		Write:      false,
		Admin:      true,
	}
	if err := am.SetPermission(perm); err != nil {
		t.Fatalf("SetPermission failed: %v", err)
	}
	if err := am.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	// Should be admin now
	if !am.IsAdmin(username) {
		t.Error("User should be admin after granting admin permission")
	}
}

func TestAuthManager_DeleteUser(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	username := "testuser"
	password := "password123"

	// Create user
	_, err := am.CreateUser(username, password)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Verify user exists and not disabled
	user, err := am.GetUser(username)
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}

	if user.Disabled {
		t.Error("User should not be disabled initially")
	}

	// Delete user
	err = am.DeleteUser(username)
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	// Verify user is disabled
	user, err = am.GetUser(username)
	if err != nil {
		t.Fatalf("GetUser failed after delete: %v", err)
	}

	if !user.Disabled {
		t.Error("User should be disabled after delete")
	}

	// Authentication should fail
	_, err = am.Authenticate(username, password)
	if err != ErrInvalidCredentials {
		t.Fatalf("Authenticate should fail for disabled user, got: %v", err)
	}
}

func TestAuthManager_BootstrapAdmin(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	adminUsername := am.config.AdminUsername
	adminPassword := am.config.AdminPassword

	// Bootstrap admin
	err := am.BootstrapAdmin()
	if err != nil {
		t.Fatalf("BootstrapAdmin failed: %v", err)
	}

	// Verify admin user exists
	user, err := am.GetUser(adminUsername)
	if err != nil {
		t.Fatalf("GetUser failed for admin: %v", err)
	}

	if user.Username != adminUsername {
		t.Errorf("Admin username = %s, want %s", user.Username, adminUsername)
	}

	// Verify admin can authenticate
	_, err = am.Authenticate(adminUsername, adminPassword)
	if err != nil {
		t.Fatalf("Admin authentication failed: %v", err)
	}

	// Load permissions
	if err := am.LoadAll(); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	// Verify admin has admin permission
	if !am.IsAdmin(adminUsername) {
		t.Error("Bootstrap admin should have admin permission")
	}

	// Bootstrap again - should be idempotent
	err = am.BootstrapAdmin()
	if err != nil {
		t.Errorf("BootstrapAdmin should be idempotent: %v", err)
	}
}

func TestAuthManager_GetUser_NotFound(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	_, err := am.GetUser("nonexistent")
	if err != ErrUserNotFound {
		t.Fatalf("GetUser should return ErrUserNotFound, got: %v", err)
	}
}

func TestAuthManager_CheckPermission_NoContext(t *testing.T) {
	am, _, cleanup := setupTestAuthManager(t)
	defer cleanup()

	ctx := context.Background() // No auth claims

	err := am.CheckPermission(ctx, "blog", PermRead)
	if err != ErrUnauthorized {
		t.Fatalf("CheckPermission should fail with ErrUnauthorized without context, got: %v", err)
	}
}
