package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- HTTP Handler Tests for embedding config endpoints ----

func TestHandleEmbeddingConfigs_List(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	// Save a couple of configs
	_ = s.SaveEmbeddingConfig(&EmbeddingConfig{
		ID: "cfg-a", Name: "A", Provider: "openai", Model: "ada", Dimensions: 1536,
	})
	_ = s.SaveEmbeddingConfig(&EmbeddingConfig{
		ID: "cfg-b", Name: "B", Provider: "ollama", Model: "nomic", Dimensions: 768,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/embedding-configs", nil)
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	configs, ok := resp["configs"].([]interface{})
	if !ok {
		t.Fatal("expected configs array in response")
	}
	if len(configs) != 2 {
		t.Errorf("expected 2 configs, got %d", len(configs))
	}
}

func TestHandleEmbeddingConfigs_Create(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	body := `{"id":"cfg-new","name":"New Config","provider":"openai","model":"text-embedding-3-small","dimensions":1536,"apiKey":"sk-test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/embedding-configs", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigs(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var config EmbeddingConfig
	if err := json.NewDecoder(w.Body).Decode(&config); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if config.ID != "cfg-new" {
		t.Errorf("ID = %q, want %q", config.ID, "cfg-new")
	}
	if config.CreatedAt == 0 {
		t.Error("CreatedAt should be set")
	}
}

func TestHandleEmbeddingConfigs_CreateMissingFields(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	// Missing required fields (no name, provider, model)
	body := `{"id":"cfg-bad"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/embedding-configs", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleEmbeddingConfigs_CreateInvalidProvider(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	body := `{"id":"cfg-bad","name":"Bad","provider":"invalid","model":"m","dimensions":100}`
	req := httptest.NewRequest(http.MethodPost, "/v1/embedding-configs", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleEmbeddingConfigs_CreateInvalidDimensions(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	body := `{"id":"cfg-bad","name":"Bad","provider":"openai","model":"m","dimensions":0}`
	req := httptest.NewRequest(http.MethodPost, "/v1/embedding-configs", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleEmbeddingConfigs_CreateInvalidJSON(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v1/embedding-configs", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleEmbeddingConfigs_MethodNotAllowed(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/v1/embedding-configs", nil)
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigs(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", w.Code, w.Body.String())
	}
}

// ---- Detail handler tests ----

func TestHandleEmbeddingConfigDetail_Get(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	_ = s.SaveEmbeddingConfig(&EmbeddingConfig{
		ID: "cfg-get", Name: "Get Me", Provider: "openai", Model: "ada", Dimensions: 1536,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/embedding-configs/cfg-get", nil)
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigDetail(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var config EmbeddingConfig
	if err := json.NewDecoder(w.Body).Decode(&config); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if config.ID != "cfg-get" {
		t.Errorf("ID = %q, want %q", config.ID, "cfg-get")
	}
}

func TestHandleEmbeddingConfigDetail_GetNotFound(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/v1/embedding-configs/nonexistent", nil)
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleEmbeddingConfigDetail_Update(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	_ = s.SaveEmbeddingConfig(&EmbeddingConfig{
		ID: "cfg-upd", Name: "Original", Provider: "openai", Model: "ada", Dimensions: 1536,
	})

	body := `{"name":"Updated","provider":"ollama","model":"nomic","dimensions":768,"apiUrl":"http://localhost:11434"}`
	req := httptest.NewRequest(http.MethodPut, "/v1/embedding-configs/cfg-upd", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigDetail(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var config EmbeddingConfig
	if err := json.NewDecoder(w.Body).Decode(&config); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if config.ID != "cfg-upd" {
		t.Errorf("ID should be forced to path param, got %q", config.ID)
	}
	if config.Name != "Updated" {
		t.Errorf("Name = %q, want %q", config.Name, "Updated")
	}
}

func TestHandleEmbeddingConfigDetail_UpdateInvalidProvider(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	_ = s.SaveEmbeddingConfig(&EmbeddingConfig{
		ID: "cfg-upd2", Name: "Test", Provider: "openai", Model: "ada", Dimensions: 1536,
	})

	body := `{"name":"Test","provider":"invalid","model":"m","dimensions":100}`
	req := httptest.NewRequest(http.MethodPut, "/v1/embedding-configs/cfg-upd2", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigDetail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleEmbeddingConfigDetail_UpdateInvalidDimensions(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	body := `{"name":"Test","provider":"openai","model":"m","dimensions":-1}`
	req := httptest.NewRequest(http.MethodPut, "/v1/embedding-configs/cfg-upd3", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigDetail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleEmbeddingConfigDetail_UpdateInvalidJSON(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/v1/embedding-configs/cfg-x", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigDetail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleEmbeddingConfigDetail_Delete(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	_ = s.SaveEmbeddingConfig(&EmbeddingConfig{
		ID: "cfg-del", Name: "Delete Me", Provider: "openai", Model: "ada", Dimensions: 1536,
		IsDefault: false,
	})

	req := httptest.NewRequest(http.MethodDelete, "/v1/embedding-configs/cfg-del", nil)
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigDetail(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify deleted
	_, err := s.GetEmbeddingConfig("cfg-del")
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func TestHandleEmbeddingConfigDetail_DeleteDefault(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	_ = s.SaveEmbeddingConfig(&EmbeddingConfig{
		ID: "cfg-def", Name: "Default", Provider: "openai", Model: "ada", Dimensions: 1536,
		IsDefault: true,
	})

	req := httptest.NewRequest(http.MethodDelete, "/v1/embedding-configs/cfg-def", nil)
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigDetail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for deleting default, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleEmbeddingConfigDetail_DeleteNotFound(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/v1/embedding-configs/nonexistent", nil)
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleEmbeddingConfigDetail_MethodNotAllowed(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPatch, "/v1/embedding-configs/cfg-x", nil)
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigDetail(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleEmbeddingConfigDetail_InvalidPath(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	// Path too short to extract ID
	req := httptest.NewRequest(http.MethodGet, "/v1/", nil)
	w := httptest.NewRecorder()
	s.handleEmbeddingConfigDetail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ---- Set Default Handler ----

func TestHandleSetDefaultEmbeddingConfig_Success(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	_ = s.SaveEmbeddingConfig(&EmbeddingConfig{
		ID: "cfg-1", Name: "Config 1", Provider: "openai", Model: "ada", Dimensions: 1536,
	})

	body := `{"id":"cfg-1"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/embedding-configs/default", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSetDefaultEmbeddingConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify default was set
	got, err := s.GetEmbeddingConfig("cfg-1")
	if err != nil {
		t.Fatalf("GetEmbeddingConfig: %v", err)
	}
	if !got.IsDefault {
		t.Error("config should be set as default")
	}
}

func TestHandleSetDefaultEmbeddingConfig_NotFound(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	body := `{"id":"nonexistent"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/embedding-configs/default", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSetDefaultEmbeddingConfig(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSetDefaultEmbeddingConfig_InvalidJSON(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v1/embedding-configs/default", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	s.handleSetDefaultEmbeddingConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSetDefaultEmbeddingConfig_MethodNotAllowed(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/v1/embedding-configs/default", nil)
	w := httptest.NewRecorder()
	s.handleSetDefaultEmbeddingConfig(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", w.Code, w.Body.String())
	}
}

// ---- InitializeEmbeddingFromConfig with known providers ----

func TestInitializeEmbeddingFromConfig_OpenAI(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	config := &EmbeddingConfig{
		ID:         "cfg-openai",
		Provider:   "openai",
		Model:      "text-embedding-3-small",
		Dimensions: 1536,
		APIKey:     "sk-test",
	}
	s.InitializeEmbeddingFromConfig(config)

	if s.Embedding == nil {
		t.Fatal("Embedding should not be nil for openai provider")
	}
	if s.Embedding.Model() != "text-embedding-3-small" {
		t.Errorf("Model = %q, want %q", s.Embedding.Model(), "text-embedding-3-small")
	}
	if s.Embedding.Dimensions() != 1536 {
		t.Errorf("Dimensions = %d, want 1536", s.Embedding.Dimensions())
	}

	// Cleanup worker
	if s.EmbeddingWorker != nil {
		s.EmbeddingWorker.Stop()
	}
}

func TestInitializeEmbeddingFromConfig_Ollama(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	config := &EmbeddingConfig{
		ID:         "cfg-ollama",
		Provider:   "ollama",
		Model:      "nomic-embed-text",
		Dimensions: 768,
		APIURL:     "http://localhost:11434",
	}
	s.InitializeEmbeddingFromConfig(config)

	if s.Embedding == nil {
		t.Fatal("Embedding should not be nil for ollama provider")
	}
	if s.Embedding.Model() != "nomic-embed-text" {
		t.Errorf("Model = %q, want %q", s.Embedding.Model(), "nomic-embed-text")
	}

	if s.EmbeddingWorker != nil {
		s.EmbeddingWorker.Stop()
	}
}

func TestInitializeEmbeddingFromConfig_Cohere(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	config := &EmbeddingConfig{
		ID:         "cfg-cohere",
		Provider:   "cohere",
		Model:      "embed-english-v3.0",
		Dimensions: 1024,
		APIKey:     "co-test",
	}
	s.InitializeEmbeddingFromConfig(config)

	if s.Embedding == nil {
		t.Fatal("Embedding should not be nil for cohere provider")
	}
	if s.Embedding.Model() != "embed-english-v3.0" {
		t.Errorf("Model = %q, want %q", s.Embedding.Model(), "embed-english-v3.0")
	}

	if s.EmbeddingWorker != nil {
		s.EmbeddingWorker.Stop()
	}
}

func TestInitializeEmbeddingFromConfig_Voyage(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	config := &EmbeddingConfig{
		ID:         "cfg-voyage",
		Provider:   "voyage",
		Model:      "voyage-3",
		Dimensions: 1024,
		APIKey:     "vo-test",
	}
	s.InitializeEmbeddingFromConfig(config)

	if s.Embedding == nil {
		t.Fatal("Embedding should not be nil for voyage provider")
	}
	if s.Embedding.Model() != "voyage-3" {
		t.Errorf("Model = %q, want %q", s.Embedding.Model(), "voyage-3")
	}

	if s.EmbeddingWorker != nil {
		s.EmbeddingWorker.Stop()
	}
}

// ---- Create with all valid providers ----

func TestHandleEmbeddingConfigs_CreateAllProviders(t *testing.T) {
	s, cleanup := newTestServerForEmbeddingConfig(t)
	defer cleanup()

	providers := []string{"openai", "ollama", "cohere", "voyage"}
	for _, provider := range providers {
		body := `{"id":"cfg-` + provider + `","name":"` + provider + `","provider":"` + provider + `","model":"m","dimensions":100}`
		req := httptest.NewRequest(http.MethodPost, "/v1/embedding-configs", strings.NewReader(body))
		w := httptest.NewRecorder()
		s.handleEmbeddingConfigs(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("provider %q: expected 201, got %d: %s", provider, w.Code, w.Body.String())
		}
	}
}
