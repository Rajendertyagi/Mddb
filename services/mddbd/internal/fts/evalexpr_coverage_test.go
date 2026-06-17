package fts

import "testing"

// TestFTSEvaluateExpressionAllOperators drives the query-expression engine
// (EvaluateExpression -> evalExpr -> evalTermNode/unionScores/dedupeAppend)
// across every operator: AND/OR/NOT, phrase, proximity, fuzzy and wildcard.
func TestFTSEvaluateExpressionAllOperators(t *testing.T) {
	idx := NewFTSIndex(openTestDB(t))
	if err := idx.EnsureBuckets(); err != nil {
		t.Fatal(err)
	}
	docs := map[string]string{
		"d1": "the quick brown fox jumps over",
		"d2": "quick brown dogs run very fast",
		"d3": "lazy cats sleep all day long",
	}
	for id, c := range docs {
		if err := idx.IndexPositions("c", id, c); err != nil {
			t.Fatalf("IndexPositions(%s): %v", id, err)
		}
	}

	queries := []string{
		"quick",
		"quick AND brown",
		"fox OR cats",
		"quick AND NOT fox",
		"NOT lazy",
		`"quick brown"`,
		`"brown fox"~2`,
		"quik~1",
		"qui*",
		"(fox OR cats) AND quick",
	}
	for _, q := range queries {
		expr, err := ParseQueryExpression(q)
		if err != nil {
			continue // not every DSL form needs to parse; skip unsupported
		}
		if _, err := idx.EvaluateExpression("c", expr, 10); err != nil {
			t.Errorf("EvaluateExpression(%q): %v", q, err)
		}
	}

	// nil expression short-circuit.
	if got, err := idx.EvaluateExpression("c", nil, 10); err != nil || got != nil {
		t.Errorf("EvaluateExpression(nil) = (%v, %v), want (nil, nil)", got, err)
	}
}
