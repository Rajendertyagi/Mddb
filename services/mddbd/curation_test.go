package main

import (
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func newCurationManager(t *testing.T) *CurationManager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cur.db")
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cm := NewCurationManager(db)
	if err := cm.EnsureBucket(); err != nil {
		t.Fatal(err)
	}
	return cm
}

func TestCurationManager_SetValidation(t *testing.T) {
	cm := newCurationManager(t)

	if err := cm.Set(nil); err == nil {
		t.Error("expected error for nil rule")
	}
	if err := cm.Set(&CurationRule{}); err == nil {
		t.Error("expected error for missing collection")
	}
	if err := cm.Set(&CurationRule{Collection: "c"}); err == nil {
		t.Error("expected error for missing query")
	}
	if err := cm.Set(&CurationRule{Collection: "c", Query: "q", MatchMode: "regex"}); err == nil {
		t.Error("expected error for invalid match_mode")
	}
	if err := cm.Set(&CurationRule{Collection: "c", Query: "q", Pins: []PinnedDoc{{Key: ""}}}); err == nil {
		t.Error("expected error for empty pin key")
	}
}

func TestCurationManager_CreateGetListDelete(t *testing.T) {
	cm := newCurationManager(t)

	r1 := &CurationRule{
		Collection: "blog",
		Query:      "rust",
		Enabled:    true,
		Pins:       []PinnedDoc{{Key: "rust-best", Position: 1}},
	}
	if err := cm.Set(r1); err != nil {
		t.Fatal(err)
	}
	if r1.ID == "" {
		t.Error("expected ID to be assigned")
	}
	if r1.CreatedAt == 0 || r1.UpdatedAt == 0 {
		t.Error("expected timestamps to be set")
	}
	if r1.MatchMode != "exact" {
		t.Errorf("expected default match_mode=exact, got %q", r1.MatchMode)
	}

	got, ok := cm.Get(r1.ID)
	if !ok {
		t.Fatal("rule should be retrievable")
	}
	if got.Query != "rust" {
		t.Errorf("mismatched query: %q", got.Query)
	}

	r2 := &CurationRule{Collection: "blog", Query: "go", Enabled: true}
	if err := cm.Set(r2); err != nil {
		t.Fatal(err)
	}

	if list := cm.ListByCollection("blog"); len(list) != 2 {
		t.Errorf("expected 2 rules in blog, got %d", len(list))
	}
	if list := cm.ListByCollection("missing"); len(list) != 0 {
		t.Errorf("expected 0 rules in missing, got %d", len(list))
	}
	if all := cm.ListAll(); len(all) != 2 {
		t.Errorf("ListAll: expected 2, got %d", len(all))
	}

	if err := cm.Delete(r1.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := cm.Get(r1.ID); ok {
		t.Error("rule must be gone after Delete")
	}
	if list := cm.ListByCollection("blog"); len(list) != 1 {
		t.Errorf("after delete, expected 1 rule, got %d", len(list))
	}
}

func TestCurationManager_UpdateReplacesRule(t *testing.T) {
	cm := newCurationManager(t)
	r := &CurationRule{Collection: "c", Query: "q1", Enabled: true}
	if err := cm.Set(r); err != nil {
		t.Fatal(err)
	}
	origID := r.ID
	origCreated := r.CreatedAt

	r.Query = "q2"
	if err := cm.Set(r); err != nil {
		t.Fatal(err)
	}
	if r.ID != origID {
		t.Error("update must preserve id")
	}
	if r.CreatedAt != origCreated {
		t.Error("update must preserve CreatedAt")
	}
	got, _ := cm.Get(r.ID)
	if got.Query != "q2" {
		t.Errorf("expected updated query q2, got %q", got.Query)
	}
	// Still exactly one rule in the collection — no dupe.
	if list := cm.ListByCollection("c"); len(list) != 1 {
		t.Errorf("expected 1, got %d", len(list))
	}
}

func TestCurationManager_LoadAllRehydrates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cur.db")
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}

	cm := NewCurationManager(db)
	if err := cm.EnsureBucket(); err != nil {
		t.Fatal(err)
	}
	if err := cm.Set(&CurationRule{Collection: "c", Query: "q", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// Reopen and LoadAll
	db2, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db2.Close() }()
	cm2 := NewCurationManager(db2)
	if err := cm2.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if cm2.indexSize() != 1 {
		t.Errorf("expected 1 rule after reload, got %d", cm2.indexSize())
	}
}

func TestCurationManager_MatchingRules(t *testing.T) {
	cm := newCurationManager(t)

	_ = cm.Set(&CurationRule{Collection: "c", Query: "rust tutorial", MatchMode: "exact", Enabled: true})
	_ = cm.Set(&CurationRule{Collection: "c", Query: "tutorial", MatchMode: "contains", Enabled: true})
	_ = cm.Set(&CurationRule{Collection: "c", Query: "disabled", MatchMode: "exact", Enabled: false})
	_ = cm.Set(&CurationRule{Collection: "other", Query: "tutorial", MatchMode: "contains", Enabled: true})

	// exact match
	if hits := cm.MatchingRules("c", "rust tutorial"); len(hits) != 2 {
		// Matches "rust tutorial" exactly AND contains "tutorial"
		t.Errorf("expected 2 matches for 'rust tutorial', got %d", len(hits))
	}
	// contains only
	if hits := cm.MatchingRules("c", "python tutorial"); len(hits) != 1 {
		t.Errorf("expected 1 contains match, got %d", len(hits))
	}
	// disabled rule never fires
	if hits := cm.MatchingRules("c", "disabled"); len(hits) != 0 {
		t.Errorf("disabled rule must never match, got %d", len(hits))
	}
	// wrong collection
	if hits := cm.MatchingRules("c", "foo"); len(hits) != 0 {
		t.Errorf("unrelated query should match nothing, got %d", len(hits))
	}
	// case-insensitive
	if hits := cm.MatchingRules("c", "RUST TUTORIAL"); len(hits) != 2 {
		t.Errorf("expected case-insensitive match, got %d", len(hits))
	}
	// empty query
	if hits := cm.MatchingRules("c", "   "); len(hits) != 0 {
		t.Errorf("empty query must not match anything, got %d", len(hits))
	}
}

func TestCollectPinsAndHides_MergeDedup(t *testing.T) {
	rules := []*CurationRule{
		{Pins: []PinnedDoc{{Key: "a", Position: 1}}, Hides: []string{"x"}},
		{Pins: []PinnedDoc{{Key: "a", Position: 5}, {Key: "b", Position: 2}}, Hides: []string{"y"}},
	}
	hides, pins := collectPinsAndHides(rules)

	if !hides["x"] || !hides["y"] {
		t.Errorf("hides merge failed: %v", hides)
	}
	if len(pins) != 2 {
		t.Fatalf("expected 2 pins, got %d", len(pins))
	}
	// Second rule overwrites "a" → Position 5
	var aPos int
	for _, p := range pins {
		if p.Key == "a" {
			aPos = p.Position
		}
	}
	if aPos != 5 {
		t.Errorf("expected later rule to override pin position, got %d", aPos)
	}
}

func TestSplicePinnedFTS_InsertsAtPosition(t *testing.T) {
	base := []FTSResultWithDoc{
		{Document: Doc{Key: "o1"}, Score: 3},
		{Document: Doc{Key: "o2"}, Score: 2},
		{Document: Doc{Key: "o3"}, Score: 1},
	}
	pinned := []pinnedFTS{
		{Result: FTSResultWithDoc{Document: Doc{Key: "p1"}, Pinned: true}, Position: 1},
		{Result: FTSResultWithDoc{Document: Doc{Key: "p2"}, Pinned: true}, Position: 3},
	}
	got := splicePinnedFTS(base, pinned)
	wantKeys := []string{"p1", "o1", "p2", "o2", "o3"}
	if len(got) != len(wantKeys) {
		t.Fatalf("length mismatch: got %d, want %d", len(got), len(wantKeys))
	}
	for i, k := range wantKeys {
		if got[i].Document.Key != k {
			t.Errorf("slot %d: expected %q, got %q", i, k, got[i].Document.Key)
		}
	}
}

func TestSplicePinnedFTS_AppendWithPositionZero(t *testing.T) {
	base := []FTSResultWithDoc{{Document: Doc{Key: "o1"}}}
	pinned := []pinnedFTS{
		{Result: FTSResultWithDoc{Document: Doc{Key: "p1"}, Pinned: true}, Position: 0},
	}
	got := splicePinnedFTS(base, pinned)
	if len(got) != 2 || got[0].Document.Key != "o1" || got[1].Document.Key != "p1" {
		t.Errorf("expected o1,p1 (append), got %+v", got)
	}
}

func TestSplicePinnedFTS_EmptyInputs(t *testing.T) {
	if got := splicePinnedFTS(nil, nil); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
	base := []FTSResultWithDoc{{Document: Doc{Key: "o"}}}
	if got := splicePinnedFTS(base, nil); len(got) != 1 {
		t.Errorf("no pins should pass base through, got %d", len(got))
	}
}
