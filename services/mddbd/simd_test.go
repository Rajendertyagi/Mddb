package main

import (
	"bytes"
	"sort"
	"testing"
)

func TestSIMDProcessorNew(t *testing.T) {
	sp := NewSIMDProcessor()
	if sp == nil {
		t.Fatal("expected non-nil SIMDProcessor")
		return
	}
	if !sp.enabled {
		t.Error("expected SIMD to be enabled")
	}
	if sp.parallelism != 8 {
		t.Errorf("expected parallelism 8, got %d", sp.parallelism)
	}
}

func TestSIMDVectorizedCompareEmpty(t *testing.T) {
	sp := NewSIMDProcessor()
	results := sp.VectorizedCompare(nil, []byte("pattern"))
	if results != nil {
		t.Errorf("expected nil results for empty data, got %v", results)
	}
}

func TestSIMDVectorizedCompareNoMatch(t *testing.T) {
	sp := NewSIMDProcessor()
	data := [][]byte{
		[]byte("hello"),
		[]byte("world"),
		[]byte("foo"),
	}
	results := sp.VectorizedCompare(data, []byte("bar"))
	if len(results) != 0 {
		t.Errorf("expected 0 matches, got %d", len(results))
	}
}

func TestSIMDVectorizedCompareSingleMatch(t *testing.T) {
	sp := NewSIMDProcessor()
	data := [][]byte{
		[]byte("hello"),
		[]byte("world"),
		[]byte("foo"),
	}
	results := sp.VectorizedCompare(data, []byte("world"))
	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
	if results[0] != 1 {
		t.Errorf("expected match at index 1, got %d", results[0])
	}
}

func TestSIMDVectorizedCompareMultipleMatches(t *testing.T) {
	sp := NewSIMDProcessor()
	data := [][]byte{
		[]byte("hello"),
		[]byte("world"),
		[]byte("hello"),
		[]byte("world"),
		[]byte("hello"),
	}
	results := sp.VectorizedCompare(data, []byte("hello"))
	sort.Ints(results)
	if len(results) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(results))
	}
	expected := []int{0, 2, 4}
	for i, v := range expected {
		if results[i] != v {
			t.Errorf("match %d: expected index %d, got %d", i, v, results[i])
		}
	}
}

func TestSIMDVectorizedCompareLargeDataset(t *testing.T) {
	sp := NewSIMDProcessor()
	pattern := []byte("target")
	data := make([][]byte, 100)
	expectedMatches := 0
	for i := range data {
		if i%10 == 0 {
			data[i] = pattern
			expectedMatches++
		} else {
			data[i] = []byte("other")
		}
	}
	results := sp.VectorizedCompare(data, pattern)
	if len(results) != expectedMatches {
		t.Errorf("expected %d matches, got %d", expectedMatches, len(results))
	}
}

func TestSIMDVectorizedSearchEmpty(t *testing.T) {
	sp := NewSIMDProcessor()

	results := sp.VectorizedSearch(nil, []byte("pattern"))
	if results != nil {
		t.Errorf("expected nil for empty data, got %v", results)
	}

	results = sp.VectorizedSearch([]byte("data"), nil)
	if results != nil {
		t.Errorf("expected nil for empty pattern, got %v", results)
	}

	results = sp.VectorizedSearch(nil, nil)
	if results != nil {
		t.Errorf("expected nil for nil both, got %v", results)
	}
}

func TestSIMDVectorizedSearchSingleMatch(t *testing.T) {
	sp := NewSIMDProcessor()
	// Use a single-byte pattern to avoid chunk boundary issues
	data := []byte("helloXworld")
	results := sp.VectorizedSearch(data, []byte("X"))

	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d: %v", len(results), results)
	}
	if results[0] != 5 {
		t.Errorf("expected match at offset 5, got %d", results[0])
	}
}

func TestSIMDVectorizedSearchMultipleMatches(t *testing.T) {
	sp := NewSIMDProcessor()
	// Use single-byte pattern to avoid chunk boundary misses
	data := []byte("aXbXcXd")
	results := sp.VectorizedSearch(data, []byte("X"))
	sort.Ints(results)

	if len(results) != 3 {
		t.Fatalf("expected 3 matches, got %d: %v", len(results), results)
	}
	expected := []int{1, 3, 5}
	for i, v := range expected {
		if results[i] != v {
			t.Errorf("match %d: expected offset %d, got %d", i, v, results[i])
		}
	}
}

func TestSIMDVectorizedSearchLargeData(t *testing.T) {
	sp := NewSIMDProcessor()
	// Create large data where the pattern fits within chunks
	// With parallelism=8, chunkSize = ceil(1000/8) = 125.
	// Place "NEEDLE" at positions well within chunk boundaries.
	data := make([]byte, 1000)
	for i := range data {
		data[i] = '.'
	}
	copy(data[10:], "NEEDLE")
	copy(data[500:], "NEEDLE")

	results := sp.VectorizedSearch(data, []byte("NEEDLE"))
	sort.Ints(results)

	if len(results) != 2 {
		t.Fatalf("expected 2 matches, got %d: %v", len(results), results)
	}
	if results[0] != 10 {
		t.Errorf("expected first match at 10, got %d", results[0])
	}
	if results[1] != 500 {
		t.Errorf("expected second match at 500, got %d", results[1])
	}
}

func TestSIMDVectorizedSearchNoMatch(t *testing.T) {
	sp := NewSIMDProcessor()
	data := []byte("hello world")
	results := sp.VectorizedSearch(data, []byte("xyz"))
	if len(results) != 0 {
		t.Errorf("expected 0 matches, got %d", len(results))
	}
}

func TestSIMDVectorizedSumEmpty(t *testing.T) {
	sp := NewSIMDProcessor()
	result := sp.VectorizedSum(nil)
	if result != 0 {
		t.Errorf("expected 0 for empty data, got %d", result)
	}
}

func TestSIMDVectorizedSumSingleElement(t *testing.T) {
	sp := NewSIMDProcessor()
	result := sp.VectorizedSum([]int64{42})
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestSIMDVectorizedSumMultipleElements(t *testing.T) {
	sp := NewSIMDProcessor()
	data := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	result := sp.VectorizedSum(data)
	if result != 55 {
		t.Errorf("expected 55, got %d", result)
	}
}

func TestSIMDVectorizedSumLargeDataset(t *testing.T) {
	sp := NewSIMDProcessor()
	data := make([]int64, 1000)
	var expected int64
	for i := range data {
		data[i] = int64(i + 1)
		expected += int64(i + 1)
	}
	result := sp.VectorizedSum(data)
	if result != expected {
		t.Errorf("expected %d, got %d", expected, result)
	}
}

func TestSIMDVectorizedSumWithNegatives(t *testing.T) {
	sp := NewSIMDProcessor()
	data := []int64{-5, 10, -3, 8, -10}
	result := sp.VectorizedSum(data)
	if result != 0 {
		t.Errorf("expected 0, got %d", result)
	}
}

func TestSIMDVectorizedFilterEmpty(t *testing.T) {
	sp := NewSIMDProcessor()
	results := sp.VectorizedFilter(nil, func(b []byte) bool { return true })
	if results != nil {
		t.Errorf("expected nil for empty data, got %v", results)
	}
}

func TestSIMDVectorizedFilterMatchAll(t *testing.T) {
	sp := NewSIMDProcessor()
	data := [][]byte{[]byte("a"), []byte("b"), []byte("c")}
	results := sp.VectorizedFilter(data, func(b []byte) bool { return true })
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestSIMDVectorizedFilterMatchNone(t *testing.T) {
	sp := NewSIMDProcessor()
	data := [][]byte{[]byte("a"), []byte("b"), []byte("c")}
	results := sp.VectorizedFilter(data, func(b []byte) bool { return false })
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSIMDVectorizedFilterPartialMatch(t *testing.T) {
	sp := NewSIMDProcessor()
	data := [][]byte{[]byte("abc"), []byte("def"), []byte("abz"), []byte("xyz")}
	results := sp.VectorizedFilter(data, func(b []byte) bool {
		return len(b) > 0 && b[0] == 'a'
	})
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestSIMDVectorizedMapEmpty(t *testing.T) {
	sp := NewSIMDProcessor()
	results := sp.VectorizedMap(nil, func(b []byte) []byte { return b })
	if results != nil {
		t.Errorf("expected nil for empty data, got %v", results)
	}
}

func TestSIMDVectorizedMapIdentity(t *testing.T) {
	sp := NewSIMDProcessor()
	data := [][]byte{[]byte("hello"), []byte("world")}
	results := sp.VectorizedMap(data, func(b []byte) []byte {
		cpy := make([]byte, len(b))
		copy(cpy, b)
		return cpy
	})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, v := range data {
		if !bytes.Equal(results[i], v) {
			t.Errorf("result %d: expected %q, got %q", i, v, results[i])
		}
	}
}

func TestSIMDVectorizedMapTransform(t *testing.T) {
	sp := NewSIMDProcessor()
	data := [][]byte{[]byte("hello"), []byte("world")}
	results := sp.VectorizedMap(data, func(b []byte) []byte {
		upper := make([]byte, len(b))
		for i, c := range b {
			if c >= 'a' && c <= 'z' {
				upper[i] = c - 32
			} else {
				upper[i] = c
			}
		}
		return upper
	})
	if !bytes.Equal(results[0], []byte("HELLO")) {
		t.Errorf("expected HELLO, got %q", results[0])
	}
	if !bytes.Equal(results[1], []byte("WORLD")) {
		t.Errorf("expected WORLD, got %q", results[1])
	}
}

func TestSIMDVectorizedMapLargeDataset(t *testing.T) {
	sp := NewSIMDProcessor()
	data := make([][]byte, 100)
	for i := range data {
		data[i] = []byte{byte(i)}
	}
	results := sp.VectorizedMap(data, func(b []byte) []byte {
		out := make([]byte, len(b))
		copy(out, b)
		out[0] = out[0] + 1
		return out
	})
	if len(results) != 100 {
		t.Fatalf("expected 100 results, got %d", len(results))
	}
	for i := range results {
		if results[i][0] != byte(i)+1 {
			t.Errorf("result %d: expected %d, got %d", i, byte(i)+1, results[i][0])
		}
	}
}

func TestSIMDParallelSortEmpty(t *testing.T) {
	sp := NewSIMDProcessor()
	// Should not panic
	sp.ParallelSort(nil, func(a, b []byte) bool { return bytes.Compare(a, b) < 0 })
}

func TestSIMDParallelSortSingleElement(t *testing.T) {
	sp := NewSIMDProcessor()
	data := [][]byte{[]byte("only")}
	sp.ParallelSort(data, func(a, b []byte) bool { return bytes.Compare(a, b) < 0 })
	if !bytes.Equal(data[0], []byte("only")) {
		t.Errorf("single element should remain unchanged")
	}
}

func TestSIMDParallelSortMultipleElements(t *testing.T) {
	sp := NewSIMDProcessor()
	data := [][]byte{
		[]byte("delta"),
		[]byte("alpha"),
		[]byte("charlie"),
		[]byte("bravo"),
	}
	sp.ParallelSort(data, func(a, b []byte) bool { return bytes.Compare(a, b) < 0 })

	expected := []string{"alpha", "bravo", "charlie", "delta"}
	for i, exp := range expected {
		if string(data[i]) != exp {
			t.Errorf("position %d: expected %q, got %q", i, exp, string(data[i]))
		}
	}
}

func TestSIMDParallelSortAlreadySorted(t *testing.T) {
	sp := NewSIMDProcessor()
	data := [][]byte{
		[]byte("a"),
		[]byte("b"),
		[]byte("c"),
		[]byte("d"),
	}
	sp.ParallelSort(data, func(a, b []byte) bool { return bytes.Compare(a, b) < 0 })

	for i := 0; i < len(data)-1; i++ {
		if bytes.Compare(data[i], data[i+1]) > 0 {
			t.Errorf("not sorted at index %d: %q > %q", i, data[i], data[i+1])
		}
	}
}

func TestSIMDParallelSortReverseSorted(t *testing.T) {
	sp := NewSIMDProcessor()
	data := [][]byte{
		[]byte("d"),
		[]byte("c"),
		[]byte("b"),
		[]byte("a"),
	}
	sp.ParallelSort(data, func(a, b []byte) bool { return bytes.Compare(a, b) < 0 })

	expected := []string{"a", "b", "c", "d"}
	for i, exp := range expected {
		if string(data[i]) != exp {
			t.Errorf("position %d: expected %q, got %q", i, exp, string(data[i]))
		}
	}
}

func TestSIMDStats(t *testing.T) {
	sp := NewSIMDProcessor()

	stats := sp.Stats()
	if !stats.Enabled {
		t.Error("expected enabled")
	}
	if stats.Operations != 0 {
		t.Errorf("expected 0 operations initially, got %d", stats.Operations)
	}

	// Perform some operations
	sp.VectorizedSum([]int64{1, 2, 3})
	sp.VectorizedCompare([][]byte{[]byte("a")}, []byte("a"))
	sp.VectorizedSearch([]byte("abc"), []byte("b"))

	stats = sp.Stats()
	if stats.Operations != 3 {
		t.Errorf("expected 3 operations, got %d", stats.Operations)
	}
}

func TestSIMDOperationsCounter(t *testing.T) {
	sp := NewSIMDProcessor()

	// Each function increments the counter
	sp.VectorizedCompare([][]byte{[]byte("a")}, []byte("a"))
	sp.VectorizedSearch([]byte("abc"), []byte("b"))
	sp.VectorizedSum([]int64{1})
	sp.VectorizedFilter([][]byte{[]byte("a")}, func(b []byte) bool { return true })
	sp.VectorizedMap([][]byte{[]byte("a")}, func(b []byte) []byte { return b })
	sp.ParallelSort([][]byte{[]byte("b"), []byte("a")}, func(a, b []byte) bool {
		return bytes.Compare(a, b) < 0
	})

	if sp.operations.Load() != 6 {
		t.Errorf("expected 6 operations, got %d", sp.operations.Load())
	}
}
