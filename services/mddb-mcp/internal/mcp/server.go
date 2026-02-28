package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/tradik/mddb/services/mddb-mcp/internal/mddb"
)

// Server implements MCP server for MDDB.
type Server struct {
	client mddb.Client
	addr   string
	server *http.Server
}

// NewServer creates a new MCP server.
func NewServer(client mddb.Client, addr string) *Server {
	return &Server{
		client: client,
		addr:   addr,
	}
}

// Start starts MCP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// MCP endpoints
	mux.HandleFunc("/mcp/resources", s.handleResources)
	mux.HandleFunc("/mcp/resources/read", s.handleResourceRead)
	mux.HandleFunc("/mcp/tools", s.handleTools)
	mux.HandleFunc("/mcp/tools/call", s.handleToolCall)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("mcp server error: %v", err)
		}
	}()

	return nil
}

// Stop stops MCP server.
func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Shutdown(context.Background())
	}
	return nil
}

// handleHealth handles health check.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	health, err := s.client.Health(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(health); err != nil {
		log.Printf("error encoding health response: %v", err)
	}
}

// handleResources returns list of available resources.
func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	resources := []Resource{
		{
			URI:         "mddb://health",
			Name:        "MDDB Health",
			Description: "Health status of MDDB server",
			MimeType:    "application/json",
		},
		{
			URI:         "mddb://stats",
			Name:        "MDDB Statistics",
			Description: "Server and database statistics",
			MimeType:    "application/json",
		},
		{
			URI:         "mddb://{collection}/{key}?lang={lang}",
			Name:        "MDDB Document",
			Description: "Get a document by collection, key, and language",
			MimeType:    "text/markdown",
		},
		{
			URI:         "mddb-search://{collection}",
			Name:        "MDDB Search",
			Description: "Search documents in a collection",
			MimeType:    "application/json",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"resources": resources,
	}); err != nil {
		log.Printf("error encoding resources response: %v", err)
	}
}

// handleResourceRead czyta konkretny resource.
func (s *Server) handleResourceRead(w http.ResponseWriter, r *http.Request) {
	var req ResourceReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	content, err := s.readResource(r.Context(), req.URI)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"uri":      req.URI,
				"mimeType": "application/json",
				"text":     content,
			},
		},
	}); err != nil {
		log.Printf("error encoding resource read response: %v", err)
	}
}

// handleTools returns list of available tools.
func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	tools := []Tool{
		{
			Name:        "add_document",
			Description: "Add or update a document in MDDB",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string"},
					"key":        map[string]interface{}{"type": "string"},
					"lang":       map[string]interface{}{"type": "string"},
					"content_md": map[string]interface{}{"type": "string"},
					"meta":       map[string]interface{}{"type": "object"},
				},
				"required": []string{"collection", "key", "lang", "content_md"},
			},
		},
		{
			Name:        "search_documents",
			Description: "Search documents with filters and sorting",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection":  map[string]interface{}{"type": "string"},
					"filter_meta": map[string]interface{}{"type": "object"},
					"sort":        map[string]interface{}{"type": "string"},
					"limit":       map[string]interface{}{"type": "integer"},
					"offset":      map[string]interface{}{"type": "integer"},
				},
				"required": []string{"collection"},
			},
		},
		{
			Name:        "delete_document",
			Description: "Delete a document from MDDB",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string"},
					"key":        map[string]interface{}{"type": "string"},
					"lang":       map[string]interface{}{"type": "string"},
				},
				"required": []string{"collection", "key", "lang"},
			},
		},
		{
			Name:        "get_stats",
			Description: "Get MDDB server statistics",
			InputSchema: map[string]interface{}{
				"type": "object",
			},
		},
		{
			Name:        "semantic_search",
			Description: "Search documents by meaning using semantic similarity. Use this when you need to find documents related to a concept or question, rather than filtering by exact metadata tags. Requires embedding provider to be configured on the server.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection":  map[string]interface{}{"type": "string", "description": "Collection to search in"},
					"query":       map[string]interface{}{"type": "string", "description": "Natural language search query"},
					"top_k":       map[string]interface{}{"type": "integer", "description": "Number of results to return (default: 5)"},
					"threshold":   map[string]interface{}{"type": "number", "description": "Minimum similarity score 0-1 (default: 0.0)"},
					"filter_meta": map[string]interface{}{"type": "object", "description": "Optional metadata filter to combine with semantic search"},
				},
				"required": []string{"collection", "query"},
			},
		},
		{
			Name:        "vector_reindex",
			Description: "Re-embed all documents in a collection. Use after adding many documents or changing the embedding model.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection to reindex"},
					"force":      map[string]interface{}{"type": "boolean", "description": "Force re-embed even if content hasn't changed (default: false)"},
				},
				"required": []string{"collection"},
			},
		},
		{
			Name:        "vector_stats",
			Description: "Get vector/embedding statistics including provider info and per-collection embedding coverage.",
			InputSchema: map[string]interface{}{
				"type": "object",
			},
		},
		{
			Name:        "import_url",
			Description: "Import a markdown document from a URL. Supports YAML frontmatter for metadata extraction.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Target collection"},
					"url":        map[string]interface{}{"type": "string", "description": "URL to fetch markdown from"},
					"lang":       map[string]interface{}{"type": "string", "description": "Language code (e.g. en_US)"},
					"key":        map[string]interface{}{"type": "string", "description": "Document key (auto-derived from URL if empty)"},
					"meta":       map[string]interface{}{"type": "object", "description": "Additional metadata (overrides frontmatter)"},
					"ttl":        map[string]interface{}{"type": "integer", "description": "Time-to-live in seconds (0 = no expiry)"},
				},
				"required": []string{"collection", "url", "lang"},
			},
		},
		{
			Name:        "set_ttl",
			Description: "Set or remove time-to-live on a document. The document will be automatically deleted after TTL expires.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
					"key":        map[string]interface{}{"type": "string", "description": "Document key"},
					"lang":       map[string]interface{}{"type": "string", "description": "Language code"},
					"ttl":        map[string]interface{}{"type": "integer", "description": "TTL in seconds (0 = remove TTL)"},
				},
				"required": []string{"collection", "key", "lang", "ttl"},
			},
		},
		{
			Name:        "full_text_search",
			Description: "Search documents by text content using full-text search with term matching and relevance scoring.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection to search in"},
					"query":      map[string]interface{}{"type": "string", "description": "Search query text"},
					"limit":      map[string]interface{}{"type": "integer", "description": "Max results (default: 50)"},
				},
				"required": []string{"collection", "query"},
			},
		},
		{
			Name:        "register_webhook",
			Description: "Register a webhook to receive HTTP callbacks when documents are added, updated, or deleted.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url":        map[string]interface{}{"type": "string", "description": "Webhook endpoint URL"},
					"events":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Events: doc.added, doc.updated, doc.deleted"},
					"collection": map[string]interface{}{"type": "string", "description": "Filter to specific collection (empty = all)"},
				},
				"required": []string{"url", "events"},
			},
		},
		{
			Name:        "list_webhooks",
			Description: "List all registered webhooks.",
			InputSchema: map[string]interface{}{
				"type": "object",
			},
		},
		{
			Name:        "delete_webhook",
			Description: "Delete a registered webhook by ID.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Webhook ID to delete"},
				},
				"required": []string{"id"},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"tools": tools,
	}); err != nil {
		log.Printf("error encoding tools response: %v", err)
	}
}

// handleToolCall executes tool call.
func (s *Server) handleToolCall(w http.ResponseWriter, r *http.Request) {
	var req ToolCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	result, err := s.callTool(r.Context(), req.Name, req.Arguments)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": result,
			},
		},
	}); err != nil {
		log.Printf("error encoding tool call response: %v", err)
	}
}
