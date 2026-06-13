package main

import "testing"

// TestAnnotationsCoverAllBuiltinTools — GO-016: every built-in MCP tool must
// have an annotation entry, or isToolReadOnly defaults it to write (blocking
// legitimate reads in read-only mode) and tools/list omits its hints.
func TestAnnotationsCoverAllBuiltinTools(t *testing.T) {
	for _, tool := range mcpBuiltinTools() {
		if _, ok := mcpToolAnnotations[tool.Name]; !ok {
			t.Errorf("builtin tool %q has no entry in mcpToolAnnotations", tool.Name)
		}
	}
}

// TestMemoryToolReadOnlyClassification — GO-016: read-only memory tools must be
// usable in read-only mode; writing memory tools must not be.
func TestMemoryToolReadOnlyClassification(t *testing.T) {
	ts := &MCPToolServer{}

	for _, name := range []string{"memory_recall", "memory_list_sessions", "memory_session_history"} {
		if !ts.isToolReadOnly(name) {
			t.Errorf("%s should be classified read-only (GO-016)", name)
		}
	}
	for _, name := range []string{"memory_start_session", "memory_add_message", "memory_summarize"} {
		if ts.isToolReadOnly(name) {
			t.Errorf("%s must not be classified read-only (it writes)", name)
		}
	}
}
