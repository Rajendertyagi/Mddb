package fts

import "testing"

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
func TestPositionalIndex_TokenizePositions(t *testing.T) {
	s, cleanup := newSearchFTS(t)
	defer cleanup()

	positions := s.TokenizePositions("the quick brown fox jumps over the lazy dog")
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
func TestSearchPhrase_ExactMatch(t *testing.T) {
	s, cleanup := newSearchFTS(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "blog", "ml-post", "en", "introduction to machine learning algorithms", nil)
	_ = s.Index("blog", doc1.ID, doc1.ContentMD)
	_ = s.IndexPositions("blog", doc1.ID, doc1.ContentMD)

	doc2 := addTestDoc(t, s, "blog", "dl-post", "en", "deep learning is a subset of machine intelligence", nil)
	_ = s.Index("blog", doc2.ID, doc2.ContentMD)
	_ = s.IndexPositions("blog", doc2.ID, doc2.ContentMD)

	results, err := s.SearchPhrase("blog", "machine learning", 10)
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
	s, cleanup := newSearchFTS(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "blog", "post1", "en", "learning about machines in the factory", nil)
	_ = s.Index("blog", doc1.ID, doc1.ContentMD)
	_ = s.IndexPositions("blog", doc1.ID, doc1.ContentMD)

	results, err := s.SearchPhrase("blog", "machine learning", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}
func TestSearchProximity_WithinDistance(t *testing.T) {
	s, cleanup := newSearchFTS(t)
	defer cleanup()

	// doc1: "rust" and "systems" are close (within 3 words)
	doc1 := addTestDoc(t, s, "blog", "close", "en", "rust excellent systems", nil)
	_ = s.Index("blog", doc1.ID, doc1.ContentMD)
	_ = s.IndexPositions("blog", doc1.ID, doc1.ContentMD)

	// doc2: "rust" and "systems" are far apart
	doc2 := addTestDoc(t, s, "blog", "far", "en", "rust created many years ago eventually became useful powerful amazing incredible systems", nil)
	_ = s.Index("blog", doc2.ID, doc2.ContentMD)
	_ = s.IndexPositions("blog", doc2.ID, doc2.ContentMD)

	// Search for "rust" and "systems" within 3 words
	results, err := s.SearchProximity("blog", "rust systems", 3, 10)
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
func TestSearchWildcard_Star(t *testing.T) {
	s, cleanup := newSearchFTS(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "blog", "go-post", "en", "golang programming language", nil)
	_ = s.Index("blog", doc1.ID, doc1.ContentMD)

	doc2 := addTestDoc(t, s, "blog", "py-post", "en", "python programming guide", nil)
	_ = s.Index("blog", doc2.ID, doc2.ContentMD)

	// Search for "prog*" should match "program" (stemmed from "programming")
	results, err := s.SearchWildcard("blog", "prog*", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for prog*, got %d", len(results))
	}
}
func TestSearchWildcard_QuestionMark(t *testing.T) {
	s, cleanup := newSearchFTS(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "blog", "test-post", "en", "test post content", nil)
	_ = s.Index("blog", doc1.ID, doc1.ContentMD)

	doc2 := addTestDoc(t, s, "blog", "text-post", "en", "text post content", nil)
	_ = s.Index("blog", doc2.ID, doc2.ContentMD)

	// "te?t" should match both "test" and "text"
	results, err := s.SearchWildcard("blog", "te?t", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for te?t, got %d", len(results))
	}
}
func TestSearchBoolean_AND(t *testing.T) {
	s, cleanup := newSearchFTS(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "blog", "both", "en", "rust programming language systems", nil)
	_ = s.Index("blog", doc1.ID, doc1.ContentMD)

	doc2 := addTestDoc(t, s, "blog", "only-rust", "en", "rust language overview", nil)
	_ = s.Index("blog", doc2.ID, doc2.ContentMD)

	doc3 := addTestDoc(t, s, "blog", "only-prog", "en", "programming tutorial guide", nil)
	_ = s.Index("blog", doc3.ID, doc3.ContentMD)

	parsed := ParseAdvancedQuery("rust AND programming")
	results, err := s.SearchBoolean("blog", parsed, 10)
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
	s, cleanup := newSearchFTS(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "blog", "rust-post", "en", "rust language", nil)
	_ = s.Index("blog", doc1.ID, doc1.ContentMD)

	doc2 := addTestDoc(t, s, "blog", "go-post", "en", "golang language", nil)
	_ = s.Index("blog", doc2.ID, doc2.ContentMD)

	doc3 := addTestDoc(t, s, "blog", "py-post", "en", "python language", nil)
	_ = s.Index("blog", doc3.ID, doc3.ContentMD)

	parsed := ParseAdvancedQuery("rust OR golang")
	results, err := s.SearchBoolean("blog", parsed, 10)
	if err != nil {
		t.Fatal(err)
	}

	// doc1 and doc2 should match
	if len(results) != 2 {
		t.Fatalf("expected 2 results for OR, got %d", len(results))
	}
}
func TestSearchBoolean_NOT(t *testing.T) {
	s, cleanup := newSearchFTS(t)
	defer cleanup()

	doc1 := addTestDoc(t, s, "blog", "rust-post", "en", "rust programming language", nil)
	_ = s.Index("blog", doc1.ID, doc1.ContentMD)

	doc2 := addTestDoc(t, s, "blog", "java-post", "en", "java programming language", nil)
	_ = s.Index("blog", doc2.ID, doc2.ContentMD)

	parsed := ParseAdvancedQuery("programming NOT java")
	results, err := s.SearchBoolean("blog", parsed, 10)
	if err != nil {
		t.Fatal(err)
	}

	// Only doc1 should match (has programming, doesn't have java)
	if len(results) != 1 {
		t.Fatalf("expected 1 result for NOT, got %d", len(results))
	}
}

func newSearchFTS(t *testing.T) (*FTSIndex, func()) {
	t.Helper()
	idx := NewFTSIndex(openTestDB(t))
	_ = idx.EnsureBuckets()
	return idx, func() {}
}

type testDoc struct {
	ID        string
	ContentMD string
}

// addTestDoc here is FTS-only: storage is irrelevant to these tests, which index
// the returned ID/content directly via the FTSIndex. It returns the key as the ID.
func addTestDoc(t *testing.T, _ *FTSIndex, coll, key, lang, content string, meta map[string][]string) testDoc {
	t.Helper()
	_ = coll
	_ = lang
	_ = meta
	return testDoc{ID: key, ContentMD: content}
}
