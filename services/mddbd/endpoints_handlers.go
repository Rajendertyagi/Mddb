package main

import (
	"net/http"

	json "github.com/goccy/go-json"
)

// ---- Request/Response types ----

type EndpointsResponse struct {
	HTTP []HTTPEndpoint `json:"http"`
	GRPC []GRPCMethod   `json:"grpc"`
	MCP  []MCPTool      `json:"mcp"`
}

type HTTPEndpoint struct {
	Method       string `json:"method"`
	Path         string `json:"path"`
	Description  string `json:"description"`
	RequiresAuth bool   `json:"requiresAuth"`
}

type GRPCMethod struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// ---- Handlers ----

// handleEndpoints returns a list of all available endpoints
func (s *Server) handleEndpoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	authEnabled := env("MDDB_AUTH_ENABLED", "false") == "true"

	// HTTP Endpoints
	httpEndpoints := []HTTPEndpoint{
		{Method: "GET/POST", Path: "/v1/health", Description: "Server health check", RequiresAuth: false},
		{Method: "GET", Path: "/v1/stats", Description: "Database statistics", RequiresAuth: authEnabled},
		{Method: "GET", Path: "/metrics", Description: "Prometheus metrics", RequiresAuth: false},

		// Core document operations
		{Method: "POST", Path: "/v1/add", Description: "Add/update document", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/get", Description: "Get document by key", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/search", Description: "Search documents with filters", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/delete", Description: "Delete document", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/delete-collection", Description: "Delete entire collection", RequiresAuth: authEnabled},

		// Export & backup
		{Method: "POST", Path: "/v1/export", Description: "Export collection (NDJSON/ZIP)", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/backup", Description: "Create database backup", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/restore", Description: "Restore from backup", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/truncate", Description: "Truncate revision history", RequiresAuth: authEnabled},

		// Vector search
		{Method: "POST", Path: "/v1/vector-search", Description: "Semantic search using embeddings", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/vector-reindex", Description: "Re-embed collection documents", RequiresAuth: authEnabled},
		{Method: "GET", Path: "/v1/vector-stats", Description: "Vector/embedding statistics", RequiresAuth: authEnabled},

		// Search features
		{Method: "POST", Path: "/v1/import-url", Description: "Import markdown from URL", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/set-ttl", Description: "Set document time-to-live", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/fts", Description: "Full-text search (with in-graph metadata filtering)", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/hybrid-search", Description: "Hybrid sparse+dense search (FTS + vector)", RequiresAuth: authEnabled},
		{Method: "GET/POST/DELETE", Path: "/v1/synonyms", Description: "Manage FTS synonyms", RequiresAuth: authEnabled},
		{Method: "GET/POST/DELETE", Path: "/v1/stopwords", Description: "Manage FTS stop words", RequiresAuth: authEnabled},

		// Metadata & checksum
		{Method: "GET", Path: "/v1/meta-keys", Description: "List unique metadata keys and values", RequiresAuth: authEnabled},
		{Method: "GET", Path: "/v1/checksum", Description: "Collection CRC32 checksum", RequiresAuth: authEnabled},

		// Partial update & doc-meta
		{Method: "PATCH", Path: "/v1/update", Description: "Partial document update (meta/content/ttl)", RequiresAuth: authEnabled},
		{Method: "GET", Path: "/v1/doc-meta", Description: "Get document metadata without content", RequiresAuth: authEnabled},

		// Zero-shot classification
		{Method: "POST", Path: "/v1/classify", Description: "Zero-shot document classification using embeddings", RequiresAuth: authEnabled},

		// Webhooks
		{Method: "POST", Path: "/v1/webhooks", Description: "List/register webhooks", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/webhooks/delete", Description: "Delete webhook", RequiresAuth: authEnabled},

		// Automation (triggers, crons, webhook targets)
		{Method: "GET/POST", Path: "/v1/automation", Description: "List/create automation rules", RequiresAuth: authEnabled},
		{Method: "GET/PUT/DELETE", Path: "/v1/automation/{id}", Description: "Get/update/delete automation rule", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/automation/{id}/test", Description: "Test trigger (dry run)", RequiresAuth: authEnabled},
		{Method: "GET", Path: "/v1/automation-logs", Description: "Get automation execution logs", RequiresAuth: authEnabled},

		// Schema validation
		{Method: "POST", Path: "/v1/schema/set", Description: "Set JSON schema for collection", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/schema/get", Description: "Get collection schema", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/schema/delete", Description: "Delete schema", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/schema/list", Description: "List all schemas", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/validate", Description: "Validate document metadata", RequiresAuth: authEnabled},

		// Embedding configuration
		{Method: "GET", Path: "/v1/embedding-configs", Description: "List embedding configurations", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/embedding-configs", Description: "Create embedding configuration", RequiresAuth: authEnabled},
		{Method: "GET", Path: "/v1/embedding-configs/{id}", Description: "Get embedding configuration", RequiresAuth: authEnabled},
		{Method: "PUT", Path: "/v1/embedding-configs/{id}", Description: "Update embedding configuration", RequiresAuth: authEnabled},
		{Method: "DELETE", Path: "/v1/embedding-configs/{id}", Description: "Delete embedding configuration", RequiresAuth: authEnabled},
		{Method: "POST", Path: "/v1/embedding-configs/set-default", Description: "Set default embedding config", RequiresAuth: authEnabled},

		// Replication
		{Method: "GET", Path: "/v1/replication/status", Description: "Replication/cluster status", RequiresAuth: authEnabled},

		// System info
		{Method: "GET", Path: "/v1/system/info", Description: "System information", RequiresAuth: authEnabled},
		{Method: "GET", Path: "/v1/config", Description: "Server configuration", RequiresAuth: authEnabled},
		{Method: "GET", Path: "/v1/mcp/config", Description: "MCP YAML configuration", RequiresAuth: authEnabled},
		{Method: "GET", Path: "/v1/endpoints", Description: "List all endpoints", RequiresAuth: false},
	}

	// Add GraphQL endpoint if enabled
	graphqlEnabled := env("MDDB_GRAPHQL_ENABLED", "false") == "true"
	if graphqlEnabled {
		httpEndpoints = append(httpEndpoints,
			HTTPEndpoint{Method: "POST", Path: "/graphql", Description: "GraphQL API endpoint", RequiresAuth: authEnabled},
			HTTPEndpoint{Method: "GET", Path: "/playground", Description: "GraphQL Playground", RequiresAuth: false},
		)
	}

	// Add auth endpoints if auth is enabled
	if authEnabled {
		authEndpoints := []HTTPEndpoint{
			{Method: "POST", Path: "/v1/auth/login", Description: "Login with username/password", RequiresAuth: false},
			{Method: "POST", Path: "/v1/auth/register", Description: "Register new user", RequiresAuth: true},
			{Method: "POST", Path: "/v1/auth/api-key", Description: "Create/manage API keys", RequiresAuth: true},
			{Method: "POST", Path: "/v1/auth/me", Description: "Get current user info", RequiresAuth: true},
			{Method: "POST", Path: "/v1/auth/permissions", Description: "Get user permissions", RequiresAuth: true},
			{Method: "GET", Path: "/v1/auth/users", Description: "List all users", RequiresAuth: true},
			{Method: "DELETE", Path: "/v1/auth/users/{username}", Description: "Delete user", RequiresAuth: true},
			{Method: "GET", Path: "/v1/auth/groups", Description: "List all groups", RequiresAuth: true},
			{Method: "POST", Path: "/v1/auth/groups", Description: "Create group", RequiresAuth: true},
			{Method: "GET", Path: "/v1/auth/groups/{name}", Description: "Get group details", RequiresAuth: true},
			{Method: "PUT", Path: "/v1/auth/groups/{name}", Description: "Update group", RequiresAuth: true},
			{Method: "DELETE", Path: "/v1/auth/groups/{name}", Description: "Delete group", RequiresAuth: true},
			{Method: "GET", Path: "/v1/auth/group-permissions", Description: "Get group permissions", RequiresAuth: true},
			{Method: "POST", Path: "/v1/auth/group-permissions", Description: "Set group permission", RequiresAuth: true},
		}
		httpEndpoints = append(httpEndpoints, authEndpoints...)
	}

	// gRPC Methods
	grpcMethods := []GRPCMethod{
		{Name: "Add", Description: "Add/update document"},
		{Name: "Get", Description: "Get document by key"},
		{Name: "Search", Description: "Search documents with filters"},
		{Name: "AddBatch", Description: "Batch add documents"},
		{Name: "UpdateBatch", Description: "Batch update documents"},
		{Name: "DeleteBatch", Description: "Batch delete documents"},
		{Name: "Export", Description: "Export collection (streaming)"},
		{Name: "Backup", Description: "Create database backup"},
		{Name: "Restore", Description: "Restore from backup"},
		{Name: "Truncate", Description: "Truncate revision history"},
		{Name: "Stats", Description: "Database statistics"},
		{Name: "VectorSearch", Description: "Semantic search using embeddings"},
		{Name: "VectorReindex", Description: "Re-embed collection documents"},
		{Name: "VectorStats", Description: "Vector/embedding statistics"},
		{Name: "ImportURL", Description: "Import markdown from URL"},
		{Name: "SetTTL", Description: "Set document time-to-live"},
		{Name: "FTS", Description: "Full-text search (with in-graph filtering)"},
		{Name: "HybridSearch", Description: "Hybrid sparse+dense search (FTS + vector)"},
		{Name: "RegisterWebhook", Description: "Register webhook"},
		{Name: "ListWebhooks", Description: "List webhooks"},
		{Name: "DeleteWebhook", Description: "Delete webhook"},
		{Name: "SetSchema", Description: "Set JSON schema"},
		{Name: "GetSchema", Description: "Get collection schema"},
		{Name: "DeleteSchema", Description: "Delete schema"},
		{Name: "ListSchemas", Description: "List all schemas"},
		{Name: "ValidateDocument", Description: "Validate document metadata"},
		{Name: "UpdateDocument", Description: "Partial document update (meta/content/ttl)"},
		{Name: "GetDocumentMeta", Description: "Get document metadata without content"},
		{Name: "Classify", Description: "Zero-shot document classification"},
		{Name: "ListAutomation", Description: "List automation rules"},
		{Name: "CreateAutomation", Description: "Create automation rule"},
		{Name: "UpdateAutomation", Description: "Update automation rule"},
		{Name: "DeleteAutomation", Description: "Delete automation rule"},
		{Name: "TestAutomation", Description: "Test trigger (dry run)"},
	}

	// MCP Tools
	mcpTools := []MCPTool{
		{Name: "add_document", Description: "Add/update document"},
		{Name: "search_documents", Description: "Search with filters and sorting"},
		{Name: "delete_document", Description: "Delete document"},
		{Name: "get_stats", Description: "Get server statistics"},
		{Name: "add_documents_batch", Description: "Batch add documents"},
		{Name: "delete_documents_batch", Description: "Batch delete documents"},
		{Name: "export_documents", Description: "Export collection"},
		{Name: "create_backup", Description: "Create database backup"},
		{Name: "restore_backup", Description: "Restore from backup"},
		{Name: "semantic_search", Description: "Semantic/vector search"},
		{Name: "hybrid_search", Description: "Hybrid sparse+dense search (FTS + vector)"},
		{Name: "vector_reindex", Description: "Re-embed collection"},
		{Name: "vector_stats", Description: "Vector statistics"},
		{Name: "import_url", Description: "Import from URL"},
		{Name: "set_ttl", Description: "Set document TTL"},
		{Name: "full_text_search", Description: "Full-text search (with in-graph filtering)"},
		{Name: "register_webhook", Description: "Register webhook"},
		{Name: "list_webhooks", Description: "List webhooks"},
		{Name: "delete_webhook", Description: "Delete webhook"},
		{Name: "set_schema", Description: "Set JSON schema"},
		{Name: "get_schema", Description: "Get schema"},
		{Name: "delete_schema", Description: "Delete schema"},
		{Name: "list_schemas", Description: "List schemas"},
		{Name: "validate_document", Description: "Validate metadata"},
		{Name: "update_document", Description: "Partial document update"},
		{Name: "get_document_meta", Description: "Get document metadata"},
		{Name: "classify_document", Description: "Zero-shot classification"},
		{Name: "list_automation", Description: "List automation rules"},
		{Name: "create_automation", Description: "Create automation rule"},
		{Name: "update_automation", Description: "Update automation rule"},
		{Name: "delete_automation", Description: "Delete automation rule"},
		{Name: "test_automation", Description: "Test trigger (dry run)"},
	}

	response := EndpointsResponse{
		HTTP: httpEndpoints,
		GRPC: grpcMethods,
		MCP:  mcpTools,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, `{"error":"failed to encode response"}`, http.StatusInternalServerError)
	}
}
