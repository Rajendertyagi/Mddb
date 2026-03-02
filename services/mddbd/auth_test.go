package main

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestHashPassword(t *testing.T) {
	password := "testPassword123"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash == "" {
		t.Fatal("HashPassword returned empty hash")
	}

	if hash == password {
		t.Fatal("HashPassword returned unhashed password")
	}

	// Should produce different hashes for same password (due to salt)
	hash2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed on second call: %v", err)
	}

	if hash == hash2 {
		t.Fatal("HashPassword should produce different hashes due to random salt")
	}
}

func TestVerifyPassword(t *testing.T) {
	password := "testPassword123"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	// Test correct password
	if err := VerifyPassword(password, hash); err != nil {
		t.Fatal("VerifyPassword failed to verify correct password")
	}

	// Test wrong password
	if err := VerifyPassword("wrongPassword", hash); err == nil {
		t.Fatal("VerifyPassword incorrectly verified wrong password")
	}

	// Test empty password
	if err := VerifyPassword("", hash); err == nil {
		t.Fatal("VerifyPassword incorrectly verified empty password")
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key1, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}

	// Check format
	expectedLen := len("mddb_live_") + 48
	if len(key1) != expectedLen {
		t.Fatalf("API key has wrong length: got %d, want %d", len(key1), expectedLen)
	}

	if key1[:10] != "mddb_live_" {
		t.Fatalf("API key has wrong prefix: got %s, want mddb_live_", key1[:10])
	}

	// Check uniqueness
	key2, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey failed on second call: %v", err)
	}

	if key1 == key2 {
		t.Fatal("GenerateAPIKey produced duplicate keys")
	}
}

func TestHashAPIKey(t *testing.T) {
	key := "mddb_live_abc123def456"
	hash := HashAPIKey(key)

	if hash == "" {
		t.Fatal("HashAPIKey returned empty hash")
	}

	if hash == key {
		t.Fatal("HashAPIKey returned unhashed key")
	}

	// SHA256 produces 64 hex characters
	if len(hash) != 64 {
		t.Fatalf("HashAPIKey produced wrong length: got %d, want 64", len(hash))
	}

	// Should be deterministic
	hash2 := HashAPIKey(key)
	if hash != hash2 {
		t.Fatal("HashAPIKey should be deterministic")
	}
}

func TestPermission_HasPermission(t *testing.T) {
	tests := []struct {
		name       string
		perm       Permission
		operation  PermissionType
		wantResult bool
	}{
		{
			name:       "read permission granted",
			perm:       Permission{Read: true, Write: false, Admin: false},
			operation:  PermRead,
			wantResult: true,
		},
		{
			name:       "write permission granted",
			perm:       Permission{Read: false, Write: true, Admin: false},
			operation:  PermWrite,
			wantResult: true,
		},
		{
			name:       "admin permission granted",
			perm:       Permission{Read: false, Write: false, Admin: true},
			operation:  PermAdmin,
			wantResult: true,
		},
		{
			name:       "read permission denied",
			perm:       Permission{Read: false, Write: true, Admin: false},
			operation:  PermRead,
			wantResult: false,
		},
		{
			name:       "write permission denied",
			perm:       Permission{Read: true, Write: false, Admin: false},
			operation:  PermWrite,
			wantResult: false,
		},
		{
			name:       "admin permission denied",
			perm:       Permission{Read: true, Write: true, Admin: false},
			operation:  PermAdmin,
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.perm.HasPermission(tt.operation)
			if result != tt.wantResult {
				t.Errorf("HasPermission() = %v, want %v", result, tt.wantResult)
			}
		})
	}
}

func TestAPIKey_IsExpired(t *testing.T) {
	now := time.Now().Unix()

	tests := []struct {
		name      string
		expiresAt int64
		want      bool
	}{
		{
			name:      "never expires",
			expiresAt: 0,
			want:      false,
		},
		{
			name:      "expires in future",
			expiresAt: now + 3600,
			want:      false,
		},
		{
			name:      "expired in past",
			expiresAt: now - 3600,
			want:      true,
		},
		{
			name:      "expires now (edge case)",
			expiresAt: now,
			want:      false, // "> expiresAt" means it's not expired yet
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := APIKey{ExpiresAt: tt.expiresAt}
			if got := key.IsExpired(); got != tt.want {
				t.Errorf("APIKey.IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateJWT(t *testing.T) {
	secret := "test-secret-key-12345"
	username := "testuser"
	isAdmin := true
	expiry := 24 * time.Hour

	token, err := GenerateJWT(username, isAdmin, secret, expiry)
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	if token == "" {
		t.Fatal("GenerateJWT returned empty token")
	}

	// Token should have 3 parts separated by dots (header.payload.signature)
	parts := 0
	for _, c := range token {
		if c == '.' {
			parts++
		}
	}
	if parts != 2 {
		t.Fatalf("JWT should have 3 parts separated by 2 dots, got %d dots", parts)
	}
}

func TestValidateJWT(t *testing.T) {
	secret := "test-secret-key-12345"
	username := "testuser"
	isAdmin := true
	expiry := 24 * time.Hour

	token, err := GenerateJWT(username, isAdmin, secret, expiry)
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	// Test valid token
	claims, err := ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("ValidateJWT failed for valid token: %v", err)
	}

	if claims.Username != username {
		t.Errorf("ValidateJWT username = %s, want %s", claims.Username, username)
	}

	if claims.Admin != isAdmin {
		t.Errorf("ValidateJWT admin = %v, want %v", claims.Admin, isAdmin)
	}

	// Test invalid secret
	_, err = ValidateJWT(token, "wrong-secret")
	if err == nil {
		t.Fatal("ValidateJWT should fail with wrong secret")
	}

	// Test expired token
	expiredToken, err := GenerateJWT(username, isAdmin, secret, -1*time.Hour)
	if err != nil {
		t.Fatalf("GenerateJWT failed for expired token: %v", err)
	}

	_, err = ValidateJWT(expiredToken, secret)
	if err == nil {
		t.Fatal("ValidateJWT should fail for expired token")
	}

	// Test malformed token
	_, err = ValidateJWT("invalid.token.here", secret)
	if err == nil {
		t.Fatal("ValidateJWT should fail for malformed token")
	}

	// Test empty token
	_, err = ValidateJWT("", secret)
	if err == nil {
		t.Fatal("ValidateJWT should fail for empty token")
	}
}

func TestJWTClaimsRoundTrip(t *testing.T) {
	secret := "test-secret-key-12345"
	username := "alice"
	isAdmin := false
	expiry := 1 * time.Hour

	// Generate token
	token, err := GenerateJWT(username, isAdmin, secret, expiry)
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	// Validate and extract claims
	claims, err := ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("ValidateJWT failed: %v", err)
	}

	// Verify all fields
	if claims.Username != username {
		t.Errorf("Username mismatch: got %s, want %s", claims.Username, username)
	}

	if claims.Admin != isAdmin {
		t.Errorf("Admin mismatch: got %v, want %v", claims.Admin, isAdmin)
	}

	// Check expiry is in the future
	if time.Now().Unix() >= claims.ExpiresAt.Unix() {
		t.Error("Token should not be expired yet")
	}

	// Check expiry is approximately correct (within 1 minute tolerance)
	expectedExpiry := time.Now().Add(expiry).Unix()
	actualExpiry := claims.ExpiresAt.Unix()
	diff := expectedExpiry - actualExpiry
	if diff < -60 || diff > 60 {
		t.Errorf("Expiry time mismatch: got %d, want ~%d (diff: %d)", actualExpiry, expectedExpiry, diff)
	}
}

func TestValidateJWT_HS256Algorithm(t *testing.T) {
	secret := "test-secret-key-12345"
	username := "testuser"

	// Create token
	token, err := GenerateJWT(username, false, secret, 1*time.Hour)
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	// Parse manually to verify it's HS256
	parsedToken, _ := jwt.ParseWithClaims(token, &JWTClaims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})

	if parsedToken.Method.Alg() != "HS256" {
		t.Errorf("Token should use HS256, got %s", parsedToken.Method.Alg())
	}
}

func TestGetClaimsFromContext(t *testing.T) {
	// Test with claims in context
	claims := &JWTClaims{
		Username: "testuser",
		Admin:    false,
	}

	ctx := context.WithValue(context.Background(), authContextKey, claims)

	result, ok := GetClaimsFromContext(ctx)
	if !ok {
		t.Fatal("GetClaimsFromContext should return true for valid context")
	}

	if result.Username != "testuser" {
		t.Errorf("Username = %s, want testuser", result.Username)
	}

	// Test without claims in context
	emptyCtx := context.Background()
	result, ok = GetClaimsFromContext(emptyCtx)
	if ok {
		t.Error("GetClaimsFromContext should return false for empty context")
	}

	if result != nil {
		t.Error("GetClaimsFromContext should return nil for empty context")
	}
}
