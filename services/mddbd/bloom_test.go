package main

import (
	"fmt"
	"sync"
	"testing"
)

func TestNewBloomFilterManager(t *testing.T) {
	bfm := NewBloomFilterManager()
	if bfm == nil {
		t.Fatal("NewBloomFilterManager returned nil")
	}
}

func TestBloomFilterManager_GetOrCreate(t *testing.T) {
	bfm := NewBloomFilterManager()

	f1 := bfm.GetOrCreate("blog", 1000)
	if f1 == nil {
		t.Fatal("GetOrCreate returned nil")
	}

	// Second call should return same filter
	f2 := bfm.GetOrCreate("blog", 1000)
	if f1 != f2 {
		t.Error("GetOrCreate did not return cached filter")
	}

	// Different collection should return different filter
	f3 := bfm.GetOrCreate("pages", 500)
	if f1 == f3 {
		t.Error("different collection returned same filter")
	}
}

func TestBloomFilterManager_AddAndTest(t *testing.T) {
	bfm := NewBloomFilterManager()

	bfm.Add("blog", "post1", "en")

	if !bfm.Test("blog", "post1", "en") {
		t.Error("Test returned false for added key")
	}
}

func TestBloomFilterManager_TestNonExistentCollection(t *testing.T) {
	bfm := NewBloomFilterManager()

	// Collection that was never created
	if bfm.Test("nonexistent", "key", "en") {
		t.Error("Test returned true for nonexistent collection")
	}
}

func TestBloomFilterManager_TestNonExistentKey(t *testing.T) {
	bfm := NewBloomFilterManager()

	bfm.Add("blog", "post1", "en")

	// Different key
	if bfm.Test("blog", "nonexistent", "en") {
		// Bloom filters can have false positives, so this is acceptable
		t.Log("Note: false positive detected (expected with bloom filters)")
	}
}

func TestBloomFilterManager_TestDifferentLang(t *testing.T) {
	bfm := NewBloomFilterManager()

	bfm.Add("blog", "post1", "en")

	// Same key but different lang should generally not match
	// (might occasionally due to false positives)
	result := bfm.Test("blog", "post1", "fr")
	_ = result // Just verify it doesn't panic
}

func TestBloomFilterManager_Clear(t *testing.T) {
	bfm := NewBloomFilterManager()

	bfm.Add("blog", "post1", "en")
	bfm.Clear("blog")

	// After clear, the collection filter is gone
	if bfm.Test("blog", "post1", "en") {
		t.Error("Test returned true after Clear")
	}
}

func TestBloomFilterManager_ClearNonExistent(t *testing.T) {
	bfm := NewBloomFilterManager()
	// Should not panic
	bfm.Clear("nonexistent")
}

func TestBloomFilterManager_Remove(t *testing.T) {
	bfm := NewBloomFilterManager()

	bfm.Add("blog", "post1", "en")
	bfm.Remove("blog", "post1", "en")

	// Bloom filters don't actually support deletion,
	// so the key should still test positive
	if !bfm.Test("blog", "post1", "en") {
		t.Error("Test returned false after Remove (bloom filters don't support real deletion)")
	}
}

func TestBloomFilterManager_Stats(t *testing.T) {
	bfm := NewBloomFilterManager()

	bfm.Add("blog", "post1", "en")
	bfm.Add("blog", "post2", "en")
	bfm.Add("pages", "home", "en")

	stats := bfm.Stats()

	if len(stats) != 2 {
		t.Fatalf("Stats returned %d collections, want 2", len(stats))
	}

	blogStats, ok := stats["blog"]
	if !ok {
		t.Fatal("missing stats for 'blog' collection")
	}
	if blogStats.FPRate != 0.01 {
		t.Errorf("FPRate = %f, want 0.01", blogStats.FPRate)
	}
	if blogStats.Capacity == 0 {
		t.Error("Capacity should be > 0")
	}

	pagesStats, ok := stats["pages"]
	if !ok {
		t.Fatal("missing stats for 'pages' collection")
	}
	if pagesStats.Capacity == 0 {
		t.Error("pages Capacity should be > 0")
	}
}

func TestBloomFilterManager_StatsEmpty(t *testing.T) {
	bfm := NewBloomFilterManager()

	stats := bfm.Stats()
	if len(stats) != 0 {
		t.Errorf("Stats on empty manager returned %d entries, want 0", len(stats))
	}
}

func TestBloomFilterManager_ManyKeys(t *testing.T) {
	bfm := NewBloomFilterManager()

	n := 5000
	for i := 0; i < n; i++ {
		bfm.Add("bulk", fmt.Sprintf("key-%d", i), "en")
	}

	// All added keys should be present (no false negatives)
	for i := 0; i < n; i++ {
		if !bfm.Test("bulk", fmt.Sprintf("key-%d", i), "en") {
			t.Fatalf("false negative for key-%d", i)
		}
	}
}

func TestBloomFilterManager_ConcurrentAccess(t *testing.T) {
	bfm := NewBloomFilterManager()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", n)
			bfm.Add("concurrent", key, "en")
			bfm.Test("concurrent", key, "en")
		}(i)
	}
	wg.Wait()

	// Verify all keys present
	for i := 0; i < 100; i++ {
		if !bfm.Test("concurrent", fmt.Sprintf("key-%d", i), "en") {
			t.Errorf("false negative for concurrent key-%d", i)
		}
	}
}

func TestBloomFilterManager_MultipleCollections(t *testing.T) {
	bfm := NewBloomFilterManager()

	collections := []string{"blog", "pages", "users", "config", "assets"}
	for _, coll := range collections {
		for i := 0; i < 10; i++ {
			bfm.Add(coll, fmt.Sprintf("doc-%d", i), "en")
		}
	}

	stats := bfm.Stats()
	if len(stats) != len(collections) {
		t.Errorf("Stats returned %d collections, want %d", len(stats), len(collections))
	}

	for _, coll := range collections {
		if _, ok := stats[coll]; !ok {
			t.Errorf("missing stats for collection %q", coll)
		}
	}
}

func TestBloomStats_Fields(t *testing.T) {
	bs := BloomStats{
		Capacity: 10000,
		Count:    42,
		FPRate:   0.01,
	}
	if bs.Capacity != 10000 {
		t.Errorf("Capacity = %d, want 10000", bs.Capacity)
	}
	if bs.Count != 42 {
		t.Errorf("Count = %d, want 42", bs.Count)
	}
	if bs.FPRate != 0.01 {
		t.Errorf("FPRate = %f, want 0.01", bs.FPRate)
	}
}
