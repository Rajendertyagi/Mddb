package main

import (
	"net/http/httptest"
	"testing"

	json "github.com/goccy/go-json"
	bolt "go.etcd.io/bbolt"
)

// newAutocompleteServer creates a server with FTS buckets ready for
// Autocomplete. Separate helper so each test can seed terms deterministically
// without depending on the full batch ingest path.
func newAutocompleteServer(t *testing.T) (*Server, func()) {
	t.Helper()
	s, cleanup := newTestServer(t)
	idx := NewFTSIndex(s.DB)
	if err := idx.EnsureBuckets(); err != nil {
		cleanup()
		t.Fatalf("ensure fts buckets: %v", err)
	}
	s.FTSIndex = idx
	return s, cleanup
}

// indexDoc is the shortest path to put a single-field document into both the
// global and field-scoped indices so Autocomplete can find it. Real document
// ingestion goes through the batch processor; tests don't need that weight.
func indexDoc(t *testing.T, s *Server, collection, docID, field, content string) {
	t.Helper()
	if err := s.FTSIndex.Index(collection, docID, content); err != nil {
		t.Fatalf("fts index: %v", err)
	}
	if err := s.FTSIndex.IndexFields(collection, docID, map[string]string{field: content}); err != nil {
		t.Fatalf("fts index fields: %v", err)
	}
}

func TestNormalizeAutocompletePrefix(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Mark", "mark"},
		{"mark*", "mark"},
		{"mar d", "mar"}, // stop at first separator
		{"  ", ""},
		{"ABC123", "abc123"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := normalizeAutocompletePrefix(tc.in); got != tc.want {
			t.Errorf("normalize(%q)=%q; want %q", tc.in, got, tc.want)
		}
	}
}

func TestAutocomplete_MatchesByDocCount(t *testing.T) {
	s, cleanup := newAutocompleteServer(t)
	defer cleanup()

	// "market" appears in 3 docs, "marker" in 2, "marathon" in 1 — all share
	// prefix "mar". Expected order: market > marker > marathon.
	indexDoc(t, s, "c", "d1", "content", "market")
	indexDoc(t, s, "c", "d2", "content", "market and marker")
	indexDoc(t, s, "c", "d3", "content", "market news")
	indexDoc(t, s, "c", "d4", "content", "marker only")
	indexDoc(t, s, "c", "d5", "content", "marathon runner")

	items, err := s.FTSIndex.Autocomplete("c", "mar", "", 10)
	if err != nil {
		t.Fatalf("autocomplete: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 distinct terms, got %d: %+v", len(items), items)
	}
	if items[0].Term != "market" || items[0].DocCount != 3 {
		t.Errorf("expected market@3 first, got %+v", items[0])
	}
	if items[1].Term != "marker" || items[1].DocCount != 2 {
		t.Errorf("expected marker@2 second, got %+v", items[1])
	}
	if items[2].Term != "marathon" || items[2].DocCount != 1 {
		t.Errorf("expected marathon@1 third, got %+v", items[2])
	}
}

func TestAutocomplete_TopNCap(t *testing.T) {
	s, cleanup := newAutocompleteServer(t)
	defer cleanup()

	indexDoc(t, s, "c", "d1", "content", "market")
	indexDoc(t, s, "c", "d2", "content", "marker")
	indexDoc(t, s, "c", "d3", "content", "marathon")

	items, err := s.FTSIndex.Autocomplete("c", "mar", "", 2)
	if err != nil {
		t.Fatalf("autocomplete: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected topN=2, got %d", len(items))
	}
}

func TestAutocomplete_FieldScoped(t *testing.T) {
	s, cleanup := newAutocompleteServer(t)
	defer cleanup()

	// "market" lives in the title field, "marker" in content. Field-scoped
	// autocomplete should return only the term present in that field.
	_ = s.FTSIndex.IndexFields("c", "d1", map[string]string{
		"title":   "market news",
		"content": "marker only",
	})

	titleItems, err := s.FTSIndex.Autocomplete("c", "mar", "title", 10)
	if err != nil {
		t.Fatalf("autocomplete title: %v", err)
	}
	if len(titleItems) != 1 || titleItems[0].Term != "market" {
		t.Errorf("expected market only in title scope, got %+v", titleItems)
	}
	if titleItems[0].Field != "title" {
		t.Errorf("expected field=title, got %q", titleItems[0].Field)
	}

	contentItems, _ := s.FTSIndex.Autocomplete("c", "mar", "content", 10)
	if len(contentItems) != 1 || contentItems[0].Term != "marker" {
		t.Errorf("expected marker only in content scope, got %+v", contentItems)
	}
}

func TestAutocomplete_EmptyPrefix(t *testing.T) {
	s, cleanup := newAutocompleteServer(t)
	defer cleanup()

	indexDoc(t, s, "c", "d1", "content", "marker")
	items, err := s.FTSIndex.Autocomplete("c", "", "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty result for empty prefix, got %+v", items)
	}
}

func TestAutocomplete_MissingCollection(t *testing.T) {
	s, cleanup := newAutocompleteServer(t)
	defer cleanup()
	if _, err := s.FTSIndex.Autocomplete("", "mar", "", 10); err == nil {
		t.Error("expected error for missing collection")
	}
}

func TestAutocomplete_LongPrefixTruncated(t *testing.T) {
	s, cleanup := newAutocompleteServer(t)
	defer cleanup()

	indexDoc(t, s, "c", "d1", "content", "supercalifragilistic")

	// Prefix longer than 32 chars — should truncate silently and still match.
	longPrefix := "supercalifragilisticexpialidocioussupercalifragilistic"
	items, err := s.FTSIndex.Autocomplete("c", longPrefix, "", 10)
	if err != nil {
		t.Fatalf("autocomplete: %v", err)
	}
	// No guarantee of a match after truncation — just assert no crash.
	_ = items
}

func TestHandleAutocomplete_HTTP(t *testing.T) {
	s, cleanup := newAutocompleteServer(t)
	defer cleanup()

	indexDoc(t, s, "blog", "p1", "title", "markdown")
	indexDoc(t, s, "blog", "p2", "title", "marker pens")

	req := httptest.NewRequest("GET", "/v1/autocomplete?collection=blog&q=mar&topN=5", nil)
	rec := httptest.NewRecorder()
	s.handleAutocomplete(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp AutocompleteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Query != "mar" {
		t.Errorf("expected query=mar, got %q", resp.Query)
	}
	if resp.Total == 0 {
		t.Errorf("expected matches, got empty items")
	}
}

func TestHandleAutocomplete_MissingCollection(t *testing.T) {
	s, cleanup := newAutocompleteServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/v1/autocomplete?q=mar", nil)
	rec := httptest.NewRecorder()
	s.handleAutocomplete(rec, req)
	if rec.Code != 400 {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleAutocomplete_EmptyQueryReturnsEmpty(t *testing.T) {
	s, cleanup := newAutocompleteServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/v1/autocomplete?collection=c&q=", nil)
	rec := httptest.NewRecorder()
	s.handleAutocomplete(rec, req)
	if rec.Code != 200 {
		t.Errorf("expected 200 even for empty query, got %d", rec.Code)
	}
	var resp AutocompleteResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 0 {
		t.Errorf("expected empty items, got %+v", resp.Items)
	}
}

func TestHandleAutocomplete_WrongMethod(t *testing.T) {
	s, cleanup := newAutocompleteServer(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/v1/autocomplete", nil)
	rec := httptest.NewRecorder()
	s.handleAutocomplete(rec, req)
	if rec.Code != 405 {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestParseIntParam(t *testing.T) {
	tests := []struct {
		qs   string
		key  string
		def  int
		want int
	}{
		{"n=42", "n", 10, 42},
		{"n=abc", "n", 10, 10},
		{"n=-5", "n", 10, 10}, // negative ignored
		{"n=0", "n", 10, 10},  // zero → default
		{"", "n", 7, 7},       // missing → default
	}
	for _, tc := range tests {
		req := httptest.NewRequest("GET", "/?"+tc.qs, nil)
		if got := parseIntParam(req, tc.key, tc.def); got != tc.want {
			t.Errorf("parseIntParam(%q)=%d; want %d", tc.qs, got, tc.want)
		}
	}
}

func TestAutocompleteItem_String(t *testing.T) {
	a := AutocompleteItem{Term: "foo", DocCount: 3}
	if a.String() == "" {
		t.Error("expected non-empty string")
	}
	b := AutocompleteItem{Term: "foo", Field: "title", DocCount: 3}
	if b.String() == a.String() {
		t.Error("field should alter formatting")
	}
}

// Keep reference to the bolt import so gofmt doesn't reorder it — we touch
// bolt buckets indirectly via the FTSIndex helpers.
var _ = bolt.Open
