package main

import (
	"math"
	"sort"
	"sync"
	"sync/atomic"
)

// VectorResult represents a single vector search result.
type VectorResult struct {
	DocID string
	Score float32
}

// VectorIndex is an in-memory flat index for vector similarity search.
// It stores vectors per collection and performs brute-force cosine similarity.
type VectorIndex struct {
	mu         sync.RWMutex
	collections map[string]map[string][]float32 // collection -> docID -> vector
	ready      atomic.Bool
}

// NewVectorIndex creates a new empty vector index.
func NewVectorIndex() *VectorIndex {
	return &VectorIndex{
		collections: make(map[string]map[string][]float32),
	}
}

// IsReady returns whether the index has finished loading.
func (vi *VectorIndex) IsReady() bool {
	return vi.ready.Load()
}

// SetReady marks the index as ready for queries.
func (vi *VectorIndex) SetReady() {
	vi.ready.Store(true)
}

// Add inserts or updates a vector in the index.
func (vi *VectorIndex) Add(collection, docID string, vector []float32) {
	vi.mu.Lock()
	defer vi.mu.Unlock()

	if vi.collections[collection] == nil {
		vi.collections[collection] = make(map[string][]float32)
	}
	vi.collections[collection][docID] = vector
}

// Remove deletes a vector from the index.
func (vi *VectorIndex) Remove(collection, docID string) {
	vi.mu.Lock()
	defer vi.mu.Unlock()

	if coll, ok := vi.collections[collection]; ok {
		delete(coll, docID)
	}
}

// Search finds the top-K most similar vectors to the query vector.
func (vi *VectorIndex) Search(collection string, query []float32, topK int, threshold float64) []VectorResult {
	vi.mu.RLock()
	defer vi.mu.RUnlock()

	coll, ok := vi.collections[collection]
	if !ok || len(coll) == 0 {
		return nil
	}

	if topK <= 0 {
		topK = 5
	}

	// Compute cosine similarity for all vectors
	results := make([]VectorResult, 0, len(coll))
	for docID, vec := range coll {
		score := cosineSimilarity(query, vec)
		if float64(score) >= threshold {
			results = append(results, VectorResult{DocID: docID, Score: score})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Limit to topK
	if len(results) > topK {
		results = results[:topK]
	}

	return results
}

// SearchWithFilter performs vector search only on documents matching the given docID set.
func (vi *VectorIndex) SearchWithFilter(collection string, query []float32, topK int, threshold float64, allowedDocIDs map[string]bool) []VectorResult {
	vi.mu.RLock()
	defer vi.mu.RUnlock()

	coll, ok := vi.collections[collection]
	if !ok || len(coll) == 0 {
		return nil
	}

	if topK <= 0 {
		topK = 5
	}

	results := make([]VectorResult, 0, min(len(allowedDocIDs), len(coll)))
	for docID, vec := range coll {
		if !allowedDocIDs[docID] {
			continue
		}
		score := cosineSimilarity(query, vec)
		if float64(score) >= threshold {
			results = append(results, VectorResult{DocID: docID, Score: score})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results
}

// CollectionSize returns the number of vectors in a collection.
func (vi *VectorIndex) CollectionSize(collection string) int {
	vi.mu.RLock()
	defer vi.mu.RUnlock()

	return len(vi.collections[collection])
}

// Collections returns all collection names that have vectors.
func (vi *VectorIndex) Collections() []string {
	vi.mu.RLock()
	defer vi.mu.RUnlock()

	names := make([]string, 0, len(vi.collections))
	for name := range vi.collections {
		names = append(names, name)
	}
	return names
}

// cosineSimilarity computes cosine similarity between two vectors.
// Returns value between -1 and 1, where 1 = identical direction.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / float32(math.Sqrt(float64(normA)*float64(normB)))
}
