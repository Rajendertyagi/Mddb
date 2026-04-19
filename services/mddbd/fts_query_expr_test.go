package main

import (
	"testing"
)

// --- Tokenizer ---

func TestTokenize_Basics(t *testing.T) {
	cases := []struct {
		in  string
		out []tokenType
	}{
		{"rust", []tokenType{tokTerm, tokEOF}},
		{"rust AND performance", []tokenType{tokTerm, tokAnd, tokTerm, tokEOF}},
		{"a OR b", []tokenType{tokTerm, tokOr, tokTerm, tokEOF}},
		{"NOT spam", []tokenType{tokNot, tokTerm, tokEOF}},
		{"-spam", []tokenType{tokNot, tokTerm, tokEOF}},
		{"+must have", []tokenType{tokRequire, tokTerm, tokTerm, tokEOF}},
		{`"machine learning"`, []tokenType{tokPhrase, tokEOF}},
		{`"machine learning"~5`, []tokenType{tokProximity, tokEOF}},
		{"mark*", []tokenType{tokWildcard, tokEOF}},
		{"color~1", []tokenType{tokFuzzy, tokEOF}},
		{"(a OR b) AND c", []tokenType{tokLParen, tokTerm, tokOr, tokTerm, tokRParen, tokAnd, tokTerm, tokEOF}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := tokenize(tc.in)
			if len(got) != len(tc.out) {
				t.Fatalf("len(tokens)=%d want %d: %+v", len(got), len(tc.out), got)
			}
			for i, tok := range got {
				if tok.typ != tc.out[i] {
					t.Errorf("token %d: type=%d want %d (payload=%q)", i, tok.typ, tc.out[i], tok.s)
				}
			}
		})
	}
}

func TestTokenize_FuzzyAndProximityPayload(t *testing.T) {
	toks := tokenize("color~2")
	if len(toks) < 1 || toks[0].typ != tokFuzzy || toks[0].s != "color" || toks[0].n != 2 {
		t.Errorf("fuzzy payload mismatch: %+v", toks[0])
	}
	toks = tokenize(`"rust systems"~7`)
	if len(toks) < 1 || toks[0].typ != tokProximity || toks[0].s != "rust systems" || toks[0].n != 7 {
		t.Errorf("proximity payload mismatch: %+v", toks[0])
	}
}

// --- Parser ---

func TestParseQueryExpression_Precedence(t *testing.T) {
	// AND binds tighter than OR: `a AND b OR c` parses as (a AND b) OR c.
	cases := []struct {
		in     string
		wantS  string
	}{
		{"a", "a"},
		{"a AND b", "(a AND b)"},
		{"a OR b", "(a OR b)"},
		{"a b c", "((a AND b) AND c)"}, // implicit AND
		{"a AND b OR c", "((a AND b) OR c)"},
		{"a OR b AND c", "(a OR (b AND c))"},
		{"(a OR b) AND c", "((a OR b) AND c)"},
		{"a AND (b OR c) AND d", "((a AND (b OR c)) AND d)"},
		{"NOT a", "NOT a"},
		{"a AND NOT b", "(a AND NOT b)"},
		{"a AND -b", "(a AND NOT b)"},
		{`a AND "b c"`, `(a AND "b c")`},
		{`a AND "b c"~3`, `(a AND "b c"~3)`},
		{"a~1", "a~1"},
		{"mark*", "mark*"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			expr, err := ParseQueryExpression(tc.in)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			got := expr.String()
			if got != tc.wantS {
				t.Errorf("\n  got  %s\n  want %s", got, tc.wantS)
			}
		})
	}
}

func TestParseQueryExpression_Errors(t *testing.T) {
	cases := []string{
		"(",
		"a AND",
		"(a OR b",
		"a )",
	}
	for _, q := range cases {
		t.Run(q, func(t *testing.T) {
			_, err := ParseQueryExpression(q)
			if err == nil {
				t.Error("expected parse error")
			}
		})
	}
}

func TestParseQueryExpression_EmptyReturnsNil(t *testing.T) {
	expr, err := ParseQueryExpression("   ")
	if err != nil || expr != nil {
		t.Errorf("expected (nil, nil), got (%v, %v)", expr, err)
	}
}

func TestParseQueryExpression_FuzzyDistanceClamped(t *testing.T) {
	expr, err := ParseQueryExpression("term~9")
	if err != nil {
		t.Fatal(err)
	}
	f, ok := expr.(*FuzzyExpr)
	if !ok {
		t.Fatalf("expected FuzzyExpr, got %T", expr)
	}
	if f.Distance != 2 {
		t.Errorf("distance should clamp to 2, got %d", f.Distance)
	}
}

// --- Evaluator (integration over a real FTS index) ---

// newQueryExprServer prepares an FTSIndex so evaluator tests can seed a
// small corpus and exercise AND / OR / NOT / phrase semantics end-to-end.
func newQueryExprServer(t *testing.T) (*Server, func()) {
	t.Helper()
	s, cleanup := newTestServer(t)
	s.FTSIndex = NewFTSIndex(s.DB)
	if err := s.FTSIndex.EnsureBuckets(); err != nil {
		cleanup()
		t.Fatalf("ensure FTS buckets: %v", err)
	}
	return s, cleanup
}

// indexExprDoc wires both the flat and positional indices so phrase and
// proximity clauses in the evaluator have something to match against.
func indexExprDoc(t *testing.T, s *Server, collection, docID, content string) {
	t.Helper()
	if err := s.FTSIndex.Index(collection, docID, content); err != nil {
		t.Fatalf("index: %v", err)
	}
	if err := s.FTSIndex.IndexPositions(collection, docID, content); err != nil {
		t.Fatalf("index positions: %v", err)
	}
}

func TestEvaluateExpression_And(t *testing.T) {
	s, cleanup := newQueryExprServer(t)
	defer cleanup()
	indexExprDoc(t, s, "c", "a", "rust systems programming")
	indexExprDoc(t, s, "c", "b", "rust async runtime")
	indexExprDoc(t, s, "c", "c", "golang concurrency")

	expr, err := ParseQueryExpression("rust AND systems")
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.FTSIndex.EvaluateExpression("c", expr, 10)
	if err != nil {
		t.Fatal(err)
	}
	// Only doc "a" has both terms.
	if len(res) != 1 || res[0].DocID != "a" {
		t.Errorf("expected [a], got %v", docIDsFromFTS(res))
	}
}

func TestEvaluateExpression_Or(t *testing.T) {
	s, cleanup := newQueryExprServer(t)
	defer cleanup()
	indexExprDoc(t, s, "c", "a", "rust systems programming")
	indexExprDoc(t, s, "c", "b", "golang concurrency")
	indexExprDoc(t, s, "c", "c", "python async")

	expr, _ := ParseQueryExpression("rust OR golang")
	res, _ := s.FTSIndex.EvaluateExpression("c", expr, 10)
	ids := docIDsFromFTS(res)
	if len(ids) != 2 {
		t.Errorf("expected 2 docs, got %v", ids)
	}
}

func TestEvaluateExpression_Not(t *testing.T) {
	s, cleanup := newQueryExprServer(t)
	defer cleanup()
	indexExprDoc(t, s, "c", "a", "rust systems good")
	indexExprDoc(t, s, "c", "b", "rust spam bad")
	indexExprDoc(t, s, "c", "c", "rust perfect")

	expr, _ := ParseQueryExpression("rust AND NOT spam")
	res, _ := s.FTSIndex.EvaluateExpression("c", expr, 10)
	ids := docIDsFromFTS(res)
	// "a" and "c" match rust but not spam; "b" is excluded.
	if len(ids) != 2 {
		t.Errorf("expected 2 docs, got %v", ids)
	}
	for _, id := range ids {
		if id == "b" {
			t.Errorf("doc b should be excluded by NOT spam")
		}
	}
}

func TestEvaluateExpression_NestedGrouping(t *testing.T) {
	s, cleanup := newQueryExprServer(t)
	defer cleanup()
	indexExprDoc(t, s, "c", "a", "rust systems")
	indexExprDoc(t, s, "c", "b", "golang concurrency")
	indexExprDoc(t, s, "c", "c", "python async")

	// (rust OR golang) AND (systems OR concurrency)
	// Matches: a (rust + systems), b (golang + concurrency). Not c.
	expr, err := ParseQueryExpression("(rust OR golang) AND (systems OR concurrency)")
	if err != nil {
		t.Fatal(err)
	}
	res, _ := s.FTSIndex.EvaluateExpression("c", expr, 10)
	ids := docIDsFromFTS(res)
	if len(ids) != 2 {
		t.Errorf("expected 2 docs, got %v", ids)
	}
	for _, id := range ids {
		if id == "c" {
			t.Errorf("doc c should not match — has neither (rust|golang) nor (systems|concurrency)")
		}
	}
}

func TestEvaluateExpression_PhraseAtom(t *testing.T) {
	s, cleanup := newQueryExprServer(t)
	defer cleanup()
	indexExprDoc(t, s, "c", "a", "machine learning algorithms")
	indexExprDoc(t, s, "c", "b", "learning to use machine")

	expr, _ := ParseQueryExpression(`"machine learning"`)
	res, _ := s.FTSIndex.EvaluateExpression("c", expr, 10)
	ids := docIDsFromFTS(res)
	if len(ids) != 1 || ids[0] != "a" {
		t.Errorf("expected [a] only, got %v", ids)
	}
}

func TestEvaluateExpression_EmptyReturnsNothing(t *testing.T) {
	s, cleanup := newQueryExprServer(t)
	defer cleanup()
	res, err := s.FTSIndex.EvaluateExpression("c", nil, 10)
	if err != nil || res != nil {
		t.Errorf("nil AST should yield (nil, nil), got (%v, %v)", res, err)
	}
}

// docIDsFromFTS collects ids so assertions can eyeball the matched set.
func docIDsFromFTS(res []FTSResult) []string {
	out := make([]string, len(res))
	for i, r := range res {
		out[i] = r.DocID
	}
	return out
}
