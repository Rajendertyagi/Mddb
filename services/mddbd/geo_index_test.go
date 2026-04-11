package main

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

// --- haversine accuracy ---

func TestHaversineKnownDistances(t *testing.T) {
	tests := []struct {
		name                   string
		lat1, lng1, lat2, lng2 float64
		wantKm                 float64 // reference value
		tolPct                 float64 // tolerance in percent
	}{
		{"London-Paris", 51.5074, -0.1278, 48.8566, 2.3522, 344, 1.0},
		{"NYC-LA", 40.7128, -74.0060, 34.0522, -118.2437, 3936, 1.0},
		{"Antipodes", 0, 0, 0, 180, 20015, 1.0},
		{"Same point", 52.52, 13.405, 52.52, 13.405, 0, 0.001},
		{"North pole to equator along 0°", 90, 0, 0, 0, 10007, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := haversineMeters(tt.lat1, tt.lng1, tt.lat2, tt.lng2) / 1000
			if tt.wantKm == 0 {
				if got > 0.001 {
					t.Errorf("same-point distance = %f km, want 0", got)
				}
				return
			}
			diff := math.Abs(got-tt.wantKm) / tt.wantKm * 100
			if diff > tt.tolPct {
				t.Errorf("haversine(%s) = %.2f km, want %.2f km (%.2f%% off, tol %.2f%%)",
					tt.name, got, tt.wantKm, diff, tt.tolPct)
			}
		})
	}
}

func TestValidLatLng(t *testing.T) {
	cases := []struct {
		lat, lng float64
		ok       bool
	}{
		{0, 0, true},
		{90, 180, true},
		{-90, -180, true},
		{90.0001, 0, false},
		{0, 180.0001, false},
		{math.NaN(), 0, false},
		{0, math.Inf(1), false},
	}
	for _, c := range cases {
		if got := validLatLng(c.lat, c.lng); got != c.ok {
			t.Errorf("validLatLng(%v, %v) = %v, want %v", c.lat, c.lng, got, c.ok)
		}
	}
}

// --- extractLatLng ---

func TestExtractLatLng(t *testing.T) {
	cases := []struct {
		name    string
		meta    map[string][]string
		wantOK  bool
		wantLat float64
		wantLng float64
	}{
		{"both present", map[string][]string{"geo_lat": {"52.52"}, "geo_lng": {"13.405"}}, true, 52.52, 13.405},
		{"missing lng", map[string][]string{"geo_lat": {"52.52"}}, false, 0, 0},
		{"missing lat", map[string][]string{"geo_lng": {"13.405"}}, false, 0, 0},
		{"unparseable", map[string][]string{"geo_lat": {"foo"}, "geo_lng": {"bar"}}, false, 0, 0},
		{"out of range", map[string][]string{"geo_lat": {"100"}, "geo_lng": {"0"}}, false, 0, 0},
		{"empty meta", map[string][]string{}, false, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lat, lng, ok := extractLatLng(c.meta)
			if ok != c.wantOK || lat != c.wantLat || lng != c.wantLng {
				t.Errorf("got (%v, %v, %v), want (%v, %v, %v)", lat, lng, ok, c.wantLat, c.wantLng, c.wantOK)
			}
		})
	}
}

// --- R-tree radius search ---

func TestGeoIndexRadiusSearch(t *testing.T) {
	idx := NewGeoIndex()
	// Seed 1000 random points in a ~100 km × 100 km box around Berlin.
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // G404: math/rand fine for test data
	const center_lat = 52.52
	const center_lng = 13.405
	for i := 0; i < 1000; i++ {
		// ±0.5° ≈ ±55 km latitude
		lat := center_lat + (rng.Float64() - 0.5)
		lng := center_lng + (rng.Float64() - 0.5)
		idx.Add("venues", fmt.Sprintf("doc-%d", i), lat, lng)
	}
	idx.SetReady()

	// Query 10 km radius from center.
	got := idx.Search("venues", center_lat, center_lng, 10000, 1000000, nil)
	if len(got) == 0 {
		t.Fatal("expected at least some points within 10 km")
	}
	// No false positives: every returned doc must actually be within 10 km.
	for _, r := range got {
		if r.DistanceMeters > 10000 {
			t.Errorf("false positive: doc=%s distance=%.1f m > 10000", r.DocID, r.DistanceMeters)
		}
	}
	// Results sorted by ascending distance.
	for i := 1; i < len(got); i++ {
		if got[i].DistanceMeters < got[i-1].DistanceMeters {
			t.Errorf("results not sorted ascending at index %d", i)
		}
	}

	// No false negatives: brute-force reference must match.
	brute := map[string]float64{}
	idx.mu.RLock()
	for docID, p := range idx.collections["venues"].points {
		d := haversineMeters(center_lat, center_lng, p.lat, p.lng)
		if d <= 10000 {
			brute[docID] = d
		}
	}
	idx.mu.RUnlock()
	if len(got) != len(brute) {
		t.Errorf("brute-force has %d matches, R-tree returned %d", len(brute), len(got))
	}
}

func TestGeoIndexTopKTruncation(t *testing.T) {
	idx := NewGeoIndex()
	idx.Add("c", "a", 0, 0)
	idx.Add("c", "b", 0, 0.001)
	idx.Add("c", "c", 0, 0.002)
	got := idx.Search("c", 0, 0, 1000, 2, nil)
	if len(got) != 2 {
		t.Errorf("topK=2 returned %d results", len(got))
	}
	if got[0].DocID != "a" {
		t.Errorf("closest should be 'a', got %s", got[0].DocID)
	}
}

func TestGeoIndexRadiusZeroRejected(t *testing.T) {
	idx := NewGeoIndex()
	idx.Add("c", "a", 0, 0)
	got := idx.Search("c", 0, 0, 0, 10, nil)
	if got != nil {
		t.Errorf("radius 0 should return nil, got %v", got)
	}
}

func TestGeoIndexFilterAllowed(t *testing.T) {
	idx := NewGeoIndex()
	idx.Add("c", "a", 0, 0)
	idx.Add("c", "b", 0, 0.0001)
	idx.Add("c", "c", 0, 0.0002)

	allowed := map[string]struct{}{"b": {}}
	got := idx.Search("c", 0, 0, 10000, 10, allowed)
	if len(got) != 1 || got[0].DocID != "b" {
		t.Errorf("allowed filter failed: got %v", got)
	}
}

// --- R-tree bbox (Within) search ---

func TestGeoIndexWithin(t *testing.T) {
	idx := NewGeoIndex()
	rng := rand.New(rand.NewSource(99)) //nolint:gosec // G404
	for i := 0; i < 500; i++ {
		lat := rng.Float64()*180 - 90
		lng := rng.Float64()*360 - 180
		idx.Add("c", fmt.Sprintf("doc-%d", i), lat, lng)
	}
	// Query a bbox. Brute-force the same bbox and compare sets.
	const minLat, maxLat, minLng, maxLng = 40.0, 50.0, -10.0, 10.0
	got := idx.Within("c", minLat, maxLat, minLng, maxLng, nil)
	gotSet := map[string]bool{}
	for _, r := range got {
		gotSet[r.DocID] = true
	}
	idx.mu.RLock()
	for docID, p := range idx.collections["c"].points {
		inside := p.lat >= minLat && p.lat <= maxLat && p.lng >= minLng && p.lng <= maxLng
		if inside != gotSet[docID] {
			t.Errorf("doc=%s inside=%v inSet=%v", docID, inside, gotSet[docID])
		}
	}
	idx.mu.RUnlock()
}

func TestGeoIndexWithinInvalidBbox(t *testing.T) {
	idx := NewGeoIndex()
	if got := idx.Within("c", 10, 5, 0, 0, nil); got != nil {
		t.Errorf("min>max lat should return nil, got %v", got)
	}
	if got := idx.Within("c", 0, 0, 10, 5, nil); got != nil {
		t.Errorf("min>max lng should return nil, got %v", got)
	}
}

// --- Add/Remove lifecycle ---

func TestGeoIndexAddRemove(t *testing.T) {
	idx := NewGeoIndex()
	idx.Add("c", "a", 10, 20)
	if idx.Len("c") != 1 {
		t.Errorf("Len after Add = %d, want 1", idx.Len("c"))
	}
	// Update same docID to new location.
	idx.Add("c", "a", 30, 40)
	if idx.Len("c") != 1 {
		t.Errorf("Len after update = %d, want 1 (not duplicated)", idx.Len("c"))
	}
	// Search at old location: should miss.
	old := idx.Search("c", 10, 20, 1000, 10, nil)
	if len(old) != 0 {
		t.Errorf("old location should be empty, got %v", old)
	}
	// Search at new location: should hit.
	newR := idx.Search("c", 30, 40, 1000, 10, nil)
	if len(newR) != 1 {
		t.Errorf("new location should have 1 hit, got %v", newR)
	}
	// Remove.
	idx.Remove("c", "a")
	if idx.Len("c") != 0 {
		t.Errorf("Len after Remove = %d, want 0", idx.Len("c"))
	}
}

func TestGeoIndexCollectionIsolation(t *testing.T) {
	idx := NewGeoIndex()
	idx.Add("a", "doc1", 0, 0)
	idx.Add("b", "doc1", 10, 10)
	// Same docID across collections — query in A must not see B's point.
	resA := idx.Search("a", 0, 0, 1000, 10, nil)
	if len(resA) != 1 || resA[0].DocID != "doc1" {
		t.Errorf("collection A search returned %v", resA)
	}
	// Distance from (0,0) to (10,10) should be large.
	resB := idx.Search("b", 0, 0, 1000, 10, nil)
	if len(resB) != 0 {
		t.Errorf("collection B search at distant point should be empty, got %v", resB)
	}
}

func TestGeoIndexAddFromMeta(t *testing.T) {
	idx := NewGeoIndex()
	// Explicit lat/lng wins.
	lat, lng, ok := idx.AddFromMeta("c", "d1", map[string][]string{
		"geo_lat": {"52.52"},
		"geo_lng": {"13.405"},
	})
	if !ok || lat != 52.52 || lng != 13.405 {
		t.Errorf("explicit add failed: (%v, %v, %v)", lat, lng, ok)
	}
	// Missing fields: no-op.
	_, _, ok = idx.AddFromMeta("c", "d2", map[string][]string{"other": {"x"}})
	if ok {
		t.Error("expected AddFromMeta without geo fields to no-op")
	}
	// Postcode fallback requires SetPostcodes.
	idx.SetPostcodes(NewPostcodeLookup())
	_, _, ok = idx.AddFromMeta("c", "d3", map[string][]string{
		"geo_postcode": {"SW1A1AA"},
		"geo_country":  {"GB"},
	})
	if ok {
		t.Error("empty postcode table should fall through")
	}
}

// --- Benchmarks documented in CHANGELOG ---

func BenchmarkGeoIndexSearch1K(b *testing.B) {
	benchmarkGeoSearch(b, 1000)
}

func BenchmarkGeoIndexSearch10K(b *testing.B) {
	benchmarkGeoSearch(b, 10000)
}

func BenchmarkGeoIndexSearch100K(b *testing.B) {
	benchmarkGeoSearch(b, 100000)
}

func TestGeoIndexReadyFlag(t *testing.T) {
	idx := NewGeoIndex()
	if idx.IsReady() {
		t.Error("fresh index should not be ready")
	}
	idx.SetReady()
	if !idx.IsReady() {
		t.Error("after SetReady IsReady should be true")
	}
}

func TestGeoIndexCollectionsAndLen(t *testing.T) {
	idx := NewGeoIndex()
	idx.Add("b", "1", 0, 0)
	idx.Add("a", "1", 0, 0)
	idx.Add("a", "2", 0, 1)
	// Snapshot returned sorted.
	cols := idx.Collections()
	if len(cols) != 2 || cols[0] != "a" || cols[1] != "b" {
		t.Errorf("Collections() = %v, want [a b]", cols)
	}
	if idx.Len("a") != 2 {
		t.Errorf("Len(a)=%d, want 2", idx.Len("a"))
	}
	if idx.Len("missing") != 0 {
		t.Errorf("Len(missing)=%d, want 0", idx.Len("missing"))
	}
}

func TestGeoIndexClearAndLastRebuild(t *testing.T) {
	idx := NewGeoIndex()
	idx.Add("c", "a", 0, 0)
	idx.markRebuilt("c")
	if idx.LastRebuild("c").IsZero() {
		t.Error("LastRebuild should be non-zero after markRebuilt")
	}
	idx.Clear("c")
	if idx.Len("c") != 0 {
		t.Errorf("Len(c) after Clear=%d, want 0", idx.Len("c"))
	}
	if !idx.LastRebuild("c").IsZero() {
		t.Error("LastRebuild should reset after Clear")
	}
}

func TestGeoIndexRemoveMissingNoop(t *testing.T) {
	idx := NewGeoIndex()
	// Remove from empty collection.
	idx.Remove("nope", "nope")
	// Remove a non-existent doc from an existing collection.
	idx.Add("c", "a", 0, 0)
	idx.Remove("c", "missing")
	if idx.Len("c") != 1 {
		t.Errorf("Len=%d, want 1", idx.Len("c"))
	}
}

func TestGeoIndexAddInvalidLatLng(t *testing.T) {
	idx := NewGeoIndex()
	idx.Add("c", "a", 100, 0) // out of range lat
	if idx.Len("c") != 0 {
		t.Errorf("invalid Add should be no-op, got Len=%d", idx.Len("c"))
	}
}

func TestGeoIndexSearchInvalidInputs(t *testing.T) {
	idx := NewGeoIndex()
	idx.Add("c", "a", 0, 0)
	if got := idx.Search("c", 999, 0, 1000, 10, nil); got != nil {
		t.Error("invalid lat should return nil")
	}
	if got := idx.Search("c", 0, 0, -1, 10, nil); got != nil {
		t.Error("negative radius should return nil")
	}
	if got := idx.Search("c", 0, 0, maxGeoRadiusM+1, 10, nil); got != nil {
		t.Error("radius above cap should return nil")
	}
	if got := idx.Search("missing", 0, 0, 1000, 10, nil); got != nil {
		t.Error("missing collection should return nil")
	}
}

func benchmarkGeoSearch(b *testing.B, n int) {
	idx := NewGeoIndex()
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // G404
	for i := 0; i < n; i++ {
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
