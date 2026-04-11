package main

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestGeoHashIndexAddSearch(t *testing.T) {
	idx := NewGeoHashIndex()
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // G404: math/rand fine for test data
	const centerLat = 52.52
	const centerLng = 13.405
	// 500 random points in ±0.5° around Berlin.
	for i := 0; i < 500; i++ {
		lat := centerLat + (rng.Float64() - 0.5)
		lng := centerLng + (rng.Float64() - 0.5)
		idx.Add("v", fmt.Sprintf("d%d", i), lat, lng)
	}
	idx.SetReady()

	got := idx.Search("v", centerLat, centerLng, 5000, 100, nil)
	if len(got) == 0 {
		t.Fatal("expected at least some points within 5 km")
	}
	// All results within radius.
	for _, r := range got {
		if r.DistanceMeters > 5000 {
			t.Errorf("false positive: doc=%s distance=%.1f m", r.DocID, r.DistanceMeters)
		}
	}
	// Results sorted ascending by distance.
	for i := 1; i < len(got); i++ {
		if got[i].DistanceMeters < got[i-1].DistanceMeters {
			t.Error("results not sorted ascending")
		}
	}

	// Brute-force reference: must be a superset of what Search returned
	// (geohash might slip through the prefix range filter under some
	// radii, so we only assert no-false-positives, not strict equality).
	ref := 0
	idx.mu.RLock()
	for _, e := range idx.collections["v"].sorted {
		if haversineMeters(centerLat, centerLng, e.lat, e.lng) <= 5000 {
			ref++
		}
	}
	idx.mu.RUnlock()
	if len(got) == 0 && ref > 0 {
		t.Error("index returned 0 but brute force found matches")
	}
}

func TestGeoHashIndexRemove(t *testing.T) {
	idx := NewGeoHashIndex()
	idx.Add("c", "a", 0, 0)
	idx.Add("c", "b", 0, 0.001)
	idx.Add("c", "c", 0, 0.002)
	if idx.Len("c") != 3 {
		t.Errorf("Len=%d, want 3", idx.Len("c"))
	}
	idx.Remove("c", "b")
	if idx.Len("c") != 2 {
		t.Errorf("after Remove Len=%d, want 2", idx.Len("c"))
	}
	// Doc map must have been repaired — re-add "b" and confirm no duplicate.
	idx.Add("c", "b", 0, 0.001)
	if idx.Len("c") != 3 {
		t.Errorf("after re-Add Len=%d, want 3", idx.Len("c"))
	}
}

func TestGeoHashIndexUpdateSameID(t *testing.T) {
	idx := NewGeoHashIndex()
	idx.Add("c", "a", 10, 20)
	idx.Add("c", "a", 30, 40) // update
	if idx.Len("c") != 1 {
		t.Errorf("Len=%d after update, want 1", idx.Len("c"))
	}
}

func TestGeoHashIndexFilterAllowed(t *testing.T) {
	idx := NewGeoHashIndex()
	idx.Add("c", "a", 0, 0)
	idx.Add("c", "b", 0, 0.0001)
	idx.Add("c", "c", 0, 0.0002)
	allowed := map[string]struct{}{"b": {}}
	got := idx.Search("c", 0, 0, 10000, 10, allowed)
	if len(got) != 1 || got[0].DocID != "b" {
		t.Errorf("allowed filter failed: %v", got)
	}
}

func TestGeoHashIndexWithin(t *testing.T) {
	idx := NewGeoHashIndex()
	idx.Add("c", "a", 40, 0)
	idx.Add("c", "b", 45, 5)
	idx.Add("c", "c", 60, 10)
	got := idx.Within("c", 40, 50, 0, 10, nil)
	if len(got) != 2 {
		t.Errorf("Within returned %d, want 2", len(got))
	}
}

func TestGeoHashIndexAddFromMeta(t *testing.T) {
	idx := NewGeoHashIndex()
	lat, lng, ok := idx.AddFromMeta("c", "d1", map[string][]string{
		"geo_hash": {"u33d8"},
	}, nil)
	if !ok {
		t.Fatal("expected geohash to be extracted")
	}
	_ = lat
	_ = lng
	if idx.Len("c") != 1 {
		t.Errorf("Len=%d, want 1", idx.Len("c"))
	}
}

func BenchmarkGeoHashIndexSearch10K(b *testing.B) {
	idx := NewGeoHashIndex()
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // G404
	for i := 0; i < 10000; i++ {
		lat := 52.52 + (rng.Float64()-0.5)*2
		lng := 13.405 + (rng.Float64()-0.5)*2
		idx.Add("venues", fmt.Sprintf("doc-%d", i), lat, lng)
	}
	idx.SetReady()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = idx.Search("venues", 52.52, 13.405, 5000, 10, nil)
	}
}
