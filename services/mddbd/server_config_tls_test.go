package main

import (
	"net/http"
	"testing"
)

func TestTLSConfigDefaults(t *testing.T) {
	cfg := defaultServerConfig()

	if cfg.TLS.Enabled {
		t.Error("TLS should be disabled by default")
	}
	if cfg.TLS.CertFile != "" {
		t.Error("TLS CertFile should be empty by default")
	}
	if cfg.TLS.KeyFile != "" {
		t.Error("TLS KeyFile should be empty by default")
	}
}

func TestTLSConfigFileMerge(t *testing.T) {
	cfg := defaultServerConfig()

	enabled := true
	cert := "/path/to/cert.pem"
	key := "/path/to/key.pem"
	fc := &fileConfig{
		TLS: &fileTLS{
			Enabled:  &enabled,
			CertFile: &cert,
			KeyFile:  &key,
		},
	}

	result := mergeFileConfig(cfg, fc)

	if !result.TLS.Enabled {
		t.Error("TLS should be enabled after merge")
	}
	if result.TLS.CertFile != cert {
		t.Errorf("CertFile: got %q, want %q", result.TLS.CertFile, cert)
	}
	if result.TLS.KeyFile != key {
		t.Errorf("KeyFile: got %q, want %q", result.TLS.KeyFile, key)
	}
}

func TestTLSConfigEnvVars(t *testing.T) {
	cfg := defaultServerConfig()

	t.Setenv("MDDB_TLS_ENABLED", "true")
	t.Setenv("MDDB_TLS_CERT", "/tmp/cert.pem")
	t.Setenv("MDDB_TLS_KEY", "/tmp/key.pem")

	applyEnvConfig(&cfg)

	if !cfg.TLS.Enabled {
		t.Error("TLS should be enabled via env")
	}
	if cfg.TLS.CertFile != "/tmp/cert.pem" {
		t.Errorf("CertFile: got %q", cfg.TLS.CertFile)
	}
	if cfg.TLS.KeyFile != "/tmp/key.pem" {
		t.Errorf("KeyFile: got %q", cfg.TLS.KeyFile)
	}
}

func TestPprofRegistration(t *testing.T) {
	mux := http.NewServeMux()
	registerPprof(mux)

	// Verify endpoints are registered by checking we get a handler
	// (ServeMux returns a default 404 handler, but registered routes return their handler)
	paths := []string{
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/profile",
		"/debug/pprof/symbol",
		"/debug/pprof/trace",
	}

	for _, path := range paths {
		req, _ := http.NewRequest("GET", path, nil)
		handler, pattern := mux.Handler(req)
		if handler == nil || pattern == "" {
			t.Errorf("no handler registered for %s", path)
		}
	}
}
