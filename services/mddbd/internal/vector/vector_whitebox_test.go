package vector

import (
	"math"
	"testing"
)

func TestDotProductSimilarity(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{4, 5, 6}
	got := dotProductSimilarity(a, b)
	want := float32(32) // 1*4 + 2*5 + 3*6
	if math.Abs(float64(got-want)) > 0.01 {
		t.Errorf("got %f, want %f", got, want)
	}
	if dotProductSimilarity(nil, nil) != 0 {
		t.Error("nil should return 0")
	}
}
func TestEuclideanSimilarity(t *testing.T) {
	a := []float32{0, 0}
	b := []float32{0, 0}
	sim := euclideanSimilarity(a, b)
	if sim < 0.99 {
		t.Errorf("same point: got %f, want ~1", sim)
	}
	far := euclideanSimilarity([]float32{0, 0}, []float32{100, 100})
	if far > 0.1 {
		t.Errorf("far points: got %f, want <0.1", far)
	}
	if euclideanSimilarity(nil, nil) != 0 {
		t.Error("nil should return 0")
	}
}
func TestDotProductEmpty(t *testing.T) {
	if dotProductSimilarity([]float32{}, []float32{}) != 0 {
		t.Error("empty should be 0")
	}
}
func TestEuclideanMismatch(t *testing.T) {
	if euclideanSimilarity([]float32{1}, []float32{1, 2}) != 0 {
		t.Error("mismatched should be 0")
	}
}
