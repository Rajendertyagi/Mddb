package main

import (
	"bytes"
	"encoding/json"
	"mddb/internal/fts"
	"net/http"
	"net/http/httptest"
	"testing"

	bolt "go.etcd.io/bbolt"
)

// newTestServerForHybridGeo reuses the geo test harness and layers a real
// FTS index on top so handleHybridSearch has something to score against.
// No vector provider — runVectorSearch returns nil, which handleHybridSearch
// tolerates, and the FTS-only merge path exercises every post-merge branch
// (boost + geo filter + the new distance sort) we need to cover.
func newTestServerForHybridGeo(t *testing.T) (*Server, func()) {
	t.Helper()
	s, cleanup := newTestServerForGeo(t)
	s.FTSIndex = fts.NewFTSIndex(s.DB)
	if err := s.FTSIndex.EnsureBuckets(); err != nil {
		cleanup()
		t.Fatalf("ensure FTS buckets: %v", err)
	}
	return s, cleanup
}

// seedHybridGeoDoc wires a doc's content into the FTS index and its geo
// point into both geo indices under a stable caller-chosen docID. It also
// persists a minimal Doc record so loadHybridDocs can retrieve something
// back from the `docs` bucket when handleHybridSearch hydrates results.
func seedHybridGeoDoc(t *testing.T, s *Server, collection, docID, content string, lat, lng float64) {
	t.Helper()
	d := Doc{
		ID:        docID,
		Key:       docID,
		Lang:      "en",
		ContentMD: content,
	}
	data, err := marshalDoc(&d)
	if err != nil {
		t.Fatalf("marshal doc: %v", err)
	}
	err = s.DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(s.BucketNames.Docs).Put(kDoc(collection, docID), data)
	})
	if err != nil {
		t.Fatalf("put doc: %v", err)
	}
	if err := s.FTSIndex.Index(collection, docID, content); err != nil {
		t.Fatalf("fts index: %v", err)
	}
	s.GeoIndex.Add(collection, docID, lat, lng)
	s.GeoHashIndex.Add(collection, docID, lat, lng)
}

func postHybrid(t *testing.T, s *Server, body string) (*httptest.ResponseRecorder, *HybridSearchResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/hybrid-search", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	s.handleHybridSearch(w, req)
	resp := &HybridSearchResponse{}
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), resp); err != nil {
			t.Fatalf("decode: %v — body=%s", err, w.Body.String())
		}
	}
	return w, resp
}

func TestHybridSearch_SortDistanceRequiresGeo(t *testing.T) {
	s, cleanup := newTestServerForHybridGeo(t)
	defer cleanup()

	// sort=distance without a geo filter is meaningless — every result would
	// carry distanceMeters=0 and the ordering would be arbitrary. Reject it
	// at the validator so the caller gets a clear error.
	body := `{"collection":"v","query":"hello","sort":"distance"}`
	w, _ := postHybrid(t, s, body)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400 (sort=distance without geo)", w.Code)
	}
}

func TestHybridSearch_UnknownSortRejected(t *testing.T) {
	s, cleanup := newTestServerForHybridGeo(t)
	defer cleanup()

	body := `{"collection":"v","query":"hello","sort":"madness"}`
	w, _ := postHybrid(t, s, body)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400 (unknown sort)", w.Code)
	}
}

func TestHybridSearch_DefaultSortUnchanged(t *testing.T) {
	s, cleanup := newTestServerForHybridGeo(t)
	defer cleanup()

	// Two docs containing the query term plus one noise doc. Without a geo
	// filter the response should come back ordered by BM25 score descending;
	// we don't pin which of the matching docs wins (that's up to the scorer)
	// — we just assert that the matching docs land ahead of the noise one.
	seedHybridGeoDoc(t, s, "v", "a", "market report weekly roundup", 52.52, 13.405)
	seedHybridGeoDoc(t, s, "v", "b", "market notes", 52.521, 13.406)
	seedHybridGeoDoc(t, s, "v", "c", "unrelated filler", 52.52, 13.5)

	body := `{"collection":"v","query":"market","topK":10,"algorithm":"bm25"}`
	w, resp := postHybrid(t, s, body)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if len(resp.Results) < 2 {
		t.Fatalf("expected at least 2 matching results, got %d", len(resp.Results))
	}
	// Scores should be non-increasing under default sort.
	for i := 1; i < len(resp.Results); i++ {
		if resp.Results[i-1].CombinedScore < resp.Results[i].CombinedScore {
			t.Errorf("default sort not descending at index %d: %f < %f",
				i, resp.Results[i-1].CombinedScore, resp.Results[i].CombinedScore)
		}
	}
	// The noise doc "c" must not outrank the matching ones.
	if resp.Results[0].Document.ID == "c" {
		t.Errorf("noise doc 'c' should not rank first under default sort")
	}
}

func TestHybridSearch_SortDistanceReorders(t *testing.T) {
	s, cleanup := newTestServerForHybridGeo(t)
	defer cleanup()

	// Three docs with descending FTS match quality but in reverse geographic
	// order: "a" matches best but is farthest, "c" matches weakest but is
	// nearest. Under default sort "a" wins; under sort=distance "c" wins.
	seedHybridGeoDoc(t, s, "v", "a", "market market market", 52.52, 13.5) // ~6.5km
	seedHybridGeoDoc(t, s, "v", "b", "market market", 52.52, 13.45)       // ~3km
	seedHybridGeoDoc(t, s, "v", "c", "market", 52.52, 13.406)             // ~70m
	seedHybridGeoDoc(t, s, "v", "d", "unrelated", 52.521, 13.4051)        // ~100m, no FTS hit

	// Geo filter at Berlin center with 10km radius + sort=distance.
	body := `{
		"collection":"v",
		"query":"market",
		"topK":10,
		"algorithm":"bm25",
		"geo":{"lat":52.52,"lng":13.405,"radiusMeters":10000},
		"sort":"distance"
	}`
	w, resp := postHybrid(t, s, body)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if len(resp.Results) < 3 {
		t.Fatalf("expected at least 3 results, got %d: %+v", len(resp.Results), resp.Results)
	}
	// Verify ascending distance ordering across the whole response.
	for i := 1; i < len(resp.Results); i++ {
		if resp.Results[i-1].DistanceMeters > resp.Results[i].DistanceMeters {
			t.Errorf("not sorted by distance at index %d: %f > %f",
				i, resp.Results[i-1].DistanceMeters, resp.Results[i].DistanceMeters)
		}
	}
	// And the nearest matching doc ("c") should win despite being the weakest
	// FTS match. If it doesn't, the sort didn't happen.
	if resp.Results[0].Document.ID != "c" {
		t.Errorf("expected nearest 'c' first under sort=distance, got %q",
			resp.Results[0].Document.ID)
	}
	// Rank field is recomputed after sorting — must be sequential 1..N.
	for i, r := range resp.Results {
		if r.Rank != i+1 {
			t.Errorf("rank mismatch at %d: got %d", i, r.Rank)
		}
	}
}

func TestHybridSearch_SortCombinedKeepsScoreOrder(t *testing.T) {
	s, cleanup := newTestServerForHybridGeo(t)
	defer cleanup()

	seedHybridGeoDoc(t, s, "v", "a", "market market market", 52.52, 13.5)
	seedHybridGeoDoc(t, s, "v", "b", "market", 52.52, 13.406)

	// Explicit sort=combined must keep the score-descending ordering even
	// with a geo filter in play.
	body := `{
		"collection":"v",
		"query":"market",
		"topK":10,
		"algorithm":"bm25",
		"geo":{"lat":52.52,"lng":13.405,"radiusMeters":10000},
		"sort":"combined"
	}`
	w, resp := postHybrid(t, s, body)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if len(resp.Results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	// "a" has three occurrences of "market", "b" has one — "a" must still win.
	if resp.Results[0].Document.ID != "a" {
		t.Errorf("sort=combined should keep score order; got %q first",
			resp.Results[0].Document.ID)
	}
}
