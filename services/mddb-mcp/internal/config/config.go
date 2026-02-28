package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v3"
)

// Config is the main MCP configuration structure.
type Config struct {
	MCP         MCPConfig          `yaml:"mcp"`
	MDDB        MDDBConfig         `yaml:"mddb"`
	CustomTools []CustomToolConfig `yaml:"custom_tools"`
}

// CustomToolConfig defines a single custom YAML tool.
type CustomToolConfig struct {
	Name        string             `yaml:"name"`
	Description string             `yaml:"description"`
	Action      string             `yaml:"action"` // semantic_search, search_documents, full_text_search
	Defaults    CustomToolDefaults `yaml:"defaults"`
	Parameters  []CustomToolParam  `yaml:"parameters"`
}

// CustomToolDefaults holds pre-filled arguments for the underlying action.
type CustomToolDefaults struct {
	Collection     string              `yaml:"collection"`
	TopK           int                 `yaml:"topK"`
	Threshold      float64             `yaml:"threshold"`
	IncludeContent *bool               `yaml:"includeContent"`
	Sort           string              `yaml:"sort"`
	Asc            *bool               `yaml:"asc"`
	Limit          int                 `yaml:"limit"`
	Offset         int                 `yaml:"offset"`
	FilterMeta     map[string][]string `yaml:"filterMeta"`
	Query          string              `yaml:"query"`
}

// CustomToolParam defines a parameter exposed to the AI.
type CustomToolParam struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"` // string, integer, number, boolean, object
	Required    bool   `yaml:"required"`
	Description string `yaml:"description"`
}

type MCPConfig struct {
	ListenAddress string `yaml:"listenAddress"`
}

type MDDBConfig struct {
	GRPCAddress    string `yaml:"grpcAddress"`
	RESTBaseURL    string `yaml:"restBaseUrl"`
	TransportMode  string `yaml:"transportMode"`
	TimeoutSeconds int    `yaml:"timeoutSeconds"`
	MaxRetries     int    `yaml:"maxRetries"`
}

// envConfig maps environment variables.
type envConfig struct {
	MCPListenAddress string `envconfig:"MCP_LISTEN_ADDRESS"`

	MDDBGRPCAddress   string `envconfig:"MDDB_GRPC_ADDRESS"`
	MDDBRESTBaseURL   string `envconfig:"MDDB_REST_BASE_URL"`
	MDDBTransportMode string `envconfig:"MDDB_TRANSPORT_MODE"`
	MDDBTimeoutSec    int    `envconfig:"MDDB_TIMEOUT_SECONDS"`
	MDDBMaxRetries    int    `envconfig:"MDDB_MAX_RETRIES"`
}

// Load loads config in order: defaults -> YAML -> ENV (overrides).
func Load(path string) (*Config, error) {
	cfg := defaultConfig()

	// 1) YAML (optional)
	if err := loadYAML(path, cfg); err != nil {
		return nil, err
	}

	// 2) ENV (overrides YAML values)
	if err := overrideFromEnv(cfg); err != nil {
		return nil, err
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		MCP: MCPConfig{
			ListenAddress: "0.0.0.0:9000",
		},
		MDDB: MDDBConfig{
			GRPCAddress:    "localhost:11024",
			RESTBaseURL:    "http://localhost:11023",
			TransportMode:  "grpc_with_rest_fallback",
			TimeoutSeconds: 2,
			MaxRetries:     1,
		},
	}
}

func loadYAML(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// brak pliku konfiguracyjnego jest akceptowalny
			return nil
		}
		return fmt.Errorf("read config yaml: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("unmarshal config yaml: %w", err)
	}

	return nil
}

func overrideFromEnv(cfg *Config) error {
	var e envConfig
	if err := envconfig.Process("", &e); err != nil {
		return fmt.Errorf("process env: %w", err)
	}

	if e.MCPListenAddress != "" {
		cfg.MCP.ListenAddress = e.MCPListenAddress
	}

	if e.MDDBGRPCAddress != "" {
		cfg.MDDB.GRPCAddress = e.MDDBGRPCAddress
	}
	if e.MDDBRESTBaseURL != "" {
		cfg.MDDB.RESTBaseURL = e.MDDBRESTBaseURL
	}
	if e.MDDBTransportMode != "" {
		cfg.MDDB.TransportMode = e.MDDBTransportMode
	}
	if e.MDDBTimeoutSec != 0 {
		cfg.MDDB.TimeoutSeconds = e.MDDBTimeoutSec
	}
	if e.MDDBMaxRetries != 0 {
		cfg.MDDB.MaxRetries = e.MDDBMaxRetries
	}

	return nil
}

func validate(cfg *Config) error {
	switch cfg.MDDB.TransportMode {
	case "grpc_only", "rest_only", "grpc_with_rest_fallback", "rest_with_grpc_fallback":
		// ok
	default:
		return fmt.Errorf("invalid transportMode: %s", cfg.MDDB.TransportMode)
	}

	if cfg.MDDB.GRPCAddress == "" {
		return errors.New("mddb.grpcAddress is required")
	}
	if cfg.MDDB.RESTBaseURL == "" {
		return errors.New("mddb.restBaseUrl is required")
	}

	if err := validateCustomTools(cfg.CustomTools); err != nil {
		return err
	}

	return nil
}

func validateCustomTools(tools []CustomToolConfig) error {
	builtinNames := map[string]bool{
		"add_document": true, "search_documents": true, "delete_document": true,
		"get_stats": true, "add_documents_batch": true, "delete_documents_batch": true,
		"export_documents": true, "create_backup": true, "restore_backup": true,
		"semantic_search": true, "vector_reindex": true, "vector_stats": true,
		"import_url": true, "set_ttl": true, "full_text_search": true,
		"register_webhook": true, "list_webhooks": true, "delete_webhook": true,
		"set_schema": true, "get_schema": true, "delete_schema": true,
		"list_schemas": true, "validate_document": true,
	}
	validActions := map[string]bool{
		"semantic_search": true, "search_documents": true, "full_text_search": true,
	}
	validTypes := map[string]bool{
		"string": true, "integer": true, "number": true, "boolean": true, "object": true,
	}
	seen := map[string]bool{}

	for i, t := range tools {
		if t.Name == "" {
			return fmt.Errorf("custom_tools[%d]: name is required", i)
		}
		if builtinNames[t.Name] {
			return fmt.Errorf("custom_tools[%d]: name %q conflicts with built-in tool", i, t.Name)
		}
		if seen[t.Name] {
			return fmt.Errorf("custom_tools[%d]: duplicate name %q", i, t.Name)
		}
		seen[t.Name] = true
		if !validActions[t.Action] {
			return fmt.Errorf("custom_tools[%d] (%s): invalid action %q (must be semantic_search, search_documents, or full_text_search)", i, t.Name, t.Action)
		}
		if t.Description == "" {
			return fmt.Errorf("custom_tools[%d] (%s): description is required", i, t.Name)
		}
		for j, p := range t.Parameters {
			if p.Name == "" {
				return fmt.Errorf("custom_tools[%d] (%s) param[%d]: name is required", i, t.Name, j)
			}
			if !validTypes[p.Type] {
				return fmt.Errorf("custom_tools[%d] (%s) param[%d] (%s): invalid type %q", i, t.Name, j, p.Name, p.Type)
			}
		}
	}
	return nil
}
