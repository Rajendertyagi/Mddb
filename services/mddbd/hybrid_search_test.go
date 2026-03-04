package main

import (
	"math"
	"testing"
)

// ---------- mergeAlpha ----------

func TestMergeAlpha_EmptyInputs(t *testing.T) {
	results := mergeAlpha(nil, nil, 0.5, 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}

	results = mergeAlpha([]FTSResult{}, []VectorResult{}, 0.5, 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty slices, got %d", len(results))
	}
}

func TestMergeAlpha_OnlyFTS(t *testing.T) {
	fts := []FTSResult{
		{DocID: "doc1", Score: 5.0, MatchedTerms: []string{"golang"}},
		{DocID: "doc2", Score: 3.0, MatchedTerms: []string{"tutorial"}},
	}

	results := mergeAlpha(fts, nil, 0.5, 10)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// All vector scores should be zero
	for _, r := range results {
		if r.VectorScore != 0 {
			t.Errorf("doc %s: expected vectorScore=0, got %f", r.Document.ID, r.VectorScore)
		}
	}

	// doc1 has the highest FTS score, so it should rank first
	if results[0].Document.ID != "doc1" {
		t.Errorf("expected doc1 first (highest FTS), got %s", results[0].Document.ID)
	}

	// FTS scores should be normalized; doc1 is max so normalized=1.0
	if math.Abs(results[0].FTSScore-1.0) > 1e-9 {
		t.Errorf("expected doc1 normalized FTS score=1.0, got %f", results[0].FTSScore)
	}
}

func TestMergeAlpha_OnlyVector(t *testing.T) {
	vec := []VectorResult{
		{DocID: "vecA", Score: 0.95},
		{DocID: "vecB", Score: 0.80},
	}

	results := mergeAlpha(nil, vec, 0.5, 10)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// All FTS scores should be zero
	for _, r := range results {
		if r.FTSScore != 0 {
			t.Errorf("doc %s: expected ftsScore=0, got %f", r.Document.ID, r.FTSScore)
		}
	}

	// vecA has the highest vector score
	if results[0].Document.ID != "vecA" {
		t.Errorf("expected vecA first, got %s", results[0].Document.ID)
	}

	if math.Abs(results[0].VectorScore-0.95) > 1e-6 {
		t.Errorf("expected vecA vectorScore=0.95, got %f", results[0].VectorScore)
	}
}

func TestMergeAlpha_Combined(t *testing.T) {
	fts := []FTSResult{
		{DocID: "doc1", Score: 10.0, MatchedTerms: []string{"go"}},
		{DocID: "doc2", Score: 5.0, MatchedTerms: []string{"test"}},
	}
	vec := []VectorResult{
		{DocID: "doc2", Score: 0.9},
		{DocID: "doc3", Score: 0.8},
	}

	alpha := 0.6
	results := mergeAlpha(fts, vec, alpha, 10)

	if len(results) != 3 {
		t.Fatalf("expected 3 results (doc1, doc2, doc3), got %d", len(results))
	}

	// Verify combined score formula: combined = (1-alpha)*fts_norm + alpha*vector
	// For doc1: fts_norm=1.0 (max), vector=0.0 → (0.4)*1.0 + (0.6)*0.0 = 0.4
	// For doc2: fts_norm=0.0 (min), vector=0.9 → (0.4)*0.0 + (0.6)*0.9 = 0.54
	// For doc3: fts_norm=0.0 (not in FTS), vector=0.8 → (0.4)*0.0 + (0.6)*0.8 = 0.48

	scoreMap := make(map[string]float64)
	for _, r := range results {
		scoreMap[r.Document.ID] = r.CombinedScore
	}

	expectedDoc2 := (1 - alpha) * 0.0 + alpha * float64(float32(0.9))
	if math.Abs(scoreMap["doc2"]-expectedDoc2) > 1e-6 {
		t.Errorf("doc2 combined: expected %f, got %f", expectedDoc2, scoreMap["doc2"])
	}

	expectedDoc1 := (1 - alpha) * 1.0 + alpha * 0.0
	if math.Abs(scoreMap["doc1"]-expectedDoc1) > 1e-6 {
		t.Errorf("doc1 combined: expected %f, got %f", expectedDoc1, scoreMap["doc1"])
	}

	expectedDoc3 := (1 - alpha) * 0.0 + alpha * float64(float32(0.8))
	if math.Abs(scoreMap["doc3"]-expectedDoc3) > 1e-6 {
		t.Errorf("doc3 combined: expected %f, got %f", expectedDoc3, scoreMap["doc3"])
	}
}

func TestMergeAlpha_TopKLimit(t *testing.T) {
	fts := []FTSResult{
		{DocID: "a", Score: 10.0},
		{DocID: "b", Score: 8.0},
		{DocID: "c", Score: 6.0},
		{DocID: "d", Score: 4.0},
		{DocID: "e", Score: 2.0},
	}

	results := mergeAlpha(fts, nil, 0.5, 3)

	if len(results) != 3 {
		t.Errorf("expected 3 results with topK=3, got %d", len(results))
	}

	// Results should be sorted by combined score descending
	for i := 1; i < len(results); i++ {
		if results[i].CombinedScore > results[i-1].CombinedScore {
			t.Errorf("results not sorted: index %d (%.4f) > index %d (%.4f)",
				i, results[i].CombinedScore, i-1, results[i-1].CombinedScore)
		}
	}
}

func TestMergeAlpha_Deduplication(t *testing.T) {
	// Same docID appears in both FTS and vector results
	fts := []FTSResult{
		{DocID: "shared", Score: 10.0, MatchedTerms: []string{"golang", "programming"}},
	}
	vec := []VectorResult{
		{DocID: "shared", Score: 0.85},
	}

	alpha := 0.5
	results := mergeAlpha(fts, vec, alpha, 10)

	if len(results) != 1 {
		t.Fatalf("expected 1 deduplicated result, got %d", len(results))
	}

	r := results[0]
	if r.Document.ID != "shared" {
		t.Errorf("expected doc ID 'shared', got %s", r.Document.ID)
	}

	// single FTS result → normalized to 1.0 (max==min, score>0 → 1.0)
	expectedCombined := (1-alpha)*1.0 + alpha*float64(vec[0].Score)
	if math.Abs(r.CombinedScore-expectedCombined) > 1e-9 {
		t.Errorf("combined score: expected %f, got %f", expectedCombined, r.CombinedScore)
	}

	// Verify FTS matched terms are preserved
	if len(r.MatchedTerms) != 2 {
		t.Errorf("expected 2 matched terms, got %d", len(r.MatchedTerms))
	}
}

// ---------- mergeRRF ----------

func TestMergeRRF_EmptyInputs(t *testing.T) {
	results := mergeRRF(nil, nil, 60, 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}

	results = mergeRRF([]FTSResult{}, []VectorResult{}, 60, 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty slices, got %d", len(results))
	}
}

func TestMergeRRF_Combined(t *testing.T) {
	fts := []FTSResult{
		{DocID: "doc1", Score: 10.0, MatchedTerms: []string{"go"}},
		{DocID: "doc2", Score: 5.0, MatchedTerms: []string{"test"}},
	}
	vec := []VectorResult{
		{DocID: "doc2", Score: 0.9},
		{DocID: "doc3", Score: 0.8},
	}

	rrfK := 60
	k := float64(rrfK)
	results := mergeRRF(fts, vec, rrfK, 10)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Verify RRF scoring: score = 1/(k + rank_fts) + 1/(k + rank_vector)
	// Ranks are 1-based.
	//
	// doc1: FTS rank=1, no vector → 1/(60+1) + 0 = 1/61
	// doc2: FTS rank=2, vector rank=1 → 1/(60+2) + 1/(60+1) = 1/62 + 1/61
	// doc3: no FTS, vector rank=2 → 0 + 1/(60+2) = 1/62

	scoreMap := make(map[string]float64)
	for _, r := range results {
		scoreMap[r.Document.ID] = r.CombinedScore
	}

	expectedDoc1 := 1.0 / (k + 1)
	if math.Abs(scoreMap["doc1"]-expectedDoc1) > 1e-9 {
		t.Errorf("doc1 RRF: expected %f, got %f", expectedDoc1, scoreMap["doc1"])
	}

	expectedDoc2 := 1.0/(k+2) + 1.0/(k+1)
	if math.Abs(scoreMap["doc2"]-expectedDoc2) > 1e-9 {
		t.Errorf("doc2 RRF: expected %f, got %f", expectedDoc2, scoreMap["doc2"])
	}

	expectedDoc3 := 1.0 / (k + 2)
	if math.Abs(scoreMap["doc3"]-expectedDoc3) > 1e-9 {
		t.Errorf("doc3 RRF: expected %f, got %f", expectedDoc3, scoreMap["doc3"])
	}

	// doc2 should have the highest combined RRF score (appears in both)
	if scoreMap["doc2"] <= scoreMap["doc1"] {
		t.Error("expected doc2 (in both lists) to score higher than doc1 (FTS only)")
	}
	if scoreMap["doc2"] <= scoreMap["doc3"] {
		t.Error("expected doc2 (in both lists) to score higher than doc3 (vector only)")
	}
}

func TestMergeRRF_TopKLimit(t *testing.T) {
	fts := []FTSResult{
		{DocID: "a", Score: 10.0},
		{DocID: "b", Score: 8.0},
		{DocID: "c", Score: 6.0},
		{DocID: "d", Score: 4.0},
	}
	vec := []VectorResult{
		{DocID: "e", Score: 0.9},
		{DocID: "f", Score: 0.8},
	}

	results := mergeRRF(fts, vec, 60, 2)

	if len(results) != 2 {
		t.Errorf("expected 2 results with topK=2, got %d", len(results))
	}

	// Results should be sorted by combined score descending
	for i := 1; i < len(results); i++ {
		if results[i].CombinedScore > results[i-1].CombinedScore {
			t.Errorf("results not sorted: index %d (%.6f) > index %d (%.6f)",
				i, results[i].CombinedScore, i-1, results[i-1].CombinedScore)
		}
	}
}
