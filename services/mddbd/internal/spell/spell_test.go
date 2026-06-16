package spell

import (
	"os"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestSpellModel_SuggestKnownWord(t *testing.T) {
	m := newSpellModel()
	m.train("document", 100)
	m.train("retrieval", 80)
	m.train("system", 90)

	// Known word — no correction expected
	_, ok := m.suggest("document")
	if ok {
		t.Error("suggest should not correct a known word")
	}
}

func TestSpellModel_SuggestTypo(t *testing.T) {
	m := newSpellModel()
	m.train("document", 100)

	sug, ok := m.suggest("docuemnt")
	if !ok {
		t.Fatal("suggest should return a correction for 'docuemnt'")
	}
	if sug.Corrected != "document" {
		t.Errorf("expected 'document', got %q", sug.Corrected)
	}
	if sug.Confidence <= 0 || sug.Confidence > 1 {
		t.Errorf("confidence out of range: %f", sug.Confidence)
	}
}

func TestSpellModel_NoSuggestionForUnknown(t *testing.T) {
	m := newSpellModel()
	m.train("apple", 10)

	_, ok := m.suggest("zxqwerty")
	if ok {
		t.Error("should not suggest correction for very different word")
	}
}

func TestSpellManager_AddRemoveList(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	sm := NewSpellManager(db)
	if err := sm.EnsureBucket(); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}

	if err := sm.AddWord("testcol", "en", "mddb", 50); err != nil {
		t.Fatalf("AddWord: %v", err)
	}
	if err := sm.AddWord("testcol", "en", "boltdb", 30); err != nil {
		t.Fatalf("AddWord: %v", err)
	}

	words, err := sm.ListWords("testcol", "en")
	if err != nil {
		t.Fatalf("ListWords: %v", err)
	}
	found := false
	for _, w := range words {
		if w == "mddb" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'mddb' in word list")
	}

	if err := sm.RemoveWord("testcol", "en", "mddb"); err != nil {
		t.Fatalf("RemoveWord: %v", err)
	}
	words, _ = sm.ListWords("testcol", "en")
	for _, w := range words {
		if w == "mddb" {
			t.Error("word 'mddb' should have been removed")
		}
	}
}

func TestSpellManager_CleanupText(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	sm := NewSpellManager(db)
	if err := sm.EnsureBucket(); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}

	// Train global English model
	for _, w := range []string{"document", "retrieval", "system", "search"} {
		if err := sm.AddWord("", "en", w, 100); err != nil {
			t.Fatalf("AddWord: %v", err)
		}
	}
	sm.ready.Store(true)

	cleaned := sm.Cleanup("", "en", "docuemnt retreival systtem")
	if cleaned == "docuemnt retreival systtem" {
		t.Error("expected Cleanup to correct the text, got original unchanged")
	}
}

func TestIsSpellableToken(t *testing.T) {
	tests := []struct {
		token string
		want  bool
	}{
		{"hello", true},
		{"ab", false},      // too short
		{"123", false},     // all digits
		{"http://", false}, // URL-like
		{"user@host", false},
	}
	for _, tt := range tests {
		if got := isSpellableToken(tt.token); got != tt.want {
			t.Errorf("isSpellableToken(%q) = %v, want %v", tt.token, got, tt.want)
		}
	}
}

func newTestDB(t *testing.T) (*bolt.DB, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "mddb-test-*.db")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	db, err := bolt.Open(f.Name(), 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("bolt.Open: %v", err)
	}
	return db, func() {
		if err := db.Close(); err != nil {
			t.Logf("db.Close: %v", err)
		}
		if err := os.Remove(f.Name()); err != nil {
			t.Logf("os.Remove: %v", err)
		}
	}
}
