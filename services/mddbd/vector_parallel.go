package main

import (
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
)

// parallelSearchConfig holds runtime-resolved parallel search settings.
var parallelSearchConfig struct {
	workers int
	minSize int
}

func init() {
	parallelSearchConfig.workers = runtime.NumCPU()
	if parallelSearchConfig.workers > 16 {
		parallelSearchConfig.workers = 16
	}
	if v := os.Getenv("MDDB_VECTOR_PARALLEL_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			parallelSearchConfig.workers = n
		}
	}

	parallelSearchConfig.minSize = 2048
	if v := os.Getenv("MDDB_VECTOR_PARALLEL_MIN_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			parallelSearchConfig.minSize = n
		}
	}
}

// vectorEntry is a snapshot of a map entry for parallel processing.
// Copies slice headers (not underlying float data) for zero-copy fan-out.
type vectorEntry struct {
	docID  string
	vector []float32
}

// snapshotMap converts a map into a slice for parallel iteration.
// O(n) pointer copies — does not deep-copy vector data.
func snapshotMap(m map[string][]float32) []vectorEntry {
	entries := make([]vectorEntry, 0, len(m))
	for id, vec := range m {
		entries = append(entries, vectorEntry{docID: id, vector: vec})
	}
	return entries
}

// parallelScore scores entries across multiple goroutines and returns
// merged, sorted, topK-limited results. Each worker operates on a
// disjoint index range — zero contention during scoring.
// filter: nil = no filter (Search), non-nil = per-entry filter (SearchWithFilter).
func parallelScore(
	entries []vectorEntry,
	query []float32,
	topK int,
	threshold float64,
	metric SimilarityFunc,
	filter func(string) bool,
) []VectorResult {
	n := len(entries)
	if n == 0 {
		return nil
	}

	// Determine actual worker count, capped by data size
	workers := parallelSearchConfig.workers
	maxWorkers := (n + parallelSearchConfig.minSize - 1) / parallelSearchConfig.minSize
	if workers > maxWorkers {
		workers = maxWorkers
	}
	if workers < 1 {
		workers = 1
	}

	// Single worker: skip goroutine overhead
	if workers == 1 {
		results := scoreRange(entries, 0, n, query, topK, threshold, metric, filter)
		sort.Slice(results, func(i, j int) bool {
			if results[i].Score != results[j].Score {
				return results[i].Score > results[j].Score
			}
			return results[i].DocID < results[j].DocID
		})
		if len(results) > topK {
			results = results[:topK]
		}
		return results
	}

	chunkSize := (n + workers - 1) / workers
	partials := make([][]VectorResult, workers)

	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		go func(workerID int) {
			defer wg.Done()
			start := workerID * chunkSize
			end := start + chunkSize
			if end > n {
				end = n
			}
			partials[workerID] = scoreRange(entries, start, end, query, topK, threshold, metric, filter)
		}(w)
	}

	wg.Wait()

	// Merge partial results
	total := 0
	for _, p := range partials {
		total += len(p)
	}
	merged := make([]VectorResult, 0, total)
	for _, p := range partials {
		merged = append(merged, p...)
	}

	// Sort by score descending, docID ascending (tiebreaker for determinism)
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Score != merged[j].Score {
			return merged[i].Score > merged[j].Score
		}
		return merged[i].DocID < merged[j].DocID
	})

	if len(merged) > topK {
		merged = merged[:topK]
	}
	return merged
}

// scoreRange scores entries[start:end] sequentially, returning filtered results.
func scoreRange(
	entries []vectorEntry,
	start, end int,
	query []float32,
	topK int,
	threshold float64,
	metric SimilarityFunc,
	filter func(string) bool,
) []VectorResult {
	results := make([]VectorResult, 0, min(end-start, topK*2))
	for i := start; i < end; i++ {
		e := &entries[i]
		if filter != nil && !filter(e.docID) {
			continue
		}
		score := metric(query, e.vector)
		if float64(score) >= threshold {
			results = append(results, VectorResult{DocID: e.docID, Score: score})
		}
	}
	return results
}
