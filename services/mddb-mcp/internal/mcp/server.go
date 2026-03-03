package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/tradik/mddb/services/mddb-mcp/internal/config"
	"github.com/tradik/mddb/services/mddb-mcp/internal/mddb"
)

// Server implements MCP server for MDDB.
type Server struct {
	client      mddb.Client
	addr        string
	server      *http.Server
	customTools []config.CustomToolConfig
}

// NewServer creates a new MCP server.
func NewServer(client mddb.Client, addr string, customTools []config.CustomToolConfig) *Server {
	return &Server{
		client:      client,
		addr:        addr,
		customTools: customTools,
	}
}

// Start starts MCP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// MCP endpoints
	mux.HandleFunc("/mcp/resources", s.handleResources)
	mux.HandleFunc("/mcp/resources/read", s.handleResourceRead)
	mux.HandleFunc("/mcp/tools", s.handleTools)
	mux.HandleFunc("/mcp/tools/call", s.handleToolCall)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Addr:              s.addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("mcp server error: %v", err)
		}
	}()

	return nil
}

// Stop stops MCP server.
func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Shutdown(context.Background())
	}
	return nil
}

// handleHealth handles health check.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	health, err := s.client.Health(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(health); err != nil {
		log.Printf("error encoding health response: %v", err)
	}
}

// handleResources returns list of available resources.
func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	resources := []Resource{
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

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"resources": resources,
	}); err != nil {
		log.Printf("error encoding resources response: %v", err)
	}
}

// handleResourceRead czyta konkretny resource.
func (s *Server) handleResourceRead(w http.ResponseWriter, r *http.Request) {
	var req ResourceReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	content, err := s.readResource(r.Context(), req.URI)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"uri":      req.URI,
				"mimeType": "application/json",
				"text":     content,
			},
		},
	}); err != nil {
		log.Printf("error encoding resource read response: %v", err)
	}
}

// handleTools returns list of available tools.
func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"tools": allTools(s.customTools),
	}); err != nil {
		log.Printf("error encoding tools response: %v", err)
	}
}

// handleToolCall executes tool call.
func (s *Server) handleToolCall(w http.ResponseWriter, r *http.Request) {
	var req ToolCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	result, err := s.callTool(r.Context(), req.Name, req.Arguments)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": result,
			},
		},
	}); err != nil {
		log.Printf("error encoding tool call response: %v", err)
	}
}
