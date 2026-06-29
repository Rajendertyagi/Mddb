package fts

import "testing"

// TestQueryExprStringAndNode covers every query-AST node's String() formatter
// and the (otherwise-never-invoked) exprNode marker method.
func TestQueryExprStringAndNode(t *testing.T) {
	nodes := []QueryExpr{
		&TermExpr{Term: "a"},
		&FuzzyExpr{Term: "b", Distance: 2},
		&PhraseExpr{Phrase: "c d"},
		&ProximityExpr{Phrase: "e f", Distance: 3},
		&WildcardExpr{Pattern: "g*"},
		&NotExpr{Inner: &TermExpr{Term: "h"}},
		&AndExpr{Left: &TermExpr{Term: "i"}, Right: &TermExpr{Term: "j"}},
		&OrExpr{Left: &TermExpr{Term: "k"}, Right: &TermExpr{Term: "l"}},
	}
	for _, n := range nodes {
		if n.String() == "" {
			t.Errorf("%T.String() returned empty", n)
		}
		n.exprNode() // interface marker — call for coverage
	}
}
