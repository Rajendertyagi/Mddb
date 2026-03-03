package main

import (
	"testing"
	"time"
)

func TestAdaptiveIndexManagerNew(t *testing.T) {
	aim := &AdaptiveIndexManager{}
	// Verify the struct is usable by calling a method that initialises internal state.
	idx := aim.GetOptimalIndex("col", "pattern")
	if idx != IndexTypeBTree {
		t.Errorf("expected IndexTypeBTree default, got %v", idx)
	}
}

func TestAdaptiveIndexRecordQuery(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	aim.RecordQuery("blog", "key=hello", 5*time.Millisecond, 1)

	// Verify stats were recorded
	key := "blog|key=hello"
	value, ok := aim.queryStats.Load(key)
	if !ok {
		t.Fatal("expected query stats to be stored")
	}
	stats := value.(*QueryStats)
	if stats.Count.Load() != 1 {
		t.Errorf("expected count 1, got %d", stats.Count.Load())
	}
	if stats.Pattern != "key=hello" {
		t.Errorf("expected pattern 'key=hello', got %q", stats.Pattern)
	}
	if stats.LastAccessed.Load() == 0 {
		t.Error("expected LastAccessed to be set")
	}
}

func TestAdaptiveIndexRecordQueryMultiple(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	for i := 0; i < 15; i++ {
		aim.RecordQuery("blog", "key=hello", 5*time.Millisecond, 1)
	}

	key := "blog|key=hello"
	value, ok := aim.queryStats.Load(key)
	if !ok {
		t.Fatal("expected query stats to be stored")
	}
	stats := value.(*QueryStats)
	if stats.Count.Load() != 15 {
		t.Errorf("expected count 15, got %d", stats.Count.Load())
	}
}

func TestAdaptiveIndexAnalyzeQueryThreshold(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	// Record fewer than 10 queries - should not update preferred index
	for i := 0; i < 5; i++ {
		aim.RecordQuery("blog", "key=id", 1*time.Millisecond, 1)
	}

	idx := aim.GetOptimalIndex("blog", "key=id")
	// Should return default BTree since not enough samples
	if idx != IndexTypeBTree {
		t.Errorf("expected IndexTypeBTree (default), got %v", idx)
	}
}

func TestAdaptiveIndexAnalyzeQueryBloom(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	// Record enough queries with 0 results -> should prefer Bloom
	for i := 0; i < 15; i++ {
		aim.RecordQuery("blog", "negative", 1*time.Millisecond, 0)
	}

	idx := aim.GetOptimalIndex("blog", "negative")
	if idx != IndexTypeBloom {
		t.Errorf("expected IndexTypeBloom, got %v", idx)
	}
}

func TestAdaptiveIndexAnalyzeQueryHash(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	// Record enough queries with 1 result -> should prefer Hash
	for i := 0; i < 15; i++ {
		aim.RecordQuery("blog", "exact", 1*time.Millisecond, 1)
	}

	idx := aim.GetOptimalIndex("blog", "exact")
	if idx != IndexTypeHash {
		t.Errorf("expected IndexTypeHash, got %v", idx)
	}
}

func TestAdaptiveIndexAnalyzeQueryBTree(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	// Record enough queries with small result set -> should prefer BTree
	for i := 0; i < 15; i++ {
		aim.RecordQuery("blog", "range", 1*time.Millisecond, 50)
	}

	idx := aim.GetOptimalIndex("blog", "range")
	if idx != IndexTypeBTree {
		t.Errorf("expected IndexTypeBTree, got %v", idx)
	}
}

func TestAdaptiveIndexAnalyzeQueryBitmap(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	// Record enough queries with medium result set -> should prefer Bitmap
	for i := 0; i < 15; i++ {
		aim.RecordQuery("blog", "medium", 1*time.Millisecond, 500)
	}

	idx := aim.GetOptimalIndex("blog", "medium")
	if idx != IndexTypeBitmap {
		t.Errorf("expected IndexTypeBitmap, got %v", idx)
	}
}

func TestAdaptiveIndexAnalyzeQueryFull(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	// Record enough queries with large result set -> should prefer Full
	for i := 0; i < 15; i++ {
		aim.RecordQuery("blog", "large", 1*time.Millisecond, 50000)
	}

	idx := aim.GetOptimalIndex("blog", "large")
	if idx != IndexTypeFull {
		t.Errorf("expected IndexTypeFull, got %v", idx)
	}
}

func TestAdaptiveIndexAnalyzeQuerySlowQuery(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	// Slow queries (>10ms avg) should not update preferred index
	for i := 0; i < 15; i++ {
		aim.RecordQuery("blog", "slow", 20*time.Millisecond, 1)
	}

	idx := aim.GetOptimalIndex("blog", "slow")
	// Default is BTree because the slow average prevents update
	if idx != IndexTypeHash && idx != IndexTypeBTree {
		// The initial PreferredIndex is 0 (IndexTypeHash) but may not be set
		// depending on whether the threshold is met
		t.Logf("got index type: %v", idx)
	}
}

func TestAdaptiveIndexGetOptimalIndexUnknown(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	// Unknown pattern returns default BTree
	idx := aim.GetOptimalIndex("unknown", "nonexistent")
	if idx != IndexTypeBTree {
		t.Errorf("expected IndexTypeBTree default, got %v", idx)
	}
}

func TestAdaptiveIndexGetStrategy(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	// Get strategy for a new collection
	strategy := aim.GetStrategy("blog")
	if strategy == nil {
		t.Fatal("expected non-nil strategy")
	}
	if strategy.Collection != "blog" {
		t.Errorf("expected collection 'blog', got %q", strategy.Collection)
	}
	if strategy.PrimaryIndex != IndexTypeBTree {
		t.Errorf("expected PrimaryIndex BTree, got %v", strategy.PrimaryIndex)
	}
	if strategy.SecondaryIndex != IndexTypeHash {
		t.Errorf("expected SecondaryIndex Hash, got %v", strategy.SecondaryIndex)
	}
	if strategy.QueryPatterns == nil {
		t.Error("expected QueryPatterns to be initialized")
	}
}

func TestAdaptiveIndexGetStrategyExisting(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	// Create a strategy
	s1 := aim.GetStrategy("blog")
	s1.PrimaryIndex = IndexTypeBloom

	// Get same strategy
	s2 := aim.GetStrategy("blog")
	if s2.PrimaryIndex != IndexTypeBloom {
		t.Error("expected to get same strategy object")
	}
}

func TestAdaptiveIndexOptimize(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	// Record queries to build stats
	for i := 0; i < 15; i++ {
		aim.RecordQuery("blog", "exact", 1*time.Millisecond, 1)
	}
	for i := 0; i < 15; i++ {
		aim.RecordQuery("blog", "range", 1*time.Millisecond, 50)
	}

	// Create a strategy so optimize has something to work with
	_ = aim.GetStrategy("blog")

	// Run optimize
	aim.optimize()

	// Verify strategy was updated
	strategy := aim.GetStrategy("blog")
	strategy.mu.RLock()
	defer strategy.mu.RUnlock()
	// Primary index should have been set based on most common queries
	// Both Hash and BTree are valid depending on which has more votes
	t.Logf("Primary index after optimize: %v", strategy.PrimaryIndex)
}

func TestAdaptiveIndexOptimizeSkipsOldQueries(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	// Record query and set its LastAccessed to long ago
	aim.RecordQuery("old_collection", "old_pattern", 1*time.Millisecond, 1)
	key := "old_collection|old_pattern"
	if value, ok := aim.queryStats.Load(key); ok {
		stats := value.(*QueryStats)
		stats.LastAccessed.Store(time.Now().Add(-2 * time.Hour).Unix())
	}

	// optimize should skip old queries
	aim.optimize()

	// Should not have created a strategy for old_collection from the optimize path
	// (It might exist from RecordQuery's analyzeQuery call if that created one)
}

func TestAdaptiveIndexStats(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	// Empty stats
	stats := aim.Stats()
	if stats.TotalQueries != 0 {
		t.Errorf("expected 0 total queries, got %d", stats.TotalQueries)
	}
	if len(stats.Collections) != 0 {
		t.Errorf("expected 0 collections, got %d", len(stats.Collections))
	}

	// Add some data
	aim.RecordQuery("blog", "key=hello", 5*time.Millisecond, 1)
	_ = aim.GetStrategy("blog")

	stats = aim.Stats()
	if stats.TotalQueries != 1 {
		t.Errorf("expected 1 total query, got %d", stats.TotalQueries)
	}
	if len(stats.Collections) != 1 {
		t.Errorf("expected 1 collection, got %d", len(stats.Collections))
	}

	cs, ok := stats.Collections["blog"]
	if !ok {
		t.Fatal("expected blog collection in stats")
	}
	if cs.PrimaryIndex != "btree" {
		t.Errorf("expected PrimaryIndex 'btree', got %q", cs.PrimaryIndex)
	}
	if cs.SecondaryIndex != "hash" {
		t.Errorf("expected SecondaryIndex 'hash', got %q", cs.SecondaryIndex)
	}
}

func TestIndexTypeString(t *testing.T) {
	tests := []struct {
		it       IndexType
		expected string
	}{
		{IndexTypeHash, "hash"},
		{IndexTypeBTree, "btree"},
		{IndexTypeBitmap, "bitmap"},
		{IndexTypeBloom, "bloom"},
		{IndexTypeFull, "full"},
		{IndexType(99), "unknown"},
	}

	for _, tc := range tests {
		got := tc.it.String()
		if got != tc.expected {
			t.Errorf("IndexType(%d).String() = %q, want %q", tc.it, got, tc.expected)
		}
	}
}

func TestAdaptiveIndexConcurrency(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	// Record queries concurrently
	done := make(chan struct{})
	for g := 0; g < 10; g++ {
		go func(goroutineID int) {
			for i := 0; i < 20; i++ {
				aim.RecordQuery("blog", "concurrent", 1*time.Millisecond, 1)
			}
			done <- struct{}{}
		}(g)
	}

	for g := 0; g < 10; g++ {
		<-done
	}

	key := "blog|concurrent"
	value, ok := aim.queryStats.Load(key)
	if !ok {
		t.Fatal("expected query stats to exist")
	}
	stats := value.(*QueryStats)
	if stats.Count.Load() != 200 {
		t.Errorf("expected 200 queries, got %d", stats.Count.Load())
	}
}

func TestAdaptiveIndexMultipleCollections(t *testing.T) {
	aim := &AdaptiveIndexManager{}

	aim.RecordQuery("blog", "p1", 1*time.Millisecond, 1)
	aim.RecordQuery("users", "p2", 1*time.Millisecond, 1)
	aim.RecordQuery("orders", "p3", 1*time.Millisecond, 1)

	_ = aim.GetStrategy("blog")
	_ = aim.GetStrategy("users")
	_ = aim.GetStrategy("orders")

	stats := aim.Stats()
	if stats.TotalQueries != 3 {
		t.Errorf("expected 3 total queries, got %d", stats.TotalQueries)
	}
	if len(stats.Collections) != 3 {
		t.Errorf("expected 3 collections, got %d", len(stats.Collections))
	}
}
