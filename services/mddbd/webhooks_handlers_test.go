package main

import (
	"mddb/internal/webhooks"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
)

// whExtraTestServer creates a minimal Server with WebhookManager for HTTP handler tests.
func whExtraTestServer(t *testing.T) (*Server, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "wh_extra_*.db")
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

	s.WebhookManager = webhooks.NewWebhookManager(db)
	if err := s.WebhookManager.EnsureBucket(); err != nil {
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
// Test: HTTP handleWebhooks GET (list)
// ---------------------------------------------------------------------------

func TestHandleWebhooks_GET_Empty(t *testing.T) {
	s, cleanup := whExtraTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/webhooks", nil)
	w := httptest.NewRecorder()
	s.handleWebhooks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "[]") {
		t.Errorf("expected empty list, got %s", w.Body.String())
	}
}

func TestHandleWebhooks_GET_WithHooks(t *testing.T) {
	s, cleanup := whExtraTestServer(t)
	defer cleanup()

	_, _ = s.WebhookManager.Register("http://example.com/hook1", []string{"doc.added"}, "blog")

	req := httptest.NewRequest("GET", "/webhooks", nil)
	w := httptest.NewRecorder()
	s.handleWebhooks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "http://example.com/hook1") {
		t.Errorf("expected hook URL in response, got %s", w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test: HTTP handleWebhooks POST (register)
// ---------------------------------------------------------------------------

func TestHandleWebhooks_POST_Success(t *testing.T) {
	s, cleanup := whExtraTestServer(t)
	defer cleanup()

	body := `{"url":"http://example.com/hook","events":["doc.added"],"collection":"blog"}`
	req := httptest.NewRequest("POST", "/webhooks", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleWebhooks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	hooks := s.WebhookManager.List()
	if len(hooks) != 1 {
		t.Errorf("expected 1 webhook after register, got %d", len(hooks))
	}
}

func TestHandleWebhooks_POST_InvalidBody(t *testing.T) {
	s, cleanup := whExtraTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/webhooks", strings.NewReader("bad json"))
	w := httptest.NewRecorder()
	s.handleWebhooks(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleWebhooks_POST_MissingURL(t *testing.T) {
	s, cleanup := whExtraTestServer(t)
	defer cleanup()

	body := `{"events":["doc.added"]}`
	req := httptest.NewRequest("POST", "/webhooks", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleWebhooks(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleWebhooks_POST_InvalidEvent(t *testing.T) {
	s, cleanup := whExtraTestServer(t)
	defer cleanup()

	body := `{"url":"http://example.com/hook","events":["invalid.event"]}`
	req := httptest.NewRequest("POST", "/webhooks", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleWebhooks(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleWebhooks_POST_ReadOnly(t *testing.T) {
	s, cleanup := whExtraTestServer(t)
	defer cleanup()

	s.Mode = ModeRead
	body := `{"url":"http://example.com/hook","events":["doc.added"]}`
	req := httptest.NewRequest("POST", "/webhooks", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleWebhooks(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Test: HTTP handleWebhooks unsupported method
// ---------------------------------------------------------------------------

func TestHandleWebhooks_PUT_NotAllowed(t *testing.T) {
	s, cleanup := whExtraTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("PUT", "/webhooks", nil)
	w := httptest.NewRecorder()
	s.handleWebhooks(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Test: HTTP handleWebhooks when manager is nil
// ---------------------------------------------------------------------------

func TestHandleWebhooks_NilManager(t *testing.T) {
	s, cleanup := whExtraTestServer(t)
	defer cleanup()

	s.WebhookManager = nil
	req := httptest.NewRequest("GET", "/webhooks", nil)
	w := httptest.NewRecorder()
	s.handleWebhooks(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Test: HTTP handleWebhookDelete
// ---------------------------------------------------------------------------

func TestHandleWebhookDelete_Success(t *testing.T) {
	s, cleanup := whExtraTestServer(t)
	defer cleanup()

	wh, _ := s.WebhookManager.Register("http://example.com/hook", []string{"doc.added"}, "")

	body := `{"id":"` + wh.ID + `"}`
	req := httptest.NewRequest("POST", "/webhooks/delete", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleWebhookDelete(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	hooks := s.WebhookManager.List()
	if len(hooks) != 0 {
		t.Errorf("expected 0 webhooks after delete, got %d", len(hooks))
	}
}

func TestHandleWebhookDelete_MissingID(t *testing.T) {
	s, cleanup := whExtraTestServer(t)
	defer cleanup()

	body := `{}`
	req := httptest.NewRequest("POST", "/webhooks/delete", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleWebhookDelete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleWebhookDelete_InvalidBody(t *testing.T) {
	s, cleanup := whExtraTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/webhooks/delete", strings.NewReader("bad"))
	w := httptest.NewRecorder()
	s.handleWebhookDelete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleWebhookDelete_NilManager(t *testing.T) {
	s, cleanup := whExtraTestServer(t)
	defer cleanup()

	s.WebhookManager = nil
	body := `{"id":"abc"}`
	req := httptest.NewRequest("POST", "/webhooks/delete", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleWebhookDelete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
