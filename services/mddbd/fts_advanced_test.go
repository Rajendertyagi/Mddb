package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	json "github.com/goccy/go-json"
)

// --- Query Parser Tests ---

func TestParseAdvancedQuery_Simple(t *testing.T) {
	pq := ParseAdvancedQuery("rust performance")
	if pq.IsAdvanced() {
		t.Fatal("simple query should not be advanced")
	}
	if len(pq.Clauses) != 2 {
		t.Fatalf("expected 2 clauses, got %d", len(pq.Clauses))
	}
	if pq.Clauses[0].Value != "rust" || pq.Clauses[1].Value != "performance" {
		t.Fatalf("unexpected terms: %+v", pq.Clauses)
	}
}

func TestParseAdvancedQuery_Boolean(t *testing.T) {
	pq := ParseAdvancedQuery("rust AND performance")
	if !pq.HasBoolean {
		t.Fatal("expected HasBoolean=true")
	}
	if len(pq.Clauses) != 2 {
		t.Fatalf("expected 2 clauses, got %d", len(pq.Clauses))
	}
	if pq.Clauses[1].Operator != "AND" {
		t.Fatalf("expected AND operator, got %q", pq.Clauses[1].Operator)
	}
}

func TestParseAdvancedQuery_OR(t *testing.T) {
	pq := ParseAdvancedQuery("rust OR golang")
	if !pq.HasBoolean {
		t.Fatal("expected HasBoolean=true")
	}
	if pq.Clauses[1].Operator != "OR" {
		t.Fatalf("expected OR operator, got %q", pq.Clauses[1].Operator)
	}
}

func TestParseAdvancedQuery_NOT(t *testing.T) {
	pq := ParseAdvancedQuery("rust NOT java")
	if !pq.HasBoolean {
		t.Fatal("expected HasBoolean=true")
	}
	found := false
	for _, c := range pq.Clauses {
		if c.IsNegated && c.Value == "java" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected negated 'java' clause")
	}
}

func TestParseAdvancedQuery_PlusMinus(t *testing.T) {
	pq := ParseAdvancedQuery("+required -excluded optional")
	if !pq.HasBoolean {
		t.Fatal("expected HasBoolean=true")
	}
	if len(pq.Clauses) != 3 {
		t.Fatalf("expected 3 clauses, got %d", len(pq.Clauses))
	}
	if pq.Clauses[1].IsNegated != true || pq.Clauses[1].Value != "excluded" {
		t.Fatalf("expected negated 'excluded', got %+v", pq.Clauses[1])
	}
}

func TestParseAdvancedQuery_Phrase(t *testing.T) {
	pq := ParseAdvancedQuery(`"machine learning"`)
	if !pq.HasPhrase {
		t.Fatal("expected HasPhrase=true")
	}
	if len(pq.Clauses) != 1 {
		t.Fatalf("expected 1 clause, got %d", len(pq.Clauses))
	}
	if pq.Clauses[0].Type != "phrase" || pq.Clauses[0].Value != "machine learning" {
		t.Fatalf("unexpected clause: %+v", pq.Clauses[0])
	}
}

func TestParseAdvancedQuery_Proximity(t *testing.T) {
	pq := ParseAdvancedQuery(`"rust performance"~5`)
	if !pq.HasProximity {
		t.Fatal("expected HasProximity=true")
	}
	if pq.Clauses[0].Type != "proximity" {
		t.Fatalf("expected proximity type, got %s", pq.Clauses[0].Type)
	}
	if pq.Clauses[0].Distance != 5 {
		t.Fatalf("expected distance 5, got %d", pq.Clauses[0].Distance)
	}
}

func TestParseAdvancedQuery_Wildcard(t *testing.T) {
	pq := ParseAdvancedQuery("prog*")
	if !pq.HasWildcard {
		t.Fatal("expected HasWildcard=true")
	}
	if pq.Clauses[0].Type != "wildcard" {
		t.Fatalf("expected wildcard type, got %s", pq.Clauses[0].Type)
	}
}

func TestParseAdvancedQuery_Mixed(t *testing.T) {
	pq := ParseAdvancedQuery(`"machine learning" AND rust NOT java`)
	if !pq.HasBoolean || !pq.HasPhrase {
		t.Fatal("expected both HasBoolean and HasPhrase")
	}
	if !pq.IsAdvanced() {
		t.Fatal("expected IsAdvanced=true")
	}
}

// --- Wildcard Match Tests ---

func TestWildcardMatch(t *testing.T) {
	tests := []struct {
		pattern string
		text    string
		want    bool
	}{
		{"prog*", "programming", true},
		{"prog*", "program", true},
		{"prog*", "pro", false},
		{"te?t", "test", true},
		{"te?t", "text", true},
		{"te?t", "tet", false},
		{"*ing", "programming", true},
		{"*ing", "running", true},
		{"*ing", "run", false},
		{"*", "anything", true},
		{"?", "a", true},
		{"?", "ab", false},
		{"p*m*g", "programming", true},
		{"p*m*g", "prog", false},
	}

	for _, tt := range tests {
		got := wildcardMatch(tt.pattern, tt.text)
		if got != tt.want {
			t.Errorf("wildcardMatch(%q, %q) = %v, want %v", tt.pattern, tt.text, got, tt.want)
		}
	}
}

// --- Positional Index Tests ---

func TestPositionalIndex_EncodeDecodePositions(t *testing.T) {
	positions := []uint32{0, 3, 7, 15}
	encoded := encodePositions(positions)
	decoded := decodePositions(encoded)

	if len(decoded) != len(positions) {
		t.Fatalf("expected %d positions, got %d", len(positions), len(decoded))
	}
	for i, p := range positions {
		if decoded[i] != p {
			t.Fatalf("position[%d]: expected %d, got %d", i, p, decoded[i])
		}
	}
}

func TestPositionalIndex_TokenizePositions(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	positions := s.FTSIndex.TokenizePositions("the quick brown fox jumps over the lazy dog")
	// "the" and "over" are stop words and should be excluded from positions map
	if _, ok := positions["the"]; ok {
		t.Fatal("'the' should be a stop word")
	}
	// "quick" should be present with a valid position
	if _, ok := positions["quick"]; !ok {
		t.Fatal("expected 'quick' in positions")
	}
	// "brown" and "fox" should also be present
	if _, ok := positions["brown"]; !ok {
		t.Fatal("expected 'brown' in positions")
	}
	if _, ok := positions["fox"]; !ok {
		t.Fatal("expected 'fox' in positions")
	}
}

// --- Phrase Search Tests ---

func TestSearchPhrase_ExactMatch(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "blog", "ml-post", "en", "introduction to machine learning algorithms", nil)
	_ = s.FTSIndex.Index("blog", doc1.ID, doc1.ContentMD)
	_ = s.FTSIndex.IndexPositions("blog", doc1.ID, doc1.ContentMD)

	doc2 := addTestDoc(t, s, "blog", "dl-post", "en", "deep learning is a subset of machine intelligence", nil)
	_ = s.FTSIndex.Index("blog", doc2.ID, doc2.ContentMD)
	_ = s.FTSIndex.IndexPositions("blog", doc2.ID, doc2.ContentMD)

	results, err := s.FTSIndex.SearchPhrase("blog", "machine learning", 10)
	if err != nil {
		t.Fatal(err)
	}

	// Only doc1 has "machine learning" as consecutive terms
	if len(results) != 1 {
		t.Fatalf("expected 1 result for phrase 'machine learning', got %d", len(results))
	}
	if results[0].DocID != doc1.ID {
		t.Fatalf("expected doc %s, got %s", doc1.ID, results[0].DocID)
	}
}

func TestSearchPhrase_NoMatch(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "blog", "post1", "en", "learning about machines in the factory", nil)
	_ = s.FTSIndex.Index("blog", doc1.ID, doc1.ContentMD)
	_ = s.FTSIndex.IndexPositions("blog", doc1.ID, doc1.ContentMD)

	results, err := s.FTSIndex.SearchPhrase("blog", "machine learning", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

// --- Proximity Search Tests ---

func TestSearchProximity_WithinDistance(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	// doc1: "rust" and "systems" are close (within 3 words)
	doc1 := addTestDoc(t, s, "blog", "close", "en", "rust excellent systems", nil)
	_ = s.FTSIndex.Index("blog", doc1.ID, doc1.ContentMD)
	_ = s.FTSIndex.IndexPositions("blog", doc1.ID, doc1.ContentMD)

	// doc2: "rust" and "systems" are far apart
	doc2 := addTestDoc(t, s, "blog", "far", "en", "rust created many years ago eventually became useful powerful amazing incredible systems", nil)
	_ = s.FTSIndex.Index("blog", doc2.ID, doc2.ContentMD)
	_ = s.FTSIndex.IndexPositions("blog", doc2.ID, doc2.ContentMD)

	// Search for "rust" and "systems" within 3 words
	results, err := s.FTSIndex.SearchProximity("blog", "rust systems", 3, 10)
	if err != nil {
		t.Fatal(err)
	}

	// doc1 has them close, doc2 has them far apart
	if len(results) == 0 {
		t.Fatal("expected at least 1 result for proximity search")
	}
	// doc1 should be in results
	found := false
	for _, r := range results {
		if r.DocID == doc1.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected doc1 (%s) in proximity results", doc1.ID)
	}
}

// --- Wildcard Search Tests ---

func TestSearchWildcard_Star(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "blog", "go-post", "en", "golang programming language", nil)
	_ = s.FTSIndex.Index("blog", doc1.ID, doc1.ContentMD)

	doc2 := addTestDoc(t, s, "blog", "py-post", "en", "python programming guide", nil)
	_ = s.FTSIndex.Index("blog", doc2.ID, doc2.ContentMD)

	// Search for "prog*" should match "program" (stemmed from "programming")
	results, err := s.FTSIndex.SearchWildcard("blog", "prog*", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for prog*, got %d", len(results))
	}
}

func TestSearchWildcard_QuestionMark(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "blog", "test-post", "en", "test post content", nil)
	_ = s.FTSIndex.Index("blog", doc1.ID, doc1.ContentMD)

	doc2 := addTestDoc(t, s, "blog", "text-post", "en", "text post content", nil)
	_ = s.FTSIndex.Index("blog", doc2.ID, doc2.ContentMD)

	// "te?t" should match both "test" and "text"
	results, err := s.FTSIndex.SearchWildcard("blog", "te?t", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for te?t, got %d", len(results))
	}
}

// --- Boolean Search Tests ---

func TestSearchBoolean_AND(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "blog", "both", "en", "rust programming language systems", nil)
	_ = s.FTSIndex.Index("blog", doc1.ID, doc1.ContentMD)

	doc2 := addTestDoc(t, s, "blog", "only-rust", "en", "rust language overview", nil)
	_ = s.FTSIndex.Index("blog", doc2.ID, doc2.ContentMD)

	doc3 := addTestDoc(t, s, "blog", "only-prog", "en", "programming tutorial guide", nil)
	_ = s.FTSIndex.Index("blog", doc3.ID, doc3.ContentMD)

	parsed := ParseAdvancedQuery("rust AND programming")
	results, err := s.FTSIndex.SearchBoolean("blog", parsed, 10)
	if err != nil {
		t.Fatal(err)
	}

	// Only doc1 has both "rust" and "programming"
	if len(results) != 1 {
		t.Fatalf("expected 1 result for AND, got %d", len(results))
	}
	if results[0].DocID != doc1.ID {
		t.Fatalf("expected doc %s, got %s", doc1.ID, results[0].DocID)
	}
}

func TestSearchBoolean_OR(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "blog", "rust-post", "en", "rust language", nil)
	_ = s.FTSIndex.Index("blog", doc1.ID, doc1.ContentMD)

	doc2 := addTestDoc(t, s, "blog", "go-post", "en", "golang language", nil)
	_ = s.FTSIndex.Index("blog", doc2.ID, doc2.ContentMD)

	doc3 := addTestDoc(t, s, "blog", "py-post", "en", "python language", nil)
	_ = s.FTSIndex.Index("blog", doc3.ID, doc3.ContentMD)

	parsed := ParseAdvancedQuery("rust OR golang")
	results, err := s.FTSIndex.SearchBoolean("blog", parsed, 10)
	if err != nil {
		t.Fatal(err)
	}

	// doc1 and doc2 should match
	if len(results) != 2 {
		t.Fatalf("expected 2 results for OR, got %d", len(results))
	}
}

func TestSearchBoolean_NOT(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "blog", "rust-post", "en", "rust programming language", nil)
	_ = s.FTSIndex.Index("blog", doc1.ID, doc1.ContentMD)

	doc2 := addTestDoc(t, s, "blog", "java-post", "en", "java programming language", nil)
	_ = s.FTSIndex.Index("blog", doc2.ID, doc2.ContentMD)

	parsed := ParseAdvancedQuery("programming NOT java")
	results, err := s.FTSIndex.SearchBoolean("blog", parsed, 10)
	if err != nil {
		t.Fatal(err)
	}

	// Only doc1 should match (has programming, doesn't have java)
	if len(results) != 1 {
		t.Fatalf("expected 1 result for NOT, got %d", len(results))
	}
}

// --- Range Search Tests ---

func TestSearchRange_Timestamp(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	addTestDoc(t, s, "blog", "old-post", "en", "old content", nil)
	addTestDoc(t, s, "blog", "new-post", "en", "new content", nil)

	// Range filter: documents added in the last hour
	now := currentUnix()
	results, err := s.SearchRange("blog", []RangeFilter{
		{Field: "addedAt", Gte: itoa(int(now - 3600)), Lte: itoa(int(now + 60))},
	}, 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results within time range, got %d", len(results))
	}
}

func TestFilterByRange_MetaField(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "shop", "item1", "en", "cheap item", map[string][]string{"price": {"10"}})
	doc2 := addTestDoc(t, s, "shop", "item2", "en", "mid item", map[string][]string{"price": {"50"}})
	doc3 := addTestDoc(t, s, "shop", "item3", "en", "expensive item", map[string][]string{"price": {"200"}})

	input := []FTSResult{
		{DocID: doc1.ID, Score: 1.0},
		{DocID: doc2.ID, Score: 1.0},
		{DocID: doc3.ID, Score: 1.0},
	}

	// Filter: price between 20 and 100
	results, err := s.FilterByRange("shop", input, []RangeFilter{
		{Field: "price", Gte: "20", Lte: "100"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result in range 20-100, got %d", len(results))
	}
	if results[0].DocID != doc2.ID {
		t.Fatalf("expected doc %s, got %s", doc2.ID, results[0].DocID)
	}
}

// --- HTTP Handler Integration Tests ---

func TestHandleFTS_PhraseMode(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	doc := addTestDoc(t, s, "blog", "ml", "en", "introduction to machine learning algorithms", nil)
	_ = s.FTSIndex.Index("blog", doc.ID, doc.ContentMD)
	_ = s.FTSIndex.IndexPositions("blog", doc.ID, doc.ContentMD)

	body := `{"collection":"blog","query":"\"machine learning\"","mode":"auto"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/fts", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleFTS(w, req)

	if w.Result().StatusCode != 200 {
		t.Fatalf("expected 200, got %d; body: %s", w.Result().StatusCode, w.Body.String())
	}

	var resp FTSSearchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Mode != "phrase" {
		t.Fatalf("expected mode=phrase, got %s", resp.Mode)
	}
	if resp.Total != 1 {
		t.Fatalf("expected 1 result, got %d", resp.Total)
	}
}

func TestHandleFTS_WildcardMode(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	doc := addTestDoc(t, s, "blog", "go-post", "en", "golang programming language guide", nil)
	_ = s.FTSIndex.Index("blog", doc.ID, doc.ContentMD)

	body := `{"collection":"blog","query":"prog*","mode":"wildcard"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/fts", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleFTS(w, req)

	if w.Result().StatusCode != 200 {
		t.Fatalf("expected 200, got %d", w.Result().StatusCode)
	}

	var resp FTSSearchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Mode != "wildcard" {
		t.Fatalf("expected mode=wildcard, got %s", resp.Mode)
	}
	if resp.Total == 0 {
		t.Fatal("expected at least 1 result for prog*")
	}
}

func TestHandleFTS_BooleanMode(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "blog", "both", "en", "rust programming systems", nil)
	_ = s.FTSIndex.Index("blog", doc1.ID, doc1.ContentMD)

	doc2 := addTestDoc(t, s, "blog", "rust-only", "en", "rust language overview", nil)
	_ = s.FTSIndex.Index("blog", doc2.ID, doc2.ContentMD)

	body := `{"collection":"blog","query":"rust AND programming","mode":"boolean"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/fts", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleFTS(w, req)

	if w.Result().StatusCode != 200 {
		t.Fatalf("expected 200, got %d", w.Result().StatusCode)
	}

	var resp FTSSearchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Mode != "boolean" {
		t.Fatalf("expected mode=boolean, got %s", resp.Mode)
	}
	if resp.Total != 1 {
		t.Fatalf("expected 1 result for AND, got %d", resp.Total)
	}
}

func TestHandleFTS_RangeFilter(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "shop", "item1", "en", "cheap widget", map[string][]string{"price": {"10"}})
	_ = s.FTSIndex.Index("shop", doc1.ID, doc1.ContentMD)

	doc2 := addTestDoc(t, s, "shop", "item2", "en", "expensive widget", map[string][]string{"price": {"500"}})
	_ = s.FTSIndex.Index("shop", doc2.ID, doc2.ContentMD)

	body := `{"collection":"shop","query":"widget","rangeMeta":[{"field":"price","gte":"100","lte":"1000"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/fts", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleFTS(w, req)

	if w.Result().StatusCode != 200 {
		t.Fatalf("expected 200, got %d; body: %s", w.Result().StatusCode, w.Body.String())
	}

	var resp FTSSearchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 {
		t.Fatalf("expected 1 result with price range, got %d", resp.Total)
	}
}

func TestHandleFTS_AutoDetectMode(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	doc := addTestDoc(t, s, "blog", "post1", "en", "golang web programming tutorial", nil)
	_ = s.FTSIndex.Index("blog", doc.ID, doc.ContentMD)

	// Simple query with no mode specified should auto-detect as "simple"
	body := `{"collection":"blog","query":"golang"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/fts", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleFTS(w, req)

	if w.Result().StatusCode != 200 {
		t.Fatalf("expected 200, got %d", w.Result().StatusCode)
	}

	var resp FTSSearchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Mode != "simple" {
		t.Fatalf("expected mode=simple for plain query, got %s", resp.Mode)
	}
}
