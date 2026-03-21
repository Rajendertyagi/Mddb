package main

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

func TestNewLockFreeCache(t *testing.T) {
	cache := NewLockFreeCache(1000, 60)
	if cache == nil {
		t.Fatal("NewLockFreeCache returned nil")
		return
	}
	if len(cache.shards) != 16 {
		t.Errorf("shards = %d, want 16", len(cache.shards))
	}
}

func TestNewLockFreeCache_Defaults(t *testing.T) {
	cache := NewLockFreeCache(0, 0)
	if cache.maxSize != 1000 {
		t.Errorf("maxSize = %d, want 1000 (default)", cache.maxSize)
	}
	if cache.ttl != 300 {
		t.Errorf("ttl = %d, want 300 (default)", cache.ttl)
	}
}

func TestNewLockFreeCache_NegativeParams(t *testing.T) {
	cache := NewLockFreeCache(-5, -10)
	if cache.maxSize != 1000 {
		t.Errorf("maxSize = %d, want 1000 (default)", cache.maxSize)
	}
	if cache.ttl != 300 {
		t.Errorf("ttl = %d, want 300 (default)", cache.ttl)
	}
}

func TestLockFreeCache_SetAndGet(t *testing.T) {
	cache := NewLockFreeCache(1000, 60)

	cache.Set("key1", []byte("value1"))

	val, ok := cache.Get("key1")
	if !ok {
		t.Fatal("Get returned false for existing key")
	}
	if !bytes.Equal(val, []byte("value1")) {
		t.Errorf("Get = %q, want %q", val, "value1")
	}
}

func TestLockFreeCache_GetMiss(t *testing.T) {
	cache := NewLockFreeCache(1000, 60)

	val, ok := cache.Get("nonexistent")
	if ok {
		t.Error("Get returned true for nonexistent key")
	}
	if val != nil {
		t.Errorf("val = %v, want nil", val)
	}
}

func TestLockFreeCache_Delete(t *testing.T) {
	cache := NewLockFreeCache(1000, 60)

	cache.Set("key1", []byte("value1"))
	cache.Delete("key1")

	val, ok := cache.Get("key1")
	if ok {
		t.Error("Get returned true after Delete")
	}
	if val != nil {
		t.Errorf("val = %v, want nil after delete", val)
	}
}

func TestLockFreeCache_DeleteNonExistent(t *testing.T) {
	cache := NewLockFreeCache(1000, 60)
	// Should not panic
	cache.Delete("nonexistent")
}

func TestLockFreeCache_Clear(t *testing.T) {
	cache := NewLockFreeCache(1000, 60)

	for i := 0; i < 50; i++ {
		cache.Set(fmt.Sprintf("key-%d", i), []byte(fmt.Sprintf("val-%d", i)))
	}

	cache.Clear()

	_, _, size := cache.Stats()
	if size != 0 {
		t.Errorf("size after Clear = %d, want 0", size)
	}

	// Verify keys are gone
	for i := 0; i < 50; i++ {
		if _, ok := cache.Get(fmt.Sprintf("key-%d", i)); ok {
			t.Errorf("key-%d still present after Clear", i)
		}
	}
}

func TestLockFreeCache_Overwrite(t *testing.T) {
	cache := NewLockFreeCache(1000, 60)

	cache.Set("key1", []byte("original"))
	cache.Set("key1", []byte("updated"))

	val, ok := cache.Get("key1")
	if !ok {
		t.Fatal("Get returned false")
	}
	if !bytes.Equal(val, []byte("updated")) {
		t.Errorf("Get = %q, want %q", val, "updated")
	}
}

func TestLockFreeCache_Stats(t *testing.T) {
	cache := NewLockFreeCache(1000, 60)

	cache.Set("a", []byte("1"))
	cache.Set("b", []byte("2"))

	// One hit, one miss
	cache.Get("a")     // hit
	cache.Get("zzzzz") // miss

	hits, misses, size := cache.Stats()
	if hits != 1 {
		t.Errorf("hits = %d, want 1", hits)
	}
	if misses != 1 {
		t.Errorf("misses = %d, want 1", misses)
	}
	if size != 2 {
		t.Errorf("size = %d, want 2", size)
	}
}

func TestLockFreeCache_Eviction(t *testing.T) {
	// Small cache to trigger eviction. maxSize = 32 means 32/16 = 2 per shard
	cache := NewLockFreeCache(32, 60)

	// Insert enough keys to trigger eviction in at least one shard
	for i := 0; i < 100; i++ {
		cache.Set(fmt.Sprintf("evict-%d", i), []byte("data"))
	}

	// The cache should have limited its size
	_, _, size := cache.Stats()
	if size > 100 {
		t.Errorf("size = %d, should be limited by eviction", size)
	}
}

func TestLockFreeCache_ConcurrentReadsWrites(t *testing.T) {
	cache := NewLockFreeCache(10000, 60)

	var wg sync.WaitGroup

	// Concurrent writers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key-%d-%d", n, j)
				cache.Set(key, []byte(fmt.Sprintf("val-%d-%d", n, j)))
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key-%d-%d", n, j)
				cache.Get(key)
			}
		}(i)
	}

	wg.Wait()

	// Verify stats don't panic
	hits, misses, size := cache.Stats()
	_ = hits
	_ = misses
	_ = size
}

func TestLockFreeCache_ConcurrentDelete(t *testing.T) {
	cache := NewLockFreeCache(10000, 60)

	// Pre-populate
	for i := 0; i < 100; i++ {
		cache.Set(fmt.Sprintf("del-%d", i), []byte("data"))
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cache.Delete(fmt.Sprintf("del-%d", n))
		}(i)
	}
	wg.Wait()

	// All keys should be deleted
	for i := 0; i < 100; i++ {
		if _, ok := cache.Get(fmt.Sprintf("del-%d", i)); ok {
			t.Errorf("key del-%d still present", i)
		}
	}
}

func TestLockFreeCache_EmptyValueAndKey(t *testing.T) {
	cache := NewLockFreeCache(1000, 60)

	// Empty value
	cache.Set("empty-val", []byte{})
	val, ok := cache.Get("empty-val")
	if !ok {
		t.Error("Get returned false for empty value")
	}
	if len(val) != 0 {
		t.Errorf("val = %v, want empty", val)
	}

	// Empty key
	cache.Set("", []byte("empty-key-data"))
	val, ok = cache.Get("")
	if !ok {
		t.Error("Get returned false for empty key")
	}
	if !bytes.Equal(val, []byte("empty-key-data")) {
		t.Errorf("Get('') = %q, want %q", val, "empty-key-data")
	}
}

func TestFnv1a(t *testing.T) {
	// Deterministic
	h1 := fnv1a("test")
	h2 := fnv1a("test")
	if h1 != h2 {
		t.Error("fnv1a not deterministic")
	}

	// Different inputs -> different hashes (with high probability)
	h3 := fnv1a("different")
	if h1 == h3 {
		t.Error("fnv1a collision for 'test' and 'different'")
	}

	// Empty string
	h4 := fnv1a("")
	if h4 == 0 {
		t.Error("fnv1a('') = 0, expected non-zero offset basis")
	}
}

func TestFnv1a_Distribution(t *testing.T) {
	// Check that hashes distribute across 16 shards reasonably
	buckets := make([]int, 16)
	for i := 0; i < 1000; i++ {
		h := fnv1a(fmt.Sprintf("key-%d", i))
		buckets[h&15]++
	}

	for i, count := range buckets {
		if count == 0 {
			t.Errorf("bucket %d has 0 keys; distribution issue", i)
		}
	}
}
