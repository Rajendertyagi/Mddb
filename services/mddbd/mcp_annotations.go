package main

// Annotation helpers — avoid allocating bools everywhere.
var (
	boolTrue  = true
	boolFalse = false
)

func readOnly() *MCPToolAnnotations {
	return &MCPToolAnnotations{ReadOnlyHint: &boolTrue, DestructiveHint: &boolFalse, IdempotentHint: &boolTrue, OpenWorldHint: &boolFalse}
}

func readOnlyOpenWorld() *MCPToolAnnotations {
	return &MCPToolAnnotations{ReadOnlyHint: &boolTrue, DestructiveHint: &boolFalse, IdempotentHint: &boolTrue, OpenWorldHint: &boolTrue}
}

func writeIdempotent() *MCPToolAnnotations {
	return &MCPToolAnnotations{ReadOnlyHint: &boolFalse, DestructiveHint: &boolFalse, IdempotentHint: &boolTrue, OpenWorldHint: &boolFalse}
}

func writeNonIdempotent() *MCPToolAnnotations {
	return &MCPToolAnnotations{ReadOnlyHint: &boolFalse, DestructiveHint: &boolFalse, IdempotentHint: &boolFalse, OpenWorldHint: &boolFalse}
}

func destructive() *MCPToolAnnotations {
	return &MCPToolAnnotations{ReadOnlyHint: &boolFalse, DestructiveHint: &boolTrue, IdempotentHint: &boolFalse, OpenWorldHint: &boolFalse}
}

func destructiveIdempotent() *MCPToolAnnotations {
	return &MCPToolAnnotations{ReadOnlyHint: &boolFalse, DestructiveHint: &boolTrue, IdempotentHint: &boolTrue, OpenWorldHint: &boolFalse}
}

func writeOpenWorld() *MCPToolAnnotations {
	return &MCPToolAnnotations{ReadOnlyHint: &boolFalse, DestructiveHint: &boolFalse, IdempotentHint: &boolFalse, OpenWorldHint: &boolTrue}
}

// mcpToolAnnotations maps tool name → annotations.
// Tools not in this map get no annotations (defaults per spec: readOnly=false, destructive=true).
var mcpToolAnnotations = map[string]*MCPToolAnnotations{
	// --- Read-only, closed world (database reads) ---
	"get_stats":              readOnly(),
	"search_documents":       readOnly(),
	"get_document_meta":      readOnly(),
	"vector_stats":           readOnly(),
	"fts_languages":          readOnly(),
	"list_webhooks":          readOnly(),
	"get_schema":             readOnly(),
	"list_schemas":           readOnly(),
	"validate_document":      readOnly(),
	"list_synonyms":          readOnly(),
	"list_stopwords":         readOnly(),
	"get_meta_keys":          readOnly(),
	"get_checksum":           readOnly(),
	"list_automation":        readOnly(),
	"get_automation":         readOnly(),
	"get_automation_logs":    readOnly(),
	"get_collection_config":  readOnly(),
	"list_collection_configs": readOnly(),
	"list_revisions":         readOnly(),
	"find_duplicates":        readOnly(),
	"aggregate":              readOnly(),
	"test_automation":        readOnly(),
	"export_documents":       readOnly(),

	// --- Read-only, open world (network/AI calls) ---
	"semantic_search":    readOnlyOpenWorld(),
	"full_text_search":   readOnly(),
	"hybrid_search":      readOnlyOpenWorld(),
	"cross_search":       readOnlyOpenWorld(),
	"classify_document":  readOnlyOpenWorld(),

	// --- Write, idempotent (upserts) ---
	"add_document":          writeIdempotent(),
	"update_document":       writeIdempotent(),
	"set_ttl":               writeIdempotent(),
	"set_schema":            writeIdempotent(),
	"add_synonym":           writeIdempotent(),
	"add_stopwords":         writeIdempotent(),
	"set_collection_config": writeIdempotent(),
	"update_automation":     writeIdempotent(),
	"restore_revision":      writeIdempotent(),

	// --- Write, non-idempotent ---
	"add_documents_batch":    writeNonIdempotent(),
	"ingest_documents":       writeNonIdempotent(),
	"create_automation":      writeNonIdempotent(),
	"register_webhook":       writeNonIdempotent(),
	"create_backup":          writeNonIdempotent(),

	// --- Write, open world (network calls) ---
	"import_url":    writeOpenWorld(),
	"upload_file":   writeNonIdempotent(),
	"vector_reindex": writeOpenWorld(),
	"fts_reindex":    writeNonIdempotent(),

	// --- Destructive, idempotent ---
	"delete_document":      destructiveIdempotent(),
	"delete_webhook":       destructiveIdempotent(),
	"delete_schema":        destructiveIdempotent(),
	"delete_synonym":       destructiveIdempotent(),
	"delete_stopwords":     destructiveIdempotent(),
	"delete_automation":    destructiveIdempotent(),

	// --- Destructive, non-idempotent ---
	"delete_documents_batch": destructive(),
	"delete_collection":      destructive(),
	"truncate_revisions":     destructive(),
	"restore_backup":         destructive(),
}

// annotateTools applies annotations to a tool list in-place.
func annotateTools(tools []MCPTool) []MCPTool {
	for i := range tools {
		if ann, ok := mcpToolAnnotations[tools[i].Name]; ok {
			tools[i].Annotations = ann
		}
	}
	return tools
}
