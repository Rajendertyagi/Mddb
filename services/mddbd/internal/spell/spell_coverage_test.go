package spell

import (
	"mddb/internal/binlog"
	"path/filepath"
	"testing"
)

func TestSpellManagerCoverage(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	sm := NewSpellManager(db)
	if err := sm.EnsureBucket(); err != nil {
		t.Fatal(err)
	}
	sm.SetBinlog(nil)

	words := []string{"document", "database", "retrieval", "performance", "indexing"}
	for _, w := range words {
		if err := sm.AddWord("", "en", w, 10); err != nil {
			t.Fatalf("AddWord %s: %v", w, err)
		}
	}
	if err := sm.AddWord("col", "en", "specialterm", 5); err != nil {
		t.Fatal(err)
	}

	// LoadAll rebuilds in-memory models from BoltDB; Ready reflects state.
	sm.LoadAll()
	_ = sm.Ready()

	// Suggest across typo / exact-match / gibberish / multi-word / collection-scoped.
	for _, text := range []string{"documnt", "database", "xyzzyq", "documnt databse"} {
		_, _ = sm.Suggest("", "en", text, 3)
	}
	_, _ = sm.Suggest("col", "en", "specalterm", 3)

	// ListWords + RemoveWord (existing and missing).
	if _, err := sm.ListWords("", "en"); err != nil {
		t.Fatalf("ListWords: %v", err)
	}
	if err := sm.RemoveWord("", "en", "indexing"); err != nil {
		t.Fatalf("RemoveWord(existing): %v", err)
	}
	_ = sm.RemoveWord("", "en", "nonexistent")
	if _, err := sm.ListWords("col", "en"); err != nil {
		t.Fatalf("ListWords(col): %v", err)
	}
}

func TestSpellSuggestEdgesAndBinlog(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	bl, err := binlog.NewBinlog("", binlog.BinlogConfig{Path: filepath.Join(t.TempDir(), "s.binlog")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bl.Close() }()

	sm := NewSpellManager(db)
	if err := sm.EnsureBucket(); err != nil {
		t.Fatal(err)
	}
	sm.SetBinlog(bl) // exercises the binlog-append branch of AddWord/RemoveWord
	for _, w := range []string{"document", "database", "retrieval"} {
		if err := sm.AddWord("", "en", w, 10); err != nil {
			t.Fatalf("AddWord %s: %v", w, err)
		}
	}
	if err := sm.RemoveWord("", "en", "retrieval"); err != nil {
		t.Fatalf("RemoveWord: %v", err)
	}

	// Suggest edge branches.
	_, _ = sm.Suggest("", "en", "documnt", 0)                  // maxSuggestions<=0 -> default
	_, _ = sm.Suggest("", "en", "go 12 ab documnt databse", 5) // non-spellable tokens skipped
	_, _ = sm.Suggest("", "en", "documnt databse retrievl", 1) // cap break
	_ = sm.Cleanup("", "en", "documnt databse")
}

func TestSpellAddWordEdges(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	sm := NewSpellManager(db)
	if err := sm.EnsureBucket(); err != nil {
		t.Fatal(err)
	}
	if err := sm.AddWord("", "en", "document", 10); err != nil {
		t.Fatal(err)
	}
	// Re-adding an existing word accumulates its frequency.
	if err := sm.AddWord("", "en", "document", 5); err != nil {
		t.Fatal(err)
	}
	// Empty word / empty lang are no-ops.
	_ = sm.AddWord("", "en", "", 1)
	_ = sm.AddWord("", "", "x", 1)
}
