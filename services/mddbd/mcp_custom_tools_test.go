package main

import (
	"context"
	"testing"
)

// SEC-010: operator-pinned scope keys (collection / filter_meta /
// include_content / fields) must survive client args, and only declared
// parameters may pass through at all.

func customToolFixture() MCPCustomToolConfig {
	pinnedInclude := false
	return MCPCustomToolConfig{
		Name:   "public_versions",
		Action: "search_documents",
		Defaults: MCPCustomToolDefs{
			Collection:     "public-versions",
			FilterMeta:     map[string][]string{"visibility": {"public"}},
			IncludeContent: &pinnedInclude,
			Fields:         []string{"name", "currentVersion"},
		},
		Parameters: []MCPCustomToolParam{
			{Name: "limit", Type: "integer"},
		},
	}
}

func TestCustomToolPinnedScopeSurvivesUserArgs(t *testing.T) {
	merged, err := mcpMergeCustomToolArgs(customToolFixture(), map[string]interface{}{
		"collection":      "secrets",
		"filter_meta":     map[string]interface{}{},
		"include_content": true,
		"fields":          []interface{}{},
		"limit":           float64(5),
	})
	if err != nil {
		t.Fatal(err)
	}

	if merged["collection"] != "public-versions" {
		t.Errorf("collection = %v, pinned scope must win (SEC-010)", merged["collection"])
	}
	if v, ok := merged["include_content"].(bool); !ok || v {
		t.Errorf("include_content = %v, pinned data minimization must win", merged["include_content"])
	}
	if fields, ok := merged["fields"].([]string); !ok || len(fields) != 2 {
		t.Errorf("fields = %v, pinned projection must win", merged["fields"])
	}
	fm, ok := merged["filter_meta"].(map[string]interface{})
	if !ok || len(fm) == 0 {
		t.Errorf("filter_meta = %v, pinned filter must win", merged["filter_meta"])
	}
	if merged["limit"] != float64(5) {
		t.Errorf("limit = %v, declared parameter should pass through", merged["limit"])
	}
}

func TestCustomToolDropsUndeclaredParams(t *testing.T) {
	merged, err := mcpMergeCustomToolArgs(customToolFixture(), map[string]interface{}{
		"sort":   "addedAt", // not declared in Parameters
		"offset": float64(10),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, present := merged["sort"]; present {
		t.Error("undeclared 'sort' arg must be dropped (SEC-010)")
	}
	if _, present := merged["offset"]; present {
		t.Error("undeclared 'offset' arg must be dropped (SEC-010)")
	}
}

func TestCustomToolUnpinnedDeclaredScopeKeyStillWorks(t *testing.T) {
	// When the operator does NOT pin a scope key but declares it as a
	// parameter, the client may set it — locking applies to pinned keys only.
	ct := MCPCustomToolConfig{
		Name:   "open_search",
		Action: "search_documents",
		Parameters: []MCPCustomToolParam{
			{Name: "collection", Type: "string", Required: true},
		},
	}
	merged, err := mcpMergeCustomToolArgs(ct, map[string]interface{}{"collection": "blog"})
	if err != nil {
		t.Fatal(err)
	}
	if merged["collection"] != "blog" {
		t.Errorf("collection = %v, declared unpinned param should pass", merged["collection"])
	}
}

func TestCustomToolUnknownActionErrors(t *testing.T) {
	ct := MCPCustomToolConfig{Name: "x", Action: "delete_collection"}
	if _, err := mcpMergeCustomToolArgs(ct, nil); err == nil {
		t.Error("unknown/unsupported action must error")
	}
}

// GO-022: search_documents with include_content=false must not carry document
// bodies out of the store layer.
func TestDirectClientSearchIncludeContent(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()
	addTestDoc(t, s, "col", "k1", "en", "the body", nil)
	client := NewDirectClient(s)

	// Zero value (false) drops the body at the store layer.
	resp, err := client.Search(context.Background(), &MCPSearchRequest{Collection: "col"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Documents) != 1 {
		t.Fatalf("expected 1 document, got %d", len(resp.Documents))
	}
	if resp.Documents[0].ContentMD != "" {
		t.Errorf("ContentMD should be empty without IncludeContent, got %q", resp.Documents[0].ContentMD)
	}

	// Explicit opt-in keeps the body.
	resp, err = client.Search(context.Background(), &MCPSearchRequest{Collection: "col", IncludeContent: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Documents) != 1 || resp.Documents[0].ContentMD != "the body" {
		t.Errorf("ContentMD should survive with IncludeContent=true, got %+v", resp.Documents)
	}
}

// GO-022: the same guarantee for the filter_meta (idxmeta) search path.
func TestDirectClientSearchIncludeContentWithFilter(t *testing.T) {
	s, cleanup := newHandlerTestServer(t)
	defer cleanup()
	addTestDoc(t, s, "col", "k2", "en", "secret body", map[string][]string{"kind": {"page"}})
	client := NewDirectClient(s)

	resp, err := client.Search(context.Background(), &MCPSearchRequest{
		Collection: "col",
		FilterMeta: map[string][]string{"kind": {"page"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Documents) != 1 {
		t.Fatalf("expected 1 document, got %d", len(resp.Documents))
	}
	if resp.Documents[0].ContentMD != "" {
		t.Errorf("ContentMD should be empty without IncludeContent, got %q", resp.Documents[0].ContentMD)
	}
}
