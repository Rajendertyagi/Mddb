package main

import (
	"math"
	"testing"
)

// GO-013: throughput / SVG-coordinate math must never produce Inf or NaN, even
// for a zero elapsed interval or empty (maxY=0) data.

func TestPerSecond(t *testing.T) {
	if got := perSecond(100, 2); got != 50 {
		t.Errorf("perSecond(100,2) = %v, want 50", got)
	}
	for _, secs := range []float64{0, -1} {
		if got := perSecond(100, secs); got != 0 || math.IsInf(got, 0) {
			t.Errorf("perSecond(100,%v) = %v, want 0 (no Inf)", secs, got)
		}
	}
}

func TestSvgCoordsGuardZeroMaxY(t *testing.T) {
	checks := []struct {
		name string
		fn   func(val, maxY float64) float64
		want float64
	}{
		{"barHeight", barHeight, 0},
		{"barY", barY, 300},
		{"lineY", lineY, 320},
	}
	for _, c := range checks {
		got := c.fn(123, 0)
		if got != c.want || math.IsInf(got, 0) || math.IsNaN(got) {
			t.Errorf("%s(123, 0) = %v, want %v (no Inf/NaN)", c.name, got, c.want)
		}
	}
}

func TestSvgCoordsNormal(t *testing.T) {
	if got := barHeight(50, 100); got != 150 {
		t.Errorf("barHeight(50,100) = %v, want 150", got)
	}
}
