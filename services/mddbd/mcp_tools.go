package main

import (
	"context"
	"encoding/json"
	"fmt"
)

// MCPToolServer provides tool and resource call dispatch.
type MCPToolServer struct {
	client      MCPClient
	customTools []MCPCustomToolConfig
}

// mcpCallTool invokes an MCP tool by name.
func (s *MCPToolServer) mcpCallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
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
	case "full_text_search":
		return s.toolFTSSearch(ctx, args)
	case "hybrid_search":
		return s.toolHybridSearch(ctx, args)
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
	}

	resp, err := s.client.FTSSearch(ctx, req)
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
		FilterMeta:      mcpGetMetaMap(args, "filter_meta"),
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
