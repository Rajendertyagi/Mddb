package main

import (
	"context"
	"testing"
)

func TestMCPToolServerReadOnlyMode(t *testing.T) {
	ts := &MCPToolServer{
		client: nil,
		mode:   ModeRead,
	}

	// Read-only tool should succeed (get_stats has readOnlyHint=true)
	// But we have no client, so it will error on the actual call.
	// We test the mode check itself.

	// Write tool should be blocked
	_, err := ts.mcpCallTool(context.Background(), "add_document", map[string]interface{}{
		"collection": "test",
		"key":        "k1",
		"lang":       "en",
		"content_md": "hello",
	})
	if err == nil {
		t.Error("expected error for write tool in read-only mode")
	}
	if err != nil && err.Error() != `tool "add_document" is not available in read-only mode` {
		t.Errorf("unexpected error: %v", err)
	}

	// Delete tool should be blocked
	_, err = ts.mcpCallTool(context.Background(), "delete_document", map[string]interface{}{
		"collection": "test",
		"key":        "k1",
		"lang":       "en",
	})
	if err == nil {
		t.Error("expected error for delete tool in read-only mode")
	}

	// Destructive tool should be blocked
	_, err = ts.mcpCallTool(context.Background(), "delete_collection", map[string]interface{}{
		"collection": "test",
	})
	if err == nil {
		t.Error("expected error for destructive tool in read-only mode")
	}
}

func TestMCPToolServerReadOnlyAllowsReads(t *testing.T) {
	// Verify that read-only tools pass the mode check.
	// We test the annotation lookup directly since nil client panics on actual call.
	readOnlyTools := []string{"get_stats", "search_documents", "semantic_search", "full_text_search",
		"hybrid_search", "vector_stats", "list_webhooks", "get_schema", "list_schemas",
		"find_duplicates", "aggregate", "export_documents"}

	for _, name := range readOnlyTools {
		ann := mcpToolAnnotations[name]
		if ann == nil || ann.ReadOnlyHint == nil || !*ann.ReadOnlyHint {
			t.Errorf("tool %q should have readOnlyHint=true", name)
		}
	}
}

func TestMCPToolServerWriteModeDoesNotBlock(t *testing.T) {
	// Verify write tools are NOT in the readOnly annotation set
	writeTools := []string{"add_document", "delete_document", "delete_collection", "update_document"}
	for _, name := range writeTools {
		ann := mcpToolAnnotations[name]
		if ann != nil && ann.ReadOnlyHint != nil && *ann.ReadOnlyHint {
			t.Errorf("tool %q should NOT have readOnlyHint=true", name)
		}
	}
}

func TestEffectiveMode(t *testing.T) {
	tests := []struct {
		global, perProtocol, want AccessMode
	}{
		{"wr", "", "wr"},          // no override → global
		{"wr", "read", "read"},    // override wins
		{"read", "wr", "wr"},      // override wins
		{"read", "", "read"},      // no override → global
		{"write", "read", "read"}, // override wins
	}
	for _, tt := range tests {
		got := effectiveMode(tt.global, tt.perProtocol)
		if got != tt.want {
			t.Errorf("effectiveMode(%q, %q) = %q, want %q", tt.global, tt.perProtocol, got, tt.want)
		}
	}
}

func TestMCPHandlerReadOnlyMode(t *testing.T) {
	h := &MCPHandler{
		logLevel: MCPLogWarning,
		mode:     ModeRead,
	}

	// Tool call for a write tool should return isError
	resp := h.Handle(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "add_document",
			"arguments": map[string]interface{}{"collection": "test", "key": "k1", "lang": "en", "content_md": "hi"},
		},
	})

	result := resp["result"].(map[string]interface{})
	if result["isError"] != true {
		t.Error("expected isError=true for write tool in read-only MCP mode")
	}

	content := result["content"].([]map[string]interface{})
	text := content[0]["text"].(string)
	if text == "" {
		t.Error("expected error message in content")
	}
}
