package fts

import (
	"testing"
)

// corpus documents used across PMISparse tests.
var pmiCorpusDocs = []struct {
	id      string
	content string
}{
	{"doc1", "Machine learning algorithms process large datasets to identify patterns"},
	{"doc2", "Kubernetes orchestrates containerized workloads across clusters of machines"},
	{"doc3", "BM25 is a probabilistic ranking algorithm used in full text search engines"},
	{"doc4", "Natural language processing enables computers to understand human speech"},
	{"doc5", "Vector databases store and query high dimensional embeddings for semantic search"},
	{"doc6", "Hybrid search combines sparse keyword matching with dense semantic embeddings"},
	{"doc7", "Docker containers package applications with their dependencies for deployment"},
	{"doc8", "Information retrieval systems use inverted indexes for efficient text search"},
}

// indexPMICorpus indexes the full corpus into the given FTS index under the given collection.
func indexPMICorpus(t *testing.T, fts *FTSIndex, collection string) {
	t.Helper()
	for _, doc := range pmiCorpusDocs {
		if err := fts.Index(collection, doc.id, doc.content); err != nil {
			t.Fatalf("Index %s: %v", doc.id, err)
		}
	}
}

func TestPMISparseBasicSearch(t *testing.T) {
	fts, cleanup := newTestFTSIndex(t)
	defer cleanup()
	fts.SetPMIData(NewPMIData())

	indexPMICorpus(t, fts, "test")

	results, err := fts.SearchPMISparse("test", "search", 10)
	if err != nil {
		t.Fatalf("SearchPMISparse failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one result for 'search'")
	}

	for _, r := range results {
		if r.Score <= 0 {
			t.Errorf("expected positive score for doc %s, got %f", r.DocID, r.Score)
		}
	}

	// "search" appears in doc3, doc5, doc6, doc8 — verify we get multiple hits
	if len(results) < 3 {
		t.Errorf("expected at least 3 results for 'search', got %d", len(results))
	}

	// Verify results are sorted by score descending
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted: [%d].Score=%f > [%d].Score=%f",
				i, results[i].Score, i-1, results[i-1].Score)
		}
	}
}

func TestPMISparseExpansion(t *testing.T) {
	fts, cleanup := newTestFTSIndex(t)
	defer cleanup()
	fts.SetPMIData(NewPMIData())

	indexPMICorpus(t, fts, "test")

	// Search for "kubernetes" — direct hit is doc2 only.
	// PMI expansion should pick up co-occurring terms like "containerized",
	// "workloads", "clusters" etc. which may also bring in doc7 (Docker
	// containers) through shared container/deployment vocabulary.
	results, err := fts.SearchPMISparse("test", "kubernetes", 10)
	if err != nil {
		t.Fatalf("SearchPMISparse failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one result for 'kubernetes'")
	}

	// doc2 must be the top hit (direct match)
	if results[0].DocID != "doc2" {
		t.Errorf("expected doc2 as top result, got %s", results[0].DocID)
	}

	// Check if PMI expansion pulled in additional documents beyond the direct match.
	// With a small corpus PMI co-occurrence may or may not fire, so we just
	// verify that if expansion occurred some results have expansion-prefixed
	// matched terms (prefixed with "~").
	if len(results) > 1 {
		foundExpansion := false
		for _, r := range results[1:] {
			for _, mt := range r.MatchedTerms {
				if len(mt) > 0 && mt[0] == '~' {
					foundExpansion = true
					break
				}
			}
			if foundExpansion {
				break
			}
		}
		if foundExpansion {
			t.Logf("PMI expansion brought in %d extra documents beyond direct match", len(results)-1)
		}
	}
}

func TestPMISparseFuzzy(t *testing.T) {
	fts, cleanup := newTestFTSIndex(t)
	defer cleanup()
	fts.SetPMIData(NewPMIData())

	indexPMICorpus(t, fts, "test")

	// "algoritm" is a typo for "algorithm" (edit distance 1)
	results, err := fts.SearchPMISparseFuzzy("test", "algoritm", 10, 1)
	if err != nil {
		t.Fatalf("SearchPMISparseFuzzy failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected fuzzy search to find results for 'algoritm' (typo of 'algorithm')")
	}

	// doc1 ("algorithms") or doc3 ("algorithm") should appear
	found := make(map[string]bool)
	for _, r := range results {
		found[r.DocID] = true
	}
	if !found["doc1"] && !found["doc3"] {
		t.Error("expected doc1 or doc3 to match fuzzy query 'algoritm'")
	}

	// Verify scores are positive
	for _, r := range results {
		if r.Score <= 0 {
			t.Errorf("expected positive score for fuzzy result %s, got %f", r.DocID, r.Score)
		}
	}
}

func TestPMISparseNoResults(t *testing.T) {
	fts, cleanup := newTestFTSIndex(t)
	defer cleanup()
	fts.SetPMIData(NewPMIData())

	// Search on an empty collection (nothing indexed)
	results, err := fts.SearchPMISparse("empty", "query", 10)
	if err != nil {
		t.Fatalf("SearchPMISparse on empty collection failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected nil or empty results for empty collection, got %d", len(results))
	}
}

func TestPMISparseTraining(t *testing.T) {
	fts, cleanup := newTestFTSIndex(t)
	defer cleanup()
	fts.SetPMIData(NewPMIData())

	indexPMICorpus(t, fts, "test")

	// Explicitly train PMI and verify no error
	if err := fts.TrainPMI("test"); err != nil {
		t.Fatalf("TrainPMI failed: %v", err)
	}

	// Search should work after explicit training
	results, err := fts.SearchPMISparse("test", "algorithm", 10)
	if err != nil {
		t.Fatalf("SearchPMISparse after training failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected results for 'algorithm' after PMI training")
	}

	// doc3 mentions "algorithm" directly
	if results[0].DocID != "doc3" && results[0].DocID != "doc1" {
		t.Errorf("expected doc3 or doc1 as top result for 'algorithm', got %s", results[0].DocID)
	}
}

func TestPMISparseInvalidation(t *testing.T) {
	fts, cleanup := newTestFTSIndex(t)
	defer cleanup()
	fts.SetPMIData(NewPMIData())

	indexPMICorpus(t, fts, "test")

	// First search triggers lazy PMI training
	results1, err := fts.SearchPMISparse("test", "search", 10)
	if err != nil {
		t.Fatalf("first SearchPMISparse failed: %v", err)
	}
	if len(results1) == 0 {
		t.Fatal("expected results from first search")
	}

	// Add a new document with a novel term relationship
	if err := fts.Index("test", "doc9", "search optimization techniques improve retrieval performance dramatically"); err != nil {
		t.Fatalf("Index doc9: %v", err)
	}

	// Invalidate PMI so it retrains on next search
	fts.InvalidatePMI("test")

	// Second search should retrain and include new term relationships
	results2, err := fts.SearchPMISparse("test", "search", 10)
	if err != nil {
		t.Fatalf("second SearchPMISparse failed: %v", err)
	}
	if len(results2) == 0 {
		t.Fatal("expected results from second search after invalidation")
	}

	// doc9 should now appear in results since it contains "search"
	foundDoc9 := false
	for _, r := range results2 {
		if r.DocID == "doc9" {
			foundDoc9 = true
			break
		}
	}
	if !foundDoc9 {
		t.Error("expected doc9 to appear in results after PMI invalidation and retrain")
	}

	// The result set should be at least as large as before (we added a matching doc)
	if len(results2) < len(results1) {
		t.Errorf("expected at least %d results after adding doc, got %d", len(results1), len(results2))
	}
}

func TestPMISparseMultiCollection(t *testing.T) {
	fts, cleanup := newTestFTSIndex(t)
	defer cleanup()
	fts.SetPMIData(NewPMIData())

	// Index different content into two collections
	if err := fts.Index("collA", "a1", "Machine learning algorithms process large datasets to identify patterns"); err != nil {
		t.Fatalf("Index collA/a1: %v", err)
	}
	if err := fts.Index("collA", "a2", "Deep learning neural networks train on massive datasets"); err != nil {
		t.Fatalf("Index collA/a2: %v", err)
	}

	if err := fts.Index("collB", "b1", "Kubernetes orchestrates containerized workloads across clusters"); err != nil {
		t.Fatalf("Index collB/b1: %v", err)
	}
	if err := fts.Index("collB", "b2", "Docker containers package applications with dependencies"); err != nil {
		t.Fatalf("Index collB/b2: %v", err)
	}

	// Train both collections
	if err := fts.TrainPMI("collA"); err != nil {
		t.Fatalf("TrainPMI collA: %v", err)
	}
	if err := fts.TrainPMI("collB"); err != nil {
		t.Fatalf("TrainPMI collB: %v", err)
	}

	// Search collA for ML term — should only return collA docs
	resultsA, err := fts.SearchPMISparse("collA", "learning", 10)
	if err != nil {
		t.Fatalf("SearchPMISparse collA: %v", err)
	}
	for _, r := range resultsA {
		if r.DocID == "b1" || r.DocID == "b2" {
			t.Errorf("collA search returned collB doc %s", r.DocID)
		}
	}

	// Search collB for infra term — should only return collB docs
	resultsB, err := fts.SearchPMISparse("collB", "kubernetes", 10)
	if err != nil {
		t.Fatalf("SearchPMISparse collB: %v", err)
	}
	for _, r := range resultsB {
		if r.DocID == "a1" || r.DocID == "a2" {
			t.Errorf("collB search returned collA doc %s", r.DocID)
		}
	}

	// Invalidate collA — collB should remain trained and unaffected
	fts.InvalidatePMI("collA")

	resultsB2, err := fts.SearchPMISparse("collB", "kubernetes", 10)
	if err != nil {
		t.Fatalf("SearchPMISparse collB after collA invalidation: %v", err)
	}
	if len(resultsB2) != len(resultsB) {
		t.Errorf("collB results changed after collA invalidation: before=%d, after=%d", len(resultsB), len(resultsB2))
	}
}

func TestPMISparseEmptyQuery(t *testing.T) {
	fts, cleanup := newTestFTSIndex(t)
	defer cleanup()
	fts.SetPMIData(NewPMIData())

	indexPMICorpus(t, fts, "test")

	// Empty string query
	results, err := fts.SearchPMISparse("test", "", 10)
	if err != nil {
		t.Fatalf("SearchPMISparse with empty query failed: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty query, got %d results", len(results))
	}

	// Stop-words-only query
	results, err = fts.SearchPMISparse("test", "the and or but", 10)
	if err != nil {
		t.Fatalf("SearchPMISparse with stop-words query failed: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for stop-words-only query, got %d results", len(results))
	}

	// Also test fuzzy variant with empty query
	results, err = fts.SearchPMISparseFuzzy("test", "", 10, 1)
	if err != nil {
		t.Fatalf("SearchPMISparseFuzzy with empty query failed: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty fuzzy query, got %d results", len(results))
	}
}

func TestPMISparseExpandTerms(t *testing.T) {
	fts, cleanup := newTestFTSIndex(t)
	defer cleanup()
	fts.SetPMIData(NewPMIData())

	indexPMICorpus(t, fts, "test")

	// Train PMI explicitly
	if err := fts.TrainPMI("test"); err != nil {
		t.Fatalf("TrainPMI failed: %v", err)
	}

	// pmiExpand should return expansion terms for terms that appear in the corpus.
	// "search" appears in doc3, doc5, doc6, doc8 with many co-occurring terms.
	expansions := fts.pmiExpand("test", "search", 5)
	t.Logf("expansions for 'search': %v", expansions)

	// With 8 docs and a window size of 5, terms co-occurring with "search"
	// should produce at least some expansions.
	// The exact expansions depend on PPMI thresholds, so we check conservatively.
	if len(expansions) > 0 {
		for _, exp := range expansions {
			if exp.Term == "" {
				t.Error("expansion term should not be empty")
			}
			if exp.Weight <= 0 {
				t.Errorf("expansion weight should be positive, got %f for term %q", exp.Weight, exp.Term)
			}
		}

		// Verify expansions are sorted by weight descending
		for i := 1; i < len(expansions); i++ {
			if expansions[i].Weight > expansions[i-1].Weight {
				t.Errorf("expansions not sorted: [%d].Weight=%f > [%d].Weight=%f",
					i, expansions[i].Weight, i-1, expansions[i-1].Weight)
			}
		}
	}

	// Expansion for a term not in the corpus should return nil
	noExpansions := fts.pmiExpand("test", "xyznonexistent", 5)
	if noExpansions != nil {
		t.Errorf("expected nil expansions for unknown term, got %d", len(noExpansions))
	}

	// Expansion on untrained collection should return nil
	noExpansions = fts.pmiExpand("untrained_collection", "search", 5)
	if noExpansions != nil {
		t.Errorf("expected nil expansions for untrained collection, got %d", len(noExpansions))
	}
}
