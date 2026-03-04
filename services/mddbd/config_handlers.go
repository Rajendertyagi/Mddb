package main

import (
	"net/http"
	"os"

	json "github.com/goccy/go-json"
)

// ---- Request/Response types ----

type ConfigResponse struct {
	Version         string        `json:"version"`
	DatabasePath    string        `json:"databasePath"`
	Mode            string        `json:"mode"`
	HTTPAddr        string        `json:"httpAddr"`
	GRPCAddr        string        `json:"grpcAddr"`
	HTTP3Addr       string        `json:"http3Addr,omitempty"`
	AuthEnabled     bool          `json:"authEnabled"`
	MetricsEnabled  bool          `json:"metricsEnabled"`
	ExtremeMode     bool          `json:"extremeMode"`
	ReplicationRole string        `json:"replicationRole"`
	VectorConfig    *VectorConfig `json:"vectorConfig,omitempty"`
}

type VectorConfig struct {
	Enabled    bool   `json:"enabled"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions"`
	APIURL     string `json:"apiUrl"`
}

// ---- Handlers ----

// handleConfig returns the current server configuration
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Build configuration response
	response := ConfigResponse{
		Version:         VERSION,
		DatabasePath:    s.Path,
		Mode:            string(s.Mode),
		HTTPAddr:        env("MDDB_ADDR", ":11023"),
		GRPCAddr:        env("MDDB_GRPC_ADDR", ":11024"),
		AuthEnabled:     env("MDDB_AUTH_ENABLED", "false") == "true",
		MetricsEnabled:  env("MDDB_METRICS", "true") != "false",
		ExtremeMode:     s.UseExtreme,
		ReplicationRole: s.ReplicationRole,
	}

	// Add HTTP/3 address if extreme mode is enabled
	if s.UseExtreme {
		response.HTTP3Addr = env("MDDB_HTTP3_ADDR", ":11443")
	}

	// Add vector configuration if embedding provider is set
	provider := os.Getenv("MDDB_EMBEDDING_PROVIDER")
	if provider != "" {
		var apiURL, model string
		var dimensions int

		switch provider {
		case "openai":
			apiURL = envDefault("MDDB_EMBEDDING_API_URL", "https://api.openai.com/v1")
			model = envDefault("MDDB_EMBEDDING_MODEL", "text-embedding-3-small")
			dimensions = envDefaultInt("MDDB_EMBEDDING_DIMENSIONS", 1536)
		case "ollama":
			apiURL = envDefault("MDDB_EMBEDDING_API_URL", "http://localhost:11434")
			model = envDefault("MDDB_EMBEDDING_MODEL", "nomic-embed-text")
			dimensions = envDefaultInt("MDDB_EMBEDDING_DIMENSIONS", 768)
		case "voyage":
			apiURL = envDefault("MDDB_EMBEDDING_API_URL", "https://api.voyageai.com/v1")
			model = envDefault("MDDB_EMBEDDING_MODEL", "voyage-3")
			dimensions = envDefaultInt("MDDB_EMBEDDING_DIMENSIONS", 1024)
		}

		response.VectorConfig = &VectorConfig{
			Enabled:    true,
			Provider:   provider,
			Model:      model,
			Dimensions: dimensions,
			APIURL:     apiURL,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, `{"error":"failed to encode response"}`, http.StatusInternalServerError)
	}
}
