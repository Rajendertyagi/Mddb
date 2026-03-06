package main

import (
	"math"
	"sort"
	"strings"
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
// Keys may include chunk suffixes (e.g. "docID#0", "docID#1").
type VectorIndex struct {
	mu          sync.RWMutex
	collections map[string]map[string][]float32 // collection -> key -> vector
	ready       atomic.Bool
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
func (vi *VectorIndex) Search(collection string, query []float32, topK int, threshold float64, metric SimilarityFunc) []VectorResult {
	vi.mu.RLock()
	defer vi.mu.RUnlock()

	coll, ok := vi.collections[collection]
	if !ok || len(coll) == 0 {
		return nil
	}

	if topK <= 0 {
		topK = 5
	}
	if metric == nil {
		metric = cosineSimilarity
	}

	// Compute similarity for all vectors
	results := make([]VectorResult, 0, len(coll))
	for docID, vec := range coll {
		score := metric(query, vec)
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
// Handles chunk keys: if the index has "docID#0" and allowed has "docID", it matches.
func (vi *VectorIndex) SearchWithFilter(collection string, query []float32, topK int, threshold float64, allowedDocIDs map[string]bool, metric SimilarityFunc) []VectorResult {
	vi.mu.RLock()
	defer vi.mu.RUnlock()

	coll, ok := vi.collections[collection]
	if !ok || len(coll) == 0 {
		return nil
	}

	if topK <= 0 {
		topK = 5
	}
	if metric == nil {
		metric = cosineSimilarity
	}

	results := make([]VectorResult, 0, min(len(allowedDocIDs), len(coll)))
	for docID, vec := range coll {
		baseID := baseDocID(docID)
		if !allowedDocIDs[baseID] {
			continue
		}
		score := metric(query, vec)
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

// Name returns the algorithm name.
func (vi *VectorIndex) Name() string {
	return "flat"
}

// SimilarityFunc computes similarity between two vectors.
// Higher values = more similar. Used as a configurable distance metric.
type SimilarityFunc func(a, b []float32) float32

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

// dotProductSimilarity computes the dot product between two vectors.
// For normalized vectors (e.g. OpenAI embeddings) this equals cosine similarity.
func dotProductSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// euclideanSimilarity converts Euclidean distance to a similarity score.
// Returns 1/(1+dist), so closer vectors → higher score (range 0 to 1).
func euclideanSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var sum float64
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		sum += d * d
	}
	return float32(1.0 / (1.0 + math.Sqrt(sum)))
}

// ResolveSimilarity returns the SimilarityFunc for a given metric name.
// Defaults to cosineSimilarity if the name is unknown or empty.
func ResolveSimilarity(name string) SimilarityFunc {
	switch name {
	case "dot_product":
		return dotProductSimilarity
	case "euclidean":
		return euclideanSimilarity
	default:
		return cosineSimilarity
	}
}

// baseDocID extracts the base document ID from a possibly chunked key.
// "docID#0" -> "docID", "docID" -> "docID"
func baseDocID(key string) string {
	if idx := strings.IndexByte(key, '#'); idx >= 0 {
		return key[:idx]
	}
	return key
}

// DeduplicateChunkResults groups vector results by base docID and returns
// the highest score per document. Useful when chunks produce multiple results
// for the same document.
func DeduplicateChunkResults(results []VectorResult) []VectorResult {
	if len(results) == 0 {
		return results
	}

	best := make(map[string]float32)
	for _, r := range results {
		base := baseDocID(r.DocID)
		if score, exists := best[base]; !exists || r.Score > score {
			best[base] = r.Score
		}
	}

	deduped := make([]VectorResult, 0, len(best))
	for docID, score := range best {
		deduped = append(deduped, VectorResult{DocID: docID, Score: score})
	}

	sort.Slice(deduped, func(i, j int) bool {
		return deduped[i].Score > deduped[j].Score
	})

	return deduped
}
