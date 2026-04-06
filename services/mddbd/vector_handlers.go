package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	json "github.com/goccy/go-json"
	bolt "go.etcd.io/bbolt"
)

// VectorSearchRequest represents an HTTP vector search request.
type VectorSearchRequest struct {
	Collection     string              `json:"collection"`
	Query          string              `json:"query"`
	QueryVector    []float32           `json:"queryVector"`
	TopK           int                 `json:"topK"`
	Threshold      float64             `json:"threshold"`
	FilterMeta     map[string][]string `json:"filterMeta"`
	IncludeContent bool                `json:"includeContent"`
	Algorithm      string              `json:"algorithm"`      // "flat" (default), "hnsw", "ivf", "pq", "opq", "sq", "bq"
	DistanceMetric string              `json:"distanceMetric"` // "cosine" (default), "dot_product", "euclidean"
}

// VectorSearchResultItem represents a single search result.
type VectorSearchResultItem struct {
	Document Doc     `json:"document"`
	Score    float32 `json:"score"`
	Rank     int     `json:"rank"`
}

// VectorSearchResponseHTTP represents the response from vector search.
type VectorSearchResponseHTTP struct {
	Results        []VectorSearchResultItem `json:"results"`
	Total          int                      `json:"total"`
	Model          string                   `json:"model"`
	Dimensions     int                      `json:"dimensions"`
	Algorithm      string                   `json:"algorithm"`
	DistanceMetric string                   `json:"distanceMetric"`
	Stats          *SearchStats             `json:"searchStats,omitempty"`
}

// VectorReindexRequestHTTP represents a reindex request.
type VectorReindexRequestHTTP struct {
	Collection string `json:"collection"`
	Force      bool   `json:"force"`
}

// loadVectorIndex loads all vectors from BoltDB into all in-memory indexes.
func (s *Server) loadVectorIndex() {
	start := time.Now()

	// Get all collections with vectors
	counts, err := s.VectorStore.CountByCollection()
	if err != nil {
		log.Printf("ERROR: failed to count vector collections: %v", err)
		s.VectorIndex.SetReady()
		for _, searcher := range s.VectorSearchers {
			searcher.SetReady()
		}
		return
	}

	totalLoaded := 0
	for collection, count := range counts {
		records, err := s.VectorStore.LoadCollection(collection)
		if err != nil {
			log.Printf("ERROR: failed to load vectors for collection %q: %v", collection, err)
			continue
		}

		// Collect vectors for trainable indexes
		collVecs := make(map[string][]float32, len(records))

		for docID, rec := range records {
			// Add to all searchers (docID may be "id" or "id#0", "id#1", etc.)
			for name, searcher := range s.VectorSearchers {
				if name == "quantized" {
					continue // quantized index is populated separately below
				}
				searcher.Add(collection, docID, rec.Vector)
			}
			// Also add to quantized index (it will self-check if collection has quantization)
			if s.QuantizedVecIndex != nil {
				s.QuantizedVecIndex.Add(collection, docID, rec.Vector)
			}
			collVecs[docID] = rec.Vector
		}

		// Trigger training for trainable indexes (IVF, PQ)
		for _, searcher := range s.VectorSearchers {
			if trainer, ok := searcher.(Trainable); ok {
				trainer.Train(collection, collVecs)
			}
		}

		totalLoaded += count
	}

	// Mark all searchers as ready
	for _, searcher := range s.VectorSearchers {
		searcher.SetReady()
	}
	log.Printf("Vector index loaded: %d documents across %d collections in %v (algorithms: flat, hnsw, ivf, pq)",
		totalLoaded, len(counts), time.Since(start))
}

// handleVectorSearch handles POST /v1/vector-search
func (s *Server) handleVectorSearch(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var req VectorSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" {
		bad(w, errors.New("missing collection"))
		return
	}
	if req.Query == "" && len(req.QueryVector) == 0 {
		bad(w, errors.New("either query or queryVector is required"))
		return
	}

	// Select algorithm
	algo := req.Algorithm
	if algo == "" {
		algo = "flat"
	}

	// Auto-select quantized searcher if collection has quantization configured
	if algo == "flat" && s.QuantizedVecIndex != nil && s.QuantizedVecIndex.HasCollection(req.Collection) {
		algo = "quantized"
	}

	searcher, ok2 := s.VectorSearchers[algo]
	if !ok2 {
		bad(w, errors.New("unknown algorithm: "+algo+", available: flat, hnsw, ivf, pq, quantized"))
		return
	}
	if !searcher.IsReady() {
		// Fallback to flat if the requested algorithm isn't ready
		searcher = s.VectorSearchers["flat"]
		algo = "flat"
	}
	if !searcher.IsReady() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"vector index is loading, please retry"}`))
		return
	}

	// Get query vector
	var queryVector []float32
	if len(req.QueryVector) > 0 {
		queryVector = req.QueryVector
	} else if s.Embedding != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		var err error
		queryVector, err = s.Embedding.Embed(ctx, req.Query)
		if err != nil {
			bad(w, errors.New("failed to embed query: "+err.Error()))
			return
		}
	} else {
		bad(w, errors.New("no embedding provider configured and no queryVector provided"))
		return
	}

	topK := req.TopK
	if topK <= 0 {
		topK = 5
	}

	// Resolve distance metric
	metric := ResolveSimilarity(req.DistanceMetric)
	metricName := req.DistanceMetric
	if metricName == "" {
		metricName = "cosine"
	}

	// Oversample for chunk deduplication: search for more results, then deduplicate
	searchTopK := topK * 3
	if searchTopK < 20 {
		searchTopK = 20
	}

	// Search: with or without metadata filter
	var results []VectorResult
	if len(req.FilterMeta) > 0 {
		allowedIDs := s.getDocIDsByMeta(req.Collection, req.FilterMeta)
		if len(allowedIDs) == 0 {
			ok(w, VectorSearchResponseHTTP{
				Results:        []VectorSearchResultItem{},
				Total:          0,
				Algorithm:      algo,
				DistanceMetric: metricName,
			})
			return
		}
		results = searcher.SearchWithFilter(req.Collection, queryVector, searchTopK, req.Threshold, allowedIDs, metric)
	} else {
		results = searcher.Search(req.Collection, queryVector, searchTopK, req.Threshold, metric)
	}

	// Deduplicate chunk results: group by base docID, take max score
	results = DeduplicateChunkResults(results)
	if len(results) > topK {
		results = results[:topK]
	}

	// Track vector search operation
	if s.Metrics != nil {
		s.Metrics.IncOp("vector_search", algo)
	}

	// Load full documents for results
	items := make([]VectorSearchResultItem, 0, len(results))
	_ = s.DB.View(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs == nil {
			return nil
		}
		for rank, vr := range results {
			v := bDocs.Get(kDoc(req.Collection, vr.DocID))
			if v == nil {
				continue
			}
			docPtr, err := loadDoc(v)
			if err != nil {
				continue
			}
			doc := *docPtr
			if !req.IncludeContent {
				doc.ContentMD = ""
			}
			items = append(items, VectorSearchResultItem{
				Document: doc,
				Score:    vr.Score,
				Rank:     rank + 1,
			})
		}
		return nil
	})

	resp := VectorSearchResponseHTTP{
		Results:        items,
		Total:          len(items),
		Algorithm:      algo,
		DistanceMetric: metricName,
	}
	if s.Embedding != nil {
		resp.Model = s.Embedding.Model()
		resp.Dimensions = s.Embedding.Dimensions()
	}

	if searchStatsEnabled() {
		indexSize := 0
		if searcher != nil {
			indexSize = searcher.CollectionSize(req.Collection)
		}
		queryTerms := strings.Fields(req.Query)
		resp.Stats = &SearchStats{
			DurationMs:  float64(time.Since(start).Microseconds()) / 1000.0,
			QueryTerms:  queryTerms,
			IndexSize:   indexSize,
			TotalTokens: len(queryTerms),
		}
	}

	ok(w, resp)
}

// handleVectorReindex handles POST /v1/vector-reindex
func (s *Server) handleVectorReindex(w http.ResponseWriter, r *http.Request) {
	var req VectorReindexRequestHTTP
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" {
		bad(w, errors.New("missing collection"))
		return
	}
	if s.Embedding == nil {
		bad(w, errors.New("no embedding provider configured"))
		return
	}

	chunkSize := envDefaultInt("MDDB_EMBEDDING_CHUNK_SIZE", 1500)
	chunkEnabled := envDefault("MDDB_EMBEDDING_CHUNK_ENABLED", "true") == "true"

	// Resolve quantization for this collection
	var qt QuantizationType
	if s.CollectionManager != nil {
		if cfg, ok := s.CollectionManager.Get(req.Collection); ok && cfg.Quantization != "" {
			qt = ParseQuantization(cfg.Quantization)
		}
	}

	// Load all documents in collection
	type docEntry struct {
		ID        string
		ContentMD string
	}
	var docs []docEntry

	err := s.DB.View(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs == nil {
			return nil
		}
		c := bDocs.Cursor()
		prefix := []byte("doc|" + req.Collection + "|")
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			d, err := loadDoc(v)
			if err != nil {
				continue
			}
			docs = append(docs, docEntry{ID: d.ID, ContentMD: d.ContentMD})
		}
		return nil
	})
	if err != nil {
		bad(w, err)
		return
	}

	embedded, skipped, failed, totalChunks := 0, 0, 0, 0
	var errs []string

	for _, d := range docs {
		if d.ContentMD == "" {
			skipped++
			continue
		}

		// Check if already embedded with same content hash
		if !req.Force {
			existing, err := s.VectorStore.Get(req.Collection, d.ID)
			if err == nil && existing != nil {
				currentHash := ContentHash(d.ContentMD)
				if existing.ContentHash == currentHash {
					skipped++
					continue
				}
			}
		}

		// Split into chunks
		var chunks []string
		if chunkEnabled {
			chunks = ChunkText(d.ContentMD, chunkSize)
		} else {
			chunks = []string{d.ContentMD}
		}

		if len(chunks) == 0 {
			skipped++
			continue
		}

		// Generate embedding for each chunk
		var chunkEmbeddings []ChunkEmbedding
		chunkFailed := false
		for i, chunk := range chunks {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			vector, err := s.Embedding.Embed(ctx, chunk)
			cancel()
			if err != nil {
				failed++
				errs = append(errs, fmt.Sprintf("%s chunk %d: %s", d.ID, i, err.Error()))
				chunkFailed = true
				break
			}
			chunkEmbeddings = append(chunkEmbeddings, ChunkEmbedding{
				ChunkIndex: i,
				Vector:     vector,
			})
		}
		if chunkFailed {
			continue
		}

		// Store all chunks (with quantization if configured)
		contentHash := ContentHash(d.ContentMD)
		if err := s.VectorStore.PutChunksQuantized(req.Collection, d.ID, chunkEmbeddings, s.Embedding.Model(), contentHash, qt); err != nil {
			failed++
			errs = append(errs, d.ID+": store: "+err.Error())
			continue
		}

		// Update in-memory indexes
		for _, ce := range chunkEmbeddings {
			chunkKey := fmt.Sprintf("%s#%d", d.ID, ce.ChunkIndex)
			for name, searcher := range s.VectorSearchers {
				if name == "quantized" {
					continue
				}
				searcher.Add(req.Collection, chunkKey, ce.Vector)
			}
			if s.QuantizedVecIndex != nil {
				s.QuantizedVecIndex.Add(req.Collection, chunkKey, ce.Vector)
			}
		}

		// Clean stale chunks
		s.VectorStore.CleanStaleChunks(req.Collection, d.ID, len(chunkEmbeddings), s.VectorIndex)

		embedded++
		totalChunks += len(chunkEmbeddings)
	}

	// Trigger training for trainable indexes after reindex
	if embedded > 0 {
		if records, loadErr := s.VectorStore.LoadCollection(req.Collection); loadErr == nil {
			collVecs := make(map[string][]float32, len(records))
			for docID, rec := range records {
				collVecs[docID] = rec.Vector
			}
			for _, searcher := range s.VectorSearchers {
				if trainer, isTrainable := searcher.(Trainable); isTrainable {
					go trainer.Train(req.Collection, collVecs)
				}
			}
		}
	}

	ok(w, map[string]interface{}{
		"embedded":    embedded,
		"skipped":     skipped,
		"failed":      failed,
		"totalChunks": totalChunks,
		"errors":      errs,
	})
}

// handleVectorStats handles GET /v1/vector-stats
func (s *Server) handleVectorStats(w http.ResponseWriter, r *http.Request) {
	resp := map[string]interface{}{
		"enabled": s.Embedding != nil,
	}

	if s.Embedding != nil {
		resp["provider"] = s.Embedding.Model()
		resp["model"] = s.Embedding.Model()
		resp["dimensions"] = s.Embedding.Dimensions()
	}

	// Count embeddings per collection (unique documents)
	vectorCounts, _ := s.VectorStore.CountByCollection()

	// Count total chunks per collection
	chunkCounts, _ := s.VectorStore.CountChunksByCollection()

	// Count total docs per collection
	docCounts := make(map[string]int)
	_ = s.DB.View(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs == nil {
			return nil
		}
		c := bDocs.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			parts := splitKey(k)
			if len(parts) >= 2 {
				docCounts[parts[1]]++
			}
		}
		return nil
	})

	collections := make(map[string]interface{})
	allColls := make(map[string]bool)
	for c := range docCounts {
		allColls[c] = true
	}
	for c := range vectorCounts {
		allColls[c] = true
	}

	for coll := range allColls {
		collInfo := map[string]interface{}{
			"total_documents":    docCounts[coll],
			"embedded_documents": vectorCounts[coll],
			"total_chunks":       chunkCounts[coll],
		}
		// Add quantization info if configured
		if s.CollectionManager != nil {
			if cfg, cfgOK := s.CollectionManager.Get(coll); cfgOK && cfg.Quantization != "" {
				collInfo["quantization"] = cfg.Quantization
			} else {
				collInfo["quantization"] = "float32"
			}
		}
		collections[coll] = collInfo
	}

	resp["collections"] = collections
	resp["index_ready"] = s.VectorIndex.IsReady()

	// Chunk configuration
	resp["chunking"] = map[string]interface{}{
		"enabled":   envDefault("MDDB_EMBEDDING_CHUNK_ENABLED", "true") == "true",
		"chunkSize": envDefaultInt("MDDB_EMBEDDING_CHUNK_SIZE", 1500),
	}

	ok(w, resp)
}

// getDocIDsByMeta returns a set of doc IDs matching metadata filters.
func (s *Server) getDocIDsByMeta(collection string, filterMeta map[string][]string) map[string]bool {
	result := make(map[string]bool)

	_ = s.DB.View(func(tx *bolt.Tx) error {
		bIdx := tx.Bucket([]byte("idxmeta"))
		if bIdx == nil {
			return nil
		}

		var sets [][]string
		for mk, mvals := range filterMeta {
			var ids []string
			for _, mv := range mvals {
				prefix := kMetaKeyPrefix(collection, mk, mv)
				c := bIdx.Cursor()
				for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
					id := string(k[len(prefix):])
					ids = append(ids, id)
				}
			}
			ids = unique(ids)
			sets = append(sets, ids)
		}

		for _, id := range intersect(sets...) {
			result[id] = true
		}
		return nil
	})

	return result
}
