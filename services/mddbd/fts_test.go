package main

import (
	"os"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func newTestFTSIndex(t *testing.T) (*FTSIndex, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "fts_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}
	idx := NewFTSIndex(db)
	if err := idx.EnsureBuckets(); err != nil {
		_ = db.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}
	cleanup := func() {
		_ = db.Close()
		_ = os.Remove(f.Name())
	}
	return idx, cleanup
}

func TestNewFTSIndex(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	if idx.db == nil {
		t.Error("expected non-nil db")
	}
	if idx.stopWords == nil {
		t.Error("expected non-nil stopWords map")
	}
}

func TestFTSEnsureBuckets(t *testing.T) {
	f, err := os.CreateTemp("", "fts_buckets_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()

	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	idx := NewFTSIndex(db)

	// Should create buckets
	if err := idx.EnsureBuckets(); err != nil {
		t.Fatalf("EnsureBuckets failed: %v", err)
	}

	// Should be idempotent
	if err := idx.EnsureBuckets(); err != nil {
		t.Fatalf("second EnsureBuckets failed: %v", err)
	}

	// Verify buckets exist
	err = db.View(func(tx *bolt.Tx) error {
		if tx.Bucket(bucketFTS) == nil {
			t.Error("fts bucket not created")
		}
		if tx.Bucket(bucketFTSRev) == nil {
			t.Error("ftsrev bucket not created")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFTSTokenizeBasic(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	terms := idx.Tokenize("Hello World testing tokenizer")
	if _, ok := terms["hello"]; !ok {
		t.Error("expected 'hello' in terms")
	}
	if _, ok := terms["world"]; !ok {
		t.Error("expected 'world' in terms")
	}
	if _, ok := terms["testing"]; !ok {
		t.Error("expected 'testing' in terms")
	}
	if _, ok := terms["tokenizer"]; !ok {
		t.Error("expected 'tokenizer' in terms")
	}
}

func TestFTSTokenizeStopWords(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	terms := idx.Tokenize("the quick brown fox is not a dog")

	// "the", "is", "not" are stop words
	if _, ok := terms["the"]; ok {
		t.Error("'the' should be filtered as stop word")
	}
	if _, ok := terms["is"]; ok {
		t.Error("'is' should be filtered as stop word")
	}
	if _, ok := terms["not"]; ok {
		t.Error("'not' should be filtered as stop word")
	}

	// "quick", "brown", "fox", "dog" should be present
	if _, ok := terms["quick"]; !ok {
		t.Error("expected 'quick' in terms")
	}
	if _, ok := terms["brown"]; !ok {
		t.Error("expected 'brown' in terms")
	}
	if _, ok := terms["fox"]; !ok {
		t.Error("expected 'fox' in terms")
	}
	if _, ok := terms["dog"]; !ok {
		t.Error("expected 'dog' in terms")
	}
}

func TestFTSTokenizeShortWords(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	terms := idx.Tokenize("I a go fast")

	// Single-char words should be excluded (length < 2)
	if _, ok := terms["i"]; ok {
		t.Error("single char 'i' should be excluded")
	}
	if _, ok := terms["a"]; ok {
		t.Error("single char 'a' should be excluded")
	}
	// "go" is a stop word
	if _, ok := terms["go"]; ok {
		t.Error("'go' should be filtered as stop word")
	}
	// "fast" should be present
	if _, ok := terms["fast"]; !ok {
		t.Error("expected 'fast' in terms")
	}
}

func TestFTSTokenizeFrequency(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	terms := idx.Tokenize("hello hello hello world world")

	if count, ok := terms["hello"]; !ok || count != 3 {
		t.Errorf("expected hello:3, got %d", count)
	}
	if count, ok := terms["world"]; !ok || count != 2 {
		t.Errorf("expected world:2, got %d", count)
	}
}

func TestFTSTokenizeEmpty(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	terms := idx.Tokenize("")
	if len(terms) != 0 {
		t.Errorf("expected empty terms for empty input, got %d", len(terms))
	}
}

func TestFTSTokenizeOnlyStopWords(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	terms := idx.Tokenize("the and or but is was are")
	if len(terms) != 0 {
		t.Errorf("expected empty terms for all stop words, got %d terms", len(terms))
	}
}

func TestFTSTokenizeSpecialCharacters(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	terms := idx.Tokenize("hello-world foo_bar test@email")
	if _, ok := terms["hello"]; !ok {
		t.Error("expected 'hello' after splitting on hyphen")
	}
	if _, ok := terms["world"]; !ok {
		t.Error("expected 'world' after splitting on hyphen")
	}
	if _, ok := terms["foo"]; !ok {
		t.Error("expected 'foo' after splitting on underscore")
	}
	if _, ok := terms["bar"]; !ok {
		t.Error("expected 'bar' after splitting on underscore")
	}
}

func TestFTSTokenizeNumbers(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	terms := idx.Tokenize("version 123 release2024")
	if _, ok := terms["version"]; !ok {
		t.Error("expected 'version' in terms")
	}
	if _, ok := terms["123"]; !ok {
		t.Error("expected '123' in terms (digits are allowed)")
	}
	if _, ok := terms["release2024"]; !ok {
		t.Error("expected 'release2024' in terms (mixed alphanumeric)")
	}
}

func TestFTSIndexAndSearch(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	if err := idx.Index("blog", "doc1", "Golang programming language tutorial"); err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if err := idx.Index("blog", "doc2", "Python programming for beginners"); err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if err := idx.Index("blog", "doc3", "Rust systems programming language"); err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	// Search for "programming"
	results, err := idx.Search("blog", "programming", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results for 'programming', got %d", len(results))
	}

	// Search for "golang"
	results, err = idx.Search("blog", "golang", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'golang', got %d", len(results))
	}
	if len(results) > 0 && results[0].DocID != "doc1" {
		t.Errorf("expected doc1, got %s", results[0].DocID)
	}
}

func TestFTSSearchMultipleTerms(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	if err := idx.Index("docs", "d1", "golang programming language tutorial"); err != nil {
		t.Fatalf("Index d1: %v", err)
	}
	if err := idx.Index("docs", "d2", "python programming beginners guide"); err != nil {
		t.Fatalf("Index d2: %v", err)
	}
	if err := idx.Index("docs", "d3", "golang tutorial advanced topics"); err != nil {
		t.Fatalf("Index d3: %v", err)
	}

	// Search for "golang tutorial" should match d1 (both terms) and d3 (both terms), and maybe d2 (no match)
	results, err := idx.Search("docs", "golang tutorial", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// d1 and d3 should match (they have both terms or at least one)
	found := make(map[string]bool)
	for _, r := range results {
		found[r.DocID] = true
	}
	if !found["d1"] {
		t.Error("expected d1 to match 'golang tutorial'")
	}
	if !found["d3"] {
		t.Error("expected d3 to match 'golang tutorial'")
	}
}

func TestFTSSearchNoResults(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	if err := idx.Index("blog", "doc1", "golang programming language"); err != nil {
		t.Fatalf("Index: %v", err)
	}

	results, err := idx.Search("blog", "javascript", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestFTSSearchEmptyQuery(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	if err := idx.Index("blog", "doc1", "golang programming"); err != nil {
		t.Fatalf("Index: %v", err)
	}

	results, err := idx.Search("blog", "", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if results != nil {
		t.Error("expected nil results for empty query")
	}
}

func TestFTSSearchOnlyStopWords(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	if err := idx.Index("blog", "doc1", "golang programming"); err != nil {
		t.Fatalf("Index: %v", err)
	}

	results, err := idx.Search("blog", "the and or", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if results != nil {
		t.Error("expected nil results for query with only stop words")
	}
}

func TestFTSSearchLimit(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	for i := 0; i < 20; i++ {
		_ = idx.Index("col", strings.Repeat("x", i+2), "common search term repeated")
	}

	results, err := idx.Search("col", "common search term", 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) > 5 {
		t.Errorf("expected at most 5 results, got %d", len(results))
	}
}

func TestFTSSearchDifferentCollections(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	if err := idx.Index("blog", "doc1", "golang programming"); err != nil {
		t.Fatalf("Index blog: %v", err)
	}
	if err := idx.Index("docs", "doc1", "golang programming"); err != nil {
		t.Fatalf("Index docs: %v", err)
	}

	// Search in "blog" only
	results, err := idx.Search("blog", "golang", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result in blog collection, got %d", len(results))
	}

	// Search in "docs" only
	results, err = idx.Search("docs", "golang", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result in docs collection, got %d", len(results))
	}
}

func TestFTSSearchSorting(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	// doc1 has "golang" once, doc2 has it multiple times
	_ = idx.Index("col", "doc1", "golang tutorial")
	_ = idx.Index("col", "doc2", "golang golang golang advanced golang")

	results, err := idx.Search("col", "golang", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	// Results should be sorted by score descending
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted by score: [%d].Score=%f > [%d].Score=%f",
				i, results[i].Score, i-1, results[i-1].Score)
		}
	}
}

func TestFTSIndexUpdate(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	// Index initially
	if err := idx.Index("blog", "doc1", "golang programming language"); err != nil {
		t.Fatalf("Index: %v", err)
	}

	results, _ := idx.Search("blog", "golang", 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Update the document - old terms should be removed
	if err := idx.Index("blog", "doc1", "python scripting language"); err != nil {
		t.Fatalf("Index: %v", err)
	}

	// "golang" should no longer match
	results, _ = idx.Search("blog", "golang", 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results for 'golang' after update, got %d", len(results))
	}

	// "python" should match
	results, _ = idx.Search("blog", "python", 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'python' after update, got %d", len(results))
	}
}

func TestFTSRemove(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	if err := idx.Index("blog", "doc1", "golang programming language"); err != nil {
		t.Fatalf("Index: %v", err)
	}

	// Verify it's indexed
	results, _ := idx.Search("blog", "golang", 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result before remove, got %d", len(results))
	}

	// Remove
	if err := idx.Remove("blog", "doc1"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify removed
	results, _ = idx.Search("blog", "golang", 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results after remove, got %d", len(results))
	}
}

func TestFTSRemoveNonexistent(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	// Removing a doc that was never indexed should not error
	if err := idx.Remove("blog", "nonexistent"); err != nil {
		t.Fatalf("Remove of nonexistent doc failed: %v", err)
	}
}

func TestFTSIndexEmptyContent(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	// Empty content yields no terms, should return nil
	err := idx.Index("blog", "doc1", "")
	if err != nil {
		t.Fatalf("Index of empty content should not error: %v", err)
	}

	// All stop words content
	err = idx.Index("blog", "doc2", "the and or but")
	if err != nil {
		t.Fatalf("Index of stop-words-only content should not error: %v", err)
	}
}

func TestFTSSearchResultMatchedTerms(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	if err := idx.Index("col", "doc1", "golang programming language tutorial advanced"); err != nil {
		t.Fatalf("Index: %v", err)
	}

	results, err := idx.Search("col", "golang tutorial", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Matched terms should include "golang" and "tutorial"
	matched := make(map[string]bool)
	for _, term := range results[0].MatchedTerms {
		matched[term] = true
	}
	if !matched["golang"] {
		t.Error("expected 'golang' in matched terms")
	}
	if !matched["tutorial"] {
		t.Error("expected 'tutorial' in matched terms")
	}
}

func TestFTSSearchScore(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	_ = idx.Index("col", "doc1", "golang programming") // matches 1 of 2 query terms
	_ = idx.Index("col", "doc2", "golang tutorial")    // matches 2 of 2 query terms

	results, err := idx.Search("col", "golang tutorial", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// doc2 should score higher (matches both terms)
	scoreDoc1, scoreDoc2 := 0.0, 0.0
	for _, r := range results {
		if r.DocID == "doc1" {
			scoreDoc1 = r.Score
		}
		if r.DocID == "doc2" {
			scoreDoc2 = r.Score
		}
	}
	if scoreDoc2 <= scoreDoc1 {
		t.Errorf("expected doc2 to score higher than doc1: doc2=%f, doc1=%f", scoreDoc2, scoreDoc1)
	}
}

func TestFTSKeyFormat(t *testing.T) {
	key := ftsKey("blog", "golang", "doc1")
	expected := "fts|blog|golang|doc1"
	if string(key) != expected {
		t.Errorf("expected %q, got %q", expected, string(key))
	}
}

func TestFTSRevKeyFormat(t *testing.T) {
	key := ftsRevKey("blog", "doc1")
	expected := "ftsrev|blog|doc1"
	if string(key) != expected {
		t.Errorf("expected %q, got %q", expected, string(key))
	}
}

func TestFTSSetBinlog(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	// SetBinlog with nil should not panic
	idx.SetBinlog(nil)
	if idx.binlog != nil {
		t.Error("expected binlog to be nil")
	}
}

func TestFTSSearchZeroLimit(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	for i := 0; i < 5; i++ {
		_ = idx.Index("col", strings.Repeat("d", i+2), "common term repeated")
	}

	// limit=0 should return all results (no truncation)
	results, err := idx.Search("col", "common term", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 5 {
		t.Errorf("expected 5 results with limit=0, got %d", len(results))
	}
}

func TestFTSTokenizeUnicode(t *testing.T) {
	idx, cleanup := newTestFTSIndex(t)
	defer cleanup()

	terms := idx.Tokenize("cafe resume uber")
	if _, ok := terms["cafe"]; !ok {
		t.Error("expected 'cafe' in terms")
	}
	if _, ok := terms["resume"]; !ok {
		t.Error("expected 'resume' in terms")
	}
	if _, ok := terms["uber"]; !ok {
		t.Error("expected 'uber' in terms")
	}
}
