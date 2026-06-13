package main

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// docToolCountRe captures the number in every phrasing the docs use to state the
// built-in MCP tool total — "N MCP tools", "N built-in tools", "N built-in MCP
// tools", "N tools for Claude". It deliberately does NOT match the per-category
// counts in docs/MCP.md ("9 key tools", "6 tools", "4 tools"), which are not the
// total.
var docToolCountRe = regexp.MustCompile(`(\d+)\s+(?:built-in MCP tools|built-in tools|MCP tools|tools for Claude)`)

// TestMCPToolCountDocsInSync guards DOC-001: the hard-coded built-in MCP tool
// count in README.md and docs/MCP.md must equal len(mcpBuiltinTools()). When a
// tool is added or removed the count changes here and this test fails until the
// docs are updated — preventing the drift that left the docs at 67 while the
// code defined 77. Runs as part of the normal `go test ./...` CI job.
func TestMCPToolCountDocsInSync(t *testing.T) {
	want := strconv.Itoa(len(mcpBuiltinTools()))

	// Paths are relative to this package directory (services/mddbd).
	for _, path := range []string{"../../README.md", "../../docs/MCP.md"} {
		data, err := os.ReadFile(path) // #nosec G304 -- path is one of two hardcoded repo doc files, not user input
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		matches := docToolCountRe.FindAllStringSubmatch(string(data), -1)
		if len(matches) == 0 {
			t.Errorf("%s: found no MCP tool-count phrase — the guard regex may be stale", path)
			continue
		}
		for _, m := range matches {
			if m[1] != want {
				t.Errorf("%s: documents %q MCP tools but code defines %s (in %q). "+
					"Update the docs (and run `make mcp-tools-count`).", path, m[1], want, m[0])
			}
		}
	}
}
