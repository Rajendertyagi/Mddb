package main

import (
	"math"
	"mddb/internal/fts"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestParseBoostKey(t *testing.T) {
	tests := []struct {
		in     string
		wantK  string
		wantV  string
		wantOK bool
	}{
		{"tag:breed", "tag", "breed", true},
		{"category:health-care", "category", "health-care", true},
		{"a:b:c", "a", "b:c", true}, // first colon splits
		{":value", "", "", false},
		{"key:", "", "", false},
		{"nocolon", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range tests {
		gotK, gotV, gotOK := parseBoostKey(tc.in)
		if gotK != tc.wantK || gotV != tc.wantV || gotOK != tc.wantOK {
			t.Errorf("parseBoostKey(%q) = (%q, %q, %v); want (%q, %q, %v)",
				tc.in, gotK, gotV, gotOK, tc.wantK, tc.wantV, tc.wantOK)
		}
	}
}

func TestNormalizeBoostFactor(t *testing.T) {
	tests := []struct {
		in, want float64
	}{
		{5.0, 5.0},
		{1.0, 1.0},
		{0.5, 0.5},
		{-2.0, 0.5},
		{-4.0, 0.25},
		{0.0, 1.0},
	}
	for _, tc := range tests {
		got := normalizeBoostFactor(tc.in)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("normalizeBoostFactor(%v) = %v; want %v", tc.in, got, tc.want)
		}
	}
}

func TestDocMultiplier(t *testing.T) {
	groups := []boostGroup{
		{factor: 5.0, docs: map[string]bool{"doc1": true, "doc2": true}},
		{factor: 0.5, docs: map[string]bool{"doc2": true, "doc3": true}},
	}
	tests := []struct {
		docID string
		want  float64
	}{
		{"doc1", 5.0}, // only first group
		{"doc2", 2.5}, // 5.0 * 0.5
		{"doc3", 0.5},
		{"doc4", 1.0}, // no match
	}
	for _, tc := range tests {
		got := docMultiplier(groups, tc.docID)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("docMultiplier(%q) = %v; want %v", tc.docID, got, tc.want)
		}
	}

	if m := docMultiplier(nil, "x"); m != 1.0 {
		t.Errorf("docMultiplier(nil) = %v; want 1.0", m)
	}

	tiny := []boostGroup{
		{factor: 1e-10, docs: map[string]bool{"d": true}},
		{factor: 1e-10, docs: map[string]bool{"d": true}},
	}
	if m := docMultiplier(tiny, "d"); m != BoostMinMultiplier {
		t.Errorf("floor not applied: got %v, want %v", m, BoostMinMultiplier)
	}
}

func TestApplyBoostFTS_NoBoost(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()

	results := []fts.FTSResult{
		{DocID: "a", Score: 1.0},
		{DocID: "b", Score: 2.0},
	}
	got := s.applyBoostFTS("c", results, nil)
	if len(got) != 2 || got[0].Score != 1.0 || got[1].Score != 2.0 {
		t.Errorf("scores mutated without boost: %+v", got)
	}
}

func TestApplyBoostFTS_PositiveBoost(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()

	seedMeta(t, s, "posts", "a", "tag", "breed")
	seedMeta(t, s, "posts", "b", "tag", "other")

	results := []fts.FTSResult{
		{DocID: "a", Score: 1.0},
		{DocID: "b", Score: 2.0},
	}
	got := s.applyBoostFTS("posts", results, map[string]float64{"tag:breed": 5.0})

	if got[0].DocID != "a" || math.Abs(got[0].Score-5.0) > 1e-9 {
		t.Errorf("expected a@5.0 first, got %+v", got[0])
	}
	if got[1].DocID != "b" || got[1].Score != 2.0 {
		t.Errorf("expected b unchanged @2.0, got %+v", got[1])
	}
}

func TestApplyBoostFTS_NegativeDemotes(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()

	seedMeta(t, s, "posts", "a", "tag", "spam")
	seedMeta(t, s, "posts", "b", "tag", "ham")

	results := []fts.FTSResult{
		{DocID: "a", Score: 10.0},
		{DocID: "b", Score: 2.0},
	}
	got := s.applyBoostFTS("posts", results, map[string]float64{"tag:spam": -4.0})

	// a: 10.0 / 4.0 = 2.5 — still higher than b (2.0).
	if got[0].DocID != "a" || math.Abs(got[0].Score-2.5) > 1e-9 {
		t.Errorf("expected a demoted to 2.5, got %+v", got[0])
	}
}

func TestApplyBoostFTS_SkipsInvalidKeys(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()

	seedMeta(t, s, "posts", "a", "tag", "x")
	results := []fts.FTSResult{{DocID: "a", Score: 1.0}}

	got := s.applyBoostFTS("posts", results, map[string]float64{
		"malformed": 5.0,
		":empty":    5.0,
		"trailing:": 5.0,
		"tag:x":     3.0, // only this one is valid
	})
	if math.Abs(got[0].Score-3.0) > 1e-9 {
		t.Errorf("only tag:x should apply: got %v, want 3.0", got[0].Score)
	}
}

func TestApplyBoostFTS_ZeroFactorNoop(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()

	seedMeta(t, s, "posts", "a", "tag", "x")
	results := []fts.FTSResult{{DocID: "a", Score: 7.0}}
	got := s.applyBoostFTS("posts", results, map[string]float64{"tag:x": 0.0})
	if got[0].Score != 7.0 {
		t.Errorf("zero factor must be noop, got %v", got[0].Score)
	}
}

func TestApplyBoostHybrid_ResortsAndRanks(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()

	seedMeta(t, s, "posts", "a", "pri", "high")
	seedMeta(t, s, "posts", "b", "pri", "low")
	seedMeta(t, s, "posts", "c", "pri", "high")

	items := []HybridSearchResultItem{
		{Document: Doc{ID: "b"}, CombinedScore: 0.9, Rank: 1},
		{Document: Doc{ID: "a"}, CombinedScore: 0.5, Rank: 2},
		{Document: Doc{ID: "c"}, CombinedScore: 0.3, Rank: 3},
	}
	got := s.applyBoostHybrid("posts", items, map[string]float64{"pri:high": 3.0})

	// After boost: a = 1.5, c = 0.9, b = 0.9 — a first, ranks recomputed.
	if got[0].Document.ID != "a" || got[0].Rank != 1 {
		t.Errorf("expected a@rank1 first, got %+v", got[0])
	}
	if math.Abs(got[0].CombinedScore-1.5) > 1e-9 {
		t.Errorf("expected boosted score 1.5, got %v", got[0].CombinedScore)
	}
	// Ranks recomputed sequentially.
	for i, it := range got {
		if it.Rank != i+1 {
			t.Errorf("rank mismatch at %d: got %d", i, it.Rank)
		}
	}
}

func TestApplyBoostHybrid_EmptyInputs(t *testing.T) {
	s, cleanup := newTestServer(t)
	defer cleanup()

	if got := s.applyBoostHybrid("c", nil, map[string]float64{"k:v": 2.0}); got != nil {
		t.Errorf("expected nil for nil input, got %v", got)
	}
	items := []HybridSearchResultItem{{Document: Doc{ID: "x"}, CombinedScore: 1.0}}
	got := s.applyBoostHybrid("c", items, nil)
	if len(got) != 1 || got[0].CombinedScore != 1.0 {
		t.Errorf("expected unchanged, got %+v", got)
	}
}

// seedMeta writes a single metadata entry directly into idxmeta so the
// boost lookup can find it without exercising the full ingestion path.
func seedMeta(t *testing.T, s *Server, collection, docID, metaKey, metaValue string) {
	t.Helper()
	err := s.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("idxmeta"))
		key := append(kMetaKeyPrefix(collection, metaKey, metaValue), []byte(docID)...)
		return b.Put(key, []byte{1})
	})
	if err != nil {
		t.Fatalf("seedMeta: %v", err)
	}
}
