package main

import (
	"context"
	"fmt"
)

// MCPCustomToolConfig defines a single custom YAML tool.
type MCPCustomToolConfig struct {
	Name        string               `yaml:"name"`
	Description string               `yaml:"description"`
	Action      string               `yaml:"action"` // semantic_search, search_documents, full_text_search
	Defaults    MCPCustomToolDefs    `yaml:"defaults"`
	Parameters  []MCPCustomToolParam `yaml:"parameters"`
}

// MCPCustomToolDefs holds pre-filled arguments for the underlying action.
type MCPCustomToolDefs struct {
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

// MCPCustomToolParam defines a parameter exposed to the AI.
type MCPCustomToolParam struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"` // string, integer, number, boolean, object
	Required    bool   `yaml:"required"`
	Description string `yaml:"description"`
}

// mcpBuiltinTools returns the list of hardcoded MCP tools.
func mcpBuiltinTools() []MCPTool {
	return []MCPTool{
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
					"algorithm":  map[string]interface{}{"type": "string", "description": "Scoring algorithm: tfidf (default), bm25, bm25f, or pmisparse"},
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
			InputSchema: map[string]interface{}{"type": "object"},
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
			InputSchema: map[string]interface{}{"type": "object"},
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
		{
			Name:        "add_documents_batch",
			Description: "Add multiple documents to a collection in a single batch operation.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
					"documents":  map[string]interface{}{"type": "array", "description": "Array of documents with key, lang, content_md, meta fields"},
				},
				"required": []string{"collection", "documents"},
			},
		},
		{
			Name:        "delete_documents_batch",
			Description: "Delete multiple documents from a collection in a single batch operation.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
					"documents":  map[string]interface{}{"type": "array", "description": "Array of documents with key and lang fields"},
				},
				"required": []string{"collection", "documents"},
			},
		},
		{
			Name:        "export_documents",
			Description: "Export documents from a collection in NDJSON format.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection":  map[string]interface{}{"type": "string", "description": "Collection to export"},
					"filter_meta": map[string]interface{}{"type": "object", "description": "Optional metadata filter"},
					"format":      map[string]interface{}{"type": "string", "description": "Export format: ndjson (default) or zip"},
				},
				"required": []string{"collection"},
			},
		},
		{
			Name:        "create_backup",
			Description: "Create a backup of the MDDB database.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"to": map[string]interface{}{"type": "string", "description": "Backup destination path"},
				},
			},
		},
		{
			Name:        "restore_backup",
			Description: "Restore the MDDB database from a backup file.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"from": map[string]interface{}{"type": "string", "description": "Backup file path to restore from"},
				},
				"required": []string{"from"},
			},
		},
		{
			Name:        "update_document",
			Description: "Partially update a document. Update metadata and/or content independently without re-sending the entire document. Omit fields to leave them unchanged.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
					"key":        map[string]interface{}{"type": "string", "description": "Document key"},
					"lang":       map[string]interface{}{"type": "string", "description": "Language code"},
					"meta":       map[string]interface{}{"type": "object", "description": "New metadata (replaces all). Use {} to clear."},
					"content_md": map[string]interface{}{"type": "string", "description": "New markdown content (replaces existing)"},
					"ttl":        map[string]interface{}{"type": "integer", "description": "New TTL in seconds (0 = no expiry)"},
				},
				"required": []string{"collection", "key", "lang"},
			},
		},
		{
			Name:        "get_document_meta",
			Description: "Get document metadata without content. Lightweight read that returns only key, lang, meta, and timestamps.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
					"key":        map[string]interface{}{"type": "string", "description": "Document key"},
					"lang":       map[string]interface{}{"type": "string", "description": "Language code"},
				},
				"required": []string{"collection", "key", "lang"},
			},
		},
		{
			Name:        "classify_document",
			Description: "Zero-shot document classification. Given candidate labels and either a document reference or raw text, ranks labels by semantic similarity using embeddings. No training data required.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name (for doc reference)"},
					"key":        map[string]interface{}{"type": "string", "description": "Document key (for doc reference)"},
					"lang":       map[string]interface{}{"type": "string", "description": "Language code (for doc reference)"},
					"text":       map[string]interface{}{"type": "string", "description": "Raw text to classify (alternative to doc reference)"},
					"labels":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Candidate labels to rank by similarity"},
					"top_k":      map[string]interface{}{"type": "integer", "description": "Return top K labels (0 = all, default: all)"},
					"multi":      map[string]interface{}{"type": "boolean", "description": "If true, return all labels above threshold"},
					"threshold":  map[string]interface{}{"type": "number", "description": "Minimum similarity score (default: 0.0)"},
				},
				"required": []string{"labels"},
			},
		},
	}
}

// mcpCustomToolToMCPTool converts a YAML custom tool definition into an MCPTool.
func mcpCustomToolToMCPTool(ct MCPCustomToolConfig) MCPTool {
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

	return MCPTool{
		Name:        ct.Name,
		Description: ct.Description,
		InputSchema: schema,
	}
}

// mcpAllTools returns built-in tools plus custom tools from config.
func mcpAllTools(customDefs []MCPCustomToolConfig) []MCPTool {
	tools := mcpBuiltinTools()
	for _, ct := range customDefs {
		tools = append(tools, mcpCustomToolToMCPTool(ct))
	}
	return tools
}

// mcpCallCustomTool merges user-provided args with defaults, then delegates to the built-in tool.
func (s *MCPToolServer) mcpCallCustomTool(ctx context.Context, ct MCPCustomToolConfig, userArgs map[string]interface{}) (string, error) {
	merged := make(map[string]interface{})

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
			merged["filter_meta"] = mcpMetaToInterface(d.FilterMeta)
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
			merged["filter_meta"] = mcpMetaToInterface(d.FilterMeta)
		}
	case "full_text_search":
		if d.Limit > 0 {
			merged["limit"] = float64(d.Limit)
		}
	default:
		return "", fmt.Errorf("unknown custom tool action: %s", ct.Action)
	}

	for k, v := range userArgs {
		merged[k] = v
	}

	return s.mcpCallTool(ctx, ct.Action, merged)
}

func mcpMetaToInterface(meta map[string][]string) map[string]interface{} {
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

// validateMCPCustomTools validates custom tool definitions.
func validateMCPCustomTools(tools []MCPCustomToolConfig) error {
	builtinNames := map[string]bool{
		"add_document": true, "search_documents": true, "delete_document": true,
		"get_stats": true, "add_documents_batch": true, "delete_documents_batch": true,
		"export_documents": true, "create_backup": true, "restore_backup": true,
		"semantic_search": true, "vector_reindex": true, "vector_stats": true,
		"import_url": true, "set_ttl": true, "full_text_search": true,
		"register_webhook": true, "list_webhooks": true, "delete_webhook": true,
		"set_schema": true, "get_schema": true, "delete_schema": true,
		"list_schemas": true, "validate_document": true,
		"update_document": true, "get_document_meta": true,
		"classify_document": true,
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
