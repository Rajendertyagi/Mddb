package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultServerConfig(t *testing.T) {
	cfg := defaultServerConfig()

	if !cfg.HTTP.Enabled {
		t.Error("HTTP should be enabled by default")
	}
	if cfg.HTTP.Addr != ":11023" {
		t.Errorf("expected HTTP addr :11023, got %q", cfg.HTTP.Addr)
	}
	if !cfg.GRPC.Enabled {
		t.Error("gRPC should be enabled by default")
	}
	if cfg.GRPC.Addr != ":11024" {
		t.Errorf("expected gRPC addr :11024, got %q", cfg.GRPC.Addr)
	}
	if !cfg.MCP.Enabled {
		t.Error("MCP should be enabled by default")
	}
	if cfg.MCP.Addr != ":9000" {
		t.Errorf("expected MCP addr :9000, got %q", cfg.MCP.Addr)
	}
	if cfg.MCP.Stdio {
		t.Error("MCP stdio should be false by default")
	}
	if cfg.HTTP3.Enabled {
		t.Error("HTTP/3 should be disabled by default")
	}
	if cfg.HTTP3.Addr != ":11443" {
		t.Errorf("expected HTTP/3 addr :11443, got %q", cfg.HTTP3.Addr)
	}
}

func TestApplyEnvConfig(t *testing.T) {
	cfg := defaultServerConfig()

	// Set env vars
	t.Setenv("MDDB_HTTP_ENABLED", "false")
	t.Setenv("MDDB_GRPC_ENABLED", "false")
	t.Setenv("MDDB_MCP_ENABLED", "false")
	t.Setenv("MDDB_MCP_PORT", "9000")
	t.Setenv("MDDB_HTTP_PORT", "8080")
	t.Setenv("MDDB_GRPC_PORT", "50051")

	applyEnvConfig(&cfg)

	if cfg.HTTP.Enabled {
		t.Error("HTTP should be disabled")
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("expected HTTP addr :8080, got %q", cfg.HTTP.Addr)
	}
	if cfg.GRPC.Enabled {
		t.Error("gRPC should be disabled")
	}
	if cfg.GRPC.Addr != ":50051" {
		t.Errorf("expected gRPC addr :50051, got %q", cfg.GRPC.Addr)
	}
	if cfg.MCP.Enabled {
		t.Error("MCP should be disabled")
	}
	if cfg.MCP.Addr != ":9000" {
		t.Errorf("expected MCP addr :9000, got %q", cfg.MCP.Addr)
	}
}

func TestApplyEnvConfig_LegacyAddr(t *testing.T) {
	cfg := defaultServerConfig()

	t.Setenv("MDDB_ADDR", ":9999")
	t.Setenv("MDDB_GRPC_ADDR", ":8888")

	applyEnvConfig(&cfg)

	if cfg.HTTP.Addr != ":9999" {
		t.Errorf("expected HTTP addr :9999 from MDDB_ADDR, got %q", cfg.HTTP.Addr)
	}
	if cfg.GRPC.Addr != ":8888" {
		t.Errorf("expected gRPC addr :8888, got %q", cfg.GRPC.Addr)
	}
}

func TestApplyEnvConfig_HTTPAddrOverridesLegacy(t *testing.T) {
	cfg := defaultServerConfig()

	// Both legacy and new are set — new wins because it's applied after
	t.Setenv("MDDB_ADDR", ":9999")
	t.Setenv("MDDB_HTTP_ADDR", ":7777")

	applyEnvConfig(&cfg)

	if cfg.HTTP.Addr != ":7777" {
		t.Errorf("MDDB_HTTP_ADDR should override MDDB_ADDR, got %q", cfg.HTTP.Addr)
	}
}

func TestPortToAddr(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"9000", ":9000"},
		{":9000", ":9000"},
		{"", ""},
	}
	for _, tc := range tests {
		got := portToAddr(tc.input)
		if got != tc.expected {
			t.Errorf("portToAddr(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestLoadConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "mddb.yaml")

	content := `
http:
  enabled: false
  addr: ":8080"
grpc:
  enabled: true
  addr: ":50051"
mcp:
  enabled: false
  addr: ":9000"
  stdio: true
`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	fc, err := loadConfigFile(cfgFile)
	if err != nil {
		t.Fatal(err)
	}

	cfg := defaultServerConfig()
	cfg = mergeFileConfig(cfg, fc)

	if cfg.HTTP.Enabled {
		t.Error("HTTP should be disabled from config file")
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("expected HTTP addr :8080, got %q", cfg.HTTP.Addr)
	}
	if !cfg.GRPC.Enabled {
		t.Error("gRPC should be enabled from config file")
	}
	if cfg.GRPC.Addr != ":50051" {
		t.Errorf("expected gRPC addr :50051, got %q", cfg.GRPC.Addr)
	}
	if cfg.MCP.Enabled {
		t.Error("MCP should be disabled from config file")
	}
	if cfg.MCP.Addr != ":9000" {
		t.Errorf("expected MCP addr :9000, got %q", cfg.MCP.Addr)
	}
	if !cfg.MCP.Stdio {
		t.Error("MCP stdio should be true from config file")
	}
}

func TestLoadConfigFile_NotFound(t *testing.T) {
	fc, err := loadConfigFile("/nonexistent/path.yaml")
	if err != nil {
		t.Fatal("should not error on missing file")
	}
	if fc != nil {
		t.Error("should return nil for missing file")
	}
}

func TestLoadConfigFile_PartialConfig(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "partial.yaml")

	// Only set MCP, leave HTTP and gRPC at defaults
	content := `
mcp:
  enabled: false
`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	fc, err := loadConfigFile(cfgFile)
	if err != nil {
		t.Fatal(err)
	}

	cfg := defaultServerConfig()
	cfg = mergeFileConfig(cfg, fc)

	// HTTP and gRPC should keep defaults
	if !cfg.HTTP.Enabled {
		t.Error("HTTP should keep default (enabled)")
	}
	if cfg.HTTP.Addr != ":11023" {
		t.Error("HTTP addr should keep default")
	}
	if !cfg.GRPC.Enabled {
		t.Error("gRPC should keep default (enabled)")
	}
	// MCP should be disabled
	if cfg.MCP.Enabled {
		t.Error("MCP should be disabled from partial config")
	}
}

func TestApplyCLIFlags(t *testing.T) {
	cfg := defaultServerConfig()

	applyCLIFlags(&cfg, "false", ":8080", "false", ":50051", "true", ":9000", "false", "true", ":12443")

	if cfg.HTTP.Enabled {
		t.Error("HTTP should be disabled via CLI flag")
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("expected HTTP addr :8080, got %q", cfg.HTTP.Addr)
	}
	if cfg.GRPC.Enabled {
		t.Error("gRPC should be disabled via CLI flag")
	}
	if cfg.GRPC.Addr != ":50051" {
		t.Errorf("expected gRPC addr :50051, got %q", cfg.GRPC.Addr)
	}
	if !cfg.MCP.Enabled {
		t.Error("MCP should be enabled via CLI flag")
	}
	if cfg.MCP.Addr != ":9000" {
		t.Errorf("expected MCP addr :9000, got %q", cfg.MCP.Addr)
	}
	if !cfg.HTTP3.Enabled {
		t.Error("HTTP/3 should be enabled via CLI flag")
	}
	if cfg.HTTP3.Addr != ":12443" {
		t.Errorf("expected HTTP/3 addr :12443, got %q", cfg.HTTP3.Addr)
	}
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		input    string
		fallback bool
		expected bool
	}{
		{"true", false, true},
		{"false", true, false},
		{"1", false, true},
		{"0", true, false},
		{"yes", false, false}, // invalid, returns fallback
		{"", true, true},      // invalid, returns fallback
	}
	for _, tc := range tests {
		got := parseBool(tc.input, tc.fallback)
		if got != tc.expected {
			t.Errorf("parseBool(%q, %v) = %v, want %v", tc.input, tc.fallback, got, tc.expected)
		}
	}
}
