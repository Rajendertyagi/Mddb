package main

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewDocumentCacheDefaults(t *testing.T) {
	// Zero/negative values should use defaults
	cache := NewDocumentCache(0, 0)
	if cache.maxSize != 1000 {
		t.Errorf("expected default maxSize 1000, got %d", cache.maxSize)
	}
	if cache.ttl != 300 {
		t.Errorf("expected default ttl 300, got %d", cache.ttl)
	}
}

func TestNewDocumentCacheNegativeValues(t *testing.T) {
	cache := NewDocumentCache(-5, -10)
	if cache.maxSize != 1000 {
		t.Errorf("expected default maxSize 1000 for negative input, got %d", cache.maxSize)
	}
	if cache.ttl != 300 {
		t.Errorf("expected default ttl 300 for negative input, got %d", cache.ttl)
	}
}

func TestNewDocumentCacheCustomValues(t *testing.T) {
	cache := NewDocumentCache(500, 60)
	if cache.maxSize != 500 {
		t.Errorf("expected maxSize 500, got %d", cache.maxSize)
	}
	if cache.ttl != 60 {
		t.Errorf("expected ttl 60, got %d", cache.ttl)
	}
}

func TestCacheSetAndGet(t *testing.T) {
	cache := NewDocumentCache(10, 300)

	cache.Set("key1", []byte("value1"))
	data, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected to find key1 in cache")
	}
	if string(data) != "value1" {
		t.Errorf("expected 'value1', got %q", string(data))
	}
}

func TestCacheGetMiss(t *testing.T) {
	cache := NewDocumentCache(10, 300)

	data, ok := cache.Get("nonexistent")
	if ok {
		t.Error("expected miss for nonexistent key")
	}
	if data != nil {
		t.Error("expected nil data for miss")
	}
}

func TestCacheGetExpired(t *testing.T) {
	cache := NewDocumentCache(10, 1) // 1 second TTL

	cache.Set("key1", []byte("value1"))

	// Wait for expiration
	time.Sleep(2 * time.Second)

	data, ok := cache.Get("key1")
	if ok {
		t.Error("expected miss for expired entry")
	}
	if data != nil {
		t.Error("expected nil data for expired entry")
	}
}

func TestCacheOverwrite(t *testing.T) {
	cache := NewDocumentCache(10, 300)

	cache.Set("key1", []byte("value1"))
	cache.Set("key1", []byte("value2"))

	data, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected to find key1 in cache")
	}
	if string(data) != "value2" {
		t.Errorf("expected 'value2' after overwrite, got %q", string(data))
	}
}

func TestCacheDelete(t *testing.T) {
	cache := NewDocumentCache(10, 300)

	cache.Set("key1", []byte("value1"))
	cache.Delete("key1")

	_, ok := cache.Get("key1")
	if ok {
		t.Error("expected key1 to be deleted")
	}
}

func TestCacheDeleteNonexistent(t *testing.T) {
	cache := NewDocumentCache(10, 300)

	// Deleting a key that does not exist should not panic
	cache.Delete("nonexistent")
}

func TestCacheClear(t *testing.T) {
	cache := NewDocumentCache(10, 300)

	cache.Set("key1", []byte("value1"))
	cache.Set("key2", []byte("value2"))
	cache.Set("key3", []byte("value3"))

	cache.Clear()

	_, ok1 := cache.Get("key1")
	_, ok2 := cache.Get("key2")
	_, ok3 := cache.Get("key3")
	if ok1 || ok2 || ok3 {
		t.Error("expected all keys to be cleared")
	}
}

func TestCacheStats(t *testing.T) {
	cache := NewDocumentCache(10, 300)

	cache.Set("key1", []byte("value1"))

	// Hit
	cache.Get("key1")
	// Miss
	cache.Get("nonexistent")

	hits, misses, size := cache.Stats()
	if hits != 1 {
		t.Errorf("expected 1 hit, got %d", hits)
	}
	if misses != 1 {
		t.Errorf("expected 1 miss, got %d", misses)
	}
	if size != 1 {
		t.Errorf("expected size 1, got %d", size)
	}
}

func TestCacheStatsMultiple(t *testing.T) {
	cache := NewDocumentCache(10, 300)

	cache.Set("a", []byte("1"))
	cache.Set("b", []byte("2"))

	// 2 hits
	cache.Get("a")
	cache.Get("b")
	// 3 misses
	cache.Get("c")
	cache.Get("d")
	cache.Get("e")

	hits, misses, size := cache.Stats()
	if hits != 2 {
		t.Errorf("expected 2 hits, got %d", hits)
	}
	if misses != 3 {
		t.Errorf("expected 3 misses, got %d", misses)
	}
	if size != 2 {
		t.Errorf("expected size 2, got %d", size)
	}
}

func TestCacheEviction(t *testing.T) {
	maxSize := 3
	cache := NewDocumentCache(maxSize, 300)

	// Fill to capacity
	cache.Set("key1", []byte("value1"))
	cache.Set("key2", []byte("value2"))
	cache.Set("key3", []byte("value3"))

	// This should trigger eviction (one entry removed)
	cache.Set("key4", []byte("value4"))

	_, _, size := cache.Stats()
	if size != maxSize {
		t.Errorf("expected size %d after eviction, got %d", maxSize, size)
	}

	// key4 should be present
	data, ok := cache.Get("key4")
	if !ok {
		t.Error("expected key4 to be in cache after eviction")
	}
	if string(data) != "value4" {
		t.Errorf("expected 'value4', got %q", string(data))
	}
}

func TestCacheEvictionMultiple(t *testing.T) {
	cache := NewDocumentCache(2, 300)

	cache.Set("a", []byte("1"))
	cache.Set("b", []byte("2"))

	// Both should trigger eviction, but cache stays at max size
	cache.Set("c", []byte("3"))
	cache.Set("d", []byte("4"))

	_, _, size := cache.Stats()
	if size != 2 {
		t.Errorf("expected size 2, got %d", size)
	}
}

func TestCacheConcurrentAccess(t *testing.T) {
	cache := NewDocumentCache(100, 300)
	var wg sync.WaitGroup

	// Concurrent writers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i)
			cache.Set(key, []byte(fmt.Sprintf("value-%d", i)))
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i)
			cache.Get(key)
		}(i)
	}

	// Concurrent deleters
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i)
			cache.Delete(key)
		}(i)
	}

	wg.Wait()
	// If we get here without deadlock or panic, the test passes
}

func TestCacheConcurrentSetAndClear(t *testing.T) {
	cache := NewDocumentCache(50, 300)
	var wg sync.WaitGroup

	// Concurrent writers and clearers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cache.Set(fmt.Sprintf("key-%d", i), []byte("val"))
		}(i)
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.Clear()
		}()
	}

	wg.Wait()
}

func TestBuildCacheKey(t *testing.T) {
	tests := []struct {
		collection, key, lang, expected string
	}{
		{"blog", "post1", "en", "blog|post1|en"},
		{"docs", "readme", "fr_FR", "docs|readme|fr_FR"},
		{"", "", "", "||"},
		{"a", "b", "c", "a|b|c"},
	}

	for _, tt := range tests {
		result := BuildCacheKey(tt.collection, tt.key, tt.lang)
		if result != tt.expected {
			t.Errorf("BuildCacheKey(%q, %q, %q) = %q, want %q",
				tt.collection, tt.key, tt.lang, result, tt.expected)
		}
	}
}

func TestCacheEmptyData(t *testing.T) {
	cache := NewDocumentCache(10, 300)

	// Storing empty byte slice
	cache.Set("empty", []byte{})
	data, ok := cache.Get("empty")
	if !ok {
		t.Fatal("expected to find 'empty' key in cache")
	}
	if len(data) != 0 {
		t.Errorf("expected empty data, got %d bytes", len(data))
	}
}

func TestCacheNilData(t *testing.T) {
	cache := NewDocumentCache(10, 300)

	cache.Set("nil", nil)
	data, ok := cache.Get("nil")
	if !ok {
		t.Fatal("expected to find 'nil' key in cache")
	}
	if data != nil {
		t.Errorf("expected nil data, got %v", data)
	}
}

func TestCacheLargeData(t *testing.T) {
	cache := NewDocumentCache(10, 300)

	large := make([]byte, 1024*1024) // 1MB
	for i := range large {
		large[i] = byte(i % 256)
	}

	cache.Set("large", large)
	data, ok := cache.Get("large")
	if !ok {
		t.Fatal("expected to find large entry in cache")
	}
	if len(data) != len(large) {
		t.Errorf("expected %d bytes, got %d", len(large), len(data))
	}
}

func TestCacheStatsAfterClear(t *testing.T) {
	cache := NewDocumentCache(10, 300)

	cache.Set("a", []byte("1"))
	cache.Get("a")
	cache.Get("missing")

	cache.Clear()

	hits, misses, size := cache.Stats()
	if size != 0 {
		t.Errorf("expected size 0 after clear, got %d", size)
	}
	// Hits and misses are not reset by Clear
	if hits != 1 {
		t.Errorf("expected hits 1 (not reset by clear), got %d", hits)
	}
	if misses != 1 {
		t.Errorf("expected misses 1 (not reset by clear), got %d", misses)
	}
}

func TestCacheDeleteThenSet(t *testing.T) {
	cache := NewDocumentCache(10, 300)

	cache.Set("key1", []byte("v1"))
	cache.Delete("key1")
	cache.Set("key1", []byte("v2"))

	data, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected to find key1 after re-set")
	}
	if string(data) != "v2" {
		t.Errorf("expected 'v2', got %q", string(data))
	}
}

// TestCacheStatsRaceGetAndStats is the GO-006 regression: Get increments
// hits/misses with atomic.AddUint64 under only an RLock, so a plain read in
// Stats() would race a concurrent Get. Run with `go test -race`.
func TestCacheStatsRaceGetAndStats(t *testing.T) {
	c := NewDocumentCache(100, 60)
	defer c.Close()
	c.Set("k", []byte("v"))

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Readers hammering Get (hit) and a missing key (miss) → counter writes.
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, _ = c.Get("k")
				_, _ = c.Get("absent")
			}
		}()
	}
	// Concurrent Stats() readers.
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, _, _ = c.Stats()
			}
		}()
	}

	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()

	hits, misses, _ := c.Stats()
	if hits == 0 || misses == 0 {
		t.Errorf("expected non-zero hits and misses, got hits=%d misses=%d", hits, misses)
	}
}

// TestCacheCloseIdempotent documents that Close() is safe to call repeatedly
// (sync.Once) — restore and shutdown paths may both close the same cache.
func TestCacheCloseIdempotent(t *testing.T) {
	c := NewDocumentCache(10, 60)
	c.Close()
	c.Close() // must not panic on a second close
}
