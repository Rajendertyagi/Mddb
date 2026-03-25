package main

import (
	"context"
	"testing"
)

func TestMCPToolServerReadOnlyPerProtocol(t *testing.T) {
	ts := &MCPToolServer{client: nil, mode: ModeRead}

	_, err := ts.mcpCallTool(context.Background(), "add_document", map[string]interface{}{
		"collection": "test", "key": "k1", "lang": "en", "content_md": "hello",
	})
	if err == nil || err.Error() != `tool "add_document" is not available in read-only mode` {
		t.Errorf("expected read-only error, got: %v", err)
	}
}

func TestMCPToolServerReadOnlyGlobalMode(t *testing.T) {
	// Bug fix: globalMode=read, mode="" should still block writes
	ts := &MCPToolServer{client: nil, globalMode: ModeRead, mode: ""}

	_, err := ts.mcpCallTool(context.Background(), "add_document", map[string]interface{}{
		"collection": "test", "key": "k1", "lang": "en", "content_md": "hello",
	})
	if err == nil || err.Error() != `tool "add_document" is not available in read-only mode` {
		t.Errorf("globalMode=read should block writes, got: %v", err)
	}

	_, err = ts.mcpCallTool(context.Background(), "delete_collection", map[string]interface{}{
		"collection": "test",
	})
	if err == nil {
		t.Error("globalMode=read should block destructive tools")
	}
}

func TestMCPToolServerFollowerBlocksWrites(t *testing.T) {
	// Simulates follower scenario: globalMode forced to "read" by replication
	ts := &MCPToolServer{client: nil, globalMode: ModeRead, mode: ""}

	_, err := ts.mcpCallTool(context.Background(), "restore_backup", map[string]interface{}{
		"from": "/tmp/backup",
	})
	if err == nil {
		t.Error("follower (globalMode=read) should block restore_backup")
	}
}

func TestMCPToolServerGlobalReadPerProtocolRW(t *testing.T) {
	// Per-protocol override can re-enable writes even when global is read.
	// effectiveMode(read, wr) = wr, so write tools should pass the mode check.
	em := effectiveMode(ModeRead, ModeRW)
	if em != ModeRW {
		t.Errorf("effectiveMode(read, wr) = %q, want wr", em)
	}

	// Verify the mode check would NOT block add_document
	ann := mcpToolAnnotations["add_document"]
	isReadOnly := ann != nil && ann.ReadOnlyHint != nil && *ann.ReadOnlyHint
	if isReadOnly {
		t.Error("add_document should not be readOnly")
	}
	// With em=wr, the guard `if em == ModeRead` is false → tool proceeds (no block)
}

func TestMCPToolServerReadOnlyAllowsReads(t *testing.T) {
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

func TestMCPToolServerWriteToolsNotReadOnly(t *testing.T) {
	writeTools := []string{"add_document", "delete_document", "delete_collection", "update_document",
		"restore_backup", "create_backup", "ingest_documents", "vector_reindex"}
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
		{"read", "", "read"},      // no override → global (THE BUG FIX)
		{"write", "read", "read"}, // override wins
	}
	for _, tt := range tests {
		got := effectiveMode(tt.global, tt.perProtocol)
		if got != tt.want {
			t.Errorf("effectiveMode(%q, %q) = %q, want %q", tt.global, tt.perProtocol, got, tt.want)
		}
	}
}

func TestMCPHandlerReadOnlyGlobal(t *testing.T) {
	// globalMode=read, no per-protocol override — should block writes
	h := &MCPHandler{logLevel: MCPLogWarning, globalMode: ModeRead}

	resp := h.Handle(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{
			"name":      "add_document",
			"arguments": map[string]interface{}{"collection": "test", "key": "k1", "lang": "en", "content_md": "hi"},
		},
	})

	result := resp["result"].(map[string]interface{})
	if result["isError"] != true {
		t.Error("expected isError=true for write tool when globalMode=read")
	}
}

func TestMCPHandlerReadOnlyPerProtocol(t *testing.T) {
	h := &MCPHandler{logLevel: MCPLogWarning, mode: ModeRead}

	resp := h.Handle(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{
			"name":      "delete_collection",
			"arguments": map[string]interface{}{"collection": "test"},
		},
	})

	result := resp["result"].(map[string]interface{})
	if result["isError"] != true {
		t.Error("expected isError=true for destructive tool in MCP read-only mode")
	}
}

func TestMCPCustomToolAllowedInReadOnly(t *testing.T) {
	customTools := []MCPCustomToolConfig{
		{Name: "version_check", Description: "Check version", Action: "full_text_search"},
		{Name: "kb_search", Description: "Search KB", Action: "semantic_search"},
		{Name: "list_docs", Description: "List docs", Action: "search_documents"},
	}

	ts := &MCPToolServer{client: nil, globalMode: ModeRead, customTools: customTools}

	for _, ct := range customTools {
		if !ts.isToolReadOnly(ct.Name) {
			t.Errorf("custom tool %q (action=%s) should be read-only", ct.Name, ct.Action)
		}
	}
}

func TestMCPCustomToolUnknownActionBlockedInReadOnly(t *testing.T) {
	// A custom tool with unknown action should be blocked
	ts := &MCPToolServer{
		client:      nil,
		globalMode:  ModeRead,
		customTools: []MCPCustomToolConfig{{Name: "bad_tool", Action: "unknown_action"}},
	}

	if ts.isToolReadOnly("bad_tool") {
		t.Error("custom tool with unknown action should NOT be read-only")
	}
}

func TestMCPCustomToolNotBlockedInFollowerMode(t *testing.T) {
	// Regression test for issue #27: custom tools with read-only actions
	// should work in follower mode (globalMode=read, no per-protocol override).
	// We verify via isToolReadOnly since nil client panics on actual call.
	customTools := []MCPCustomToolConfig{
		{Name: "version_check", Description: "test", Action: "full_text_search",
			Defaults: MCPCustomToolDefs{Collection: "versions", Limit: 1}},
	}

	ts := &MCPToolServer{client: nil, globalMode: ModeRead, mode: "", customTools: customTools}

	if !ts.isToolReadOnly("version_check") {
		t.Error("custom tool with full_text_search action should be read-only (issue #27)")
	}

	// Builtin write tools should still be blocked
	if ts.isToolReadOnly("add_document") {
		t.Error("add_document should NOT be read-only")
	}
}

func TestMCPBuiltinToolsDisable(t *testing.T) {
	// Default: builtin tools included
	tools := mcpAllTools(nil)
	if len(tools) < 40 {
		t.Errorf("expected 40+ builtin tools, got %d", len(tools))
	}

	// With env var: no builtin tools
	t.Setenv("MDDB_MCP_BUILTIN_TOOLS", "false")
	tools = mcpAllTools(nil)
	if len(tools) != 0 {
		t.Errorf("expected 0 tools with MDDB_MCP_BUILTIN_TOOLS=false, got %d", len(tools))
	}
}
