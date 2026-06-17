package spell

import "testing"

// TestSpellGlobalFallbackAndLimit covers Suggest's global-model fallback (a
// collection query for a word only the global model knows) and the
// maxSuggestions break.
func TestSpellGlobalFallbackAndLimit(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	sm := NewSpellManager(db)
	if err := sm.EnsureBucket(); err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"document", "database"} {
		if err := sm.AddWord("", "en", w, 10); err != nil {
			t.Fatal(err)
		}
	}
	if err := sm.AddWord("col", "en", "specialterm", 5); err != nil {
		t.Fatal(err)
	}

	// "documnt" is unknown to the col model but known globally -> fallback branch.
	if _, sug := sm.Suggest("col", "en", "documnt", 3); len(sug) == 0 {
		t.Error("expected a global-fallback suggestion for 'documnt' under collection 'col'")
	}
	// Two typos but a limit of 1 -> the maxSuggestions break fires.
	if _, sug := sm.Suggest("", "en", "documnt databse", 1); len(sug) != 1 {
		t.Errorf("limit 1: got %d suggestions, want 1", len(sug))
	}
}
