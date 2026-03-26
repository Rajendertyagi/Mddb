package main

import (
	"os"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func testDB(t *testing.T) *bolt.DB {
	t.Helper()
	f, err := os.CreateTemp("", "mcp-apikeys-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_ = os.Remove(f.Name())
	})
	return db
}

func TestMCPAPIKeyStoreCreateAndGet(t *testing.T) {
	store := newMCPAPIKeyStore(testDB(t))

	ak, fullKey, err := store.Create("test-app", 0)
	if err != nil {
		t.Fatal(err)
	}
	if ak.Name != "test-app" {
		t.Errorf("expected name test-app, got %s", ak.Name)
	}
	if fullKey == "" || len(fullKey) < 20 {
		t.Errorf("expected long key, got %q", fullKey)
	}

	// Get
	got, err := store.Get(fullKey)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "test-app" {
		t.Errorf("expected test-app, got %s", got.Name)
	}
}

func TestMCPAPIKeyStoreList(t *testing.T) {
	store := newMCPAPIKeyStore(testDB(t))

	_, _, _ = store.Create("app1", 0)
	_, _, _ = store.Create("app2", 0)

	keys, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
	// Full key should NOT be exposed in list
	for _, k := range keys {
		if len(k.KeyPrefix) > 16 {
			t.Errorf("key prefix too long (should be truncated): %s", k.KeyPrefix)
		}
	}
}

func TestMCPAPIKeyStoreDelete(t *testing.T) {
	store := newMCPAPIKeyStore(testDB(t))

	_, fullKey, _ := store.Create("to-delete", 0)

	err := store.Delete(fullKey)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Get(fullKey)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestMCPAPIKeyStoreDisable(t *testing.T) {
	store := newMCPAPIKeyStore(testDB(t))

	_, fullKey, _ := store.Create("to-disable", 0)

	err := store.Disable(fullKey)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Get(fullKey)
	if err == nil {
		t.Error("expected error for disabled key")
	}
}

func TestMCPAPIKeyStoreExpiry(t *testing.T) {
	store := newMCPAPIKeyStore(testDB(t))

	// Create with expiry in the past
	_, fullKey, _ := store.Create("expired", 1) // Unix timestamp 1 = 1970

	_, err := store.Get(fullKey)
	if err == nil {
		t.Error("expected error for expired key")
	}
}

func TestMCPAPIKeyStoreCount(t *testing.T) {
	store := newMCPAPIKeyStore(testDB(t))

	if store.Count() != 0 {
		t.Errorf("expected 0, got %d", store.Count())
	}

	_, _, _ = store.Create("a", 0)
	_, _, _ = store.Create("b", 0)

	if store.Count() != 2 {
		t.Errorf("expected 2, got %d", store.Count())
	}
}

func TestMCPAPIKeyMiddlewareWithStore(t *testing.T) {
	store := newMCPAPIKeyStore(testDB(t))
	_, fullKey, _ := store.Create("dynamic-app", 0)

	m := &MCPAPIKeyMiddleware{
		enabled:  true,
		keys:     make(map[string]string),
		keyStore: store,
		cacheTTL: 60_000_000_000,
		cache:    make(map[string]*apiKeyCacheEntry),
	}

	// Static key should fail
	_, ok := m.validateKey("sk-nonexistent")
	if ok {
		t.Error("nonexistent key should fail")
	}

	// Dynamic key should succeed
	name, ok := m.validateKey(fullKey)
	if !ok {
		t.Error("dynamic key should succeed")
	}
	if name != "dynamic-app" {
		t.Errorf("expected name dynamic-app, got %s", name)
	}

	// Should be cached now
	m.cacheMu.RLock()
	_, cached := m.cache[fullKey]
	m.cacheMu.RUnlock()
	if !cached {
		t.Error("key should be cached after lookup")
	}
}

func TestMCPAPIKeyMiddlewareInvalidateCache(t *testing.T) {
	m := &MCPAPIKeyMiddleware{
		cache: map[string]*apiKeyCacheEntry{
			"key1": {name: "a", valid: true},
			"key2": {name: "b", valid: true},
		},
	}

	m.InvalidateCache()

	m.cacheMu.RLock()
	if len(m.cache) != 0 {
		t.Errorf("expected empty cache after invalidation, got %d entries", len(m.cache))
	}
	m.cacheMu.RUnlock()
}
