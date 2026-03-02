package main

import (
	"fmt"
	"net/http"
)

// ---- Handlers ----

// handleMCPConfig returns the MCP server configuration in YAML format
func (s *Server) handleMCPConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get addresses from environment
	httpAddr := env("MDDB_ADDR", ":11023")
	grpcAddr := env("MDDB_GRPC_ADDR", ":11024")

	// Generate MCP YAML configuration
	yaml := fmt.Sprintf(`mcp:
  listenAddress: "0.0.0.0:9000"

mddb:
  grpcAddress: "localhost%s"
  restBaseUrl: "http://localhost%s"
  # grpc_only | rest_only | grpc_with_rest_fallback | rest_with_grpc_fallback
  transportMode: "grpc_with_rest_fallback"
  timeoutSeconds: 2
  maxRetries: 1

# Custom tools configuration (optional)
# Uncomment and customize as needed:
#
# custom_tools:
#   - name: "semantic_search"
#     defaults:
#       collection: "docs"
#       topK: 5
#       threshold: 0.7
#       includeContent: true
#
#   - name: "search_documents"
#     defaults:
#       collection: "docs"
#       limit: 10
#       sortBy: "addedAt"
#       sortAsc: false
#
#   - name: "full_text_search"
#     defaults:
#       collection: "docs"
#       limit: 10
`, grpcAddr, httpAddr)

	w.Header().Set("Content-Type", "text/yaml")
	if _, err := w.Write([]byte(yaml)); err != nil {
		http.Error(w, "failed to write response", http.StatusInternalServerError)
	}
}
