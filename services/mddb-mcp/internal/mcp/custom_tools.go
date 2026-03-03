package mcp

import (
	"context"
	"fmt"

	"github.com/tradik/mddb/services/mddb-mcp/internal/config"
)

// builtinTools returns the list of hardcoded MCP tools.
// Shared between handler.go (stdio) and server.go (HTTP).
func builtinTools() []Tool {
	return []Tool{
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
					"algorithm":   map[string]interface{}{"type": "string", "description": "Vector search algorithm: flat (exact, default), hnsw (approximate), ivf (clustered), pq (compressed)"},
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
			Description: "Search documents by text content using full-text search with term matching and relevance scoring. Supports typo tolerance via fuzzy parameter.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection to search in"},
					"query":      map[string]interface{}{"type": "string", "description": "Search query text"},
					"limit":      map[string]interface{}{"type": "integer", "description": "Max results (default: 50)"},
					"algorithm":  map[string]interface{}{"type": "string", "description": "Scoring algorithm: tfidf (default) or bm25"},
					"fuzzy":      map[string]interface{}{"type": "integer", "description": "Typo tolerance: 0 (off, default), 1 (1 char typo), 2 (2 char typos)"},
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
		{
			Name:        "set_schema",
			Description: "Set JSON Schema for collection metadata validation. Documents added to this collection will be validated against the schema.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
					"schema":     map[string]interface{}{"type": "string", "description": "JSON Schema as a string"},
				},
				"required": []string{"collection", "schema"},
			},
		},
		{
			Name:        "get_schema",
			Description: "Get JSON Schema for a collection.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
				},
				"required": []string{"collection"},
			},
		},
		{
			Name:        "delete_schema",
			Description: "Delete/disable schema validation for a collection.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
				},
				"required": []string{"collection"},
			},
		},
		{
			Name:        "list_schemas",
			Description: "List all collection schemas.",
			InputSchema: map[string]interface{}{
				"type": "object",
			},
		},
		{
			Name:        "validate_document",
			Description: "Validate document metadata against collection schema without adding the document.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
					"meta":       map[string]interface{}{"type": "object", "description": "Document metadata to validate"},
				},
				"required": []string{"collection", "meta"},
			},
		},
	}
}

// customToolToMCPTool converts a YAML custom tool definition into an MCP Tool.
func customToolToMCPTool(ct config.CustomToolConfig) Tool {
	properties := map[string]interface{}{}
	var required []string

	for _, p := range ct.Parameters {
		prop := map[string]interface{}{
			"type": p.Type,
		}
		if p.Description != "" {
			prop["description"] = p.Description
		}
		properties[p.Name] = prop
		if p.Required {
			required = append(required, p.Name)
		}
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}

	return Tool{
		Name:        ct.Name,
		Description: ct.Description,
		InputSchema: schema,
	}
}

// allTools returns built-in tools plus custom tools from config.
func allTools(customDefs []config.CustomToolConfig) []Tool {
	tools := builtinTools()
	for _, ct := range customDefs {
		tools = append(tools, customToolToMCPTool(ct))
	}
	return tools
}

// callCustomTool merges user-provided args with defaults, then delegates to
// the underlying built-in tool function.
func (s *Server) callCustomTool(ctx context.Context, ct config.CustomToolConfig, userArgs map[string]interface{}) (string, error) {
	merged := make(map[string]interface{})

	// Apply defaults based on action type
	d := ct.Defaults
	if d.Collection != "" {
		merged["collection"] = d.Collection
	}
	if d.Query != "" {
		merged["query"] = d.Query
	}

	switch ct.Action {
	case "semantic_search":
		if d.TopK > 0 {
			merged["top_k"] = float64(d.TopK)
		}
		if d.Threshold > 0 {
			merged["threshold"] = d.Threshold
		}
		if d.FilterMeta != nil {
			merged["filter_meta"] = metaToInterface(d.FilterMeta)
		}

	case "search_documents":
		if d.Sort != "" {
			merged["sort"] = d.Sort
		}
		if d.Asc != nil {
			merged["asc"] = *d.Asc
		}
		if d.Limit > 0 {
			merged["limit"] = float64(d.Limit)
		}
		if d.Offset > 0 {
			merged["offset"] = float64(d.Offset)
		}
		if d.FilterMeta != nil {
			merged["filter_meta"] = metaToInterface(d.FilterMeta)
		}

	case "full_text_search":
		if d.Limit > 0 {
			merged["limit"] = float64(d.Limit)
		}

	default:
		return "", fmt.Errorf("unknown custom tool action: %s", ct.Action)
	}

	// User args override defaults
	for k, v := range userArgs {
		merged[k] = v
	}

	// Delegate to the built-in tool by action name
	return s.callTool(ctx, ct.Action, merged)
}

// metaToInterface converts map[string][]string to map[string]interface{} for the arg parser.
func metaToInterface(meta map[string][]string) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range meta {
		items := make([]interface{}, len(v))
		for i, s := range v {
			items[i] = s
		}
		result[k] = items
	}
	return result
}
