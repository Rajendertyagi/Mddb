package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	json "github.com/goccy/go-json"
)

// ---------- 1. handleConfig - GET success ----------

func TestHandleConfig_Success(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
	s.handleConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var resp ConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.DatabasePath != s.Path {
		t.Errorf("expected database path %q, got %q", s.Path, resp.DatabasePath)
	}
	if resp.Mode != string(s.Mode) {
		t.Errorf("expected mode %q, got %q", s.Mode, resp.Mode)
	}
}

// ---------- 2. handleConfig - wrong method ----------

func TestHandleConfig_MethodNotAllowed(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete}
	for _, method := range methods {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/v1/config", nil)
		s.handleConfig(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: expected 405, got %d", method, rec.Code)
		}
	}
}

// ---------- 3. handleConfig - default addresses ----------

func TestHandleConfig_DefaultAddresses(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	// Ensure env vars are not set so defaults are used
	_ = os.Unsetenv("MDDB_ADDR")
	_ = os.Unsetenv("MDDB_GRPC_ADDR")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
	s.handleConfig(rec, req)

	var resp ConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.HTTPAddr != ":11023" {
		t.Errorf("expected default HTTPAddr :11023, got %q", resp.HTTPAddr)
	}
	if resp.GRPCAddr != ":11024" {
		t.Errorf("expected default GRPCAddr :11024, got %q", resp.GRPCAddr)
	}
}

// ---------- 4. handleConfig - no vector config when provider unset ----------

func TestHandleConfig_NoVectorConfig(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	_ = os.Unsetenv("MDDB_EMBEDDING_PROVIDER")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
	s.handleConfig(rec, req)

	var resp ConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.VectorConfig != nil {
		t.Errorf("expected nil VectorConfig when no provider set, got %+v", resp.VectorConfig)
	}
}

// ---------- 5. handleConfig - vector config with openai provider ----------

func TestHandleConfig_VectorConfigOpenAI(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	// Set embedding provider to openai
	_ = os.Setenv("MDDB_EMBEDDING_PROVIDER", "openai")
	defer func() { _ = os.Unsetenv("MDDB_EMBEDDING_PROVIDER") }()
	_ = os.Unsetenv("MDDB_EMBEDDING_API_URL")
	_ = os.Unsetenv("MDDB_EMBEDDING_MODEL")
	_ = os.Unsetenv("MDDB_EMBEDDING_DIMENSIONS")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
	s.handleConfig(rec, req)

	var resp ConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.VectorConfig == nil {
		t.Fatal("expected non-nil VectorConfig for openai provider")
	}
	if !resp.VectorConfig.Enabled {
		t.Error("expected VectorConfig.Enabled=true")
	}
	if resp.VectorConfig.Provider != "openai" {
		t.Errorf("expected provider openai, got %q", resp.VectorConfig.Provider)
	}
	if resp.VectorConfig.Model != "text-embedding-3-small" {
		t.Errorf("expected default model text-embedding-3-small, got %q", resp.VectorConfig.Model)
	}
	if resp.VectorConfig.Dimensions != 1536 {
		t.Errorf("expected dimensions 1536, got %d", resp.VectorConfig.Dimensions)
	}
}

// ---------- 6. handleConfig - vector config with ollama provider ----------

func TestHandleConfig_VectorConfigOllama(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	_ = os.Setenv("MDDB_EMBEDDING_PROVIDER", "ollama")
	defer func() { _ = os.Unsetenv("MDDB_EMBEDDING_PROVIDER") }()
	_ = os.Unsetenv("MDDB_EMBEDDING_API_URL")
	_ = os.Unsetenv("MDDB_EMBEDDING_MODEL")
	_ = os.Unsetenv("MDDB_EMBEDDING_DIMENSIONS")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
	s.handleConfig(rec, req)

	var resp ConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.VectorConfig == nil {
		t.Fatal("expected non-nil VectorConfig for ollama provider")
	}
	if resp.VectorConfig.Provider != "ollama" {
		t.Errorf("expected provider ollama, got %q", resp.VectorConfig.Provider)
	}
	if resp.VectorConfig.Model != "nomic-embed-text" {
		t.Errorf("expected default model nomic-embed-text, got %q", resp.VectorConfig.Model)
	}
	if resp.VectorConfig.Dimensions != 768 {
		t.Errorf("expected dimensions 768, got %d", resp.VectorConfig.Dimensions)
	}
}

// ---------- 7. handleConfig - vector config with voyage provider ----------

func TestHandleConfig_VectorConfigVoyage(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	_ = os.Setenv("MDDB_EMBEDDING_PROVIDER", "voyage")
	defer func() { _ = os.Unsetenv("MDDB_EMBEDDING_PROVIDER") }()
	_ = os.Unsetenv("MDDB_EMBEDDING_API_URL")
	_ = os.Unsetenv("MDDB_EMBEDDING_MODEL")
	_ = os.Unsetenv("MDDB_EMBEDDING_DIMENSIONS")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
	s.handleConfig(rec, req)

	var resp ConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.VectorConfig == nil {
		t.Fatal("expected non-nil VectorConfig for voyage provider")
	}
	if resp.VectorConfig.Provider != "voyage" {
		t.Errorf("expected provider voyage, got %q", resp.VectorConfig.Provider)
	}
	if resp.VectorConfig.Model != "voyage-3" {
		t.Errorf("expected default model voyage-3, got %q", resp.VectorConfig.Model)
	}
	if resp.VectorConfig.Dimensions != 1024 {
		t.Errorf("expected dimensions 1024, got %d", resp.VectorConfig.Dimensions)
	}
}

// ---------- 8. handleConfig - extreme mode disabled ----------

func TestHandleConfig_ExtremeModeDisabled(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	s.UseExtreme = false

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
	s.handleConfig(rec, req)

	var resp ConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.ExtremeMode {
		t.Error("expected ExtremeMode=false")
	}
	if resp.HTTP3Addr != "" {
		t.Errorf("expected empty HTTP3Addr when extreme mode disabled, got %q", resp.HTTP3Addr)
	}
}

// ---------- 9. handleConfig - extreme mode enabled ----------

func TestHandleConfig_ExtremeModeEnabled(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	s.UseExtreme = true

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
	s.handleConfig(rec, req)

	var resp ConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !resp.ExtremeMode {
		t.Error("expected ExtremeMode=true")
	}
	if resp.HTTP3Addr == "" {
		t.Error("expected non-empty HTTP3Addr when extreme mode enabled")
	}
}

// ---------- 10. handleConfig - auth not enabled by default ----------

func TestHandleConfig_AuthDisabledByDefault(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()

	_ = os.Unsetenv("MDDB_AUTH_ENABLED")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
	s.handleConfig(rec, req)

	var resp ConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.AuthEnabled {
		t.Error("expected AuthEnabled=false by default")
	}
}
