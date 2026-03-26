package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---- API Key Auth Tests ----

func TestMCPAPIKeyMiddlewareDisabled(t *testing.T) {
	m := &MCPAPIKeyMiddleware{enabled: false}
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/mcp", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("disabled middleware should pass through, got %d", w.Code)
	}
}

func TestMCPAPIKeyMiddlewareMissingKey(t *testing.T) {
	m := &MCPAPIKeyMiddleware{enabled: true, keys: map[string]string{"sk-test123": "test"}}
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/mcp", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing key, got %d", w.Code)
	}
}

func TestMCPAPIKeyMiddlewareInvalidKey(t *testing.T) {
	m := &MCPAPIKeyMiddleware{enabled: true, keys: map[string]string{"sk-test123": "test"}}
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("X-API-Key", "sk-wrong")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for invalid key, got %d", w.Code)
	}
}

func TestMCPAPIKeyMiddlewareValidKey(t *testing.T) {
	m := &MCPAPIKeyMiddleware{enabled: true, keys: map[string]string{"sk-test123": "myapp"}}
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.Header.Get("X-MCP-Key-Name")
		if name != "myapp" {
			t.Errorf("expected key name 'myapp', got %q", name)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("X-API-Key", "sk-test123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid key, got %d", w.Code)
	}
}

func TestMCPAPIKeyMiddlewareBearerToken(t *testing.T) {
	m := &MCPAPIKeyMiddleware{enabled: true, keys: map[string]string{"sk-bearer-key": "bearer-test"}}
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer sk-bearer-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for Bearer token, got %d", w.Code)
	}
}

func TestMCPAPIKeyMiddlewareQueryParam(t *testing.T) {
	m := &MCPAPIKeyMiddleware{enabled: true, keys: map[string]string{"sk-query-key": "query-test"}}
	handler := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/sse?api_key=sk-query-key", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for query param key, got %d", w.Code)
	}
}

// ---- Rate Limiter Tests ----

func TestMCPRateLimiterDisabled(t *testing.T) {
	rl := &MCPRateLimiter{enabled: false}
	handler := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/mcp", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("disabled rate limiter should pass through, got %d", w.Code)
	}
}

func TestMCPRateLimiterAllowsWithinLimit(t *testing.T) {
	rl := &MCPRateLimiter{
		enabled: true,
		limit:   5,
		window:  60_000_000_000, // 60s in nanoseconds
		burst:   2,
		by:      "ip",
		clients: make(map[string]*rateLimitBucket),
	}
	handler := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 7 requests should pass (limit=5, burst=2)
	for i := 0; i < 7; i++ {
		req := httptest.NewRequest("POST", "/mcp", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("request %d should pass (within limit+burst), got %d", i+1, w.Code)
		}
	}

	// 8th request should be rate limited
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 after limit exceeded, got %d", w.Code)
	}

	// Check rate limit headers
	if w.Header().Get("X-RateLimit-Limit") != "5" {
		t.Errorf("expected X-RateLimit-Limit=5, got %s", w.Header().Get("X-RateLimit-Limit"))
	}
	if w.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("expected X-RateLimit-Remaining=0, got %s", w.Header().Get("X-RateLimit-Remaining"))
	}
}

func TestMCPRateLimiterDifferentClients(t *testing.T) {
	rl := &MCPRateLimiter{
		enabled: true,
		limit:   1,
		window:  60_000_000_000,
		burst:   0,
		by:      "ip",
		clients: make(map[string]*rateLimitBucket),
	}
	handler := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Client A: 1 request OK
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Error("client A first request should pass")
	}

	// Client A: 2nd request blocked
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Error("client A second request should be blocked")
	}

	// Client B: separate limit, should pass
	req2 := httptest.NewRequest("POST", "/mcp", nil)
	req2.RemoteAddr = "10.0.0.2:1234"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req2)
	if w.Code != http.StatusOK {
		t.Error("client B first request should pass (separate limit)")
	}
}

// ---- Request Logger Tests ----

func TestMCPRequestLoggerDisabled(t *testing.T) {
	rl := &MCPRequestLogger{enabled: false}
	handler := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/mcp", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("disabled logger should pass through, got %d", w.Code)
	}
}

func TestMCPRequestLoggerCaptures(t *testing.T) {
	rl := &MCPRequestLogger{enabled: true, level: "info"}
	handler := rl.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("POST", "/mcp", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestStatusWriter(t *testing.T) {
	w := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: w, status: 200}
	sw.WriteHeader(http.StatusNotFound)
	if sw.status != http.StatusNotFound {
		t.Errorf("expected 404, got %d", sw.status)
	}
}

func TestExtractAPIKey(t *testing.T) {
	tests := []struct {
		name, header, value, want string
	}{
		{"X-API-Key", "X-API-Key", "sk-123", "sk-123"},
		{"Bearer", "Authorization", "Bearer sk-456", "sk-456"},
		{"empty", "", "", ""},
	}
	for _, tt := range tests {
		req := httptest.NewRequest("POST", "/mcp", nil)
		if tt.header != "" {
			req.Header.Set(tt.header, tt.value)
		}
		got := extractAPIKey(req)
		if got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}

	// Query param
	req := httptest.NewRequest("GET", "/sse?api_key=sk-789", nil)
	if got := extractAPIKey(req); got != "sk-789" {
		t.Errorf("query param: got %q, want sk-789", got)
	}
}
