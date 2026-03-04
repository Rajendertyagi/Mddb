package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	json "github.com/goccy/go-json"
	bolt "go.etcd.io/bbolt"
)

// newTestServerForAutomation creates a Server with AutomationManager and FTSIndex for testing.
func newTestServerForAutomation(t *testing.T) (*Server, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{
		DB:   db,
		Path: dbPath,
		Mode: ModeRW,
		BucketNames: BucketNames{
			Docs:    []byte("docs"),
			IdxMeta: []byte("idxmeta"),
			Rev:     []byte("rev"),
			ByKey:   []byte("bykey"),
		},
		Cache: NewDocumentCache(100, 60),
	}

	if err := s.ensureBuckets(); err != nil {
		db.Close()
		t.Fatal(err)
	}

	// Set up FTSIndex
	s.FTSIndex = NewFTSIndex(db)
	if err := s.FTSIndex.EnsureBuckets(); err != nil {
		db.Close()
		t.Fatal(err)
	}

	// Set up AutomationManager
	am := NewAutomationManager(db)
	am.SetServer(s)
	if err := am.EnsureBucket(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	s.AutomationManager = am

	return s, func() { db.Close() }
}

// --- HTTP Handler Tests ---

func TestHandleAutomation_ListEmpty(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/v1/automation", nil)
	w := httptest.NewRecorder()
	s.handleAutomation(w, req)

	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	rules := body["rules"].([]interface{})
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}

func TestHandleAutomation_CreateWebhook(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	payload := `{"type":"webhook","name":"Test Hook","url":"https://example.com/hook","enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/automation", strings.NewReader(payload))
	w := httptest.NewRecorder()
	s.handleAutomation(w, req)

	resp := w.Result()
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}

	var created AutomationRule
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Error("expected ID to be set")
	}
	if created.Type != "webhook" {
		t.Errorf("expected type webhook, got %s", created.Type)
	}
	if created.Method != "POST" {
		t.Errorf("expected default method POST, got %s", created.Method)
	}
}

func TestHandleAutomation_CreateInvalidJSON(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v1/automation", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	s.handleAutomation(w, req)

	resp := w.Result()
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandleAutomation_CreateMissingFields(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	payload := `{"type":"webhook","name":"No URL"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/automation", strings.NewReader(payload))
	w := httptest.NewRecorder()
	s.handleAutomation(w, req)

	resp := w.Result()
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandleAutomation_MethodNotAllowed(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/v1/automation", nil)
	w := httptest.NewRecorder()
	s.handleAutomation(w, req)

	resp := w.Result()
	if resp.StatusCode != 405 {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func TestHandleAutomation_ListWithTypeFilter(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	// Create a webhook and trigger
	wh, _ := s.AutomationManager.Create(AutomationRule{Type: "webhook", Name: "Hook", URL: "https://example.com", Enabled: true})
	s.AutomationManager.Create(AutomationRule{Type: "trigger", Name: "Trig", Collection: "blog", SearchType: "fts", Query: "test", Threshold: 50, WebhookID: wh.ID, Enabled: true})

	// Filter by webhook
	req := httptest.NewRequest(http.MethodGet, "/v1/automation?type=webhook", nil)
	w := httptest.NewRecorder()
	s.handleAutomation(w, req)

	var body map[string]interface{}
	json.NewDecoder(w.Result().Body).Decode(&body)
	total := int(body["total"].(float64))
	if total != 1 {
		t.Errorf("expected 1 webhook, got %d", total)
	}
}

func TestHandleAutomationDetail_GetUpdateDelete(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	// Create webhook
	wh, _ := s.AutomationManager.Create(AutomationRule{Type: "webhook", Name: "Hook", URL: "https://example.com", Enabled: true})

	// GET
	req := httptest.NewRequest(http.MethodGet, "/v1/automation/"+wh.ID, nil)
	w := httptest.NewRecorder()
	s.handleAutomationDetail(w, req)
	if w.Result().StatusCode != 200 {
		t.Fatalf("GET expected 200, got %d", w.Result().StatusCode)
	}

	// PUT
	updatePayload := `{"name":"Updated Hook","url":"https://updated.com","enabled":true}`
	req = httptest.NewRequest(http.MethodPut, "/v1/automation/"+wh.ID, strings.NewReader(updatePayload))
	w = httptest.NewRecorder()
	s.handleAutomationDetail(w, req)
	if w.Result().StatusCode != 200 {
		body, _ := io.ReadAll(w.Result().Body)
		t.Fatalf("PUT expected 200, got %d: %s", w.Result().StatusCode, body)
	}

	var updated AutomationRule
	json.NewDecoder(w.Result().Body).Decode(&updated)
	if updated.Name != "Updated Hook" {
		t.Errorf("expected name 'Updated Hook', got %q", updated.Name)
	}

	// DELETE
	req = httptest.NewRequest(http.MethodDelete, "/v1/automation/"+wh.ID, nil)
	w = httptest.NewRecorder()
	s.handleAutomationDetail(w, req)
	if w.Result().StatusCode != 200 {
		t.Fatalf("DELETE expected 200, got %d", w.Result().StatusCode)
	}

	// Verify deleted
	req = httptest.NewRequest(http.MethodGet, "/v1/automation/"+wh.ID, nil)
	w = httptest.NewRecorder()
	s.handleAutomationDetail(w, req)
	if w.Result().StatusCode != 404 {
		t.Fatalf("GET after delete expected 404, got %d", w.Result().StatusCode)
	}
}

func TestHandleAutomationDetail_NotFound(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/v1/automation/nonexistent", nil)
	w := httptest.NewRecorder()
	s.handleAutomationDetail(w, req)
	if w.Result().StatusCode != 404 {
		t.Fatalf("expected 404, got %d", w.Result().StatusCode)
	}
}

func TestHandleAutomationDetail_DeleteNotFound(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/v1/automation/nonexistent", nil)
	w := httptest.NewRecorder()
	s.handleAutomationDetail(w, req)
	if w.Result().StatusCode != 404 {
		t.Fatalf("expected 404, got %d", w.Result().StatusCode)
	}
}

func TestHandleAutomationDetail_MethodNotAllowed(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPatch, "/v1/automation/someid", nil)
	w := httptest.NewRecorder()
	s.handleAutomationDetail(w, req)
	if w.Result().StatusCode != 405 {
		t.Fatalf("expected 405, got %d", w.Result().StatusCode)
	}
}

func TestHandleAutomationDetail_MissingID(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/v1/automation/", nil)
	w := httptest.NewRecorder()
	s.handleAutomationDetail(w, req)
	if w.Result().StatusCode != 400 {
		t.Fatalf("expected 400, got %d", w.Result().StatusCode)
	}
}

func TestHandleAutomationTest_Success(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	// Index a document in FTS
	s.FTSIndex.Index("blog", "doc1", "golang programming tutorial")

	// Create webhook + trigger
	wh, _ := s.AutomationManager.Create(AutomationRule{Type: "webhook", Name: "Hook", URL: "https://example.com", Enabled: true})
	tr, _ := s.AutomationManager.Create(AutomationRule{
		Type: "trigger", Name: "Trig", Collection: "blog",
		SearchType: "fts", Query: "golang", Threshold: 0, WebhookID: wh.ID, Enabled: true,
	})

	// Test trigger
	req := httptest.NewRequest(http.MethodPost, "/v1/automation/"+tr.ID+"/test", nil)
	w := httptest.NewRecorder()
	s.handleAutomationDetail(w, req)
	if w.Result().StatusCode != 200 {
		body, _ := io.ReadAll(w.Result().Body)
		t.Fatalf("expected 200, got %d: %s", w.Result().StatusCode, body)
	}

	var result map[string]interface{}
	json.NewDecoder(w.Result().Body).Decode(&result)
	total := int(result["total"].(float64))
	if total < 1 {
		t.Errorf("expected at least 1 match, got %d", total)
	}
}

func TestHandleAutomationTest_NonTrigger(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	wh, _ := s.AutomationManager.Create(AutomationRule{Type: "webhook", Name: "Hook", URL: "https://example.com", Enabled: true})

	req := httptest.NewRequest(http.MethodPost, "/v1/automation/"+wh.ID+"/test", nil)
	w := httptest.NewRecorder()
	s.handleAutomationDetail(w, req)
	if w.Result().StatusCode != 400 {
		t.Fatalf("expected 400 for testing non-trigger, got %d", w.Result().StatusCode)
	}
}

func TestHandleAutomationTest_NotFound(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v1/automation/nonexistent/test", nil)
	w := httptest.NewRecorder()
	s.handleAutomationDetail(w, req)
	if w.Result().StatusCode != 404 {
		t.Fatalf("expected 404, got %d", w.Result().StatusCode)
	}
}

func TestHandleAutomationTest_WrongMethod(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	wh, _ := s.AutomationManager.Create(AutomationRule{Type: "webhook", Name: "Hook", URL: "https://example.com", Enabled: true})

	req := httptest.NewRequest(http.MethodGet, "/v1/automation/"+wh.ID+"/test", nil)
	w := httptest.NewRecorder()
	s.handleAutomationDetail(w, req)
	if w.Result().StatusCode != 405 {
		t.Fatalf("expected 405, got %d", w.Result().StatusCode)
	}
}

func TestHandleAutomation_ReadOnlyMode(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()
	s.Mode = ModeRead

	payload := `{"type":"webhook","name":"Hook","url":"https://example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/automation", strings.NewReader(payload))
	w := httptest.NewRecorder()
	s.handleAutomation(w, req)
	if w.Result().StatusCode != 403 {
		t.Fatalf("expected 403 in read-only mode, got %d", w.Result().StatusCode)
	}
}

func TestHandleAutomationDetail_ReadOnlyPut(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	wh, _ := s.AutomationManager.Create(AutomationRule{Type: "webhook", Name: "Hook", URL: "https://example.com", Enabled: true})

	s.Mode = ModeRead
	req := httptest.NewRequest(http.MethodPut, "/v1/automation/"+wh.ID, strings.NewReader(`{"name":"Updated","url":"https://example.com"}`))
	w := httptest.NewRecorder()
	s.handleAutomationDetail(w, req)
	if w.Result().StatusCode != 403 {
		t.Fatalf("expected 403 in read-only mode, got %d", w.Result().StatusCode)
	}
}

func TestHandleAutomationDetail_ReadOnlyDelete(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	wh, _ := s.AutomationManager.Create(AutomationRule{Type: "webhook", Name: "Hook", URL: "https://example.com", Enabled: true})

	s.Mode = ModeRead
	req := httptest.NewRequest(http.MethodDelete, "/v1/automation/"+wh.ID, nil)
	w := httptest.NewRecorder()
	s.handleAutomationDetail(w, req)
	if w.Result().StatusCode != 403 {
		t.Fatalf("expected 403 in read-only mode, got %d", w.Result().StatusCode)
	}
}

// --- Trigger Evaluation Tests ---

func TestEvaluateTriggers_NoTriggers(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	// Should not panic with no triggers
	s.AutomationManager.EvaluateTriggers("blog", Doc{ID: "doc1"})
}

func TestEvaluateTriggers_NoServer(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()
	// am.server is nil
	am.EvaluateTriggers("blog", Doc{ID: "doc1"})
}

func TestEvalFTS_NoIndex(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()
	s.FTSIndex = nil

	trigger := &AutomationRule{SearchType: "fts", Query: "test", Collection: "blog", Threshold: 0}
	score, matched := s.AutomationManager.evalFTS(trigger, &Doc{ID: "doc1"})
	if matched {
		t.Error("expected no match when FTSIndex is nil")
	}
	if score != 0 {
		t.Errorf("expected score 0, got %f", score)
	}
}

func TestEvalFTS_Match(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	// Index a document
	s.FTSIndex.Index("blog", "doc1", "golang programming is awesome")

	trigger := &AutomationRule{SearchType: "fts", Query: "golang", Collection: "blog", Threshold: 0}
	score, matched := s.AutomationManager.evalFTS(trigger, &Doc{ID: "doc1"})
	if !matched {
		t.Error("expected match for indexed document")
	}
	if score <= 0 {
		t.Errorf("expected positive score, got %f", score)
	}
}

func TestEvalFTS_NoMatch(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	s.FTSIndex.Index("blog", "doc1", "golang programming")

	trigger := &AutomationRule{SearchType: "fts", Query: "golang", Collection: "blog", Threshold: 0}
	score, matched := s.AutomationManager.evalFTS(trigger, &Doc{ID: "doc999"})
	if matched {
		t.Error("expected no match for non-indexed document")
	}
	if score != 0 {
		t.Errorf("expected score 0, got %f", score)
	}
}

func TestEvalFTS_ThresholdNotMet(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	s.FTSIndex.Index("blog", "doc1", "golang programming")

	trigger := &AutomationRule{SearchType: "fts", Query: "golang", Collection: "blog", Threshold: 999}
	score, matched := s.AutomationManager.evalFTS(trigger, &Doc{ID: "doc1"})
	if matched {
		t.Error("expected no match when threshold too high")
	}
	if score <= 0 {
		t.Errorf("expected positive score even if not matched, got %f", score)
	}
}

func TestEvalVector_NoEmbedding(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	trigger := &AutomationRule{SearchType: "vector", Query: "test", Collection: "blog", Threshold: 50}
	score, matched := s.AutomationManager.evalVector(trigger, &Doc{ID: "doc1"})
	if matched {
		t.Error("expected no match when Embedding is nil")
	}
	if score != 0 {
		t.Errorf("expected score 0, got %f", score)
	}
}

func TestRunTrigger_NoServer(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	trigger := &AutomationRule{SearchType: "fts"}
	matches, err := am.RunTrigger(trigger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matches != nil {
		t.Errorf("expected nil matches, got %v", matches)
	}
}

func TestRunTrigger_UnknownSearchType(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	trigger := &AutomationRule{SearchType: "unknown"}
	matches, err := s.AutomationManager.RunTrigger(trigger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matches != nil {
		t.Errorf("expected nil matches for unknown searchType, got %v", matches)
	}
}

func TestRunTriggerFTS(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	// Index documents
	s.FTSIndex.Index("blog", "doc1", "golang programming tutorial")
	s.FTSIndex.Index("blog", "doc2", "python data science")
	s.FTSIndex.Index("blog", "doc3", "golang web development")

	trigger := &AutomationRule{
		SearchType: "fts",
		Collection: "blog",
		Query:      "golang",
		Threshold:  0,
	}
	matches, err := s.AutomationManager.RunTrigger(trigger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) < 2 {
		t.Errorf("expected at least 2 matches for 'golang', got %d", len(matches))
	}
	for _, m := range matches {
		if m.Collection != "blog" {
			t.Errorf("expected collection 'blog', got %q", m.Collection)
		}
	}
}

func TestRunTriggerFTS_NoIndex(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()
	s.FTSIndex = nil

	trigger := &AutomationRule{SearchType: "fts", Collection: "blog", Query: "test", Threshold: 0}
	matches, err := s.AutomationManager.RunTrigger(trigger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matches != nil {
		t.Errorf("expected nil matches, got %v", matches)
	}
}

func TestRunTriggerFTS_WithThreshold(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	s.FTSIndex.Index("blog", "doc1", "golang programming tutorial")

	// High threshold should filter out results
	trigger := &AutomationRule{
		SearchType: "fts",
		Collection: "blog",
		Query:      "golang",
		Threshold:  9999,
	}
	matches, err := s.AutomationManager.RunTrigger(trigger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches with high threshold, got %d", len(matches))
	}
}

func TestRunTriggerVector_NoEmbedding(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	trigger := &AutomationRule{SearchType: "vector", Collection: "blog", Query: "test", Threshold: 50}
	matches, err := s.AutomationManager.RunTrigger(trigger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matches != nil {
		t.Errorf("expected nil matches, got %v", matches)
	}
}

func TestRunTriggerAndFire_WebhookNotFound(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	trigger := &AutomationRule{
		ID:         "trigger1",
		SearchType: "fts",
		Collection: "blog",
		Query:      "test",
		WebhookID:  "nonexistent",
	}
	// Should not panic
	s.AutomationManager.RunTriggerAndFire(trigger)
}

func TestRunTriggerAndFire_WebhookDisabled(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	wh, _ := s.AutomationManager.Create(AutomationRule{Type: "webhook", Name: "Hook", URL: "https://example.com", Enabled: false})

	trigger := &AutomationRule{
		ID:         "trigger1",
		SearchType: "fts",
		Collection: "blog",
		Query:      "test",
		WebhookID:  wh.ID,
	}
	// Should not panic
	s.AutomationManager.RunTriggerAndFire(trigger)
}

func TestEvaluateSingleTrigger_NoServer(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	trigger := &AutomationRule{SearchType: "fts"}
	// Should not panic
	am.evaluateSingleTrigger(trigger, &Doc{ID: "doc1"})
}

func TestEvaluateSingleTrigger_WebhookNotFound(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	s.FTSIndex.Index("blog", "doc1", "golang test content")

	trigger := &AutomationRule{
		ID:         "trigger1",
		SearchType: "fts",
		Collection: "blog",
		Query:      "golang",
		Threshold:  0,
		WebhookID:  "nonexistent",
	}
	// Should not panic even when webhook doesn't exist
	s.AutomationManager.evaluateSingleTrigger(trigger, &Doc{ID: "doc1"})
}

func TestEvaluateTriggers_FTSMatch(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	s.FTSIndex.Index("blog", "doc1", "golang programming guide")

	// Create webhook + trigger
	wh, _ := s.AutomationManager.Create(AutomationRule{Type: "webhook", Name: "Hook", URL: "https://example.com", Enabled: true})
	s.AutomationManager.Create(AutomationRule{
		Type: "trigger", Name: "Trig", Collection: "blog",
		SearchType: "fts", Query: "golang", Threshold: 0, WebhookID: wh.ID, Enabled: true,
	})

	// Should not panic
	s.AutomationManager.EvaluateTriggers("blog", Doc{ID: "doc1"})
}

// --- Webhook Firing Tests ---

func TestFireAutomationWebhook(t *testing.T) {
	// Create a test HTTP server to receive the webhook
	var receivedBody []byte
	var receivedHeaders http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		receivedHeaders = r.Header
		w.WriteHeader(200)
	}))
	defer ts.Close()

	webhook := &AutomationRule{
		ID:     "wh1",
		Type:   "webhook",
		URL:    ts.URL,
		Method: "POST",
		Headers: map[string]string{
			"X-Custom": "test-value",
		},
	}
	trigger := &AutomationRule{
		ID:   "tr1",
		Name: "Test Trigger",
	}
	doc := &Doc{ID: "doc1", ContentMD: "test content"}

	fireAutomationWebhook(webhook, trigger, doc, "blog", 85.5)

	// Verify payload
	if len(receivedBody) == 0 {
		t.Fatal("expected webhook to receive a body")
	}

	var payload TriggerPayload
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if payload.Event != "trigger.matched" {
		t.Errorf("expected event 'trigger.matched', got %q", payload.Event)
	}
	if payload.Trigger.ID != "tr1" {
		t.Errorf("expected trigger ID 'tr1', got %q", payload.Trigger.ID)
	}
	if payload.Trigger.Name != "Test Trigger" {
		t.Errorf("expected trigger name 'Test Trigger', got %q", payload.Trigger.Name)
	}
	if payload.Collection != "blog" {
		t.Errorf("expected collection 'blog', got %q", payload.Collection)
	}
	if payload.Score != 85.5 {
		t.Errorf("expected score 85.5, got %f", payload.Score)
	}
	if payload.Timestamp == 0 {
		t.Error("expected timestamp to be set")
	}

	// Verify headers
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", receivedHeaders.Get("Content-Type"))
	}
	if receivedHeaders.Get("X-MDDB-Event") != "trigger.matched" {
		t.Errorf("expected X-MDDB-Event header, got %q", receivedHeaders.Get("X-MDDB-Event"))
	}
	if receivedHeaders.Get("X-MDDB-Trigger-ID") != "tr1" {
		t.Errorf("expected X-MDDB-Trigger-ID header, got %q", receivedHeaders.Get("X-MDDB-Trigger-ID"))
	}
	if receivedHeaders.Get("X-MDDB-Webhook-ID") != "wh1" {
		t.Errorf("expected X-MDDB-Webhook-ID header, got %q", receivedHeaders.Get("X-MDDB-Webhook-ID"))
	}
	if receivedHeaders.Get("X-Custom") != "test-value" {
		t.Errorf("expected custom header X-Custom, got %q", receivedHeaders.Get("X-Custom"))
	}
}

func TestFireAutomationWebhook_DefaultMethod(t *testing.T) {
	var receivedMethod string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
	}))
	defer ts.Close()

	webhook := &AutomationRule{ID: "wh1", URL: ts.URL} // no Method set
	trigger := &AutomationRule{ID: "tr1", Name: "Trig"}

	fireAutomationWebhook(webhook, trigger, nil, "blog", 50)

	if receivedMethod != "POST" {
		t.Errorf("expected default method POST, got %q", receivedMethod)
	}
}

func TestFireAutomationWebhook_NilDoc(t *testing.T) {
	var receivedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	defer ts.Close()

	webhook := &AutomationRule{ID: "wh1", URL: ts.URL, Method: "POST"}
	trigger := &AutomationRule{ID: "tr1", Name: "Trig"}

	fireAutomationWebhook(webhook, trigger, nil, "blog", 50)

	var payload TriggerPayload
	json.Unmarshal(receivedBody, &payload)
	if payload.Document != nil {
		t.Error("expected nil document in payload")
	}
}

// --- Cron Scheduler Tests ---

func TestCronScheduler_NewAndStop(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	cs := NewCronScheduler(s)
	cs.Start()
	cs.Stop()
}

func TestCronScheduler_Reload_Empty(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	cs := NewCronScheduler(s)
	cs.Start()
	defer cs.Stop()

	// Reload with no crons should not panic
	cs.Reload()
	if len(cs.entryMap) != 0 {
		t.Errorf("expected 0 entries, got %d", len(cs.entryMap))
	}
}

func TestCronScheduler_Reload_WithCrons(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	// Create webhook + trigger + cron
	wh, _ := s.AutomationManager.Create(AutomationRule{Type: "webhook", Name: "Hook", URL: "https://example.com", Enabled: true})
	tr, _ := s.AutomationManager.Create(AutomationRule{
		Type: "trigger", Name: "Trig", Collection: "blog",
		SearchType: "fts", Query: "test", Threshold: 50, WebhookID: wh.ID, Enabled: true,
	})
	s.AutomationManager.Create(AutomationRule{
		Type: "cron", Name: "Cron", Schedule: "0 0 * * * *",
		TriggerID: tr.ID, Enabled: true,
	})

	cs := NewCronScheduler(s)
	cs.Start()
	defer cs.Stop()

	cs.Reload()
	if len(cs.entryMap) != 1 {
		t.Errorf("expected 1 cron entry, got %d", len(cs.entryMap))
	}
}

func TestCronScheduler_Reload_DisabledCronSkipped(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	wh, _ := s.AutomationManager.Create(AutomationRule{Type: "webhook", Name: "Hook", URL: "https://example.com", Enabled: true})
	tr, _ := s.AutomationManager.Create(AutomationRule{
		Type: "trigger", Name: "Trig", Collection: "blog",
		SearchType: "fts", Query: "test", Threshold: 50, WebhookID: wh.ID, Enabled: true,
	})
	s.AutomationManager.Create(AutomationRule{
		Type: "cron", Name: "Disabled Cron", Schedule: "0 0 * * * *",
		TriggerID: tr.ID, Enabled: false,
	})

	cs := NewCronScheduler(s)
	cs.Start()
	defer cs.Stop()

	cs.Reload()
	if len(cs.entryMap) != 0 {
		t.Errorf("expected 0 entries for disabled cron, got %d", len(cs.entryMap))
	}
}

func TestCronScheduler_Reload_InvalidSchedule(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	wh, _ := s.AutomationManager.Create(AutomationRule{Type: "webhook", Name: "Hook", URL: "https://example.com", Enabled: true})
	tr, _ := s.AutomationManager.Create(AutomationRule{
		Type: "trigger", Name: "Trig", Collection: "blog",
		SearchType: "fts", Query: "test", Threshold: 50, WebhookID: wh.ID, Enabled: true,
	})

	// Manually insert a cron with invalid schedule (bypass validation)
	s.AutomationManager.mu.Lock()
	s.AutomationManager.rules = append(s.AutomationManager.rules, AutomationRule{
		ID: "bad-cron", Type: "cron", Name: "Bad", Enabled: true,
		Schedule: "not-a-schedule", TriggerID: tr.ID,
	})
	s.AutomationManager.mu.Unlock()

	cs := NewCronScheduler(s)
	cs.Start()
	defer cs.Stop()

	// Should not panic with invalid schedule
	cs.Reload()
	if len(cs.entryMap) != 0 {
		t.Errorf("expected 0 entries for invalid schedule, got %d", len(cs.entryMap))
	}
}

func TestCronScheduler_Reload_MissingTrigger(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	// Manually insert a cron with non-existent trigger
	s.AutomationManager.mu.Lock()
	s.AutomationManager.rules = append(s.AutomationManager.rules, AutomationRule{
		ID: "orphan-cron", Type: "cron", Name: "Orphan", Enabled: true,
		Schedule: "0 0 * * * *", TriggerID: "nonexistent",
	})
	s.AutomationManager.mu.Unlock()

	cs := NewCronScheduler(s)
	cs.Start()
	defer cs.Stop()

	cs.Reload()
	if len(cs.entryMap) != 0 {
		t.Errorf("expected 0 entries for missing trigger, got %d", len(cs.entryMap))
	}
}

func TestCronScheduler_ReloadClearsPrevious(t *testing.T) {
	s, cleanup := newTestServerForAutomation(t)
	defer cleanup()

	wh, _ := s.AutomationManager.Create(AutomationRule{Type: "webhook", Name: "Hook", URL: "https://example.com", Enabled: true})
	tr, _ := s.AutomationManager.Create(AutomationRule{
		Type: "trigger", Name: "Trig", Collection: "blog",
		SearchType: "fts", Query: "test", Threshold: 50, WebhookID: wh.ID, Enabled: true,
	})
	cr, _ := s.AutomationManager.Create(AutomationRule{
		Type: "cron", Name: "Cron", Schedule: "0 0 * * * *",
		TriggerID: tr.ID, Enabled: true,
	})

	cs := NewCronScheduler(s)
	cs.Start()
	defer cs.Stop()

	cs.Reload()
	if len(cs.entryMap) != 1 {
		t.Fatalf("expected 1 entry after first reload, got %d", len(cs.entryMap))
	}

	// Delete the cron and reload
	s.AutomationManager.Delete(cr.ID)
	cs.Reload()
	if len(cs.entryMap) != 0 {
		t.Errorf("expected 0 entries after deleting cron, got %d", len(cs.entryMap))
	}
}

// --- SetServer / SetBinlog Tests ---

func TestAutomationSetServer(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	if am.server != nil {
		t.Error("expected server to be nil initially")
	}

	s := &Server{}
	am.SetServer(s)
	if am.server != s {
		t.Error("expected server to be set")
	}
}

func TestAutomationSetBinlog(t *testing.T) {
	am, cleanup := setupAutomationTest(t)
	defer cleanup()

	if am.binlog != nil {
		t.Error("expected binlog to be nil initially")
	}

	// Create temp binlog
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	bl, err := NewBinlog(dbPath, BinlogConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer bl.Close()

	am.SetBinlog(bl)
	if am.binlog != bl {
		t.Error("expected binlog to be set")
	}
}

// --- Binlog Integration Tests ---

func TestAutomationCreateWithBinlog(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	bl, err := NewBinlog(dbPath, BinlogConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer bl.Close()

	am := NewAutomationManager(db)
	am.EnsureBucket()
	am.SetBinlog(bl)

	_, err = am.Create(AutomationRule{Type: "webhook", Name: "Hook", URL: "https://example.com"})
	if err != nil {
		t.Fatalf("create with binlog failed: %v", err)
	}
}

func TestAutomationDeleteWithBinlog(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	bl, err := NewBinlog(dbPath, BinlogConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer bl.Close()

	am := NewAutomationManager(db)
	am.EnsureBucket()
	am.SetBinlog(bl)

	wh, _ := am.Create(AutomationRule{Type: "webhook", Name: "Hook", URL: "https://example.com"})
	err = am.Delete(wh.ID)
	if err != nil {
		t.Fatalf("delete with binlog failed: %v", err)
	}
}

// --- TriggerPayload Serialization ---

func TestTriggerPayloadJSON(t *testing.T) {
	payload := TriggerPayload{
		Event:      "trigger.matched",
		Trigger:    TriggerPayloadTrigger{ID: "tr1", Name: "Test"},
		Collection: "blog",
		Score:      85.5,
		Timestamp:  1700000000,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	if !bytes.Contains(data, []byte(`"trigger.matched"`)) {
		t.Error("expected 'trigger.matched' in JSON")
	}
	if !bytes.Contains(data, []byte(`"blog"`)) {
		t.Error("expected 'blog' in JSON")
	}
}

// Prevent unused import
var _ = os.TempDir
