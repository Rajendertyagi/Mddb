package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	json "github.com/goccy/go-json"
	bolt "go.etcd.io/bbolt"
)

var (
	bucketAuthUsers   = []byte("auth_users")
	bucketAuthAPIKeys = []byte("auth_apikeys")
	bucketAuthPerms   = []byte("auth_permissions")
)

// AuthManager manages authentication and authorization
type AuthManager struct {
	db      *bolt.DB
	config  AuthConfig
	enabled bool

	// In-memory caches
	mu          sync.RWMutex
	users       map[string]*User
	apiKeys     map[string]*APIKey       // keyHash -> APIKey
	permissions map[string][]*Permission // username -> permissions
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(db *bolt.DB, config AuthConfig) *AuthManager {
	return &AuthManager{
		db:          db,
		config:      config,
		enabled:     true,
		users:       make(map[string]*User),
		apiKeys:     make(map[string]*APIKey),
		permissions: make(map[string][]*Permission),
	}
}

// IsEnabled returns whether auth is enabled
func (am *AuthManager) IsEnabled() bool {
	return am.enabled
}

// EnsureBuckets creates auth buckets if they don't exist
func (am *AuthManager) EnsureBuckets() error {
	return am.db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketAuthUsers); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketAuthAPIKeys); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketAuthPerms); err != nil {
			return err
		}
		return nil
	})
}

// LoadAll loads all auth data from database into memory
func (am *AuthManager) LoadAll() error {
	var users []*User
	var apiKeys []*APIKey
	var permissions []*Permission

	err := am.db.View(func(tx *bolt.Tx) error {
		// Load users
		b := tx.Bucket(bucketAuthUsers)
		if b != nil {
			if err := b.ForEach(func(k, v []byte) error {
				var u User
				if err := json.Unmarshal(v, &u); err != nil {
					return nil // skip corrupt entries
				}
				users = append(users, &u)
				return nil
			}); err != nil {
				return err
			}
		}

		// Load API keys
		b = tx.Bucket(bucketAuthAPIKeys)
		if b != nil {
			if err := b.ForEach(func(k, v []byte) error {
				var key APIKey
				if err := json.Unmarshal(v, &key); err != nil {
					return nil // skip corrupt entries
				}
				apiKeys = append(apiKeys, &key)
				return nil
			}); err != nil {
				return err
			}
		}

		// Load permissions
		b = tx.Bucket(bucketAuthPerms)
		if b != nil {
			if err := b.ForEach(func(k, v []byte) error {
				var perm Permission
				if err := json.Unmarshal(v, &perm); err != nil {
					return nil // skip corrupt entries
				}
				permissions = append(permissions, &perm)
				return nil
			}); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Update in-memory caches
	am.mu.Lock()
	am.users = make(map[string]*User)
	am.apiKeys = make(map[string]*APIKey)
	am.permissions = make(map[string][]*Permission)

	for _, u := range users {
		am.users[u.Username] = u
	}

	for _, k := range apiKeys {
		am.apiKeys[k.KeyHash] = k
	}

	for _, p := range permissions {
		am.permissions[p.Username] = append(am.permissions[p.Username], p)
	}

	am.mu.Unlock()
	return nil
}

// BootstrapAdmin creates initial admin user if it doesn't exist
func (am *AuthManager) BootstrapAdmin() error {
	if am.config.AdminUsername == "" || am.config.AdminPassword == "" {
		log.Println("No admin credentials configured, skipping bootstrap")
		return nil
	}

	// Check if admin already exists
	am.mu.RLock()
	_, exists := am.users[am.config.AdminUsername]
	am.mu.RUnlock()

	if exists {
		log.Printf("Admin user '%s' already exists", am.config.AdminUsername)
		return nil
	}

	// Create admin user
	user, err := am.CreateUser(am.config.AdminUsername, am.config.AdminPassword)
	if err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	}

	// Grant admin permissions (wildcard collection)
	perm := &Permission{
		Username:   user.Username,
		Collection: "*",
		Read:       true,
		Write:      true,
		Admin:      true,
	}

	if err := am.SetPermission(perm); err != nil {
		return fmt.Errorf("bootstrap admin permissions: %w", err)
	}

	log.Printf("✓ Admin user '%s' created successfully", am.config.AdminUsername)
	return nil
}

// ---- User management ----

// CreateUser creates a new user
func (am *AuthManager) CreateUser(username, password string) (*User, error) {
	if username == "" || password == "" {
		return nil, errors.New("username and password required")
	}

	// Check if user exists
	am.mu.RLock()
	if _, exists := am.users[username]; exists {
		am.mu.RUnlock()
		return nil, ErrUserExists
	}
	am.mu.RUnlock()

	// Hash password
	passwordHash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &User{
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now().Unix(),
		Disabled:     false,
	}

	// Save to database
	data, _ := json.Marshal(user)
	if err := am.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAuthUsers)
		return b.Put([]byte("user|"+username), data)
	}); err != nil {
		return nil, err
	}

	// Update cache
	am.mu.Lock()
	am.users[username] = user
	am.mu.Unlock()

	return user, nil
}

// GetUser retrieves a user by username
func (am *AuthManager) GetUser(username string) (*User, error) {
	am.mu.RLock()
	user, exists := am.users[username]
	am.mu.RUnlock()

	if !exists {
		return nil, ErrUserNotFound
	}

	return user, nil
}

// DeleteUser soft-deletes a user
func (am *AuthManager) DeleteUser(username string) error {
	am.mu.RLock()
	user, exists := am.users[username]
	am.mu.RUnlock()

	if !exists {
		return ErrUserNotFound
	}

	user.Disabled = true

	// Update database
	data, _ := json.Marshal(user)
	if err := am.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAuthUsers)
		return b.Put([]byte("user|"+username), data)
	}); err != nil {
		return err
	}

	// Update cache
	am.mu.Lock()
	am.users[username] = user
	am.mu.Unlock()

	return nil
}

// Authenticate validates username and password
func (am *AuthManager) Authenticate(username, password string) (*User, error) {
	user, err := am.GetUser(username)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if user.Disabled {
		return nil, ErrInvalidCredentials
	}

	if err := VerifyPassword(password, user.PasswordHash); err != nil {
		return nil, ErrInvalidCredentials
	}

	return user, nil
}

// ---- API Key management ----

// CreateAPIKey creates a new API key for a user
func (am *AuthManager) CreateAPIKey(username, description string, expiresAt int64) (string, error) {
	// Verify user exists
	if _, err := am.GetUser(username); err != nil {
		return "", err
	}

	// Generate API key
	key, err := GenerateAPIKey()
	if err != nil {
		return "", err
	}

	keyHash := HashAPIKey(key)

	apiKey := &APIKey{
		KeyHash:     keyHash,
		Username:    username,
		CreatedAt:   time.Now().Unix(),
		ExpiresAt:   expiresAt,
		Description: description,
	}

	// Save to database
	data, _ := json.Marshal(apiKey)
	if err := am.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAuthAPIKeys)
		return b.Put([]byte("apikey|"+keyHash), data)
	}); err != nil {
		return "", err
	}

	// Update cache
	am.mu.Lock()
	am.apiKeys[keyHash] = apiKey
	am.mu.Unlock()

	return key, nil // Return plaintext key (only shown once)
}

// ValidateAPIKey validates an API key and returns the username
func (am *AuthManager) ValidateAPIKey(key string) (string, error) {
	keyHash := HashAPIKey(key)

	am.mu.RLock()
	apiKey, exists := am.apiKeys[keyHash]
	am.mu.RUnlock()

	if !exists {
		return "", ErrAPIKeyNotFound
	}

	if apiKey.IsExpired() {
		return "", ErrAPIKeyExpired
	}

	return apiKey.Username, nil
}

// ---- Permission management ----

// SetPermission sets permissions for a user on a collection
func (am *AuthManager) SetPermission(perm *Permission) error {
	// Verify user exists
	if _, err := am.GetUser(perm.Username); err != nil {
		return err
	}

	// Save to database
	key := fmt.Sprintf("perm|%s|%s", perm.Username, perm.Collection)
	data, _ := json.Marshal(perm)
	if err := am.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAuthPerms)
		return b.Put([]byte(key), data)
	}); err != nil {
		return err
	}

	// Update cache
	am.mu.Lock()
	// Remove existing permission for this collection
	filtered := []*Permission{}
	for _, p := range am.permissions[perm.Username] {
		if p.Collection != perm.Collection {
			filtered = append(filtered, p)
		}
	}
	filtered = append(filtered, perm)
	am.permissions[perm.Username] = filtered
	am.mu.Unlock()

	return nil
}

// GetPermissions returns all permissions for a user
func (am *AuthManager) GetPermissions(username string) []*Permission {
	am.mu.RLock()
	defer am.mu.RUnlock()

	perms := am.permissions[username]
	result := make([]*Permission, len(perms))
	copy(result, perms)
	return result
}

// CheckPermission checks if user has permission for operation on collection
func (am *AuthManager) CheckPermission(ctx context.Context, collection string, operation PermissionType) error {
	if !am.enabled {
		return nil // Auth disabled = allow all
	}

	claims, ok := GetClaimsFromContext(ctx)
	if !ok {
		return ErrUnauthorized
	}

	// Admins bypass all checks
	if claims.Admin {
		return nil
	}

	am.mu.RLock()
	defer am.mu.RUnlock()

	perms := am.permissions[claims.Username]

	// Check collection-specific permission
	for _, p := range perms {
		if p.Collection == collection && p.HasPermission(operation) {
			return nil
		}
	}

	// Check wildcard permission
	for _, p := range perms {
		if p.Collection == "*" && p.HasPermission(operation) {
			return nil
		}
	}

	return ErrForbidden
}

// IsAdmin checks if user has admin privileges
func (am *AuthManager) IsAdmin(username string) bool {
	am.mu.RLock()
	defer am.mu.RUnlock()

	perms := am.permissions[username]
	for _, p := range perms {
		if p.Admin {
			return true
		}
	}

	return false
}
