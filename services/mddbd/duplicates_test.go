package main

import (
	"mddb/internal/vector"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleFindDuplicates_MissingCollection(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v1/find-duplicates", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	s.handleFindDuplicates(w, req)

	if w.Result().StatusCode != 400 {
		t.Fatalf("expected 400, got %d", w.Result().StatusCode)
	}
}

func TestHandleFindDuplicates_InvalidMode(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v1/find-duplicates",
		strings.NewReader(`{"collection":"blog","mode":"invalid"}`))
	w := httptest.NewRecorder()
	s.handleFindDuplicates(w, req)

	if w.Result().StatusCode != 400 {
		t.Fatalf("expected 400, got %d", w.Result().StatusCode)
	}
}

func TestHandleFindDuplicates_ExactMode_NoDuplicates(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	// Add some docs (different content, no duplicates)
	addTestDoc(t, s, "blog", "post1", "en", "Hello world", nil)
	addTestDoc(t, s, "blog", "post2", "en", "Goodbye world", nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/find-duplicates",
		strings.NewReader(`{"collection":"blog","mode":"exact"}`))
	w := httptest.NewRecorder()
	s.handleFindDuplicates(w, req)

	if w.Result().StatusCode != 200 {
		t.Fatalf("expected 200, got %d", w.Result().StatusCode)
	}
}

func TestHandleFindDuplicates_DefaultMode(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	addTestDoc(t, s, "blog", "post1", "en", "content", nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/find-duplicates",
		strings.NewReader(`{"collection":"blog"}`))
	w := httptest.NewRecorder()
	s.handleFindDuplicates(w, req)

	if w.Result().StatusCode != 200 {
		t.Fatalf("expected 200, got %d", w.Result().StatusCode)
	}
}

func TestHandleFindDuplicates_InvalidJSON(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v1/find-duplicates",
		strings.NewReader(`not json`))
	w := httptest.NewRecorder()
	s.handleFindDuplicates(w, req)

	if w.Result().StatusCode != 400 {
		t.Fatalf("expected 400, got %d", w.Result().StatusCode)
	}
}

// --- Union-Find tests ---

func TestUnionFind_Basic(t *testing.T) {
	uf := newUnionFind(5)

	uf.union(0, 1)
	uf.union(2, 3)
	uf.union(1, 3)

	// 0,1,2,3 should be in the same group
	root := uf.find(0)
	for _, i := range []int{1, 2, 3} {
		if uf.find(i) != root {
			t.Errorf("expected %d and 0 in same group", i)
		}
	}

	// 4 should be separate
	if uf.find(4) == root {
		t.Error("expected 4 to be in a separate group")
	}
}

func TestUnionFind_SingleElement(t *testing.T) {
	uf := newUnionFind(1)
	if uf.find(0) != 0 {
		t.Error("single element should be its own root")
	}
}

func TestUnionFind_NoUnion(t *testing.T) {
	uf := newUnionFind(3)
	for i := 0; i < 3; i++ {
		if uf.find(i) != i {
			t.Errorf("element %d should be its own root", i)
		}
	}
}

// --- findExactDuplicates tests ---

func TestFindExactDuplicates_Groups(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	docs := []docVec{
		{docID: "doc1", contentHash: "aaa"},
		{docID: "doc2", contentHash: "aaa"},
		{docID: "doc3", contentHash: "bbb"},
		{docID: "doc4", contentHash: "aaa"},
		{docID: "doc5", contentHash: ""},
	}

	groups := s.findExactDuplicates("blog", docs, false)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].Documents) != 3 {
		t.Errorf("expected 3 docs in group, got %d", len(groups[0].Documents))
	}
	if groups[0].Type != "exact" {
		t.Errorf("expected type 'exact', got %q", groups[0].Type)
	}
}

func TestFindExactDuplicates_NoDuplicates(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	docs := []docVec{
		{docID: "doc1", contentHash: "aaa"},
		{docID: "doc2", contentHash: "bbb"},
	}

	groups := s.findExactDuplicates("blog", docs, false)
	if len(groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(groups))
	}
}

// --- findSimilarDuplicates tests ---

func TestFindSimilarDuplicates_IdenticalVectors(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	vec := []float32{1.0, 0.0, 0.0}
	docs := []docVec{
		{docID: "doc1", vector: vec},
		{docID: "doc2", vector: vec},
		{docID: "doc3", vector: []float32{0.0, 1.0, 0.0}},
	}

	groups := s.findSimilarDuplicates("blog", docs, 0.9, vector.CosineSimilarity, false)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].Documents) != 2 {
		t.Errorf("expected 2 docs in group, got %d", len(groups[0].Documents))
	}
}

func TestFindSimilarDuplicates_NoPairs(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	docs := []docVec{
		{docID: "doc1", vector: []float32{1.0, 0.0, 0.0}},
		{docID: "doc2", vector: []float32{0.0, 1.0, 0.0}},
	}

	groups := s.findSimilarDuplicates("blog", docs, 0.9, vector.CosineSimilarity, false)
	if len(groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(groups))
	}
}

func TestFindSimilarDuplicates_TooFewDocs(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	docs := []docVec{
		{docID: "doc1", vector: []float32{1.0}},
	}

	groups := s.findSimilarDuplicates("blog", docs, 0.9, nil, false)
	if groups != nil {
		t.Errorf("expected nil for single doc, got %v", groups)
	}
}
