package main

// VectorSearcher is the interface for all vector search algorithms.
type VectorSearcher interface {
	// Add inserts or updates a vector in the index.
	Add(collection, docID string, vector []float32)
	// Remove deletes a vector from the index.
	Remove(collection, docID string)
	// Search finds the top-K most similar vectors to the query vector.
	// metric may be nil, in which case cosine similarity is used.
	Search(collection string, query []float32, topK int, threshold float64, metric SimilarityFunc) []VectorResult
	// SearchWithFilter searches only among allowed doc IDs.
	// metric may be nil, in which case cosine similarity is used.
	SearchWithFilter(collection string, query []float32, topK int, threshold float64, allowed map[string]bool, metric SimilarityFunc) []VectorResult
	// CollectionSize returns the number of vectors in a collection.
	CollectionSize(collection string) int
	// Collections returns all collection names that have vectors.
	Collections() []string
	// IsReady returns whether the index is loaded and ready.
	IsReady() bool
	// SetReady marks the index as ready for queries.
	SetReady()
	// Name returns the algorithm name (e.g. "flat", "hnsw", "ivf", "pq", "opq", "sq", "bq").
	Name() string
}

// Trainable is an optional interface for indexes that require training (e.g. IVF, PQ).
type Trainable interface {
	// Train builds the index structure from the current vectors.
	// Called after loading vectors or after reindexing.
	Train(collection string, vectors map[string][]float32)
}
