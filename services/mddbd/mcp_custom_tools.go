package main

import (
	"context"
	"fmt"
	"os"
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
					"collection":      map[string]interface{}{"type": "string", "description": "Collection to search in"},
					"query":           map[string]interface{}{"type": "string", "description": "Natural language search query"},
					"top_k":           map[string]interface{}{"type": "integer", "description": "Number of results to return (default: 5)"},
					"threshold":       map[string]interface{}{"type": "number", "description": "Minimum similarity score 0-1 (default: 0.0)"},
					"filter_meta":     map[string]interface{}{"type": "object", "description": "Optional metadata filter to combine with semantic search"},
					"algorithm":       map[string]interface{}{"type": "string", "description": "Vector search algorithm: flat (exact, default), hnsw (approximate), ivf (clustered), pq (compressed)"},
					"distance_metric": map[string]interface{}{"type": "string", "description": "Distance metric: cosine (default), dot_product, euclidean"},
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
			Description: "Search documents by text content using full-text search with term matching and relevance scoring. Supports typo tolerance via fuzzy parameter and multi-language stemming.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection to search in"},
					"query":      map[string]interface{}{"type": "string", "description": "Search query text"},
					"limit":      map[string]interface{}{"type": "integer", "description": "Max results (default: 50)"},
					"algorithm":  map[string]interface{}{"type": "string", "description": "Scoring algorithm: tfidf (default), bm25, bm25f, or pmisparse"},
					"fuzzy":      map[string]interface{}{"type": "integer", "description": "Typo tolerance: 0 (off, default), 1 (1 char typo), 2 (2 char typos)"},
					"lang":       map[string]interface{}{"type": "string", "description": "Language for stemming/stop words (e.g. en, pl, de, fr, es). Default: server default language"},
				},
				"required": []string{"collection", "query"},
			},
		},
		{
			Name:        "fts_reindex",
			Description: "Reindex full-text search for a collection. Re-applies language-aware stemming and stop words using each document's lang field.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection to reindex"},
				},
				"required": []string{"collection"},
			},
		},
		{
			Name:        "fts_languages",
			Description: "List all supported FTS languages with their codes and names.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
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
		{
			Name:        "hybrid_search",
			Description: "Combined sparse (FTS) + dense (vector) search with alpha blending or Reciprocal Rank Fusion. Requires both FTS index and embedding provider.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection":       map[string]interface{}{"type": "string", "description": "Collection to search"},
					"query":            map[string]interface{}{"type": "string", "description": "Search query (used for both FTS and embedding)"},
					"top_k":            map[string]interface{}{"type": "integer", "description": "Number of results (default: 10)"},
					"algorithm":        map[string]interface{}{"type": "string", "description": "FTS algorithm: bm25, bm25f (default: bm25)"},
					"vector_algorithm": map[string]interface{}{"type": "string", "description": "Vector algorithm: flat, hnsw, ivf, pq (default: flat)"},
					"strategy":         map[string]interface{}{"type": "string", "description": "Fusion strategy: alpha or rrf (default: alpha)"},
					"alpha":            map[string]interface{}{"type": "number", "description": "Alpha weight 0-1 (0=keyword, 1=semantic, default: 0.5)"},
					"rrf_k":            map[string]interface{}{"type": "integer", "description": "RRF k parameter (default: 60)"},
					"fuzzy":            map[string]interface{}{"type": "integer", "description": "Typo tolerance: 0, 1, 2 (default: 0)"},
					"threshold":        map[string]interface{}{"type": "number", "description": "Min vector similarity 0-1"},
					"distance_metric":  map[string]interface{}{"type": "string", "description": "Distance metric: cosine (default), dot_product, euclidean"},
					"filter_meta":      map[string]interface{}{"type": "object", "description": "Metadata filter"},
				},
				"required": []string{"collection", "query"},
			},
		},
		{
			Name:        "delete_collection",
			Description: "Delete an entire collection and all its documents, revisions, and metadata indices.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name to delete"},
				},
				"required": []string{"collection"},
			},
		},
		{
			Name:        "truncate_revisions",
			Description: "Truncate revision history for a collection, keeping only the N most recent revisions per document.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
					"keep_revs":  map[string]interface{}{"type": "integer", "description": "Number of revisions to keep (0 = delete all history)"},
					"drop_cache": map[string]interface{}{"type": "boolean", "description": "Clear cache after truncation"},
				},
				"required": []string{"collection", "keep_revs"},
			},
		},
		{
			Name:        "list_synonyms",
			Description: "List all synonym entries for a collection. Synonyms expand FTS queries with related terms.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
				},
				"required": []string{"collection"},
			},
		},
		{
			Name:        "add_synonym",
			Description: "Add or update a synonym group for a term. Synonyms are bidirectional in FTS queries.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
					"term":       map[string]interface{}{"type": "string", "description": "Base term"},
					"synonyms":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "List of synonyms"},
				},
				"required": []string{"collection", "term", "synonyms"},
			},
		},
		{
			Name:        "delete_synonym",
			Description: "Delete a synonym group for a term in a collection.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
					"term":       map[string]interface{}{"type": "string", "description": "Term to remove synonyms for"},
				},
				"required": []string{"collection", "term"},
			},
		},
		{
			Name:        "list_stopwords",
			Description: "List all stop words (default + custom) for a collection. Stop words are excluded from FTS indexing.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
				},
				"required": []string{"collection"},
			},
		},
		{
			Name:        "add_stopwords",
			Description: "Add custom stop words to a collection's FTS index.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
					"words":      map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Words to add as stop words"},
				},
				"required": []string{"collection", "words"},
			},
		},
		{
			Name:        "delete_stopwords",
			Description: "Remove custom stop words from a collection.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
					"words":      map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Words to remove"},
				},
				"required": []string{"collection", "words"},
			},
		},
		{
			Name:        "get_meta_keys",
			Description: "List all unique metadata keys and their values for a collection. Useful for discovering available filter options.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
				},
				"required": []string{"collection"},
			},
		},
		{
			Name:        "get_checksum",
			Description: "Get a CRC32 checksum for a collection that changes when documents are modified. Useful for cache invalidation.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
				},
				"required": []string{"collection"},
			},
		},
		{
			Name:        "list_automation",
			Description: "List all automation rules (webhooks, triggers, crons). Optionally filter by type.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type": map[string]interface{}{"type": "string", "description": "Filter by rule type: webhook, trigger, or cron"},
				},
			},
		},
		{
			Name:        "create_automation",
			Description: "Create a new automation rule (webhook target, search trigger, or cron schedule).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type":       map[string]interface{}{"type": "string", "description": "Rule type: webhook, trigger, or cron"},
					"name":       map[string]interface{}{"type": "string", "description": "Rule name"},
					"enabled":    map[string]interface{}{"type": "boolean", "description": "Whether rule is enabled (default: true)"},
					"url":        map[string]interface{}{"type": "string", "description": "Webhook URL (type=webhook)"},
					"method":     map[string]interface{}{"type": "string", "description": "HTTP method POST/GET/PUT (type=webhook)"},
					"collection": map[string]interface{}{"type": "string", "description": "Target collection (type=trigger)"},
					"searchType": map[string]interface{}{"type": "string", "description": "Search type: fts, vector, hybrid (type=trigger)"},
					"query":      map[string]interface{}{"type": "string", "description": "Search query (type=trigger)"},
					"threshold":  map[string]interface{}{"type": "number", "description": "Score threshold 0-100 (type=trigger)"},
					"webhookId":  map[string]interface{}{"type": "string", "description": "Target webhook ID (type=trigger/cron)"},
					"events":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Events: insert, update, delete (type=trigger)"},
					"schedule":   map[string]interface{}{"type": "string", "description": "Cron expression (type=cron)"},
					"triggerId":  map[string]interface{}{"type": "string", "description": "Target trigger ID (type=cron)"},
				},
				"required": []string{"type", "name"},
			},
		},
		{
			Name:        "get_automation",
			Description: "Get a specific automation rule by ID.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Rule ID"},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "update_automation",
			Description: "Update an existing automation rule.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":         map[string]interface{}{"type": "string", "description": "Rule ID to update"},
					"name":       map[string]interface{}{"type": "string", "description": "Updated name"},
					"enabled":    map[string]interface{}{"type": "boolean", "description": "Enable/disable"},
					"url":        map[string]interface{}{"type": "string", "description": "Updated webhook URL"},
					"collection": map[string]interface{}{"type": "string", "description": "Updated collection"},
					"query":      map[string]interface{}{"type": "string", "description": "Updated query"},
					"threshold":  map[string]interface{}{"type": "number", "description": "Updated threshold"},
					"schedule":   map[string]interface{}{"type": "string", "description": "Updated cron schedule"},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "delete_automation",
			Description: "Delete an automation rule by ID.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Rule ID to delete"},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "test_automation",
			Description: "Test a trigger rule by running its search and returning matches (dry run, no webhook fired).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string", "description": "Trigger rule ID to test"},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "get_automation_logs",
			Description: "List automation execution logs with optional filtering by rule ID and status.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"limit":   map[string]interface{}{"type": "integer", "description": "Max entries (default: 50)"},
					"cursor":  map[string]interface{}{"type": "string", "description": "Pagination cursor"},
					"rule_id": map[string]interface{}{"type": "string", "description": "Filter by rule ID"},
					"status":  map[string]interface{}{"type": "string", "description": "Filter by status: success, error, skipped"},
				},
			},
		},
		// --- Collection Config ---
		{
			Name:        "get_collection_config",
			Description: "Get configuration attributes for a collection (type, description, icon, color, custom metadata).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
				},
				"required": []string{"collection"},
			},
		},
		{
			Name:        "set_collection_config",
			Description: "Set or update configuration attributes for a collection.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection":  map[string]interface{}{"type": "string", "description": "Collection name"},
					"type":        map[string]interface{}{"type": "string", "description": "Collection type: default, website, images, audio, documents"},
					"description": map[string]interface{}{"type": "string", "description": "Human-readable description"},
					"icon":        map[string]interface{}{"type": "string", "description": "Emoji or icon identifier"},
					"color":       map[string]interface{}{"type": "string", "description": "Hex color code (e.g. #3B82F6)"},
					"custom_meta": map[string]interface{}{"type": "object", "description": "Custom key-value metadata"},
				},
				"required": []string{"collection"},
			},
		},
		{
			Name:        "list_collection_configs",
			Description: "List all collections that have custom configuration set.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		// --- Cross-Collection Search ---
		{
			Name:        "cross_search",
			Description: "Search across multiple collections using a source document's embedding or a text query. Useful for finding related content across different collection types (e.g. matching images to blog posts).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"source_collection":  map[string]interface{}{"type": "string", "description": "Collection containing the source document"},
					"source_doc_id":      map[string]interface{}{"type": "string", "description": "Source document ID whose embedding to use as query"},
					"query":              map[string]interface{}{"type": "string", "description": "Text query (alternative to source_doc_id)"},
					"target_collections": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Collections to search in"},
					"top_k":              map[string]interface{}{"type": "integer", "description": "Number of results (default: 10)"},
					"threshold":          map[string]interface{}{"type": "number", "description": "Minimum similarity threshold 0-1"},
					"algorithm":          map[string]interface{}{"type": "string", "description": "Vector algorithm: flat (default), hnsw, ivf, pq, sq, bq"},
					"distance_metric":    map[string]interface{}{"type": "string", "description": "Distance metric: cosine (default), dot_product, euclidean"},
					"include_content":    map[string]interface{}{"type": "boolean", "description": "Include document content in results"},
				},
				"required": []string{"target_collections"},
			},
		},

		// --- Bulk Ingest ---
		{
			Name:        "ingest_documents",
			Description: "Bulk ingest documents with URL-based key derivation, YAML frontmatter extraction, content deduplication, and automatic metadata injection. Optimized for scraping and ETL pipelines.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Target collection"},
					"documents": map[string]interface{}{
						"type":        "array",
						"description": "Array of documents to ingest",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"url":                 map[string]interface{}{"type": "string", "description": "Source URL (key derived from URL if key is empty)"},
								"key":                 map[string]interface{}{"type": "string", "description": "Document key (optional if url is provided)"},
								"lang":                map[string]interface{}{"type": "string", "description": "Language code (e.g. en, pl)"},
								"content_md":          map[string]interface{}{"type": "string", "description": "Markdown content"},
								"meta":                map[string]interface{}{"type": "object", "description": "Metadata key-value pairs"},
								"extract_frontmatter": map[string]interface{}{"type": "boolean", "description": "Parse YAML frontmatter from content"},
								"scraped_at":          map[string]interface{}{"type": "integer", "description": "Unix timestamp of when content was collected"},
								"scraper":             map[string]interface{}{"type": "string", "description": "Source identifier"},
								"ttl":                 map[string]interface{}{"type": "integer", "description": "Time-to-live in seconds"},
							},
							"required": []string{"lang", "content_md"},
						},
					},
					"options": map[string]interface{}{
						"type":        "object",
						"description": "Ingest options",
						"properties": map[string]interface{}{
							"skip_duplicates":           map[string]interface{}{"type": "boolean", "description": "Skip documents whose content matches existing (CRC32 hash)"},
							"skip_embeddings":           map[string]interface{}{"type": "boolean", "description": "Skip embedding generation"},
							"skip_fts":                  map[string]interface{}{"type": "boolean", "description": "Skip full-text indexing"},
							"skip_webhooks":             map[string]interface{}{"type": "boolean", "description": "Skip webhook firing"},
							"auto_configure_collection": map[string]interface{}{"type": "boolean", "description": "Auto-set collection type to 'scraping'"},
							"save_revision":             map[string]interface{}{"type": "boolean", "description": "Save revision history for each document"},
						},
					},
				},
				"required": []string{"collection", "documents"},
			},
		},

		// --- File Upload ---
		{
			Name:        "upload_file",
			Description: "Upload a file and convert it to markdown. Supports md, txt, html, pdf, and docx formats. Plain text and markdown files are stored as-is; other formats are auto-converted to markdown. File content is passed as base64-encoded string.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Target collection"},
					"filename":   map[string]interface{}{"type": "string", "description": "Original filename with extension (e.g. report.pdf). Extension determines conversion format."},
					"content":    map[string]interface{}{"type": "string", "description": "Base64-encoded file content"},
					"key":        map[string]interface{}{"type": "string", "description": "Document key (optional, derived from filename if empty)"},
					"lang":       map[string]interface{}{"type": "string", "description": "Language code (e.g. en_US, pl_PL)"},
					"meta":       map[string]interface{}{"type": "object", "description": "Metadata key-value pairs"},
					"ttl":        map[string]interface{}{"type": "integer", "description": "Time-to-live in seconds (0 = no expiry)"},
				},
				"required": []string{"collection", "filename", "content", "lang"},
			},
		},

		// --- Revisions ---
		{
			Name:        "list_revisions",
			Description: "List revision history for a document. Shows all saved versions with timestamps.",
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
			Name:        "restore_revision",
			Description: "Restore a document to a previous revision by timestamp.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection": map[string]interface{}{"type": "string", "description": "Collection name"},
					"key":        map[string]interface{}{"type": "string", "description": "Document key"},
					"lang":       map[string]interface{}{"type": "string", "description": "Language code"},
					"timestamp":  map[string]interface{}{"type": "integer", "description": "Unix timestamp of the revision to restore"},
				},
				"required": []string{"collection", "key", "lang", "timestamp"},
			},
		},

		// --- Duplicate Detection ---
		{
			Name:        "find_duplicates",
			Description: "Find duplicate and similar documents within a collection. Detects exact duplicates (same content hash) and semantically similar documents (above similarity threshold). Requires documents to have embeddings for similar mode.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"collection":      map[string]interface{}{"type": "string", "description": "Collection to scan for duplicates"},
					"mode":            map[string]interface{}{"type": "string", "description": "Detection mode: exact, similar, or both (default: both)"},
					"threshold":       map[string]interface{}{"type": "number", "description": "Similarity threshold 0-1 for similar mode (default: 0.9)"},
					"max_docs":        map[string]interface{}{"type": "integer", "description": "Max documents to process (default: 5000)"},
					"distance_metric": map[string]interface{}{"type": "string", "description": "Distance metric: cosine (default), dot_product, euclidean"},
					"include_content": map[string]interface{}{"type": "boolean", "description": "Include document content in results"},
				},
				"required": []string{"collection"},
			},
		},

		// --- Memory RAG ---
		{
			Name:        "memory_start_session",
			Description: "Start a new memory/conversation session for RAG. Returns a session ID that can be used to add messages and recall context later.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"user_id":  map[string]interface{}{"type": "string", "description": "User identifier for the session"},
					"scenario": map[string]interface{}{"type": "string", "description": "Scenario or context name (e.g. 'customer_support', 'code_review')"},
					"title":    map[string]interface{}{"type": "string", "description": "Human-readable session title"},
					"meta":     map[string]interface{}{"type": "object", "description": "Additional metadata key-value pairs"},
					"ttl":      map[string]interface{}{"type": "integer", "description": "Session TTL in seconds (default: 30 days)"},
				},
				"required": []string{"user_id"},
			},
		},
		{
			Name:        "memory_add_message",
			Description: "Add a message to an existing memory session. Messages are automatically embedded for semantic recall.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"session_id": map[string]interface{}{"type": "string", "description": "Session ID returned from memory_start_session"},
					"role":       map[string]interface{}{"type": "string", "description": "Message role: user, assistant, system, or tool"},
					"content":    map[string]interface{}{"type": "string", "description": "Message content (markdown supported)"},
					"meta":       map[string]interface{}{"type": "object", "description": "Additional metadata (e.g. topic, source, tool_call)"},
				},
				"required": []string{"session_id", "role", "content"},
			},
		},
		{
			Name:        "memory_recall",
			Description: "Semantically recall relevant messages from past conversations. Uses hybrid search (vector + keyword) to find the most relevant context across all sessions or filtered by user/session.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query":           map[string]interface{}{"type": "string", "description": "Natural language query to recall relevant context"},
					"user_id":         map[string]interface{}{"type": "string", "description": "Filter recall to sessions belonging to this user"},
					"session_id":      map[string]interface{}{"type": "string", "description": "Filter recall to a specific session"},
					"role":            map[string]interface{}{"type": "string", "description": "Filter by message role (user, assistant, system, tool)"},
					"top_k":           map[string]interface{}{"type": "integer", "description": "Number of results (default: 10)"},
					"threshold":       map[string]interface{}{"type": "number", "description": "Min similarity score 0-1 (default: 0.0)"},
					"strategy":        map[string]interface{}{"type": "string", "description": "Search strategy: hybrid (default), semantic, keyword"},
					"include_content": map[string]interface{}{"type": "boolean", "description": "Include full message content (default: false)"},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "memory_summarize",
			Description: "Generate a summary of a conversation session. Stores the summary as a document with embeddings for future recall.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"session_id": map[string]interface{}{"type": "string", "description": "Session ID to summarize"},
					"user_id":    map[string]interface{}{"type": "string", "description": "User ID for validation"},
				},
				"required": []string{"session_id"},
			},
		},
		{
			Name:        "memory_list_sessions",
			Description: "List memory/conversation sessions with optional filtering by user, scenario, and sorting.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"user_id":  map[string]interface{}{"type": "string", "description": "Filter sessions by user ID"},
					"scenario": map[string]interface{}{"type": "string", "description": "Filter sessions by scenario"},
					"limit":    map[string]interface{}{"type": "integer", "description": "Max results (default: 50)"},
					"offset":   map[string]interface{}{"type": "integer", "description": "Results offset for pagination"},
					"sort":     map[string]interface{}{"type": "string", "description": "Sort by: createdAt (default), updatedAt"},
				},
			},
		},
		{
			Name:        "memory_session_history",
			Description: "Get the full message history of a specific conversation session, ordered chronologically.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"session_id": map[string]interface{}{"type": "string", "description": "Session ID to fetch history for"},
					"limit":      map[string]interface{}{"type": "integer", "description": "Max messages (default: 100)"},
					"offset":     map[string]interface{}{"type": "integer", "description": "Message offset for pagination"},
				},
				"required": []string{"session_id"},
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

// mcpAllTools returns built-in tools plus custom tools from config, with annotations and output schemas.
// Set MDDB_MCP_BUILTIN_TOOLS=false to expose only custom tools (no built-in tools).
func mcpAllTools(customDefs []MCPCustomToolConfig) []MCPTool {
	var tools []MCPTool
	if os.Getenv("MDDB_MCP_BUILTIN_TOOLS") != "false" {
		tools = mcpBuiltinTools()
	}
	for _, ct := range customDefs {
		tools = append(tools, mcpCustomToolToMCPTool(ct))
	}
	tools = annotateTools(tools)
	tools = applyOutputSchemas(tools)
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
		"import_url": true, "set_ttl": true, "full_text_search": true, "fts_reindex": true, "fts_languages": true,
		"hybrid_search":    true,
		"register_webhook": true, "list_webhooks": true, "delete_webhook": true,
		"set_schema": true, "get_schema": true, "delete_schema": true,
		"list_schemas": true, "validate_document": true,
		"update_document": true, "get_document_meta": true,
		"classify_document": true,
		"delete_collection": true, "truncate_revisions": true,
		"list_revisions": true, "restore_revision": true,
		"list_synonyms": true, "add_synonym": true, "delete_synonym": true,
		"list_stopwords": true, "add_stopwords": true, "delete_stopwords": true,
		"get_meta_keys": true, "get_checksum": true,
		"list_automation": true, "create_automation": true, "get_automation": true,
		"update_automation": true, "delete_automation": true, "test_automation": true,
		"get_automation_logs":   true,
		"get_collection_config": true, "set_collection_config": true, "list_collection_configs": true,
		"cross_search":     true,
		"find_duplicates":  true,
		"ingest_documents": true,
		"upload_file":      true,
	}
	validActions := map[string]bool{
		"semantic_search": true, "search_documents": true, "full_text_search": true, "fts_languages": true,
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
