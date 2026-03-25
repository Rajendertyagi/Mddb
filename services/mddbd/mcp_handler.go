package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"os"
)

// MCPHandler handles MCP JSON-RPC requests via stdio.
type MCPHandler struct {
	client       MCPClient
	customTools  []MCPCustomToolConfig
	serverInfo   MCPServerInfo
	instructions string // system prompt for LLM — how to use this server
}

// NewMCPHandler creates a new MCP handler.
func NewMCPHandler(client MCPClient, customTools []MCPCustomToolConfig) *MCPHandler {
	return &MCPHandler{
		client:      client,
		customTools: customTools,
		serverInfo:  MCPServerInfo{Name: "mddbd"},
	}
}

// NewMCPHandlerWithConfig creates a new MCP handler with custom server info and instructions.
func NewMCPHandlerWithConfig(client MCPClient, customTools []MCPCustomToolConfig, info MCPServerInfo, instructions string) *MCPHandler {
	if info.Name == "" {
		info.Name = "mddbd"
	}
	return &MCPHandler{
		client:       client,
		customTools:  customTools,
		serverInfo:   info,
		instructions: instructions,
	}
}

// Handle processes MCP request and returns response.
func (h *MCPHandler) Handle(req map[string]interface{}) map[string]interface{} {
	method, _ := req["method"].(string)
	id := req["id"]
	ctx := context.Background()

	var result map[string]interface{}
	var errObj map[string]interface{}

	switch method {
	case "initialize":
		return h.handleInitialize(req)
	case "resources/list":
		result = h.handleResourcesList()
	case "resources/read":
		result = h.handleResourcesRead(ctx, req)
	case "tools/list":
		result = h.handleToolsList()
	case "tools/call":
		result = h.handleToolsCall(ctx, req)
	case "ping":
		result = map[string]interface{}{"result": "pong"}
	default:
		errObj = map[string]interface{}{
			"code":    -32601,
			"message": "Method not found",
		}
	}

	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
	}

	if errObj != nil {
		response["error"] = errObj
	} else {
		response["result"] = result
	}

	return response
}

func (h *MCPHandler) handleInitialize(req map[string]interface{}) map[string]interface{} {
	id := req["id"]

	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"resources": map[string]interface{}{
				"subscribe":   false,
				"listChanged": false,
			},
			"tools": map[string]interface{}{
				"listChanged": false,
			},
		},
		"serverInfo": h.buildServerInfo(),
	}
	if h.instructions != "" {
		result["instructions"] = h.instructions
	}

	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
}

// buildServerInfo constructs the serverInfo object for MCP initialize response.
func (h *MCPHandler) buildServerInfo() map[string]interface{} {
	info := map[string]interface{}{
		"name":    h.serverInfo.Name,
		"version": VERSION,
	}
	if h.serverInfo.Description != "" {
		info["description"] = h.serverInfo.Description
	}
	if h.serverInfo.Vendor != "" {
		info["vendor"] = h.serverInfo.Vendor
	}
	if h.serverInfo.Homepage != "" {
		info["homepage"] = h.serverInfo.Homepage
	}
	return info
}

func (h *MCPHandler) handleResourcesList() map[string]interface{} {
	resources := []MCPResource{
		{
			URI:         "mddb://health",
			Name:        "MDDB Health",
			Description: "Health status of MDDB server",
			MimeType:    "application/json",
		},
		{
			URI:         "mddb://stats",
			Name:        "MDDB Statistics",
			Description: "Server and database statistics",
			MimeType:    "application/json",
		},
		{
			URI:         "mddb://{collection}/{key}?lang={lang}",
			Name:        "MDDB Document",
			Description: "Get a document by collection, key, and language",
			MimeType:    "text/markdown",
		},
		{
			URI:         "mddb-search://{collection}",
			Name:        "MDDB Search",
			Description: "Search documents in a collection",
			MimeType:    "application/json",
		},
	}

	return map[string]interface{}{
		"resources": resources,
	}
}

func (h *MCPHandler) handleResourcesRead(ctx context.Context, req map[string]interface{}) map[string]interface{} {
	params, _ := req["params"].(map[string]interface{})
	uri, _ := params["uri"].(string)

	ts := &MCPToolServer{client: h.client, customTools: h.customTools}
	content, err := ts.readResource(ctx, uri)
	if err != nil {
		return map[string]interface{}{
			"error": map[string]interface{}{
				"code":    -32000,
				"message": err.Error(),
			},
		}
	}

	return map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"uri":      uri,
				"mimeType": "application/json",
				"text":     content,
			},
		},
	}
}

func (h *MCPHandler) handleToolsList() map[string]interface{} {
	return map[string]interface{}{
		"tools": mcpAllTools(h.customTools),
	}
}

func (h *MCPHandler) handleToolsCall(ctx context.Context, req map[string]interface{}) map[string]interface{} {
	params, _ := req["params"].(map[string]interface{})
	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]interface{})

	ts := &MCPToolServer{client: h.client, customTools: h.customTools}
	result, err := ts.mcpCallTool(ctx, name, args)
	if err != nil {
		return map[string]interface{}{
			"error": map[string]interface{}{
				"code":    -32000,
				"message": err.Error(),
			},
		}
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": result,
			},
		},
	}
}

// HandleJSON processes JSON request and returns JSON response.
func (h *MCPHandler) HandleJSON(reqJSON []byte) ([]byte, error) {
	var req map[string]interface{}
	if err := json.Unmarshal(reqJSON, &req); err != nil {
		return nil, err
	}

	resp := h.Handle(req)
	return json.Marshal(resp)
}

// runMCPStdio runs the MCP stdio loop on the Server.
func (s *Server) runMCPStdio() {
	log.SetOutput(os.Stderr) // MCP uses stdout for protocol

	customTools := loadMCPCustomTools()
	client := NewDirectClient(s)
	handler := NewMCPHandlerWithConfig(client, customTools, s.MCPInfo, s.MCPInstructions)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 4*1024*1024), 4*1024*1024) // 4MB buffer

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		resp, err := handler.HandleJSON(line)
		if err != nil {
			log.Printf("MCP handler error: %v", err)
			continue
		}

		_, _ = os.Stdout.Write(resp)
		_, _ = os.Stdout.Write([]byte("\n"))
	}

	if err := scanner.Err(); err != nil {
		log.Printf("MCP stdio scanner error: %v", err)
	}
}
