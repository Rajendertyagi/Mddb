package main

import (
	"bytes"
	"encoding/json"
	"mddb/internal/geo"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- Ray-cast unit tests ---

// --- Validation ---

func TestValidatePolygon_RejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		p    *geo.GeoJSONPolygon
	}{
		{"nil", nil},
		{"wrong type", &geo.GeoJSONPolygon{Type: "Point", Coordinates: [][][]float64{{{0, 0}, {1, 0}, {0, 1}}}}},
		{"no rings", &geo.GeoJSONPolygon{Type: "Polygon"}},
		{"too few points", &geo.GeoJSONPolygon{Coordinates: [][][]float64{{{0, 0}, {1, 0}}}}},
		{"point missing lat", &geo.GeoJSONPolygon{Coordinates: [][][]float64{{{0}, {1, 0}, {0, 1}}}}},
		{"lat out of range", &geo.GeoJSONPolygon{Coordinates: [][][]float64{{{0, 100}, {1, 0}, {0, 1}}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := geo.ValidatePolygon(c.p); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestValidatePolygon_AcceptsGood(t *testing.T) {
	p := &geo.GeoJSONPolygon{
		Type: "Polygon",
		Coordinates: [][][]float64{
			{{13, 52}, {14, 52}, {14, 53}, {13, 53}, {13, 52}},
		},
	}
	if err := geo.ValidatePolygon(p); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Type omitted is also allowed — handler fills it in from the endpoint path.
	p.Type = ""
	if err := geo.ValidatePolygon(p); err != nil {
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
