package fts

import (
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func newTestSynonymManager(t *testing.T) *SynonymManager {
	t.Helper()
	dir := t.TempDir()
	db, err := bolt.Open(filepath.Join(dir, "test.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sm := NewSynonymManager(db)
	if err := sm.EnsureBucket(); err != nil {
		t.Fatal(err)
	}
	return sm
}

func TestSynonymManagerSetGet(t *testing.T) {
	sm := newTestSynonymManager(t)

	err := sm.Set("docs", "big", []string{"large", "huge", "enormous"})
	if err != nil {
		t.Fatal(err)
	}

	syns := sm.Get("docs", "big")
	if len(syns) != 3 {
		t.Errorf("expected 3 synonyms, got %d", len(syns))
	}

	// Should not include the term itself
	for _, s := range syns {
		if s == "big" {
			t.Error("synonym list should not include the term itself")
		}
	}
}

func TestSynonymManagerDelete(t *testing.T) {
	sm := newTestSynonymManager(t)

	_ = sm.Set("docs", "fast", []string{"quick", "rapid"})
	_ = sm.Delete("docs", "fast")

	syns := sm.Get("docs", "fast")
	if len(syns) != 0 {
		t.Errorf("expected 0 synonyms after delete, got %d", len(syns))
	}
}

func TestSynonymManagerList(t *testing.T) {
	sm := newTestSynonymManager(t)

	_ = sm.Set("docs", "big", []string{"large"})
	_ = sm.Set("docs", "fast", []string{"quick"})

	all := sm.List("docs")
	if len(all) != 2 {
		t.Errorf("expected 2 entries, got %d", len(all))
	}
}

func TestSynonymManagerExpand(t *testing.T) {
	sm := newTestSynonymManager(t)

	_ = sm.Set("docs", "big", []string{"large", "huge"})

	// Forward expansion: "big" -> also get "large", "huge"
	expanded := sm.Expand("docs", []string{"big"})
	if len(expanded) < 3 {
		t.Errorf("expected at least 3 terms after expansion, got %d: %v", len(expanded), expanded)
	}

	// Reverse expansion: "large" -> also get "big", "huge"
	expanded = sm.Expand("docs", []string{"large"})
	found := make(map[string]bool)
	for _, e := range expanded {
		found[e] = true
	}
	if !found["big"] {
		t.Error("reverse expansion should include 'big' when searching 'large'")
	}
	if !found["huge"] {
		t.Error("reverse expansion should include 'huge' when searching 'large'")
	}
}

func TestSynonymManagerExpandNoSynonyms(t *testing.T) {
	sm := newTestSynonymManager(t)

	// No synonyms configured - should return original terms
	expanded := sm.Expand("docs", []string{"hello"})
	if len(expanded) != 1 || expanded[0] != "hello" {
		t.Errorf("expected unchanged terms, got %v", expanded)
	}
}

func TestSynonymManagerMultiCollection(t *testing.T) {
	sm := newTestSynonymManager(t)

	_ = sm.Set("docs", "big", []string{"large"})
	_ = sm.Set("blog", "big", []string{"huge"})

	docsSyns := sm.Get("docs", "big")
	blogSyns := sm.Get("blog", "big")

	if len(docsSyns) != 1 || docsSyns[0] != "large" {
		t.Errorf("docs collection: expected [large], got %v", docsSyns)
	}
	if len(blogSyns) != 1 || blogSyns[0] != "huge" {
		t.Errorf("blog collection: expected [huge], got %v", blogSyns)
	}
}

func TestSynonymManagerPersistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Write synonyms
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	sm := NewSynonymManager(db)
	_ = sm.EnsureBucket()
	_ = sm.Set("docs", "error", []string{"bug", "fault"})
	_ = db.Close()

	// Reopen and load
	db2, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db2.Close() }()

	sm2 := NewSynonymManager(db2)
	_ = sm2.EnsureBucket()
	if err := sm2.LoadAll(); err != nil {
		t.Fatal(err)
	}

	syns := sm2.Get("docs", "error")
	if len(syns) != 2 {
		t.Errorf("expected 2 synonyms after reload, got %d", len(syns))
	}
}

func TestSynonymManagerLoadDefaults(t *testing.T) {
	sm := newTestSynonymManager(t)

	if err := sm.LoadDefaults("docs"); err != nil {
		t.Fatal(err)
	}

	// Should have loaded the default groups
	syns := sm.Get("docs", "big")
	if len(syns) == 0 {
		t.Error("expected default synonyms for 'big'")
	}

	syns = sm.Get("docs", "error")
	if len(syns) == 0 {
		t.Error("expected default synonyms for 'error'")
	}
}

func TestSynonymManagerNormalization(t *testing.T) {
	sm := newTestSynonymManager(t)

	// Terms should be lowercased and trimmed
	_ = sm.Set("docs", "  BIG  ", []string{" Large ", "HUGE"})

	syns := sm.Get("docs", "big")
	if len(syns) != 2 {
		t.Errorf("expected 2 synonyms, got %d: %v", len(syns), syns)
	}

	for _, s := range syns {
		if s != "large" && s != "huge" {
			t.Errorf("expected lowercase synonym, got %q", s)
		}
	}
}

func TestSynonymManagerEmptyInputs(t *testing.T) {
	sm := newTestSynonymManager(t)

	err := sm.Set("", "term", []string{"syn"})
	if err == nil {
		t.Error("expected error for empty collection")
	}

	err = sm.Set("docs", "", []string{"syn"})
	if err == nil {
		t.Error("expected error for empty term")
	}
}
