package main

import (
	"mddb/internal/schema"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
)

// schemaExtraTestServer creates a minimal Server with schema.SchemaManager for HTTP handler tests.
func schemaExtraTestServer(t *testing.T) (*Server, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "schema_extra_*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}

	s := &Server{
		DB:   db,
		Path: f.Name(),
		Mode: ModeRW,
		BucketNames: BucketNames{
			Docs:    []byte("docs"),
			IdxMeta: []byte("idxmeta"),
			Rev:     []byte("rev"),
			ByKey:   []byte("bykey"),
		},
	}

	if err := s.ensureBuckets(); err != nil {
		_ = db.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}

	s.SchemaManager = schema.NewSchemaManager(db)
	if err := s.SchemaManager.EnsureBucket(); err != nil {
		_ = db.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}

	cleanup := func() {
		_ = db.Close()
		_ = os.Remove(f.Name())
	}
	return s, cleanup
}

// ---------------------------------------------------------------------------
// Test: HTTP handleSchemaSet
// ---------------------------------------------------------------------------

func TestHandleSchemaSet_Success(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	body := `{"collection":"articles","schema":"{\"required\":[\"title\"]}"}`
	req := httptest.NewRequest("POST", "/schema/set", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSchemaSet(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify it was stored
	raw, found := s.SchemaManager.Get("articles")
	if !found {
		t.Error("expected schema to be stored")
	}
	if raw == "" {
		t.Error("expected non-empty raw schema")
	}
}

func TestHandleSchemaSet_MissingCollection(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	body := `{"schema":"{\"required\":[\"title\"]}"}`
	req := httptest.NewRequest("POST", "/schema/set", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSchemaSet(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSchemaSet_MissingSchema(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	body := `{"collection":"articles"}`
	req := httptest.NewRequest("POST", "/schema/set", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSchemaSet(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSchemaSet_InvalidJSON(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	body := `{"collection":"articles","schema":"not valid json"}`
	req := httptest.NewRequest("POST", "/schema/set", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSchemaSet(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSchemaSet_InvalidBody(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/schema/set", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	s.handleSchemaSet(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Test: HTTP handleSchemaGet
// ---------------------------------------------------------------------------

func TestHandleSchemaGet_Found(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	_ = s.SchemaManager.Set("articles", `{"required":["title"]}`)

	body := `{"collection":"articles"}`
	req := httptest.NewRequest("POST", "/schema/get", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSchemaGet(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"enabled":true`) {
		t.Errorf("expected enabled=true in response, got %s", w.Body.String())
	}
}

func TestHandleSchemaGet_NotFound(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	body := `{"collection":"nope"}`
	req := httptest.NewRequest("POST", "/schema/get", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSchemaGet(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Errorf("expected enabled=false in response, got %s", w.Body.String())
	}
}

func TestHandleSchemaGet_MissingCollection(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	body := `{}`
	req := httptest.NewRequest("POST", "/schema/get", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSchemaGet(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSchemaGet_InvalidBody(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/schema/get", strings.NewReader("bad"))
	w := httptest.NewRecorder()
	s.handleSchemaGet(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Test: HTTP handleSchemaDelete
// ---------------------------------------------------------------------------

func TestHandleSchemaDelete_Success(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	_ = s.SchemaManager.Set("articles", `{"required":["title"]}`)

	body := `{"collection":"articles"}`
	req := httptest.NewRequest("POST", "/schema/delete", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSchemaDelete(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	_, found := s.SchemaManager.Get("articles")
	if found {
		t.Error("expected schema to be deleted")
	}
}

func TestHandleSchemaDelete_MissingCollection(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	body := `{}`
	req := httptest.NewRequest("POST", "/schema/delete", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSchemaDelete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSchemaDelete_InvalidBody(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/schema/delete", strings.NewReader("bad"))
	w := httptest.NewRecorder()
	s.handleSchemaDelete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Test: HTTP handleSchemaList
// ---------------------------------------------------------------------------

func TestHandleSchemaList_Empty(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/schema/list", nil)
	w := httptest.NewRecorder()
	s.handleSchemaList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"schemas":[]`) {
		t.Errorf("expected empty schemas list, got %s", w.Body.String())
	}
}

func TestHandleSchemaList_WithSchemas(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	_ = s.SchemaManager.Set("a", `{"required":["x"]}`)
	_ = s.SchemaManager.Set("b", `{"required":["y"]}`)

	req := httptest.NewRequest("GET", "/schema/list", nil)
	w := httptest.NewRecorder()
	s.handleSchemaList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"collection":"a"`) || !strings.Contains(body, `"collection":"b"`) {
		t.Errorf("expected both collections in response, got %s", body)
	}
}

// ---------------------------------------------------------------------------
// Test: HTTP handleValidate
// ---------------------------------------------------------------------------

func TestHandleValidate_Valid(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	_ = s.SchemaManager.Set("articles", `{"required":["title"],"properties":{"title":{"type":"string"}}}`)

	body := `{"collection":"articles","meta":{"title":["My Title"]}}`
	req := httptest.NewRequest("POST", "/schema/validate", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleValidate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"valid":true`) {
		t.Errorf("expected valid=true, got %s", w.Body.String())
	}
}

func TestHandleValidate_Invalid(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	_ = s.SchemaManager.Set("articles", `{"required":["title"]}`)

	body := `{"collection":"articles","meta":{}}`
	req := httptest.NewRequest("POST", "/schema/validate", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleValidate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"valid":false`) {
		t.Errorf("expected valid=false, got %s", w.Body.String())
	}
}

func TestHandleValidate_MissingCollection(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	body := `{"meta":{"title":["x"]}}`
	req := httptest.NewRequest("POST", "/schema/validate", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleValidate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleValidate_NoSchema(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	body := `{"collection":"noschemacol","meta":{"anything":["goes"]}}`
	req := httptest.NewRequest("POST", "/schema/validate", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleValidate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"valid":true`) {
		t.Errorf("expected valid=true when no schema, got %s", w.Body.String())
	}
}

func TestHandleValidate_InvalidBody(t *testing.T) {
	s, cleanup := schemaExtraTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/schema/validate", strings.NewReader("bad"))
	w := httptest.NewRecorder()
	s.handleValidate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Test: parseSchema edge cases
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Test: validateType edge cases
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Test: contains helper
// ---------------------------------------------------------------------------

func TestContains_Found(t *testing.T) {
	if !slices.Contains([]string{"a", "b", "c"}, "b") {
		t.Error("expected true")
	}
}

func TestContains_NotFound(t *testing.T) {
	if slices.Contains([]string{"a", "b", "c"}, "d") {
		t.Error("expected false")
	}
}

func TestContains_EmptySlice(t *testing.T) {
	if slices.Contains([]string{}, "a") {
		t.Error("expected false for empty slice")
	}
}

// ---------------------------------------------------------------------------
// Test: schema.SchemaManager SetBinlog
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Test: validateMeta with multiple property rules combined
// ---------------------------------------------------------------------------
