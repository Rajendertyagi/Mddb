package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	json "github.com/goccy/go-json"
	bolt "go.etcd.io/bbolt"
)

// HybridSearchRequest represents an HTTP hybrid search request.
type HybridSearchRequest struct {
	Collection      string              `json:"collection"`
	Query           string              `json:"query"`
	TopK            int                 `json:"topK"`
	Algorithm       string              `json:"algorithm"`       // FTS algorithm: "bm25" (default), "bm25f"
	VectorAlgorithm string              `json:"vectorAlgorithm"` // vector algorithm: "flat" (default), "hnsw", "ivf", "pq", "sq"
	Alpha           float64             `json:"alpha"`           // weight for alpha blending: 0=keyword only, 1=semantic only (default 0.5)
	Strategy        string              `json:"strategy"`        // "alpha" (default) or "rrf"
	RRFK            int                 `json:"rrfK"`            // RRF parameter k (default 60)
	Fuzzy           int                 `json:"fuzzy"`           // typo tolerance: 0, 1, 2
	Threshold       float64             `json:"threshold"`       // min vector similarity 0-1
	FilterMeta      map[string][]string `json:"filterMeta"`
	IncludeContent  bool                `json:"includeContent"`
	FieldWeights    map[string]float64  `json:"fieldWeights,omitempty"`
	DisableStem     bool                `json:"disableStem"`
	DisableSynonyms bool                `json:"disableSynonyms"`
}

// HybridSearchResultItem represents a single hybrid search result.
type HybridSearchResultItem struct {
	Document      Doc      `json:"document"`
	CombinedScore float64  `json:"combinedScore"`
	FTSScore      float64  `json:"ftsScore"`
	VectorScore   float64  `json:"vectorScore"`
	MatchedTerms  []string `json:"matchedTerms,omitempty"`
	Rank          int      `json:"rank"`
}

// HybridSearchResponse represents the response from hybrid search.
type HybridSearchResponse struct {
	Results         []HybridSearchResultItem `json:"results"`
	Total           int                      `json:"total"`
	Strategy        string                   `json:"strategy"`
	Alpha           float64                  `json:"alpha,omitempty"`
	RRFK            int                      `json:"rrfK,omitempty"`
	FTSAlgorithm    string                   `json:"ftsAlgorithm"`
	VectorAlgorithm string                   `json:"vectorAlgorithm"`
}

// handleHybridSearch handles POST /v1/hybrid-search
func (s *Server) handleHybridSearch(w http.ResponseWriter, r *http.Request) {
	var req HybridSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" || req.Query == "" {
		bad(w, errors.New("missing required fields: collection, query"))
		return
	}

	// Defaults
	if req.TopK <= 0 {
		req.TopK = 10
	}
	if req.Algorithm == "" {
		req.Algorithm = "bm25"
	}
	if req.VectorAlgorithm == "" {
		req.VectorAlgorithm = "flat"
	}
	if req.Strategy == "" {
		req.Strategy = "alpha"
	}
	if req.Strategy == "alpha" && req.Alpha == 0 {
		req.Alpha = 0.5
	}
	if req.RRFK <= 0 {
		req.RRFK = 60
	}
	if req.Fuzzy < 0 {
		req.Fuzzy = 0
	}
	if req.Fuzzy > 2 {
		req.Fuzzy = 2
	}

	// Validate strategy
	if req.Strategy != "alpha" && req.Strategy != "rrf" {
		bad(w, fmt.Errorf("unknown strategy: %s, available: alpha, rrf", req.Strategy))
		return
	}

	// ---- Step 1: Run FTS search ----
	ftsResults, err := s.runFTSSearch(req)
	if err != nil {
		bad(w, fmt.Errorf("FTS search failed: %w", err))
		return
	}

	// ---- Step 2: Run vector search ----
	vectorResults, err := s.runVectorSearch(r.Context(), req)
	if err != nil {
		bad(w, fmt.Errorf("vector search failed: %w", err))
		return
	}

	// ---- Step 3: Merge results ----
	var merged []HybridSearchResultItem
	switch req.Strategy {
	case "rrf":
		merged = mergeRRF(ftsResults, vectorResults, req.RRFK, req.TopK)
	default: // "alpha"
		merged = mergeAlpha(ftsResults, vectorResults, req.Alpha, req.TopK)
	}

	// ---- Step 4: Load full documents ----
	items := s.loadHybridDocs(req.Collection, merged, req.IncludeContent)

	resp := HybridSearchResponse{
		Results:         items,
		Total:           len(items),
		Strategy:        req.Strategy,
		FTSAlgorithm:    req.Algorithm,
		VectorAlgorithm: req.VectorAlgorithm,
	}
	if req.Strategy == "alpha" {
		resp.Alpha = req.Alpha
	}
	if req.Strategy == "rrf" {
		resp.RRFK = req.RRFK
	}

	ok(w, resp)
}

// runFTSSearch executes the FTS portion of hybrid search.
func (s *Server) runFTSSearch(req HybridSearchRequest) ([]FTSResult, error) {
	if s.FTSIndex == nil {
		return nil, nil
	}

	// Per-query stemming/synonym control
	origStemmer := s.FTSIndex.stemmer
	origSynonyms := s.FTSIndex.synonymManager
	if req.DisableStem {
		s.FTSIndex.stemmer = nil
	}
	if req.DisableSynonyms {
		s.FTSIndex.synonymManager = nil
	}
	defer func() {
		s.FTSIndex.stemmer = origStemmer
		s.FTSIndex.synonymManager = origSynonyms
	}()

	// Pre-filter by metadata if provided
	var allowed map[string]bool
	if len(req.FilterMeta) > 0 {
		allowed = s.getDocIDsByMeta(req.Collection, req.FilterMeta)
		if len(allowed) == 0 {
			return nil, nil
		}
	}

	// Oversample: get more results for better merging
	searchLimit := req.TopK * 3
	if searchLimit < 50 {
		searchLimit = 50
	}

	tokens := s.FTSIndex.TokenizeQuery(req.Collection, req.Query)

	var results []FTSResult
	var err error

	switch req.Algorithm {
	case "bm25f":
		if req.Fuzzy > 0 {
			results, err = s.FTSIndex.SearchBM25FFuzzy(req.Collection, tokens, searchLimit, req.Fuzzy, req.FieldWeights)
		} else {
			results, err = s.FTSIndex.SearchBM25F(req.Collection, tokens, searchLimit, req.FieldWeights)
		}
	case "bm25":
		if req.Fuzzy > 0 {
			results, err = s.FTSIndex.SearchBM25Fuzzy(req.Collection, req.Query, searchLimit, req.Fuzzy)
		} else {
			results, err = s.FTSIndex.SearchBM25(req.Collection, req.Query, searchLimit)
		}
	case "pmisparse":
		if req.Fuzzy > 0 {
			results, err = s.FTSIndex.SearchPMISparseFuzzy(req.Collection, req.Query, searchLimit, req.Fuzzy)
		} else {
			results, err = s.FTSIndex.SearchPMISparse(req.Collection, req.Query, searchLimit)
		}
	default:
		if req.Fuzzy > 0 {
			results, err = s.FTSIndex.SearchBM25Fuzzy(req.Collection, req.Query, searchLimit, req.Fuzzy)
		} else {
			results, err = s.FTSIndex.SearchBM25(req.Collection, req.Query, searchLimit)
		}
	}

	if err != nil {
		return nil, err
	}

	// Apply metadata filter if provided
	if allowed != nil {
		filtered := results[:0]
		for _, r := range results {
			if allowed[r.DocID] {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	return results, nil
}

// runVectorSearch executes the vector portion of hybrid search.
func (s *Server) runVectorSearch(ctx context.Context, req HybridSearchRequest) ([]VectorResult, error) {
	if s.Embedding == nil || len(s.VectorSearchers) == 0 {
		return nil, nil
	}

	searcher, ok2 := s.VectorSearchers[req.VectorAlgorithm]
	if !ok2 {
		searcher = s.VectorSearchers["flat"]
	}
	if !searcher.IsReady() {
		searcher = s.VectorSearchers["flat"]
	}
	if !searcher.IsReady() {
		return nil, nil
	}

	embedCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	queryVector, err := s.Embedding.Embed(embedCtx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}

	searchTopK := req.TopK * 3
	if searchTopK < 20 {
		searchTopK = 20
	}

	var results []VectorResult
	if len(req.FilterMeta) > 0 {
		allowedIDs := s.getDocIDsByMeta(req.Collection, req.FilterMeta)
		if len(allowedIDs) == 0 {
			return nil, nil
		}
		results = searcher.SearchWithFilter(req.Collection, queryVector, searchTopK, req.Threshold, allowedIDs)
	} else {
		results = searcher.Search(req.Collection, queryVector, searchTopK, req.Threshold)
	}

	results = DeduplicateChunkResults(results)
	return results, nil
}

// mergeAlpha combines FTS and vector results using alpha blending.
// combined = alpha * normalizedFTS + (1-alpha) * vectorScore
func mergeAlpha(ftsResults []FTSResult, vectorResults []VectorResult, alpha float64, topK int) []HybridSearchResultItem {
	// Normalize FTS scores to 0-1 range
	var ftsMin, ftsMax float64
	if len(ftsResults) > 0 {
		ftsMin = ftsResults[0].Score
		ftsMax = ftsResults[0].Score
		for _, r := range ftsResults[1:] {
			if r.Score < ftsMin {
				ftsMin = r.Score
			}
			if r.Score > ftsMax {
				ftsMax = r.Score
			}
		}
	}

	normalizeFTS := func(score float64) float64 {
		if ftsMax == ftsMin {
			if ftsMax > 0 {
				return 1.0
			}
			return 0.0
		}
		return (score - ftsMin) / (ftsMax - ftsMin)
	}

	// Build combined map
	type combinedEntry struct {
		ftsScore     float64
		vectorScore  float64
		matchedTerms []string
	}
	combined := make(map[string]*combinedEntry)

	for _, r := range ftsResults {
		combined[r.DocID] = &combinedEntry{
			ftsScore:     normalizeFTS(r.Score),
			matchedTerms: r.MatchedTerms,
		}
	}
	for _, r := range vectorResults {
		e, ok := combined[r.DocID]
		if !ok {
			e = &combinedEntry{}
			combined[r.DocID] = e
		}
		e.vectorScore = float64(r.Score)
	}

	// Calculate combined scores
	results := make([]HybridSearchResultItem, 0, len(combined))
	for docID, e := range combined {
		combinedScore := (1-alpha)*e.ftsScore + alpha*e.vectorScore
		results = append(results, HybridSearchResultItem{
			Document:      Doc{ID: docID},
			CombinedScore: combinedScore,
			FTSScore:      e.ftsScore,
			VectorScore:   e.vectorScore,
			MatchedTerms:  e.matchedTerms,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CombinedScore > results[j].CombinedScore
	})

	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	return results
}

// mergeRRF combines FTS and vector results using Reciprocal Rank Fusion.
// score = 1/(k + rank_fts) + 1/(k + rank_vector)
func mergeRRF(ftsResults []FTSResult, vectorResults []VectorResult, rrfK int, topK int) []HybridSearchResultItem {
	type combinedEntry struct {
		rrfScore     float64
		ftsScore     float64
		vectorScore  float64
		matchedTerms []string
	}
	combined := make(map[string]*combinedEntry)

	k := float64(rrfK)

	for rank, r := range ftsResults {
		e := &combinedEntry{
			ftsScore:     r.Score,
			matchedTerms: r.MatchedTerms,
		}
		e.rrfScore = 1.0 / (k + float64(rank+1))
		combined[r.DocID] = e
	}

	for rank, r := range vectorResults {
		e, ok := combined[r.DocID]
		if !ok {
			e = &combinedEntry{}
			combined[r.DocID] = e
		}
		e.vectorScore = float64(r.Score)
		e.rrfScore += 1.0 / (k + float64(rank+1))
	}

	results := make([]HybridSearchResultItem, 0, len(combined))
	for docID, e := range combined {
		results = append(results, HybridSearchResultItem{
			Document:      Doc{ID: docID},
			CombinedScore: e.rrfScore,
			FTSScore:      e.ftsScore,
			VectorScore:   e.vectorScore,
			MatchedTerms:  e.matchedTerms,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CombinedScore > results[j].CombinedScore
	})

	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	return results
}

// loadHybridDocs loads full documents for hybrid search results.
func (s *Server) loadHybridDocs(collection string, items []HybridSearchResultItem, includeContent bool) []HybridSearchResultItem {
	results := make([]HybridSearchResultItem, 0, len(items))
	_ = s.DB.View(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs == nil {
			return nil
		}
		rank := 0
		for _, item := range items {
			v := bDocs.Get(kDoc(collection, item.Document.ID))
			if v == nil {
				continue
			}
			docPtr, err := loadDoc(v)
			if err != nil {
				continue
			}
			if docPtr.ExpiresAt > 0 && docPtr.ExpiresAt < currentUnix() {
				continue
			}
			doc := *docPtr
			if !includeContent {
				doc.ContentMD = ""
			}
			rank++
			results = append(results, HybridSearchResultItem{
				Document:      doc,
				CombinedScore: item.CombinedScore,
				FTSScore:      item.FTSScore,
				VectorScore:   item.VectorScore,
				MatchedTerms:  item.MatchedTerms,
				Rank:          rank,
			})
		}
		return nil
	})
	return results
}
