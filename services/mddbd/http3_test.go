package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Test: generateTLSConfig
// ---------------------------------------------------------------------------

func TestGenerateTLSConfig(t *testing.T) {
	tlsConfig, err := generateTLSConfig()
	if err != nil {
		t.Fatalf("generateTLSConfig: %v", err)
	}
	if tlsConfig == nil {
		t.Fatal("expected non-nil TLS config")
		return
	}
	if len(tlsConfig.Certificates) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(tlsConfig.Certificates))
	}
	if len(tlsConfig.NextProtos) != 1 || tlsConfig.NextProtos[0] != "h3" {
		t.Errorf("expected NextProtos=[h3], got %v", tlsConfig.NextProtos)
	}
}

// ---------------------------------------------------------------------------
// Test: NewHTTP3Server
// ---------------------------------------------------------------------------

func TestNewHTTP3Server(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	h3, err := NewHTTP3Server(":0", handler)
	if err != nil {
		t.Fatalf("NewHTTP3Server: %v", err)
	}
	if h3 == nil {
		t.Fatal("expected non-nil HTTP3Server")
		return
	}
	if h3.server == nil {
		t.Error("expected non-nil internal server")
	}
	if h3.handler == nil {
		t.Error("expected non-nil handler")
	}
	if h3.addr != ":0" {
		t.Errorf("expected addr=:0, got %s", h3.addr)
	}
}

// ---------------------------------------------------------------------------
// Test: HTTP3Server Close without Start
// ---------------------------------------------------------------------------

func TestHTTP3Server_CloseWithoutStart(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h3, err := NewHTTP3Server(":0", handler)
	if err != nil {
		t.Fatalf("NewHTTP3Server: %v", err)
	}

	// Closing a server that was never started should not panic
	err = h3.Close()
	if err != nil {
		t.Logf("Close returned error (may be expected): %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test: HTTP3Middleware
// ---------------------------------------------------------------------------

func TestHTTP3Middleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := HTTP3Middleware(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Check Alt-Svc header
	altSvc := w.Header().Get("Alt-Svc")
	if altSvc == "" {
		t.Error("expected Alt-Svc header to be set")
	}
	if altSvc != `h3=":443"; ma=2592000` {
		t.Errorf("unexpected Alt-Svc value: %s", altSvc)
	}

	// Check X-Protocol header
	xProto := w.Header().Get("X-Protocol")
	if xProto == "" {
		t.Error("expected X-Protocol header to be set")
	}
}

func TestHTTP3Middleware_PreservesNextHandler(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("X-Custom", "test")
		w.WriteHeader(http.StatusCreated)
	})

	wrapped := HTTP3Middleware(inner)

	req := httptest.NewRequest("POST", "/data", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if !called {
		t.Error("expected inner handler to be called")
	}
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	if w.Header().Get("X-Custom") != "test" {
		t.Error("expected custom header from inner handler")
	}
}

// ---------------------------------------------------------------------------
// Test: generateTLSConfig produces valid cert
// ---------------------------------------------------------------------------

func TestGenerateTLSConfig_ValidCert(t *testing.T) {
	tlsConfig, err := generateTLSConfig()
	if err != nil {
		t.Fatalf("generateTLSConfig: %v", err)
	}

	cert := tlsConfig.Certificates[0]
	if len(cert.Certificate) == 0 {
		t.Error("expected non-empty certificate chain")
	}
	if cert.PrivateKey == nil {
		t.Error("expected non-nil private key")
	}
}
