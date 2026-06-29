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
		"list_curation_rules": true, "create_curation_rule": true, "update_curation_rule": true, "delete_curation_rule": true,
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
