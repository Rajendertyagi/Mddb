package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// ServerConfig holds all configurable server settings.
// Precedence: CLI flags > env vars > config file > defaults.
type ServerConfig struct {
	HTTP        HTTPConfig        `yaml:"http" json:"http"`
	GRPC        GRPCConfig        `yaml:"grpc" json:"grpc"`
	MCP         MCPConfig         `yaml:"mcp" json:"mcp"`
	HTTP3       HTTP3Config       `yaml:"http3" json:"http3"`
	FTS         FTSConfig         `yaml:"fts" json:"fts"`
	Compression CompressionConfig `yaml:"compression" json:"compression"`
	Vector      VectorExtConfig   `yaml:"vector" json:"vector"`
}

// FTSConfig controls full-text search features.
type FTSConfig struct {
	StemmingEnabled bool `yaml:"stemmingEnabled" json:"stemmingEnabled"`
	SynonymsEnabled bool `yaml:"synonymsEnabled" json:"synonymsEnabled"`
}

// CompressionConfig controls document compression.
type CompressionConfig struct {
	Enabled         bool `yaml:"enabled" json:"enabled"`
	SmallThreshold  int  `yaml:"smallThreshold" json:"smallThreshold"`
	MediumThreshold int  `yaml:"mediumThreshold" json:"mediumThreshold"`
}

// VectorExtConfig controls extended vector search options.
type VectorExtConfig struct {
	DefaultAlgorithm string `yaml:"defaultAlgorithm" json:"defaultAlgorithm"`
	BQRerankFactor   int    `yaml:"bqRerankFactor" json:"bqRerankFactor"`
}

// HTTPConfig controls the HTTP/JSON API server.
type HTTPConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Addr    string `yaml:"addr" json:"addr"`
}

// GRPCConfig controls the gRPC server.
type GRPCConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Addr    string `yaml:"addr" json:"addr"`
}

// MCPConfig controls the MCP protocol server.
type MCPConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Addr    string `yaml:"addr" json:"addr"`
	Stdio   bool   `yaml:"stdio" json:"stdio"`
}

// HTTP3Config controls the HTTP/3 (QUIC) server (extreme mode).
type HTTP3Config struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Addr    string `yaml:"addr" json:"addr"`
}

// defaultServerConfig returns the default configuration.
func defaultServerConfig() ServerConfig {
	return ServerConfig{
		HTTP:        HTTPConfig{Enabled: true, Addr: ":11023"},
		GRPC:        GRPCConfig{Enabled: true, Addr: ":11024"},
		MCP:         MCPConfig{Enabled: true, Addr: ":9000", Stdio: false},
		HTTP3:       HTTP3Config{Enabled: false, Addr: ":11443"},
		FTS:         FTSConfig{StemmingEnabled: true, SynonymsEnabled: true},
		Compression: CompressionConfig{Enabled: true, SmallThreshold: 1024, MediumThreshold: 10240},
		Vector:      VectorExtConfig{DefaultAlgorithm: "flat", BQRerankFactor: 10},
	}
}

// loadServerConfig loads configuration with precedence: CLI flags > env vars > config file > defaults.
func loadServerConfig() ServerConfig {
	cfg := defaultServerConfig()

	// 1. Parse CLI flags (but don't apply yet — need config file path first)
	var (
		configFile   string
		httpEnabled  = flag.String("http-enabled", "", "Enable HTTP API server (true/false)")
		httpAddr     = flag.String("http-addr", "", "HTTP API listen address (e.g. :11023)")
		grpcEnabled  = flag.String("grpc-enabled", "", "Enable gRPC server (true/false)")
		grpcAddr     = flag.String("grpc-addr", "", "gRPC listen address (e.g. :11024)")
		mcpEnabled   = flag.String("mcp-enabled", "", "Enable MCP protocol (true/false)")
		mcpAddr      = flag.String("mcp-addr", "", "MCP HTTP listen address (e.g. :9000)")
		mcpStdio     = flag.String("mcp-stdio", "", "Run in MCP stdio mode (true/false)")
		http3Enabled = flag.String("http3-enabled", "", "Enable HTTP/3 server (true/false)")
		http3Addr    = flag.String("http3-addr", "", "HTTP/3 listen address (e.g. :11443)")
	)
	flag.StringVar(&configFile, "config", "", "Path to YAML config file")
	flag.StringVar(&configFile, "c", "", "Path to YAML config file (shorthand)")
	flag.Parse()

	// 2. Load config file (lowest priority after defaults)
	cfgPath := configFile
	if cfgPath == "" {
		cfgPath = os.Getenv("MDDB_CONFIG")
	}
	if cfgPath != "" {
		fileCfg, err := loadConfigFile(cfgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: failed to load config file %s: %v\n", cfgPath, err)
		} else {
			cfg = mergeFileConfig(cfg, fileCfg)
		}
	}

	// 3. Apply env vars (override config file)
	applyEnvConfig(&cfg)

	// 4. Apply CLI flags (highest priority)
	applyCLIFlags(&cfg, *httpEnabled, *httpAddr, *grpcEnabled, *grpcAddr, *mcpEnabled, *mcpAddr, *mcpStdio, *http3Enabled, *http3Addr)

	return cfg
}

// fileConfig mirrors ServerConfig for YAML unmarshalling with pointer fields
// so we can distinguish "not set" from "set to zero value".
type fileConfig struct {
	HTTP        *fileHTTP        `yaml:"http"`
	GRPC        *fileGRPC        `yaml:"grpc"`
	MCP         *fileMCP         `yaml:"mcp"`
	HTTP3       *fileHTTP3       `yaml:"http3"`
	FTS         *fileFTS         `yaml:"fts"`
	Compression *fileCompression `yaml:"compression"`
	Vector      *fileVector      `yaml:"vector"`
}

type fileFTS struct {
	StemmingEnabled *bool `yaml:"stemmingEnabled"`
	SynonymsEnabled *bool `yaml:"synonymsEnabled"`
}

type fileCompression struct {
	Enabled         *bool `yaml:"enabled"`
	SmallThreshold  *int  `yaml:"smallThreshold"`
	MediumThreshold *int  `yaml:"mediumThreshold"`
}

type fileVector struct {
	DefaultAlgorithm *string `yaml:"defaultAlgorithm"`
	BQRerankFactor   *int    `yaml:"bqRerankFactor"`
}

type fileHTTP struct {
	Enabled *bool   `yaml:"enabled"`
	Addr    *string `yaml:"addr"`
}

type fileGRPC struct {
	Enabled *bool   `yaml:"enabled"`
	Addr    *string `yaml:"addr"`
}

type fileMCP struct {
	Enabled *bool   `yaml:"enabled"`
	Addr    *string `yaml:"addr"`
	Stdio   *bool   `yaml:"stdio"`
}

type fileHTTP3 struct {
	Enabled *bool   `yaml:"enabled"`
	Addr    *string `yaml:"addr"`
}

func loadConfigFile(path string) (*fileConfig, error) {
	// #nosec G304 -- Expected configuration file path
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var fc fileConfig
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	return &fc, nil
}

func mergeFileConfig(cfg ServerConfig, fc *fileConfig) ServerConfig {
	if fc == nil {
		return cfg
	}
	if fc.HTTP != nil {
		if fc.HTTP.Enabled != nil {
			cfg.HTTP.Enabled = *fc.HTTP.Enabled
		}
		if fc.HTTP.Addr != nil {
			cfg.HTTP.Addr = *fc.HTTP.Addr
		}
	}
	if fc.GRPC != nil {
		if fc.GRPC.Enabled != nil {
			cfg.GRPC.Enabled = *fc.GRPC.Enabled
		}
		if fc.GRPC.Addr != nil {
			cfg.GRPC.Addr = *fc.GRPC.Addr
		}
	}
	if fc.MCP != nil {
		if fc.MCP.Enabled != nil {
			cfg.MCP.Enabled = *fc.MCP.Enabled
		}
		if fc.MCP.Addr != nil {
			cfg.MCP.Addr = *fc.MCP.Addr
		}
		if fc.MCP.Stdio != nil {
			cfg.MCP.Stdio = *fc.MCP.Stdio
		}
	}
	if fc.HTTP3 != nil {
		if fc.HTTP3.Enabled != nil {
			cfg.HTTP3.Enabled = *fc.HTTP3.Enabled
		}
		if fc.HTTP3.Addr != nil {
			cfg.HTTP3.Addr = *fc.HTTP3.Addr
		}
	}
	if fc.FTS != nil {
		if fc.FTS.StemmingEnabled != nil {
			cfg.FTS.StemmingEnabled = *fc.FTS.StemmingEnabled
		}
		if fc.FTS.SynonymsEnabled != nil {
			cfg.FTS.SynonymsEnabled = *fc.FTS.SynonymsEnabled
		}
	}
	if fc.Compression != nil {
		if fc.Compression.Enabled != nil {
			cfg.Compression.Enabled = *fc.Compression.Enabled
		}
		if fc.Compression.SmallThreshold != nil {
			cfg.Compression.SmallThreshold = *fc.Compression.SmallThreshold
		}
		if fc.Compression.MediumThreshold != nil {
			cfg.Compression.MediumThreshold = *fc.Compression.MediumThreshold
		}
	}
	if fc.Vector != nil {
		if fc.Vector.DefaultAlgorithm != nil {
			cfg.Vector.DefaultAlgorithm = *fc.Vector.DefaultAlgorithm
		}
		if fc.Vector.BQRerankFactor != nil {
			cfg.Vector.BQRerankFactor = *fc.Vector.BQRerankFactor
		}
	}
	return cfg
}

func applyEnvConfig(cfg *ServerConfig) {
	// HTTP
	if v := os.Getenv("MDDB_HTTP_ENABLED"); v != "" {
		cfg.HTTP.Enabled = parseBool(v, cfg.HTTP.Enabled)
	}
	// MDDB_ADDR is the legacy env var for HTTP address
	if v := os.Getenv("MDDB_ADDR"); v != "" {
		cfg.HTTP.Addr = v
	}
	if v := os.Getenv("MDDB_HTTP_ADDR"); v != "" {
		cfg.HTTP.Addr = v
	}
	if v := os.Getenv("MDDB_HTTP_PORT"); v != "" {
		cfg.HTTP.Addr = portToAddr(v)
	}

	// gRPC
	if v := os.Getenv("MDDB_GRPC_ENABLED"); v != "" {
		cfg.GRPC.Enabled = parseBool(v, cfg.GRPC.Enabled)
	}
	if v := os.Getenv("MDDB_GRPC_ADDR"); v != "" {
		cfg.GRPC.Addr = v
	}
	if v := os.Getenv("MDDB_GRPC_PORT"); v != "" {
		cfg.GRPC.Addr = portToAddr(v)
	}

	// MCP
	if v := os.Getenv("MDDB_MCP_ENABLED"); v != "" {
		cfg.MCP.Enabled = parseBool(v, cfg.MCP.Enabled)
	}
	if v := os.Getenv("MDDB_MCP_ADDR"); v != "" {
		cfg.MCP.Addr = v
	}
	if v := os.Getenv("MDDB_MCP_PORT"); v != "" {
		cfg.MCP.Addr = portToAddr(v)
	}
	if v := os.Getenv("MDDB_MCP_STDIO"); v != "" {
		cfg.MCP.Stdio = parseBool(v, cfg.MCP.Stdio)
	}

	// HTTP/3
	// MDDB_EXTREME is the legacy env var — maps to HTTP3 enabled
	if v := os.Getenv("MDDB_EXTREME"); v != "" {
		cfg.HTTP3.Enabled = parseBool(v, cfg.HTTP3.Enabled)
	}
	if v := os.Getenv("MDDB_HTTP3_ENABLED"); v != "" {
		cfg.HTTP3.Enabled = parseBool(v, cfg.HTTP3.Enabled)
	}
	if v := os.Getenv("MDDB_HTTP3_ADDR"); v != "" {
		cfg.HTTP3.Addr = v
	}
	if v := os.Getenv("MDDB_HTTP3_PORT"); v != "" {
		cfg.HTTP3.Addr = portToAddr(v)
	}

	// FTS
	if v := os.Getenv("MDDB_FTS_STEMMING"); v != "" {
		cfg.FTS.StemmingEnabled = parseBool(v, cfg.FTS.StemmingEnabled)
	}
	if v := os.Getenv("MDDB_FTS_SYNONYMS"); v != "" {
		cfg.FTS.SynonymsEnabled = parseBool(v, cfg.FTS.SynonymsEnabled)
	}

	// Compression
	if v := os.Getenv("MDDB_COMPRESSION_ENABLED"); v != "" {
		cfg.Compression.Enabled = parseBool(v, cfg.Compression.Enabled)
	}
	if v := os.Getenv("MDDB_COMPRESSION_SMALL_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Compression.SmallThreshold = n
		}
	}
	if v := os.Getenv("MDDB_COMPRESSION_MEDIUM_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Compression.MediumThreshold = n
		}
	}

	// Vector
	if v := os.Getenv("MDDB_VECTOR_DEFAULT_ALGORITHM"); v != "" {
		cfg.Vector.DefaultAlgorithm = v
	}
	if v := os.Getenv("MDDB_VECTOR_BQ_RERANK_FACTOR"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Vector.BQRerankFactor = n
		}
	}
}

func applyCLIFlags(cfg *ServerConfig, httpEnabled, httpAddr, grpcEnabled, grpcAddr, mcpEnabled, mcpAddr, mcpStdio, http3Enabled, http3Addr string) {
	if httpEnabled != "" {
		cfg.HTTP.Enabled = parseBool(httpEnabled, cfg.HTTP.Enabled)
	}
	if httpAddr != "" {
		cfg.HTTP.Addr = httpAddr
	}
	if grpcEnabled != "" {
		cfg.GRPC.Enabled = parseBool(grpcEnabled, cfg.GRPC.Enabled)
	}
	if grpcAddr != "" {
		cfg.GRPC.Addr = grpcAddr
	}
	if mcpEnabled != "" {
		cfg.MCP.Enabled = parseBool(mcpEnabled, cfg.MCP.Enabled)
	}
	if mcpAddr != "" {
		cfg.MCP.Addr = mcpAddr
	}
	if mcpStdio != "" {
		cfg.MCP.Stdio = parseBool(mcpStdio, cfg.MCP.Stdio)
	}
	if http3Enabled != "" {
		cfg.HTTP3.Enabled = parseBool(http3Enabled, cfg.HTTP3.Enabled)
	}
	if http3Addr != "" {
		cfg.HTTP3.Addr = http3Addr
	}
}

func parseBool(s string, fallback bool) bool {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return fallback
	}
	return b
}

// portToAddr converts a plain port number "9000" to ":9000" address format.
func portToAddr(port string) string {
	if port == "" {
		return ""
	}
	// Already has colon prefix
	if port[0] == ':' {
		return port
	}
	return ":" + port
}
