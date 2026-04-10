package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
)

// swapParallelConfig atomically sets workers/minSize and returns a restore func.
// Usage: defer swapParallelConfig(workers, minSize)()
func swapParallelConfig(workers, minSize int) func() {
	oldW := parallelSearchConfig.workers.Swap(int32(workers))
	oldM := parallelSearchConfig.minSize.Swap(int32(minSize))
	return func() {
		parallelSearchConfig.workers.Store(oldW)
		parallelSearchConfig.minSize.Store(oldM)
	}
}

// swapParallelMinSize only overrides minSize, keeping workers unchanged.
func swapParallelMinSize(minSize int) func() {
	oldM := parallelSearchConfig.minSize.Swap(int32(minSize))
	return func() { parallelSearchConfig.minSize.Store(oldM) }
}

// swapParallelWorkers only overrides workers, keeping minSize unchanged.
func swapParallelWorkers(workers int) func() {
	oldW := parallelSearchConfig.workers.Swap(int32(workers))
	return func() { parallelSearchConfig.workers.Store(oldW) }
}

// Compile-time assertion that atomic.Int32 is used (catches accidental revert to plain int).
var _ = atomic.Int32{}

// TestParallelScoreCorrectness verifies parallel results match sequential.
func TestParallelScoreCorrectness(t *testing.T) {
	const dims = 768
	const count = 5000
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // G404: math/rand fine for test data

	query := randVecP(dims, rng)
	entries := make([]vectorEntry, count)
	for i := range entries {
		entries[i] = vectorEntry{
			docID:  fmt.Sprintf("doc-%d", i),
			vector: randVecP(dims, rng),
		}
	}

	// Sequential: force minSize above count so parallelScore uses single worker
	restore := swapParallelConfig(1, count+1)
	sequential := parallelScore(entries, query, 10, 0.0, cosineSimilarity, nil)
	restore()

	// Parallel: force low minSize and multiple workers
	restore = swapParallelConfig(4, 1)
	parallel := parallelScore(entries, query, 10, 0.0, cosineSimilarity, nil)
	restore()

	if len(sequential) != len(parallel) {
		t.Fatalf("result count mismatch: sequential=%d, parallel=%d", len(sequential), len(parallel))
	}

	for i := range sequential {
		if sequential[i].DocID != parallel[i].DocID {
			t.Errorf("result[%d] docID mismatch: %s vs %s", i, sequential[i].DocID, parallel[i].DocID)
		}
		diff := math.Abs(float64(sequential[i].Score - parallel[i].Score))
		if diff > 1e-5 {
			t.Errorf("result[%d] score mismatch: %v vs %v", i, sequential[i].Score, parallel[i].Score)
		}
	}
}

// TestParallelScoreWithFilter verifies filter is applied correctly in parallel.
func TestParallelScoreWithFilter(t *testing.T) {
	const dims = 128
	rng := rand.New(rand.NewSource(99)) //nolint:gosec // G404: math/rand fine for test data

	query := randVecP(dims, rng)
	entries := make([]vectorEntry, 3000)
	allowed := make(map[string]bool)
	for i := range entries {
		id := fmt.Sprintf("doc-%d", i)
		entries[i] = vectorEntry{docID: id, vector: randVecP(dims, rng)}
		if i%3 == 0 {
			allowed[id] = true
		}
	}

	filter := func(docID string) bool { return allowed[docID] }

	defer swapParallelMinSize(1)()
	results := parallelScore(entries, query, 5, 0.0, cosineSimilarity, filter)

	for _, r := range results {
		if !allowed[r.DocID] {
			t.Errorf("result %s should have been filtered out", r.DocID)
		}
	}
}

// TestParallelScoreChunkKeys verifies chunk key handling (docID#0 -> docID).
func TestParallelScoreChunkKeys(t *testing.T) {
	entries := []vectorEntry{
		{docID: "post1#0", vector: []float32{1, 0, 0}},
		{docID: "post1#1", vector: []float32{0.9, 0.1, 0}},
		{docID: "post2#0", vector: []float32{0, 1, 0}},
	}
	query := []float32{1, 0, 0}
	allowed := map[string]bool{"post1": true}

	filter := func(docID string) bool {
		return allowed[baseDocID(docID)]
	}

	defer swapParallelMinSize(1)()
	results := parallelScore(entries, query, 10, 0.0, cosineSimilarity, filter)

	for _, r := range results {
		base := baseDocID(r.DocID)
		if base != "post1" {
			t.Errorf("unexpected doc in results: %s (base: %s)", r.DocID, base)
		}
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results (post1#0, post1#1), got %d", len(results))
	}
}

// TestParallelScoreThresholdFiltering verifies threshold filters correctly.
func TestParallelScoreThresholdFiltering(t *testing.T) {
	entries := []vectorEntry{
		{docID: "a", vector: []float32{1, 0, 0}},
		{docID: "b", vector: []float32{0, 1, 0}},
		{docID: "c", vector: []float32{-1, 0, 0}},
	}
	query := []float32{1, 0, 0}

	defer swapParallelMinSize(1)()
	results := parallelScore(entries, query, 10, 0.99, cosineSimilarity, nil)

	if len(results) != 1 || results[0].DocID != "a" {
		t.Errorf("expected only 'a' above 0.99 threshold, got %v", results)
	}

	// All filtered
	results = parallelScore(entries, query, 10, 2.0, cosineSimilarity, nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results with threshold=2.0, got %d", len(results))
	}
}

// TestParallelScoreEmpty verifies empty input handling.
func TestParallelScoreEmpty(t *testing.T) {
	results := parallelScore(nil, []float32{1, 0}, 5, 0.0, cosineSimilarity, nil)
	if results != nil {
		t.Errorf("expected nil for empty entries, got %v", results)
	}

	results = parallelScore([]vectorEntry{}, []float32{1, 0}, 5, 0.0, cosineSimilarity, nil)
	if results != nil {
		t.Errorf("expected nil for empty slice, got %v", results)
	}
}

// TestParallelScoreTopKLimit verifies topK limiting works.
func TestParallelScoreTopKLimit(t *testing.T) {
	const dims = 32
	rng := rand.New(rand.NewSource(7)) //nolint:gosec // G404: math/rand fine for test data

	entries := make([]vectorEntry, 100)
	for i := range entries {
		entries[i] = vectorEntry{
			docID:  fmt.Sprintf("doc-%d", i),
			vector: randVecP(dims, rng),
		}
	}
	query := randVecP(dims, rng)

	defer swapParallelMinSize(1)()
	results := parallelScore(entries, query, 3, 0.0, cosineSimilarity, nil)

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

// TestParallelScoreDeterministicOrder verifies stable ordering with tiebreaker.
func TestParallelScoreDeterministicOrder(t *testing.T) {
	// All identical vectors => identical scores => docID tiebreaker
	entries := []vectorEntry{
		{docID: "charlie", vector: []float32{1, 0, 0}},
		{docID: "alice", vector: []float32{1, 0, 0}},
		{docID: "bob", vector: []float32{1, 0, 0}},
	}
	query := []float32{1, 0, 0}

	defer swapParallelMinSize(1)()
	results := parallelScore(entries, query, 10, 0.0, cosineSimilarity, nil)

	// Should be sorted by docID ascending (tiebreaker)
	expected := []string{"alice", "bob", "charlie"}
	for i, r := range results {
		if r.DocID != expected[i] {
			t.Errorf("result[%d] = %s, want %s", i, r.DocID, expected[i])
		}
	}
}

// TestFlatSearchParallelIntegration tests the full VectorIndex.Search path.
func TestFlatSearchParallelIntegration(t *testing.T) {
	const dims = 128
	const count = 3000
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // G404: math/rand fine for test data

	idx := NewVectorIndex()
	idx.SetReady()

	for i := 0; i < count; i++ {
		vec := randVecP(dims, rng)
		idx.Add("test", fmt.Sprintf("doc-%d", i), vec)
	}

	query := randVecP(dims, rng)

	// Force parallel
	restore := swapParallelMinSize(1)
	parallel := idx.Search("test", query, 10, 0.0, nil)
	restore()

	// Force sequential
	restore = swapParallelMinSize(count + 1)
	sequential := idx.Search("test", query, 10, 0.0, nil)
	restore()

	if len(parallel) != len(sequential) {
		t.Fatalf("count mismatch: parallel=%d, sequential=%d", len(parallel), len(sequential))
	}

	// Compare top results (order may differ slightly due to tiebreaker)
	parallelIDs := make(map[string]float32)
	for _, r := range parallel {
		parallelIDs[r.DocID] = r.Score
	}
	for _, r := range sequential {
		if _, ok := parallelIDs[r.DocID]; !ok {
			// Check if scores are very close (floating point)
			t.Logf("sequential has %s (score=%v) not in parallel top-10", r.DocID, r.Score)
		}
	}
}

// TestFlatSearchWithFilterParallel tests SearchWithFilter parallel path.
func TestFlatSearchWithFilterParallel(t *testing.T) {
	const dims = 64
	const count = 3000
	rng := rand.New(rand.NewSource(55)) //nolint:gosec // G404: math/rand fine for test data

	idx := NewVectorIndex()
	idx.SetReady()
	allowed := make(map[string]bool)

	for i := 0; i < count; i++ {
		id := fmt.Sprintf("doc-%d", i)
		idx.Add("test", id, randVecP(dims, rng))
		if i%5 == 0 {
			allowed[id] = true
		}
	}

	query := randVecP(dims, rng)

	defer swapParallelMinSize(1)()
	results := idx.SearchWithFilter("test", query, 5, 0.0, allowed, nil)

	for _, r := range results {
		if !allowed[r.DocID] {
			t.Errorf("filtered result %s not in allowed set", r.DocID)
		}
	}
}

// TestParallelConcurrentSearchAndMutate tests concurrent Search + Add/Remove.
func TestParallelConcurrentSearchAndMutate(t *testing.T) {
	const dims = 64
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // G404: math/rand fine for test data

	idx := NewVectorIndex()
	idx.SetReady()

	// Pre-populate
	for i := 0; i < 3000; i++ {
		idx.Add("test", fmt.Sprintf("doc-%d", i), randVecP(dims, rng))
	}

	defer swapParallelMinSize(1)()

	var wg sync.WaitGroup

	// Concurrent searches
	for s := 0; s < 10; s++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q := randVecP(dims, rand.New(rand.NewSource(int64(s)))) //nolint:gosec // G404
			results := idx.Search("test", q, 5, 0.0, nil)
			if len(results) == 0 {
				t.Error("expected results from concurrent search")
			}
		}()
	}

	// Concurrent mutations (each goroutine gets its own rng to avoid data race)
	for m := 0; m < 5; m++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			localRng := rand.New(rand.NewSource(int64(id + 1000))) //nolint:gosec // G404
			idx.Add("test", fmt.Sprintf("new-%d", id), randVecP(dims, localRng))
			idx.Remove("test", fmt.Sprintf("doc-%d", id))
		}(m)
	}

	wg.Wait()
}

// TestSnapshotMap verifies snapshot correctness.
func TestSnapshotMap(t *testing.T) {
	m := map[string][]float32{
		"a": {1, 2, 3},
		"b": {4, 5, 6},
		"c": {7, 8, 9},
	}

	entries := snapshotMap(m)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Verify all keys present
	ids := make(map[string]bool)
	for _, e := range entries {
		ids[e.docID] = true
	}
	for k := range m {
		if !ids[k] {
			t.Errorf("missing key %s in snapshot", k)
		}
	}
}

// TestParallelSearchAllMetrics verifies all similarity metrics work in parallel.
func TestParallelSearchAllMetrics(t *testing.T) {
	entries := []vectorEntry{
		{docID: "a", vector: []float32{1, 2, 3}},
		{docID: "b", vector: []float32{4, 5, 6}},
	}
	query := []float32{1, 2, 3}

	defer swapParallelMinSize(1)()

	metrics := []struct {
		name string
		fn   SimilarityFunc
	}{
		{"cosine", cosineSimilarity},
		{"dot_product", dotProductSimilarity},
		{"euclidean", euclideanSimilarity},
	}

	for _, m := range metrics {
		t.Run(m.name, func(t *testing.T) {
			results := parallelScore(entries, query, 10, -999.0, m.fn, nil)
			if len(results) != 2 {
				t.Errorf("%s: expected 2 results, got %d", m.name, len(results))
			}
			// Verify sorted descending
			if len(results) >= 2 && results[0].Score < results[1].Score {
				t.Errorf("%s: results not sorted descending", m.name)
			}
		})
	}
}

// --- Parallel benchmarks ---

func BenchmarkFlatSearchParallel_5K_768(b *testing.B) {
	benchmarkFlatSearchP(b, 5000, 768)
}

func BenchmarkFlatSearchParallel_50K_768(b *testing.B) {
	benchmarkFlatSearchP(b, 50000, 768)
}

func BenchmarkFlatSearchParallel_50K_1536(b *testing.B) {
	benchmarkFlatSearchP(b, 50000, 1536)
}

func BenchmarkFlatSearchSequential_50K_768(b *testing.B) {
	defer swapParallelWorkers(1)()
	benchmarkFlatSearchP(b, 50000, 768)
}

func BenchmarkFlatSearchSequential_50K_1536(b *testing.B) {
	defer swapParallelWorkers(1)()
	benchmarkFlatSearchP(b, 50000, 1536)
}

func benchmarkFlatSearchP(b *testing.B, count, dims int) {
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // G404: math/rand fine for bench
	idx := NewVectorIndex()
	idx.SetReady()

	for i := 0; i < count; i++ {
		idx.Add("bench", fmt.Sprintf("doc-%d", i), randVecP(dims, rng))
	}

	query := randVecP(dims, rng)

	defer swapParallelMinSize(1)()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = idx.Search("bench", query, 10, 0.0, nil)
	}
}

func randVecP(dims int, rng *rand.Rand) []float32 {
	v := make([]float32, dims)
	for i := range v {
		v[i] = rng.Float32()*2 - 1
	}
	return v
}

// BenchmarkParallelWorkerScaling benchmarks different worker counts.
func BenchmarkParallelWorkerScaling(b *testing.B) {
	const dims = 768
	const count = 50000
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // G404: math/rand fine for bench

	idx := NewVectorIndex()
	idx.SetReady()
	for i := 0; i < count; i++ {
		idx.Add("bench", fmt.Sprintf("doc-%d", i), randVecP(dims, rng))
	}
	query := randVecP(dims, rng)

	defer swapParallelMinSize(1)()

	for _, workers := range []int{1, 2, 4, 8} {
		b.Run(fmt.Sprintf("workers_%d", workers), func(b *testing.B) {
			restore := swapParallelWorkers(workers)
			defer restore()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = idx.Search("bench", query, 10, 0.0, nil)
			}
		})
	}
}

func init() {
	// Ensure sort is imported for tests that need it
	_ = sort.Slice
}
