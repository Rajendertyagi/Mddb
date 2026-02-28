package mcp

import (
	"context"
	"encoding/json"

	"github.com/tradik/mddb/services/mddb-mcp/internal/mddb"
)

// Handler handles MCP requests via stdio.
type Handler struct {
	client mddb.Client
}

// NewHandler creates a new MCP handler.
func NewHandler(client mddb.Client) *Handler {
	return &Handler{
		client: client,
	}
}

// Handle processes MCP request and returns response.
func (h *Handler) Handle(req map[string]interface{}) map[string]interface{} {
	method, _ := req["method"].(string)
	id := req["id"]
	ctx := context.Background()

	var result map[string]interface{}
	var err map[string]interface{}

	switch method {
	case "initialize":
		// initialize already returns full JSON-RPC response
		return h.handleInitialize(req)
	case "resources/list":
		result = h.handleResourcesList()
	case "resources/read":
		result = h.handleResourcesRead(ctx, req)
	case "tools/list":
		result = h.handleToolsList()
	case "tools/call":
		result = h.handleToolsCall(ctx, req)
	case "ping":
		result = map[string]interface{}{"result": "pong"}
	default:
		err = map[string]interface{}{
			"code":    -32601,
			"message": "Method not found",
		}
	}

	// Wrap response in JSON-RPC format
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
	}

	if err != nil {
		response["error"] = err
	} else {
		response["result"] = result
	}

	return response
}

func (h *Handler) handleInitialize(req map[string]interface{}) map[string]interface{} {
	// Extract request ID for JSON-RPC response
	id := req["id"]

	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"resources": map[string]interface{}{
					"subscribe":   false,
					"listChanged": false,
				},
				"tools": map[string]interface{}{
					"listChanged": false,
				},
			},
			"serverInfo": map[string]interface{}{
				"name":    "mddb-mcp",
				"version": "1.0.0",
			},
		},
	}
}

func (h *Handler) handleResourcesList() map[string]interface{} {
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

	return map[string]interface{}{
		"resources": resources,
	}
}

func (h *Handler) handleResourcesRead(ctx context.Context, req map[string]interface{}) map[string]interface{} {
	params, _ := req["params"].(map[string]interface{})
	uri, _ := params["uri"].(string)

	s := &Server{client: h.client}
	content, err := s.readResource(ctx, uri)
	if err != nil {
		return map[string]interface{}{
			"error": map[string]interface{}{
				"code":    -32000,
				"message": err.Error(),
			},
		}
	}

	return map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"uri":      uri,
				"mimeType": "application/json",
				"text":     content,
			},
		},
	}
}

func (h *Handler) handleToolsList() map[string]interface{} {
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

	return map[string]interface{}{
		"tools": tools,
	}
}

func (h *Handler) handleToolsCall(ctx context.Context, req map[string]interface{}) map[string]interface{} {
	params, _ := req["params"].(map[string]interface{})
	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]interface{})

	s := &Server{client: h.client}
	result, err := s.callTool(ctx, name, args)
	if err != nil {
		return map[string]interface{}{
			"error": map[string]interface{}{
				"code":    -32000,
				"message": err.Error(),
			},
		}
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": result,
			},
		},
	}
}

// HandleJSON processes JSON request and returns JSON response.
func (h *Handler) HandleJSON(reqJSON []byte) ([]byte, error) {
	var req map[string]interface{}
	if err := json.Unmarshal(reqJSON, &req); err != nil {
		return nil, err
	}

	resp := h.Handle(req)
	return json.Marshal(resp)
}
