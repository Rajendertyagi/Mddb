package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- Ray-cast unit tests ---

func TestPointInRing_Triangle(t *testing.T) {
	// Triangle in Berlin: corners around Brandenburg Gate, Potsdamer Platz,
	// Hauptbahnhof. Each as [lng, lat] (GeoJSON order).
	ring := [][]float64{
		{13.3777, 52.5163}, // Brandenburg Gate
		{13.3758, 52.5096}, // Potsdamer Platz
		{13.3692, 52.5250}, // Hauptbahnhof
		{13.3777, 52.5163}, // close
	}
	// Centroid-ish point that is obviously inside.
	if !pointInRing(52.5170, 13.3740, ring) {
		t.Error("expected point inside triangle")
	}
	// Far away point clearly outside.
	if pointInRing(48.8566, 2.3522, ring) {
		t.Error("Paris should be outside Berlin triangle")
	}
	// Just outside the bounding box but still within lat range.
	if pointInRing(52.5163, 13.50, ring) {
		t.Error("point east of ring should be outside")
	}
}

func TestPointInRing_DegenerateRejected(t *testing.T) {
	// Ring with only 2 points cannot enclose anything — must return false
	// rather than panic on the edge-wrapping index math.
	ring := [][]float64{{0, 0}, {1, 1}}
	if pointInRing(0.5, 0.5, ring) {
		t.Error("2-point ring cannot contain any point")
	}
}

func TestPointInPolygon_WithHole(t *testing.T) {
	// 10x10 square with a 4x4 hole centered inside.
	coords := [][][]float64{
		// outer
		{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}},
		// hole
		{{3, 3}, {7, 3}, {7, 7}, {3, 7}, {3, 3}},
	}
	// Point inside outer but outside hole: matches.
	if !pointInPolygon(1, 1, coords) {
		t.Error("(1,1) should be inside the square, outside the hole")
	}
	// Point inside hole: rejected.
	if pointInPolygon(5, 5, coords) {
		t.Error("(5,5) is inside the hole — must not match")
	}
	// Point outside outer: rejected.
	if pointInPolygon(20, 20, coords) {
		t.Error("(20,20) is outside — must not match")
	}
}

func TestPointInMultiPolygon_Union(t *testing.T) {
	// Two disjoint 1x1 squares.
	mp := [][][][]float64{
		{{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}}},
		{{{10, 10}, {11, 10}, {11, 11}, {10, 11}, {10, 10}}},
	}
	if !pointInMultiPolygon(0.5, 0.5, mp) {
		t.Error("(0.5,0.5) in first square")
	}
	if !pointInMultiPolygon(10.5, 10.5, mp) {
		t.Error("(10.5,10.5) in second square")
	}
	if pointInMultiPolygon(5, 5, mp) {
		t.Error("(5,5) is between the squares — no match")
	}
}

func TestPolygonBounds_OuterPlusHoles(t *testing.T) {
	coords := [][][]float64{
		{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}},
		{{3, 3}, {7, 3}, {7, 7}, {3, 7}, {3, 3}},
	}
	minLat, maxLat, minLng, maxLng, ok := polygonBounds(coords)
	if !ok {
		t.Fatal("expected ok")
	}
	if minLat != 0 || maxLat != 10 || minLng != 0 || maxLng != 10 {
		t.Errorf("bbox mismatch: got lat[%v,%v] lng[%v,%v]", minLat, maxLat, minLng, maxLng)
	}
}

func TestPolygonBounds_Empty(t *testing.T) {
	if _, _, _, _, ok := polygonBounds(nil); ok {
		t.Error("empty coords → ok should be false")
	}
}

// --- Validation ---

func TestValidatePolygon_RejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		p    *GeoJSONPolygon
	}{
		{"nil", nil},
		{"wrong type", &GeoJSONPolygon{Type: "Point", Coordinates: [][][]float64{{{0, 0}, {1, 0}, {0, 1}}}}},
		{"no rings", &GeoJSONPolygon{Type: "Polygon"}},
		{"too few points", &GeoJSONPolygon{Coordinates: [][][]float64{{{0, 0}, {1, 0}}}}},
		{"point missing lat", &GeoJSONPolygon{Coordinates: [][][]float64{{{0}, {1, 0}, {0, 1}}}}},
		{"lat out of range", &GeoJSONPolygon{Coordinates: [][][]float64{{{0, 100}, {1, 0}, {0, 1}}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := validatePolygon(c.p); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestValidatePolygon_AcceptsGood(t *testing.T) {
	p := &GeoJSONPolygon{
		Type: "Polygon",
		Coordinates: [][][]float64{
			{{13, 52}, {14, 52}, {14, 53}, {13, 53}, {13, 52}},
		},
	}
	if err := validatePolygon(p); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Type omitted is also allowed — handler fills it in from the endpoint path.
	p.Type = ""
	if err := validatePolygon(p); err != nil {
		t.Errorf("empty type should be accepted: %v", err)
	}
}

// --- HTTP handler + integration through R-tree ---

// setupPolygonCorpus seeds four points around Berlin for handler tests:
// two inside a central polygon, one outside, one filtered by metadata.
func setupPolygonCorpus(t *testing.T, s *Server) {
	t.Helper()
	seedGeoDocs(t, s, "v", [][2]float64{
		{52.520, 13.405}, // doc_0 — inside
		{52.516, 13.378}, // doc_1 — inside
		{48.857, 2.352},  // doc_2 — Paris, outside
		{52.521, 13.406}, // doc_3 — inside (will be filtered by metadata below)
	})
}

func TestHandleGeoPolygon_BasicInside(t *testing.T) {
	s, cleanup := newTestServerForGeo(t)
	defer cleanup()
	setupPolygonCorpus(t, s)

	// Berlin-center polygon covering the first two seeded docs.
	body := `{
		"collection": "v",
		"polygon": {
			"type": "Polygon",
			"coordinates": [[[13.36, 52.51], [13.42, 52.51], [13.42, 52.53], [13.36, 52.53], [13.36, 52.51]]]
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/geo-polygon", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	s.handleGeoPolygon(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp GeoPolygonResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Shape != "polygon" {
		t.Errorf("shape=%q, want \"polygon\"", resp.Shape)
	}
	// Three Berlin docs are inside, Paris is not.
	if resp.Total != 3 {
		t.Errorf("total=%d, want 3 (got docs=%v)", resp.Total, docIDs(resp.Results))
	}
}

func TestHandleGeoPolygon_RejectsBothShapes(t *testing.T) {
	s, cleanup := newTestServerForGeo(t)
	defer cleanup()

	// Supplying both polygon and multiPolygon is ambiguous — handler must reject.
	body := `{
		"collection": "v",
		"polygon": {"coordinates": [[[0,0],[1,0],[1,1],[0,0]]]},
		"multiPolygon": {"coordinates": [[[[0,0],[1,0],[1,1],[0,0]]]]}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/geo-polygon", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	s.handleGeoPolygon(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("both-shapes should 400; got %d", w.Code)
	}
}

func TestHandleGeoPolygon_RejectsNoShape(t *testing.T) {
	s, cleanup := newTestServerForGeo(t)
	defer cleanup()

	body := `{"collection": "v"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/geo-polygon", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	s.handleGeoPolygon(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("no-shape should 400; got %d", w.Code)
	}
}

func TestHandleGeoPolygon_MissingCollection(t *testing.T) {
	s, cleanup := newTestServerForGeo(t)
	defer cleanup()

	body := `{"polygon": {"coordinates": [[[0,0],[1,0],[1,1],[0,0]]]}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/geo-polygon", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	s.handleGeoPolygon(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing-collection should 400; got %d", w.Code)
	}
}

func TestHandleGeoPolygon_MultiPolygonUnion(t *testing.T) {
	s, cleanup := newTestServerForGeo(t)
	defer cleanup()
	setupPolygonCorpus(t, s)

	// Two small polygons — one around Berlin, one around Paris. Union
	// should include both clusters but not any point between them.
	body := `{
		"collection": "v",
		"multiPolygon": {
			"type": "MultiPolygon",
			"coordinates": [
				[[[13.36, 52.51], [13.42, 52.51], [13.42, 52.53], [13.36, 52.53], [13.36, 52.51]]],
				[[[2.34, 48.85], [2.36, 48.85], [2.36, 48.87], [2.34, 48.87], [2.34, 48.85]]]
			]
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/geo-polygon", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	s.handleGeoPolygon(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp GeoPolygonResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Shape != "multiPolygon" {
		t.Errorf("shape=%q", resp.Shape)
	}
	if resp.Total != 4 {
		t.Errorf("total=%d want 4 (3 Berlin + 1 Paris)", resp.Total)
	}
}

func TestHandleGeoPolygon_FilterMetaNarrowsToZero(t *testing.T) {
	s, cleanup := newTestServerForGeo(t)
	defer cleanup()
	setupPolygonCorpus(t, s)

	// No docs have this metadata — empty allow-list means early-exit with
	// an empty response instead of a full tree scan.
	body := `{
		"collection": "v",
		"polygon": {"coordinates": [[[13.36, 52.51], [13.42, 52.51], [13.42, 52.53], [13.36, 52.53], [13.36, 52.51]]]},
		"filterMeta": {"category": ["no-such-tag"]}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/geo-polygon", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	s.handleGeoPolygon(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp GeoPolygonResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 0 {
		t.Errorf("total=%d want 0 for unmatched filter", resp.Total)
	}
}

// docIDs extracts just the IDs from a polygon response — useful for test
// assertions that want to report "which doc came back".
func docIDs(items []GeoSearchResultItem) []string {
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.Document.ID
	}
	return ids
}
