package main

import (
	"errors"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// MCPFileConfig is the YAML file structure for custom tools.
type MCPFileConfig struct {
	CustomTools []MCPCustomToolConfig `yaml:"custom_tools"`
}

// loadMCPCustomTools loads custom tool definitions from YAML file (if configured).
func loadMCPCustomTools() []MCPCustomToolConfig {
	path := os.Getenv("MDDB_MCP_CONFIG")
	if path == "" {
		return nil
	}

	cfg, err := loadMCPConfig(path)
	if err != nil {
		log.Printf("WARNING: failed to load MCP config from %s: %v", path, err) // #nosec G706 -- internal log
		return nil
	}

	if err := validateMCPCustomTools(cfg.CustomTools); err != nil {
		log.Printf("WARNING: invalid MCP custom tools: %v", err)
		return nil
	}

	if len(cfg.CustomTools) > 0 {
		log.Printf("MCP: loaded %d custom tools from %s", len(cfg.CustomTools), path) // #nosec G706 -- internal log
	}

	return cfg.CustomTools
}

func loadMCPConfig(path string) (*MCPFileConfig, error) {
	// #nosec G304 -- Expected configuration dynamically loaded by MCP execution
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &MCPFileConfig{}, nil
		}
		return nil, err
	}

	var cfg MCPFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
