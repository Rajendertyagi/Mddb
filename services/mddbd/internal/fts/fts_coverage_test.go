package fts

import (
	"mddb/internal/binlog"
	"path/filepath"
	"testing"
)

// TestFTSAccessorsAndManagers exercises the small accessor/setter surface and
// the StopWord/Synonym manager CRUD paths that the higher-level tests do not
// reach directly.
func TestFTSAccessorsAndManagers(t *testing.T) {
	db := openTestDB(t)
	idx := NewFTSIndex(db)
	if err := idx.EnsureBuckets(); err != nil {
		t.Fatal(err)
	}

	swm := NewStopWordManager(db)
	if err := swm.EnsureBucket(); err != nil {
		t.Fatal(err)
	}
	if err := swm.LoadAll(); err != nil {
		t.Fatal(err)
	}
	sm := NewSynonymManager(db)
	if err := sm.EnsureBucket(); err != nil {
		t.Fatal(err)
	}

	idx.SetSynonymManager(sm)
	idx.SetStopWordManager(swm)
	idx.SetStemmer(NewPorterStemmer())
	langReg := NewLangRegistry("en")
	RegisterDefaultLanguages(langReg)
	idx.SetLangRegistry(langReg)

	if idx.Stemmer() == nil {
		t.Error("Stemmer() returned nil after SetStemmer")
	}
	if idx.SynonymManager() == nil {
		t.Error("SynonymManager() returned nil after SetSynonymManager")
	}

	// StopWordManager CRUD + binlog setter.
	swm.SetBinlog(nil)
	if err := swm.Add("col", []string{"foo", "bar"}); err != nil {
		t.Fatal(err)
	}
	if !swm.IsStopWord("col", "foo") {
		t.Error("expected 'foo' to be a custom stop word after Add")
	}
	defaults, custom := swm.List("col")
	if len(defaults) == 0 {
		t.Error("expected non-empty default stop words")
	}
	if len(custom) != 2 {
		t.Errorf("expected 2 custom stop words, got %d", len(custom))
	}
	if err := swm.Delete("col", "foo"); err != nil {
		t.Fatal(err)
	}
	if swm.IsStopWord("col", "foo") {
		t.Error("'foo' should no longer be a stop word after Delete")
	}

	// SynonymManager binlog setter.
	sm.SetBinlog(nil)

	// Tokenization + positions removal paths.
	_ = idx.TokenizeQuery("col", "the quick brown fox")
	_ = idx.TokenizeQueryLang("col", "der schnelle fuchs", "de")
	if err := idx.IndexPositions("col", "d1", "hello world"); err != nil {
		t.Fatal(err)
	}
	if err := idx.RemovePositions("col", "d1"); err != nil {
		t.Fatal(err)
	}
}

// TestQueryExprNodeMarkers calls the unexported marker methods that satisfy the
// QueryExpr interface so they register as covered.
func TestQueryExprNodeMarkers(t *testing.T) {
	(&AndExpr{}).exprNode()
	(&OrExpr{}).exprNode()
	(&NotExpr{}).exprNode()
	(&TermExpr{}).exprNode()
	(&FuzzyExpr{}).exprNode()
	(&PhraseExpr{}).exprNode()
	(&ProximityExpr{}).exprNode()
	(&WildcardExpr{}).exprNode()
}

func TestStopWordManagerLoadAllWithData(t *testing.T) {
	db := openTestDB(t)
	swm := NewStopWordManager(db)
	if err := swm.EnsureBucket(); err != nil {
		t.Fatal(err)
	}
	if err := swm.Add("c", []string{"alpha", "beta"}); err != nil {
		t.Fatal(err)
	}
	// Fresh manager over the same DB must load the persisted custom words.
	swm2 := NewStopWordManager(db)
	if err := swm2.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if !swm2.IsStopWord("c", "alpha") {
		t.Error("expected 'alpha' loaded by LoadAll")
	}
}

func TestFTSIndexingWithBinlog(t *testing.T) {
	bl, err := binlog.NewBinlog("", binlog.BinlogConfig{Path: filepath.Join(t.TempDir(), "t.binlog")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bl.Close() }()

	idx := NewFTSIndex(openTestDB(t))
	_ = idx.EnsureBuckets()
	idx.SetStemmer(NewPorterStemmer())
	langReg := NewLangRegistry("en")
	RegisterDefaultLanguages(langReg)
	idx.SetLangRegistry(langReg)
	idx.SetBinlog(bl) // exercises the binlog-flush branch of every write path

	if err := idx.Index("c", "d1", "machine learning is great"); err != nil {
		t.Fatal(err)
	}
	if err := idx.IndexWithLang("c", "d2", "deep learning networks", "en"); err != nil {
		t.Fatal(err)
	}
	if err := idx.IndexFields("c", "d3", map[string]string{"title": "neural networks"}); err != nil {
		t.Fatal(err)
	}
	if err := idx.IndexFieldsWithLang("c", "d4", map[string]string{"title": "tiefes lernen"}, "de"); err != nil {
		t.Fatal(err)
	}
	if err := idx.IndexPositions("c", "d1", "machine learning is great"); err != nil {
		t.Fatal(err)
	}
	if err := idx.IndexPositionsWithLang("c", "d2", "deep learning networks", "en"); err != nil {
		t.Fatal(err)
	}
	if err := idx.Remove("c", "d3"); err != nil {
		t.Fatal(err)
	}

	for _, q := range []string{"learning AND networks", "machine OR deep", "learning NOT machine", "\"deep learning\"", "learn*"} {
		if _, err := idx.SearchBoolean("c", ParseAdvancedQuery(q), 10); err != nil {
			t.Fatalf("SearchBoolean(%q): %v", q, err)
		}
	}
}

func TestTokenizeSynonymsAndProximity(t *testing.T) {
	db := openTestDB(t)
	idx := NewFTSIndex(db)
	_ = idx.EnsureBuckets()
	idx.SetStemmer(NewPorterStemmer())
	langReg := NewLangRegistry("en")
	RegisterDefaultLanguages(langReg)
	idx.SetLangRegistry(langReg)

	sm := NewSynonymManager(db)
	_ = sm.EnsureBucket()
	if err := sm.Set("c", "fast", []string{"quick", "rapid"}); err != nil {
		t.Fatal(err)
	}
	idx.SetSynonymManager(sm)

	// Synonym-expansion branch of the query tokenizers.
	_ = idx.TokenizeQuery("c", "fast machine")
	_ = idx.TokenizeQueryLang("c", "fast machine", "en")

	// Position-based search paths (SearchProximity -> findMinSpan, SearchPhrase).
	_ = idx.IndexPositions("c", "d1", "the quick brown fox jumps over the lazy dog")
	_ = idx.IndexPositions("c", "d2", "quick fox and lazy dog")
	if _, err := idx.SearchProximity("c", "quick dog", 5, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.SearchProximity("c", "quick fox", 2, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.SearchPhrase("c", "brown fox", 10); err != nil {
		t.Fatal(err)
	}
}

func TestProximityAndIntersectEdges(t *testing.T) {
	idx := NewFTSIndex(openTestDB(t))
	_ = idx.EnsureBuckets()
	idx.SetStemmer(NewPorterStemmer())

	_ = idx.IndexPositions("c", "d1", "alpha beta gamma delta epsilon")
	_ = idx.IndexPositions("c", "d2", "gamma alpha")
	// Proximity: in-order match, reversed, single term, too far, absent term.
	for _, c := range []struct {
		phrase string
		dist   int
	}{{"alpha gamma", 5}, {"gamma alpha", 1}, {"alpha", 3}, {"alpha epsilon", 1}, {"absent word", 3}} {
		if _, err := idx.SearchProximity("c", c.phrase, c.dist, 10); err != nil {
			t.Fatalf("SearchProximity(%q): %v", c.phrase, err)
		}
	}

	_ = idx.Index("c", "d1", "alpha beta gamma delta epsilon")
	_ = idx.Index("c", "d2", "gamma alpha")
	// Boolean intersect: AND overlap, AND no overlap, OR with a miss, triple AND.
	for _, q := range []string{"alpha AND delta", "beta AND nothere", "alpha OR zzz", "alpha AND gamma AND delta"} {
		if _, err := idx.SearchBoolean("c", ParseAdvancedQuery(q), 10); err != nil {
			t.Fatalf("SearchBoolean(%q): %v", q, err)
		}
	}
}

func TestEvalNegationAndMultiTermProximity(t *testing.T) {
	s, cleanup := newQueryExprServer(t)
	defer cleanup()
	indexExprDoc(t, s, "c", "d1", "machine learning algorithms")
	indexExprDoc(t, s, "c", "d2", "deep learning networks here")

	// Negation combinations exercise intersectScores' leftNeg/rightNeg branches.
	negs := []QueryExpr{
		&AndExpr{Left: &NotExpr{Inner: &TermExpr{Term: "machine"}}, Right: &NotExpr{Inner: &TermExpr{Term: "deep"}}},
		&AndExpr{Left: &NotExpr{Inner: &TermExpr{Term: "machine"}}, Right: &TermExpr{Term: "learning"}},
		&AndExpr{Left: &TermExpr{Term: "learning"}, Right: &NotExpr{Inner: &TermExpr{Term: "machine"}}},
	}
	for i, e := range negs {
		if _, err := s.EvaluateExpression("c", e, 10); err != nil {
			t.Fatalf("neg %d: %v", i, err)
		}
	}

	// Three-term proximity exercises findMinSpan's multi-term span path.
	_ = s.IndexPositions("c", "d3", "alpha beta gamma delta")
	if _, err := s.SearchProximity("c", "alpha gamma delta", 6, 10); err != nil {
		t.Fatal(err)
	}
}
