package vector

import (
	"testing"
)

func mmrTestVectors() map[string][]float32 {
	return map[string][]float32{
		"a1": {1, 0, 0}, // near-duplicate of a2
		"a2": {0.99, 0.01, 0},
		"b":  {0, 1, 0}, // orthogonal — diverse
		"c":  {0, 0, 1}, // orthogonal — diverse
	}
}

func mmrTestResults() []VectorResult {
	// Relevance order: a1 > a2 > b > c
	return []VectorResult{
		{DocID: "a1", Score: 0.95},
		{DocID: "a2", Score: 0.94},
		{DocID: "b", Score: 0.80},
		{DocID: "c", Score: 0.70},
	}
}

func TestMMRRerankDiversifies(t *testing.T) {
	vecs := mmrTestVectors()
	getVec := func(id string) []float32 { return vecs[id] }

	out := MMRRerank(mmrTestResults(), 0.5, 3, getVec)
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	if out[0].DocID != "a1" {
		t.Errorf("first pick must be the most relevant, got %s", out[0].DocID)
	}
	// The near-duplicate a2 must NOT be second — a diverse result wins.
	if out[1].DocID == "a2" {
		t.Errorf("near-duplicate ranked second despite MMR")
	}
	for _, r := range out[1:] {
		if r.DocID == "b" || r.DocID == "c" {
			return
		}
	}
	t.Errorf("no diverse result selected: %+v", out)
}

func TestMMRRerankLambdaOnePreservesOrder(t *testing.T) {
	vecs := mmrTestVectors()
	out := MMRRerank(mmrTestResults(), 1.0, 0, func(id string) []float32 { return vecs[id] })
	want := []string{"a1", "a2", "b", "c"}
	for i, id := range want {
		if out[i].DocID != id {
			t.Fatalf("lambda=1.0 changed order: pos %d = %s, want %s", i, out[i].DocID, id)
		}
	}
}

func TestMMRRerankKLimit(t *testing.T) {
	vecs := mmrTestVectors()
	out := MMRRerank(mmrTestResults(), 0.5, 2, func(id string) []float32 { return vecs[id] })
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	// k larger than input returns everything
	out = MMRRerank(mmrTestResults(), 0.5, 99, func(id string) []float32 { return vecs[id] })
	if len(out) != 4 {
		t.Fatalf("len = %d, want 4", len(out))
	}
}

func TestMMRRerankMissingVectors(t *testing.T) {
	// Vectors unavailable: no diversity signal, but no crash and relevance
	// ordering is preserved for the top pick.
	out := MMRRerank(mmrTestResults(), 0.5, 3, func(string) []float32 { return nil })
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	if out[0].DocID != "a1" {
		t.Errorf("first pick = %s, want a1", out[0].DocID)
	}
}

func TestMMRRerankEmptyAndSingle(t *testing.T) {
	if out := MMRRerank(nil, 0.5, 5, func(string) []float32 { return nil }); len(out) != 0 {
		t.Errorf("nil input should return empty")
	}
	single := []VectorResult{{DocID: "x", Score: 1}}
	if out := MMRRerank(single, 0.5, 5, func(string) []float32 { return nil }); len(out) != 1 {
		t.Errorf("single input should pass through")
	}
}
