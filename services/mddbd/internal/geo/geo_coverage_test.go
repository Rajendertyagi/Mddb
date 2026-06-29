package geo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGeoPolygonSearchAndValidate(t *testing.T) {
	gi := NewGeoIndex()
	gi.Add("c", "d1", 0.0, 0.0)
	gi.Add("c", "d2", 10.0, 10.0)
	gi.Add("c", "d3", -10.0, -10.0)

	// A square covering [-50,50] in both dimensions contains all three points
	// regardless of the lat/lng coordinate order convention.
	ring := [][][]float64{{{-50, -50}, {50, -50}, {50, 50}, {-50, 50}, {-50, -50}}}
	if res := gi.SearchPolygon("c", ring, nil); len(res) != 3 {
		t.Fatalf("SearchPolygon: expected 3 results, got %d", len(res))
	}
	if mres := gi.SearchMultiPolygon("c", [][][][]float64{ring}, nil); len(mres) != 3 {
		t.Fatalf("SearchMultiPolygon: expected 3 results, got %d", len(mres))
	}

	if err := ValidatePolygon(&GeoJSONPolygon{Type: "Polygon", Coordinates: ring}); err != nil {
		t.Errorf("ValidatePolygon(good): %v", err)
	}
	if err := ValidatePolygon(&GeoJSONPolygon{Type: "Polygon"}); err == nil {
		t.Error("ValidatePolygon(empty) should error")
	}
	if err := ValidateMultiPolygon(&GeoJSONMultiPolygon{Type: "MultiPolygon", Coordinates: [][][][]float64{ring}}); err != nil {
		t.Errorf("ValidateMultiPolygon(good): %v", err)
	}
	if err := ValidateMultiPolygon(&GeoJSONMultiPolygon{Type: "MultiPolygon"}); err == nil {
		t.Error("ValidateMultiPolygon(empty) should error")
	}
}

func TestGeoIndexReadiness(t *testing.T) {
	gi := NewGeoIndex()
	_ = gi.IsReady()
	ghi := NewGeoHashIndex()
	_ = ghi.IsReady()
	ghi.Add("c", "a", 52.5, 13.4)
	ghi.Add("c", "b", 52.6, 13.5)
	if got := ghi.Search("c", 52.5, 13.4, 50_000, 10, nil); len(got) == 0 {
		t.Error("expected geohash search to return nearby points")
	}
}

func TestPostcodeLoadCountry(t *testing.T) {
	csvPath := filepath.Join(t.TempDir(), "de.csv")
	if err := os.WriteFile(csvPath, []byte("10115,52.532,13.388\n80331,48.137,11.575\n"), 0600); err != nil {
		t.Fatal(err)
	}
	pl := NewPostcodeLookup()
	n, err := pl.LoadCountry("DE", csvPath)
	if err != nil {
		t.Fatalf("LoadCountry: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 postcodes loaded, got %d", n)
	}
}

func TestGeoIndexWithinAndPolygonValidationEdges(t *testing.T) {
	gi := NewGeoIndex()
	gi.Add("c", "d1", 0.0, 0.0)
	gi.Add("c", "d2", 10.0, 10.0)
	gi.Add("c", "d3", 80.0, 80.0)

	// Bounding-box query: only d1 and d2 fall inside [-20,20]x[-20,20].
	if got := gi.Within("c", -20, 20, -20, 20, nil); len(got) != 2 {
		t.Fatalf("Within: expected 2 results, got %d", len(got))
	}
	// Invalid box returns nil.
	if got := gi.Within("c", 20, -20, 0, 0, nil); got != nil {
		t.Errorf("Within(inverted box) = %v, want nil", got)
	}

	// allowedDocIDs filter narrows the result; an unknown collection yields nil.
	if got := gi.Within("c", -20, 20, -20, 20, map[string]struct{}{"d1": {}}); len(got) != 1 {
		t.Errorf("Within(filtered) = %d results, want 1", len(got))
	}
	if got := gi.Within("nope", -20, 20, -20, 20, nil); got != nil {
		t.Errorf("Within(unknown collection) = %v, want nil", got)
	}

	// ValidatePolygon error branches.
	if err := ValidatePolygon(nil); err == nil {
		t.Error("ValidatePolygon(nil) should error")
	}
	if err := ValidatePolygon(&GeoJSONPolygon{Type: "Point", Coordinates: [][][]float64{{{0, 0}, {1, 0}, {1, 1}}}}); err == nil {
		t.Error("ValidatePolygon(wrong type) should error")
	}
	if err := ValidatePolygon(&GeoJSONPolygon{Type: "Polygon", Coordinates: [][][]float64{{{0, 0}, {1, 1}}}}); err == nil {
		t.Error("ValidatePolygon(2-point ring) should error")
	}
	if err := ValidatePolygon(&GeoJSONPolygon{Type: "Polygon", Coordinates: [][][]float64{{{0}, {1}, {2}}}}); err == nil {
		t.Error("ValidatePolygon(short point) should error")
	}
	if err := ValidatePolygon(&GeoJSONPolygon{Type: "Polygon", Coordinates: [][][]float64{{{999, 999}, {1, 0}, {1, 1}}}}); err == nil {
		t.Error("ValidatePolygon(out-of-range lat/lng) should error")
	}
}
