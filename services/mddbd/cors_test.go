package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseCORSOrigins_Wildcard(t *testing.T) {
	for _, raw := range []string{"", "*", "  *  ", "   "} {
		cfg := parseCORSOrigins(raw)
		if !cfg.wildcard {
			t.Errorf("parseCORSOrigins(%q): expected wildcard", raw)
		}
	}
}

func TestParseCORSOrigins_Allowlist(t *testing.T) {
	cfg := parseCORSOrigins("https://a.example, https://b.example ,, https://c.example")
	if cfg.wildcard {
		t.Fatal("expected non-wildcard allowlist")
	}
	for _, o := range []string{"https://a.example", "https://b.example", "https://c.example"} {
		if !cfg.allowed[o] {
			t.Errorf("expected %q allowed", o)
		}
	}
	if cfg.allowed[""] {
		t.Error("empty entry must not be allowed")
	}
	if len(cfg.allowed) != 3 {
		t.Errorf("expected 3 allowed origins, got %d", len(cfg.allowed))
	}
}

func TestCORSApplyOrigin_Wildcard(t *testing.T) {
	cfg := parseCORSOrigins("*")
	rec := httptest.NewRecorder()
	cfg.applyOrigin(rec, "https://evil.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("wildcard: expected *, got %q", got)
	}
}

func TestCORSApplyOrigin_AllowedMatch(t *testing.T) {
	cfg := parseCORSOrigins("https://ok.example")
	rec := httptest.NewRecorder()
	cfg.applyOrigin(rec, "https://ok.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://ok.example" {
		t.Errorf("expected echo of allowed origin, got %q", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("expected Vary: Origin, got %q", got)
	}
}

func TestCORSApplyOrigin_DisallowedGetsNoHeader(t *testing.T) {
	cfg := parseCORSOrigins("https://ok.example")
	rec := httptest.NewRecorder()
	cfg.applyOrigin(rec, "https://evil.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("disallowed origin must get NO Allow-Origin header, got %q", got)
	}
}

func TestCORSApplyOrigin_AllowlistNoOriginHeader(t *testing.T) {
	cfg := parseCORSOrigins("https://ok.example")
	rec := httptest.NewRecorder()
	cfg.applyOrigin(rec, "") // non-browser / same-origin request
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("no Origin header => no Allow-Origin, got %q", got)
	}
}

func TestEnvCORSConfig_PrefersOriginsOverLegacy(t *testing.T) {
	t.Setenv("MDDB_CORS_ORIGINS", "https://new.example")
	t.Setenv("MDDB_CORS_ORIGIN", "https://legacy.example")
	cfg := envCORSConfig()
	if cfg.wildcard || !cfg.allowed["https://new.example"] || cfg.allowed["https://legacy.example"] {
		t.Error("MDDB_CORS_ORIGINS must take precedence over MDDB_CORS_ORIGIN")
	}
}

func TestEnvCORSConfig_LegacyFallback(t *testing.T) {
	t.Setenv("MDDB_CORS_ORIGINS", "")
	t.Setenv("MDDB_CORS_ORIGIN", "https://legacy.example")
	cfg := envCORSConfig()
	if cfg.wildcard || !cfg.allowed["https://legacy.example"] {
		t.Error("expected legacy MDDB_CORS_ORIGIN fallback")
	}
}

// TestWithCORS_DisallowedOriginIntegration drives the actual withCORS middleware.
func TestWithCORS_DisallowedOriginIntegration(t *testing.T) {
	t.Setenv("MDDB_CORS_ORIGINS", "https://ok.example")
	t.Setenv("MDDB_CORS_ORIGIN", "")
	h := withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Disallowed origin: no Allow-Origin header.
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("evil origin must not be reflected, got %q", got)
	}

	// Allowed origin: echoed + Vary.
	req2 := httptest.NewRequest(http.MethodOptions, "/v1/health", nil)
	req2.Header.Set("Origin", "https://ok.example")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if got := rec2.Header().Get("Access-Control-Allow-Origin"); got != "https://ok.example" {
		t.Errorf("allowed origin should be echoed, got %q", got)
	}
	if rec2.Code != http.StatusNoContent {
		t.Errorf("OPTIONS preflight should be 204, got %d", rec2.Code)
	}
}
