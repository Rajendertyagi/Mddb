package geo

import "testing"

// TestGeoHashScanAllLockedFallback covers scanAllLocked, the brute-force radius
// fallback used when the coarse prefix degenerates to empty. It is unreachable
// through Search (the coarse prefix floors at one char), so it is exercised
// directly here against its documented contract.
func TestGeoHashScanAllLockedFallback(t *testing.T) {
	gi := NewGeoHashIndex()
	gi.Add("c", "near", 52.5, 13.4)  // Berlin
	gi.Add("c", "far", -33.8, 151.2) // Sydney
	c := gi.collections["c"]

	gi.mu.RLock()
	all := gi.scanAllLocked(c, 52.5, 13.4, 100_000, nil) // 100km around Berlin
	gi.mu.RUnlock()
	if len(all) != 1 || all[0].DocID != "near" {
		t.Fatalf("scanAllLocked radius filter = %+v, want only 'near'", all)
	}

	// allowedDocIDs filter excludes everything -> empty result.
	gi.mu.RLock()
	filtered := gi.scanAllLocked(c, 52.5, 13.4, 100_000, map[string]struct{}{"other": {}})
	gi.mu.RUnlock()
	if len(filtered) != 0 {
		t.Errorf("scanAllLocked with excluding filter = %+v, want empty", filtered)
	}
}

// TestGeoHashWithinFilterAndEdges covers the Within filter branch plus its
// invalid-bbox and missing-collection short-circuits.
func TestGeoHashWithinFilterAndEdges(t *testing.T) {
	gi := NewGeoHashIndex()
	gi.Add("c", "a", 10, 10)
	gi.Add("c", "b", 20, 20)

	// allowedDocIDs limits the result to "a".
	got := gi.Within("c", 0, 30, 0, 30, map[string]struct{}{"a": {}})
	if len(got) != 1 || got[0].DocID != "a" {
		t.Errorf("Within with filter = %+v, want only 'a'", got)
	}
	// Inverted bbox is rejected.
	if got := gi.Within("c", 30, 0, 0, 30, nil); got != nil {
		t.Errorf("Within with minLat>maxLat = %+v, want nil", got)
	}
	// Unknown collection yields nil.
	if got := gi.Within("missing", 0, 30, 0, 30, nil); got != nil {
		t.Errorf("Within on missing collection = %+v, want nil", got)
	}
}
