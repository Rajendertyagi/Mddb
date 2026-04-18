package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// MCPToolServer provides tool and resource call dispatch.
type MCPToolServer struct {
	client      MCPClient
	customTools []MCPCustomToolConfig
	globalMode  AccessMode // server-wide mode (from MDDB_MODE / follower)
	mode        AccessMode // per-protocol override (from MDDB_MCP_MODE, "" = inherit global)
}

// isToolReadOnly returns true if a tool (builtin or custom) is safe for read-only mode.
func (s *MCPToolServer) isToolReadOnly(name string) bool {
	// Check builtin tool annotations first
	if ann, ok := mcpToolAnnotations[name]; ok {
		return ann.ReadOnlyHint != nil && *ann.ReadOnlyHint
	}
	// Check custom tools — their underlying actions are all read-only
	// (semantic_search, search_documents, full_text_search, fts_languages)
	for _, ct := range s.customTools {
		if ct.Name == name {
			return mcpCustomToolActionReadOnly[ct.Action]
		}
	}
	return false
}

// mcpCustomToolActionReadOnly maps custom tool action names to their read-only status.
var mcpCustomToolActionReadOnly = map[string]bool{
	"semantic_search":  true,
	"search_documents": true,
	"full_text_search": true,
	"fts_languages":    true,
}

// mcpCallTool invokes an MCP tool by name.
func (s *MCPToolServer) mcpCallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	// Enforce read-only mode: per-protocol override takes precedence, then global mode.
	em := effectiveMode(s.globalMode, s.mode)
	if em == ModeRead {
		if !s.isToolReadOnly(name) {
			return "", fmt.Errorf("tool %q is not available in read-only mode", name)
		}
	}

	switch name {
	case "add_document":
		return s.toolAddDocument(ctx, args)
	case "search_documents":
		return s.toolSearchDocuments(ctx, args)
	case "delete_document":
		return s.toolDeleteDocument(ctx, args)
	case "get_stats":
		return s.toolGetStats(ctx, args)
	case "add_documents_batch":
		return s.toolAddBatch(ctx, args)
	case "delete_documents_batch":
		return s.toolDeleteBatch(ctx, args)
	case "export_documents":
		return s.toolExport(ctx, args)
	case "create_backup":
		return s.toolBackup(ctx, args)
	case "restore_backup":
		return s.toolRestore(ctx, args)
	case "semantic_search":
		return s.toolSemanticSearch(ctx, args)
	case "vector_reindex":
		return s.toolVectorReindex(ctx, args)
	case "vector_stats":
		return s.toolVectorStats(ctx, args)
	case "import_url":
		return s.toolImportURL(ctx, args)
	case "set_ttl":
		return s.toolSetTTL(ctx, args)
	case "bulk_ingest_submit":
		return s.toolBulkIngestSubmit(ctx, args)
	case "bulk_ingest_status":
		return s.toolBulkIngestStatus(ctx, args)
	case "bulk_ingest_list":
		return s.toolBulkIngestList(ctx, args)
	case "bulk_ingest_cancel":
		return s.toolBulkIngestCancel(ctx, args)
	case "full_text_search":
		return s.toolFTSSearch(ctx, args)
	case "fts_reindex":
		return s.toolFTSReindex(ctx, args)
	case "fts_languages":
		return s.toolFTSLanguages(ctx, args)
	case "autocomplete":
		return s.toolAutocomplete(ctx, args)
	case "hybrid_search":
		return s.toolHybridSearch(ctx, args)
	case "geo_search":
		return s.toolGeoSearch(ctx, args)
	case "geo_within":
		return s.toolGeoWithin(ctx, args)
	case "geo_stats":
		return s.toolGeoStats(ctx, args)
	case "geo_encode":
		return s.toolGeoEncode(ctx, args)
	case "geo_decode":
		return s.toolGeoDecode(ctx, args)
	case "register_webhook":
		return s.toolRegisterWebhook(ctx, args)
	case "list_webhooks":
		return s.toolListWebhooks(ctx, args)
	case "delete_webhook":
		return s.toolDeleteWebhook(ctx, args)
	case "set_schema":
		return s.toolSetSchema(ctx, args)
	case "get_schema":
		return s.toolGetSchema(ctx, args)
	case "delete_schema":
		return s.toolDeleteSchema(ctx, args)
	case "list_schemas":
		return s.toolListSchemas(ctx, args)
	case "validate_document":
		return s.toolValidateDocument(ctx, args)
	case "update_document":
		return s.toolUpdateDocument(ctx, args)
	case "get_document_meta":
		return s.toolGetDocumentMeta(ctx, args)
	case "classify_document":
		return s.toolClassifyDocument(ctx, args)
	case "delete_collection":
		return s.toolDeleteCollection(ctx, args)
	case "truncate_revisions":
		return s.toolTruncateRevisions(ctx, args)
	case "list_revisions":
		return s.toolListRevisions(ctx, args)
	case "restore_revision":
		return s.toolRestoreRevision(ctx, args)
	case "list_synonyms":
		return s.toolListSynonyms(ctx, args)
	case "add_synonym":
		return s.toolAddSynonym(ctx, args)
	case "delete_synonym":
		return s.toolDeleteSynonym(ctx, args)
	case "list_stopwords":
		return s.toolListStopWords(ctx, args)
	case "add_stopwords":
		return s.toolAddStopWords(ctx, args)
	case "delete_stopwords":
		return s.toolDeleteStopWords(ctx, args)
	case "get_meta_keys":
		return s.toolGetMetaKeys(ctx, args)
	case "get_checksum":
		return s.toolGetChecksum(ctx, args)
	case "list_automation":
		return s.toolListAutomation(ctx, args)
	case "create_automation":
		return s.toolCreateAutomation(ctx, args)
	case "get_automation":
		return s.toolGetAutomation(ctx, args)
	case "update_automation":
		return s.toolUpdateAutomation(ctx, args)
	case "delete_automation":
		return s.toolDeleteAutomation(ctx, args)
	case "test_automation":
		return s.toolTestAutomation(ctx, args)
	case "get_automation_logs":
		return s.toolGetAutomationLogs(ctx, args)
	case "get_collection_config":
		return s.toolGetCollectionConfig(ctx, args)
	case "set_collection_config":
		return s.toolSetCollectionConfig(ctx, args)
	case "list_collection_configs":
		return s.toolListCollectionConfigs(ctx, args)
	case "cross_search":
		return s.toolCrossSearch(ctx, args)
	case "find_duplicates":
		return s.toolFindDuplicates(ctx, args)
	case "aggregate":
		return s.toolAggregate(ctx, args)
	case "ingest_documents":
		return s.toolIngest(ctx, args)
	case "upload_file":
		return s.toolUploadFile(ctx, args)
	// Memory RAG tools
	case "memory_start_session":
		return s.toolMemoryStartSession(ctx, args)
	case "memory_add_message":
		return s.toolMemoryAddMessage(ctx, args)
	case "memory_recall":
		return s.toolMemoryRecall(ctx, args)
	case "memory_summarize":
		return s.toolMemorySummarize(ctx, args)
	case "memory_list_sessions":
		return s.toolMemoryListSessions(ctx, args)
	case "memory_session_history":
		return s.toolMemorySessionHistory(ctx, args)
	default:
		for _, ct := range s.customTools {
			if ct.Name == name {
				return s.mcpCallCustomTool(ctx, ct, args)
			}
		}
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func (s *MCPToolServer) toolAddDocument(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPAddRequest{
		Collection: mcpGetString(args, "collection"),
		Key:        mcpGetString(args, "key"),
		Lang:       mcpGetString(args, "lang"),
		ContentMD:  mcpGetString(args, "content_md"),
		Meta:       mcpGetMetaMap(args, "meta"),
	}

	doc, err := s.client.Add(ctx, req)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(doc, "", "  ")
	return fmt.Sprintf("Document added successfully:\n%s", string(data)), nil
}

func (s *MCPToolServer) toolSearchDocuments(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPSearchRequest{
		Collection: mcpGetString(args, "collection"),
		FilterMeta: mcpGetMetaMap(args, "filter_meta"),
		Sort:       mcpGetString(args, "sort"),
		Limit:      mcpGetInt(args, "limit"),
		Offset:     mcpGetInt(args, "offset"),
	}
	if asc, ok := args["asc"].(bool); ok {
		req.Asc = asc
	}

	resp, err := s.client.Search(ctx, req)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolDeleteDocument(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPDeleteRequest{
		Collection: mcpGetString(args, "collection"),
		Key:        mcpGetString(args, "key"),
		Lang:       mcpGetString(args, "lang"),
	}

	if err := s.client.Delete(ctx, req); err != nil {
		return "", err
	}

	return fmt.Sprintf("Document deleted: %s/%s (%s)", req.Collection, req.Key, req.Lang), nil
}

func (s *MCPToolServer) toolGetStats(ctx context.Context, args map[string]interface{}) (string, error) {
	stats, err := s.client.Stats(ctx)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(stats, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolAddBatch(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	docsRaw, ok := args["documents"].([]interface{})
	if !ok {
		return "", fmt.Errorf("documents must be an array")
	}

	docs := make([]MCPBatchDocument, len(docsRaw))
	for i, d := range docsRaw {
		docMap, ok := d.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("invalid document at index %d", i)
		}
		docs[i] = MCPBatchDocument{
			Key:       mcpGetString(docMap, "key"),
			Lang:      mcpGetString(docMap, "lang"),
			ContentMD: mcpGetString(docMap, "content_md"),
			Meta:      mcpGetMetaMap(docMap, "meta"),
		}
	}

	resp, err := s.client.AddBatch(ctx, &MCPAddBatchRequest{
		Collection: collection,
		Documents:  docs,
	})
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolDeleteBatch(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	docsRaw, ok := args["documents"].([]interface{})
	if !ok {
		return "", fmt.Errorf("documents must be an array")
	}

	docs := make([]MCPDeleteDocument, len(docsRaw))
	for i, d := range docsRaw {
		docMap, ok := d.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("invalid document at index %d", i)
		}
		docs[i] = MCPDeleteDocument{
			Key:  mcpGetString(docMap, "key"),
			Lang: mcpGetString(docMap, "lang"),
		}
	}

	resp, err := s.client.DeleteBatch(ctx, &MCPDeleteBatchRequest{
		Collection: collection,
		Documents:  docs,
	})
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolExport(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPExportRequest{
		Collection: mcpGetString(args, "collection"),
		FilterMeta: mcpGetMetaMap(args, "filter_meta"),
		Format:     mcpGetString(args, "format"),
	}

	stream, err := s.client.Export(ctx, req)
	if err != nil {
		return "", err
	}
	defer func() { _ = stream.Close() }()

	return "Export started (stream not fully implemented in MCP yet)", nil
}

func (s *MCPToolServer) toolBackup(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPBackupRequest{
		To: mcpGetString(args, "to"),
	}

	resp, err := s.client.Backup(ctx, req)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Backup created: %s", resp.Backup), nil
}

func (s *MCPToolServer) toolRestore(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPRestoreRequest{
		From: mcpGetString(args, "from"),
	}

	resp, err := s.client.Restore(ctx, req)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Database restored from: %s", resp.Restored), nil
}

func (s *MCPToolServer) toolSemanticSearch(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPVectorSearchRequest{
		Collection:     mcpGetString(args, "collection"),
		Query:          mcpGetString(args, "query"),
		TopK:           mcpGetInt(args, "top_k"),
		IncludeContent: true,
		FilterMeta:     mcpGetMetaMap(args, "filter_meta"),
		Algorithm:      mcpGetString(args, "algorithm"),
		DistanceMetric: mcpGetString(args, "distance_metric"),
	}

	if threshold, ok := args["threshold"].(float64); ok {
		req.Threshold = threshold
	}

	resp, err := s.client.VectorSearch(ctx, req)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolVectorReindex(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPVectorReindexRequest{
		Collection: mcpGetString(args, "collection"),
	}
	if force, ok := args["force"].(bool); ok {
		req.Force = force
	}

	resp, err := s.client.VectorReindex(ctx, req)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return fmt.Sprintf("Reindex complete:\n%s", string(data)), nil
}

func (s *MCPToolServer) toolVectorStats(ctx context.Context, args map[string]interface{}) (string, error) {
	resp, err := s.client.VectorStats(ctx)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolImportURL(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPImportURLRequest{
		Collection: mcpGetString(args, "collection"),
		URL:        mcpGetString(args, "url"),
		Key:        mcpGetString(args, "key"),
		Lang:       mcpGetString(args, "lang"),
		Meta:       mcpGetMetaMap(args, "meta"),
		TTL:        int64(mcpGetInt(args, "ttl")),
	}

	doc, err := s.client.ImportURL(ctx, req)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(doc, "", "  ")
	return fmt.Sprintf("Document imported from URL:\n%s", string(data)), nil
}

func (s *MCPToolServer) toolSetTTL(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPSetTTLRequest{
		Collection: mcpGetString(args, "collection"),
		Key:        mcpGetString(args, "key"),
		Lang:       mcpGetString(args, "lang"),
		TTL:        int64(mcpGetInt(args, "ttl")),
	}

	doc, err := s.client.SetTTL(ctx, req)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(doc, "", "  ")
	return fmt.Sprintf("TTL updated:\n%s", string(data)), nil
}

func (s *MCPToolServer) toolFTSSearch(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPFTSSearchRequest{
		Collection: mcpGetString(args, "collection"),
		Query:      mcpGetString(args, "query"),
		Limit:      mcpGetInt(args, "limit"),
		Algorithm:  mcpGetString(args, "algorithm"),
		Fuzzy:      mcpGetInt(args, "fuzzy"),
		Lang:       mcpGetString(args, "lang"),
		Boost:      mcpGetFloat64Map(args, "boost"),
	}

	resp, err := s.client.FTSSearch(ctx, req)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolFTSReindex(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPFTSReindexRequest{
		Collection: mcpGetString(args, "collection"),
	}
	resp, err := s.client.FTSReindex(ctx, req)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolFTSLanguages(ctx context.Context, args map[string]interface{}) (string, error) {
	resp, err := s.client.FTSLanguages(ctx)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolHybridSearch(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPHybridSearchRequest{
		Collection:      mcpGetString(args, "collection"),
		Query:           mcpGetString(args, "query"),
		TopK:            mcpGetInt(args, "top_k"),
		Algorithm:       mcpGetString(args, "algorithm"),
		VectorAlgorithm: mcpGetString(args, "vector_algorithm"),
		Strategy:        mcpGetString(args, "strategy"),
		RRFK:            mcpGetInt(args, "rrf_k"),
		Fuzzy:           mcpGetInt(args, "fuzzy"),
		DistanceMetric:  mcpGetString(args, "distance_metric"),
		FilterMeta:      mcpGetMetaMap(args, "filter_meta"),
		Boost:           mcpGetFloat64Map(args, "boost"),
	}
	if alpha, ok := args["alpha"].(float64); ok {
		req.Alpha = alpha
	}
	if threshold, ok := args["threshold"].(float64); ok {
		req.Threshold = threshold
	}

	resp, err := s.client.HybridSearch(ctx, req)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolRegisterWebhook(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPRegisterWebhookRequest{
		URL:        mcpGetString(args, "url"),
		Collection: mcpGetString(args, "collection"),
	}

	if eventsRaw, ok := args["events"].([]interface{}); ok {
		for _, e := range eventsRaw {
			if str, ok := e.(string); ok {
				req.Events = append(req.Events, str)
			}
		}
	}

	wh, err := s.client.RegisterWebhook(ctx, req)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(wh, "", "  ")
	return fmt.Sprintf("Webhook registered:\n%s", string(data)), nil
}

func (s *MCPToolServer) toolListWebhooks(ctx context.Context, args map[string]interface{}) (string, error) {
	hooks, err := s.client.ListWebhooks(ctx)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(hooks, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolDeleteWebhook(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPDeleteWebhookRequest{
		ID: mcpGetString(args, "id"),
	}

	if err := s.client.DeleteWebhook(ctx, req); err != nil {
		return "", err
	}

	return fmt.Sprintf("Webhook deleted: %s", req.ID), nil
}

func (s *MCPToolServer) toolSetSchema(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPSetSchemaRequest{
		Collection: mcpGetString(args, "collection"),
		Schema:     mcpGetString(args, "schema"),
	}

	if err := s.client.SetSchema(ctx, req); err != nil {
		return "", err
	}

	return fmt.Sprintf("Schema set for collection: %s", req.Collection), nil
}

func (s *MCPToolServer) toolGetSchema(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")

	resp, err := s.client.GetSchema(ctx, collection)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolDeleteSchema(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")

	if err := s.client.DeleteSchema(ctx, collection); err != nil {
		return "", err
	}

	return fmt.Sprintf("Schema deleted for collection: %s", collection), nil
}

func (s *MCPToolServer) toolListSchemas(ctx context.Context, args map[string]interface{}) (string, error) {
	resp, err := s.client.ListSchemas(ctx)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolValidateDocument(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPValidateRequest{
		Collection: mcpGetString(args, "collection"),
		Meta:       mcpGetMetaMap(args, "meta"),
	}

	resp, err := s.client.ValidateDocument(ctx, req)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolUpdateDocument(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPUpdateDocumentRequest{
		Collection: mcpGetString(args, "collection"),
		Key:        mcpGetString(args, "key"),
		Lang:       mcpGetString(args, "lang"),
	}

	if meta := mcpGetMetaMap(args, "meta"); len(meta) > 0 {
		req.Meta = meta
	} else if _, ok := args["meta"]; ok {
		// Explicitly provided empty meta = clear all
		empty := make(map[string][]string)
		req.Meta = empty
	}

	if content, ok := args["content_md"].(string); ok {
		req.ContentMD = &content
	}

	if ttl, ok := args["ttl"].(float64); ok {
		ttlInt := int64(ttl)
		req.TTL = &ttlInt
	}

	doc, err := s.client.UpdateDocument(ctx, req)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(doc, "", "  ")
	return fmt.Sprintf("Document updated:\n%s", string(data)), nil
}

func (s *MCPToolServer) toolGetDocumentMeta(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPGetDocMetaRequest{
		Collection: mcpGetString(args, "collection"),
		Key:        mcpGetString(args, "key"),
		Lang:       mcpGetString(args, "lang"),
	}

	resp, err := s.client.GetDocumentMeta(ctx, req)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolClassifyDocument(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPClassifyRequest{
		Collection: mcpGetString(args, "collection"),
		Key:        mcpGetString(args, "key"),
		Lang:       mcpGetString(args, "lang"),
		Text:       mcpGetString(args, "text"),
		TopK:       mcpGetInt(args, "top_k"),
	}

	if labelsRaw, ok := args["labels"].([]interface{}); ok {
		for _, l := range labelsRaw {
			if str, ok := l.(string); ok {
				req.Labels = append(req.Labels, str)
			}
		}
	}

	if multi, ok := args["multi"].(bool); ok {
		req.Multi = multi
	}
	if threshold, ok := args["threshold"].(float64); ok {
		req.Threshold = threshold
	}

	resp, err := s.client.Classify(ctx, req)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

// --- delete_collection / truncate ---

func (s *MCPToolServer) toolDeleteCollection(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	if collection == "" {
		return "", fmt.Errorf("collection is required")
	}
	resp, err := s.client.DeleteCollection(ctx, &MCPDeleteCollectionRequest{Collection: collection})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Collection %q deleted (%d documents removed)", collection, resp.Deleted), nil
}

func (s *MCPToolServer) toolTruncateRevisions(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPTruncateRequest{
		Collection: mcpGetString(args, "collection"),
		KeepRevs:   mcpGetInt(args, "keep_revs"),
	}
	if dc, ok := args["drop_cache"].(bool); ok {
		req.DropCache = dc
	}
	resp, err := s.client.Truncate(ctx, req)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Revision history truncated: %s", resp.Status), nil
}

// --- revisions ---

func (s *MCPToolServer) toolListRevisions(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	key := mcpGetString(args, "key")
	lang := mcpGetString(args, "lang")
	if collection == "" || key == "" || lang == "" {
		return "", fmt.Errorf("collection, key, and lang are required")
	}
	resp, err := s.client.ListRevisions(ctx, collection, key, lang)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolRestoreRevision(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	key := mcpGetString(args, "key")
	lang := mcpGetString(args, "lang")
	timestamp := int64(mcpGetInt(args, "timestamp"))
	if collection == "" || key == "" || lang == "" || timestamp == 0 {
		return "", fmt.Errorf("collection, key, lang, and timestamp are required")
	}
	doc, err := s.client.RestoreRevision(ctx, collection, key, lang, timestamp)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(doc, "", "  ")
	return fmt.Sprintf("Revision restored successfully:\n%s", string(data)), nil
}

// --- synonyms ---

func (s *MCPToolServer) toolListSynonyms(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	if collection == "" {
		return "", fmt.Errorf("collection is required")
	}
	resp, err := s.client.ListSynonyms(ctx, collection)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolAddSynonym(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	term := mcpGetString(args, "term")
	if collection == "" || term == "" {
		return "", fmt.Errorf("collection and term are required")
	}
	var synonyms []string
	if raw, ok := args["synonyms"].([]interface{}); ok {
		for _, v := range raw {
			if str, ok := v.(string); ok {
				synonyms = append(synonyms, str)
			}
		}
	}
	if len(synonyms) == 0 {
		return "", fmt.Errorf("synonyms array is required and must not be empty")
	}
	if err := s.client.SetSynonym(ctx, collection, term, synonyms); err != nil {
		return "", err
	}
	return fmt.Sprintf("Synonym group set: %q -> %v", term, synonyms), nil
}

func (s *MCPToolServer) toolDeleteSynonym(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	term := mcpGetString(args, "term")
	if collection == "" || term == "" {
		return "", fmt.Errorf("collection and term are required")
	}
	if err := s.client.DeleteSynonym(ctx, collection, term); err != nil {
		return "", err
	}
	return fmt.Sprintf("Synonym group deleted for term: %q", term), nil
}

// --- stopwords ---

func (s *MCPToolServer) toolListStopWords(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	if collection == "" {
		return "", fmt.Errorf("collection is required")
	}
	resp, err := s.client.ListStopWords(ctx, collection)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolAddStopWords(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	if collection == "" {
		return "", fmt.Errorf("collection is required")
	}
	var words []string
	if raw, ok := args["words"].([]interface{}); ok {
		for _, v := range raw {
			if str, ok := v.(string); ok {
				words = append(words, str)
			}
		}
	}
	if len(words) == 0 {
		return "", fmt.Errorf("words array is required and must not be empty")
	}
	if err := s.client.AddStopWords(ctx, collection, words); err != nil {
		return "", err
	}
	return fmt.Sprintf("Added %d stop words to %q", len(words), collection), nil
}

func (s *MCPToolServer) toolDeleteStopWords(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	if collection == "" {
		return "", fmt.Errorf("collection is required")
	}
	var words []string
	if raw, ok := args["words"].([]interface{}); ok {
		for _, v := range raw {
			if str, ok := v.(string); ok {
				words = append(words, str)
			}
		}
	}
	if len(words) == 0 {
		return "", fmt.Errorf("words array is required and must not be empty")
	}
	if err := s.client.DeleteStopWords(ctx, collection, words); err != nil {
		return "", err
	}
	return fmt.Sprintf("Removed %d stop words from %q", len(words), collection), nil
}

// --- meta-keys / checksum ---

func (s *MCPToolServer) toolGetMetaKeys(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	if collection == "" {
		return "", fmt.Errorf("collection is required")
	}
	resp, err := s.client.GetMetaKeys(ctx, collection)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolGetChecksum(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	if collection == "" {
		return "", fmt.Errorf("collection is required")
	}
	resp, err := s.client.GetChecksum(ctx, collection)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

// --- automation ---

func (s *MCPToolServer) toolListAutomation(ctx context.Context, args map[string]interface{}) (string, error) {
	filterType := mcpGetString(args, "type")
	resp, err := s.client.ListAutomation(ctx, filterType)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolCreateAutomation(ctx context.Context, args map[string]interface{}) (string, error) {
	rule := mcpArgsToAutomationRule(args)
	created, err := s.client.CreateAutomation(ctx, rule)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(created, "", "  ")
	return fmt.Sprintf("Automation rule created:\n%s", string(data)), nil
}

func (s *MCPToolServer) toolGetAutomation(ctx context.Context, args map[string]interface{}) (string, error) {
	id := mcpGetString(args, "id")
	if id == "" {
		return "", fmt.Errorf("id is required")
	}
	rule, err := s.client.GetAutomation(ctx, id)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(rule, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolUpdateAutomation(ctx context.Context, args map[string]interface{}) (string, error) {
	id := mcpGetString(args, "id")
	if id == "" {
		return "", fmt.Errorf("id is required")
	}
	rule := mcpArgsToAutomationRule(args)
	updated, err := s.client.UpdateAutomation(ctx, id, rule)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(updated, "", "  ")
	return fmt.Sprintf("Automation rule updated:\n%s", string(data)), nil
}

func (s *MCPToolServer) toolDeleteAutomation(ctx context.Context, args map[string]interface{}) (string, error) {
	id := mcpGetString(args, "id")
	if id == "" {
		return "", fmt.Errorf("id is required")
	}
	if err := s.client.DeleteAutomation(ctx, id); err != nil {
		return "", err
	}
	return fmt.Sprintf("Automation rule deleted: %s", id), nil
}

func (s *MCPToolServer) toolTestAutomation(ctx context.Context, args map[string]interface{}) (string, error) {
	id := mcpGetString(args, "id")
	if id == "" {
		return "", fmt.Errorf("id is required")
	}
	result, err := s.client.TestAutomation(ctx, id)
	if err != nil {
		return "", err
	}
	return result, nil
}

func (s *MCPToolServer) toolGetAutomationLogs(ctx context.Context, args map[string]interface{}) (string, error) {
	limit := mcpGetInt(args, "limit")
	cursor := mcpGetString(args, "cursor")
	ruleID := mcpGetString(args, "rule_id")
	status := mcpGetString(args, "status")
	resp, err := s.client.ListAutomationLogs(ctx, limit, cursor, ruleID, status)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

// mcpArgsToAutomationRule converts MCP tool arguments to an AutomationRule.
func mcpArgsToAutomationRule(args map[string]interface{}) AutomationRule {
	rule := AutomationRule{
		Type:       mcpGetString(args, "type"),
		Name:       mcpGetString(args, "name"),
		Enabled:    true,
		URL:        mcpGetString(args, "url"),
		Method:     mcpGetString(args, "method"),
		Collection: mcpGetString(args, "collection"),
		SearchType: mcpGetString(args, "searchType"),
		Query:      mcpGetString(args, "query"),
		WebhookID:  mcpGetString(args, "webhookId"),
		Schedule:   mcpGetString(args, "schedule"),
		TriggerID:  mcpGetString(args, "triggerId"),
	}
	if enabled, ok := args["enabled"].(bool); ok {
		rule.Enabled = enabled
	}
	if threshold, ok := args["threshold"].(float64); ok {
		rule.Threshold = threshold
	}
	if eventsRaw, ok := args["events"].([]interface{}); ok {
		for _, e := range eventsRaw {
			if str, ok := e.(string); ok {
				rule.Events = append(rule.Events, str)
			}
		}
	}
	if headers, ok := args["headers"].(map[string]interface{}); ok {
		rule.Headers = make(map[string]string)
		for k, v := range headers {
			if str, ok := v.(string); ok {
				rule.Headers[k] = str
			}
		}
	}
	return rule
}

// --- Collection Config Tools ---

func (s *MCPToolServer) toolGetCollectionConfig(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	if collection == "" {
		return "", fmt.Errorf("collection is required")
	}
	resp, err := s.client.GetCollectionConfig(ctx, collection)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolSetCollectionConfig(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	if collection == "" {
		return "", fmt.Errorf("collection is required")
	}
	req := &MCPSetCollectionConfigRequest{
		Collection:  collection,
		Type:        mcpGetString(args, "type"),
		Description: mcpGetString(args, "description"),
		Icon:        mcpGetString(args, "icon"),
		Color:       mcpGetString(args, "color"),
	}
	if cm, ok := args["custom_meta"].(map[string]interface{}); ok {
		req.CustomMeta = make(map[string]string, len(cm))
		for k, v := range cm {
			if str, ok := v.(string); ok {
				req.CustomMeta[k] = str
			}
		}
	}
	if err := s.client.SetCollectionConfig(ctx, req); err != nil {
		return "", err
	}
	return fmt.Sprintf("Collection %q config updated (type=%s)", collection, req.Type), nil
}

func (s *MCPToolServer) toolListCollectionConfigs(ctx context.Context, args map[string]interface{}) (string, error) {
	resp, err := s.client.ListCollectionConfigs(ctx)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

// --- Cross-Search Tool ---

func (s *MCPToolServer) toolCrossSearch(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPCrossSearchRequest{
		SourceCollection: mcpGetString(args, "source_collection"),
		SourceDocID:      mcpGetString(args, "source_doc_id"),
		Query:            mcpGetString(args, "query"),
		TopK:             mcpGetInt(args, "top_k"),
		Threshold:        mcpGetFloat(args, "threshold"),
		Algorithm:        mcpGetString(args, "algorithm"),
		DistanceMetric:   mcpGetString(args, "distance_metric"),
		FilterMeta:       mcpGetMetaMap(args, "filter_meta"),
	}
	if ic, ok := args["include_content"].(bool); ok {
		req.IncludeContent = ic
	}
	// Parse target_collections array
	if tc, ok := args["target_collections"].([]interface{}); ok {
		for _, v := range tc {
			if str, ok := v.(string); ok {
				req.TargetCollections = append(req.TargetCollections, str)
			}
		}
	}
	if len(req.TargetCollections) == 0 {
		return "", fmt.Errorf("target_collections is required")
	}
	resp, err := s.client.CrossSearch(ctx, req)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolAggregate(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &AggregateRequest{
		Collection:   mcpGetString(args, "collection"),
		FilterMeta:   mcpGetMetaMap(args, "filter_meta"),
		MaxFacetSize: mcpGetInt(args, "max_facet_size"),
	}
	// Parse facets array
	if facetsRaw, ok := args["facets"].([]interface{}); ok {
		for _, f := range facetsRaw {
			if fm, ok := f.(map[string]interface{}); ok {
				req.Facets = append(req.Facets, FacetRequest{
					Field:   mcpGetString(fm, "field"),
					OrderBy: mcpGetString(fm, "order_by"),
				})
			} else if fs, ok := f.(string); ok {
				req.Facets = append(req.Facets, FacetRequest{Field: fs})
			}
		}
	}
	// Parse histograms array
	if histRaw, ok := args["histograms"].([]interface{}); ok {
		for _, h := range histRaw {
			if hm, ok := h.(map[string]interface{}); ok {
				req.Histograms = append(req.Histograms, HistogramRequest{
					Field:    mcpGetString(hm, "field"),
					Interval: mcpGetString(hm, "interval"),
				})
			} else if hs, ok := h.(string); ok {
				req.Histograms = append(req.Histograms, HistogramRequest{Field: hs})
			}
		}
	}
	resp, err := s.client.Aggregate(ctx, req)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolFindDuplicates(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPFindDuplicatesRequest{
		Collection:     mcpGetString(args, "collection"),
		Mode:           mcpGetString(args, "mode"),
		MaxDocs:        mcpGetInt(args, "max_docs"),
		DistanceMetric: mcpGetString(args, "distance_metric"),
		Threshold:      mcpGetFloat(args, "threshold"),
	}
	if ic, ok := args["include_content"].(bool); ok {
		req.IncludeContent = ic
	}
	resp, err := s.client.FindDuplicates(ctx, req)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolIngest(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	docsRaw, ok := args["documents"].([]interface{})
	if !ok {
		return "", fmt.Errorf("documents must be an array")
	}

	docs := make([]MCPIngestDocument, len(docsRaw))
	for i, d := range docsRaw {
		docMap, ok := d.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("invalid document at index %d", i)
		}
		docs[i] = MCPIngestDocument{
			URL:       mcpGetString(docMap, "url"),
			Key:       mcpGetString(docMap, "key"),
			Lang:      mcpGetString(docMap, "lang"),
			ContentMD: mcpGetString(docMap, "content_md"),
			Meta:      mcpGetMetaMap(docMap, "meta"),
			Scraper:   mcpGetString(docMap, "scraper"),
			ScrapedAt: int64(mcpGetInt(docMap, "scraped_at")),
			TTL:       int64(mcpGetInt(docMap, "ttl")),
		}
		if ef, ok := docMap["extract_frontmatter"].(bool); ok {
			docs[i].ExtractFrontmatter = ef
		}
	}

	var opts MCPIngestOptions
	if optsRaw, ok := args["options"].(map[string]interface{}); ok {
		if v, ok := optsRaw["skip_duplicates"].(bool); ok {
			opts.SkipDuplicates = v
		}
		if v, ok := optsRaw["skip_embeddings"].(bool); ok {
			opts.SkipEmbeddings = v
		}
		if v, ok := optsRaw["skip_fts"].(bool); ok {
			opts.SkipFTS = v
		}
		if v, ok := optsRaw["skip_webhooks"].(bool); ok {
			opts.SkipWebhooks = v
		}
		if v, ok := optsRaw["auto_configure_collection"].(bool); ok {
			opts.AutoConfigureCollection = v
		}
		if v, ok := optsRaw["save_revision"].(bool); ok {
			opts.SaveRevision = v
		}
	}

	resp, err := s.client.Ingest(ctx, &MCPIngestRequest{
		Collection: collection,
		Documents:  docs,
		Options:    opts,
	})
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolUploadFile(ctx context.Context, args map[string]interface{}) (string, error) {
	collection := mcpGetString(args, "collection")
	filename := mcpGetString(args, "filename")
	contentB64 := mcpGetString(args, "content")
	lang := mcpGetString(args, "lang")
	key := mcpGetString(args, "key")
	meta := mcpGetMetaMap(args, "meta")
	ttl := int64(mcpGetInt(args, "ttl"))

	if collection == "" || filename == "" || contentB64 == "" || lang == "" {
		return "", fmt.Errorf("missing required fields: collection, filename, content, lang")
	}

	// Decode base64 content
	data, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		// Try URL-safe base64
		data, err = base64.URLEncoding.DecodeString(contentB64)
		if err != nil {
			// Try raw (no padding)
			data, err = base64.RawStdEncoding.DecodeString(contentB64)
			if err != nil {
				return "", fmt.Errorf("invalid base64 content: %w", err)
			}
		}
	}

	// Determine format from filename extension
	ext := strings.ToLower(path.Ext(filename))
	format := strings.TrimPrefix(ext, ".")
	if format == "htm" {
		format = "html"
	}

	// Convert to markdown
	var contentMD string
	var converted bool

	switch format {
	case "md", "markdown", "txt", "text", "":
		contentMD = string(data)
	case "yaml", "yml", "log", "lex":
		contentMD = "```" + format + "\n" + string(data) + "\n```"
		converted = true
	case "tex", "latex":
		contentMD = texToMarkdown(data)
		converted = true
	case "html":
		contentMD = htmlToMarkdown(data)
		converted = true
	case "pdf":
		contentMD, err = pdfToMarkdown(data)
		if err != nil {
			return "", fmt.Errorf("pdf conversion: %w", err)
		}
		converted = true
	case "docx":
		contentMD, err = docxToMarkdown(data)
		if err != nil {
			return "", fmt.Errorf("docx conversion: %w", err)
		}
		converted = true
	case "odt":
		contentMD, err = odtToMarkdown(data)
		if err != nil {
			return "", fmt.Errorf("odt conversion: %w", err)
		}
		converted = true
	case "rtf":
		contentMD = rtfToMarkdown(data)
		converted = true
	default:
		return "", fmt.Errorf("unsupported format: %s (supported: md, txt, html, pdf, docx, odt, rtf, yaml, log, lex, tex)", format)
	}

	// Extract frontmatter for md/txt
	if !converted {
		fmMeta, body := parseFrontmatter(contentMD)
		if fmMeta != nil {
			contentMD = body
			if meta == nil {
				meta = fmMeta
			} else {
				for k, v := range fmMeta {
					if _, exists := meta[k]; !exists {
						meta[k] = v
					}
				}
			}
		}
	}

	// Derive key from filename if not provided
	if key == "" {
		key = deriveKeyFromFilename(filename)
	}
	if key == "" {
		return "", fmt.Errorf("cannot derive key from filename; provide key explicitly")
	}

	// Add upload metadata
	if meta == nil {
		meta = make(map[string][]string)
	}
	meta["upload_format"] = []string{format}
	meta["upload_filename"] = []string{filename}
	if converted {
		meta["upload_converted"] = []string{"true"}
	}

	// Store via MCPClient.Add
	doc, err := s.client.Add(ctx, &MCPAddRequest{
		Collection: collection,
		Key:        key,
		Lang:       lang,
		ContentMD:  contentMD,
		Meta:       meta,
	})
	_ = ttl // TTL is set via /v1/set-ttl or SetTTL MCP tool separately
	if err != nil {
		return "", err
	}

	result := map[string]interface{}{
		"key":       key,
		"format":    format,
		"converted": converted,
		"document":  doc,
	}
	out, _ := json.MarshalIndent(result, "", "  ")
	return string(out), nil
}

// --- arg helpers ---

func mcpGetString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func mcpGetInt(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

func mcpGetFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}

func mcpGetMetaMap(m map[string]interface{}, key string) map[string][]string {
	result := make(map[string][]string)
	if meta, ok := m[key].(map[string]interface{}); ok {
		for k, v := range meta {
			switch val := v.(type) {
			case string:
				result[k] = []string{val}
			case []interface{}:
				strs := make([]string, len(val))
				for i, item := range val {
					if s, ok := item.(string); ok {
						strs[i] = s
					}
				}
				result[k] = strs
			}
		}
	}
	return result
}

// mcpGetFloat64Map reads a JSON object of numeric values into a float64 map.
// Returns nil when the key is absent so downstream callers can treat empty
// and "not provided" identically.
func mcpGetFloat64Map(m map[string]interface{}, key string) map[string]float64 {
	raw, ok := m[key].(map[string]interface{})
	if !ok {
		return nil
	}
	result := make(map[string]float64, len(raw))
	for k, v := range raw {
		if f, ok := v.(float64); ok {
			result[k] = f
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
