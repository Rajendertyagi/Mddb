package automationlog

import (
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

// TestListNonExactCursor covers the cursor branches where Seek does not land on
// an exact key: one cursor beyond every key (Seek -> Last) and one before all
// keys (Seek lands, then Prev walks off the front).
func TestListNonExactCursor(t *testing.T) {
	ls, done := setupLogStoreTest(t, 0)
	defer done()
	for i := 0; i < 3; i++ {
		if err := ls.Log(Entry{RuleID: "r1", Status: "success"}); err != nil {
			t.Fatal(err)
		}
	}

	// Cursor lexically after every real key -> Seek returns nil -> c.Last().
	beyond, _, err := ls.List(50, "99999999999999999999|zzzz", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(beyond) != 3 {
		t.Errorf("beyond-cursor list = %d, want all 3", len(beyond))
	}

	// Cursor before every real key -> Seek lands on the first key, Prev walks
	// off the front -> empty page.
	before, _, err := ls.List(50, "0", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Errorf("before-cursor list = %d, want 0", len(before))
	}
}

// TestListCountNoBucket covers the nil-bucket short-circuits in List and Count
// (a store whose bucket was never created).
func TestListCountNoBucket(t *testing.T) {
	dir := t.TempDir()
	db, err := bolt.Open(filepath.Join(dir, "nb.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ls := NewStore(db, 0) // no EnsureBucket

	entries, _, err := ls.List(10, "", "", "")
	if err != nil || len(entries) != 0 {
		t.Errorf("List without bucket = (%d, %v), want (0, nil)", len(entries), err)
	}
	if n, err := ls.Count("", ""); err != nil || n != 0 {
		t.Errorf("Count without bucket = (%d, %v), want (0, nil)", n, err)
	}
}

// TestListPaginationAndFilters exercises List's cursor paging, the limit cap,
// status filtering, and Count's filtered slow path.
func TestListPaginationAndFilters(t *testing.T) {
	ls, done := setupLogStoreTest(t, 0)
	defer done()

	// Five entries: alternating status so the filter is meaningful.
	for i := 0; i < 5; i++ {
		status := "success"
		if i%2 == 1 {
			status = "error"
		}
		if err := ls.Log(Entry{RuleID: "r1", RuleType: "trigger", Status: status}); err != nil {
			t.Fatal(err)
		}
	}

	// Page 1: limit 2 -> 2 entries plus a next cursor.
	page1, next, err := ls.List(2, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 2 || next == "" {
		t.Fatalf("page1=%d next=%q, want 2 and a cursor", len(page1), next)
	}

	// Page 2: follow the cursor (exclusive) -> the next two entries.
	page2, _, err := ls.List(2, next, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 2 {
		t.Fatalf("page2=%d, want 2", len(page2))
	}
	if page2[0].ID == page1[1].ID {
		t.Error("cursor must be exclusive (page2 overlaps page1)")
	}

	// Status filter: only the error entries.
	errOnly, _, err := ls.List(50, "", "", "error")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range errOnly {
		if e.Status != "error" {
			t.Errorf("status filter leaked %q", e.Status)
		}
	}

	// limit > 500 is capped (no panic / huge alloc), still returns all 5.
	all, _, err := ls.List(600, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5 {
		t.Errorf("expected 5 entries, got %d", len(all))
	}

	// Count slow path with a filter.
	n, err := ls.Count("r1", "error")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("Count(r1,error) = %d, want 2", n)
	}
}
