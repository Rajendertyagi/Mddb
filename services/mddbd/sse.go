package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	json "github.com/goccy/go-json"
)

// SSEHub manages Server-Sent Events connections and broadcasts document change events.
// Enabled by default, configurable via MDDB_SSE_ENABLED=false.
type SSEHub struct {
	mu         sync.RWMutex
	clients    map[*sseClient]bool
	maxClients int
	enabled    bool
	keepAlive  time.Duration
}

type sseClient struct {
	ch         chan []byte
	collection string // "" = all collections, otherwise filter
}

// SSEEvent represents a document change event sent to SSE clients.
type SSEEvent struct {
	Event      string `json:"event"` // "doc.added", "doc.updated", "doc.deleted"
	Collection string `json:"collection"`
	Key        string `json:"key"`
	Lang       string `json:"lang"`
	Timestamp  int64  `json:"timestamp"`
}

// NewSSEHub creates a new SSE hub.
func NewSSEHub(enabled bool, maxClients int) *SSEHub {
	if maxClients <= 0 {
		maxClients = 1000
	}
	return &SSEHub{
		clients:    make(map[*sseClient]bool),
		maxClients: maxClients,
		enabled:    enabled,
		keepAlive:  30 * time.Second,
	}
}

// Broadcast sends an event to all connected SSE clients.
func (h *SSEHub) Broadcast(event, collection, key, lang string) {
	if !h.enabled {
		return
	}

	evt := SSEEvent{
		Event:      event,
		Collection: collection,
		Key:        key,
		Lang:       lang,
		Timestamp:  time.Now().Unix(),
	}

	data, err := json.Marshal(evt)
	if err != nil {
		return
	}

	msg := fmt.Appendf(nil, "event: %s\ndata: %s\n\n", event, data)

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		// Filter by collection if client specified one
		if client.collection != "" && client.collection != collection {
			continue
		}
		select {
		case client.ch <- msg:
		default:
			// Client buffer full, skip (non-blocking)
		}
	}
}

// ServeHTTP handles GET /v1/events SSE endpoint.
func (h *SSEHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.enabled {
		http.Error(w, `{"error":"SSE disabled"}`, http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	// Check max clients
	h.mu.RLock()
	count := len(h.clients)
	h.mu.RUnlock()
	if count >= h.maxClients {
		http.Error(w, `{"error":"too many SSE connections"}`, http.StatusServiceUnavailable)
		return
	}

	collection := r.URL.Query().Get("collection")

	client := &sseClient{
		ch:         make(chan []byte, 64),
		collection: collection,
	}

	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, client)
		h.mu.Unlock()
		close(client.ch)
	}()

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // nginx

	// Send initial connected event
	_, _ = fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\",\"collection\":%q}\n\n", collection)
	flusher.Flush()

	log.Printf("SSE client connected (collection=%q, total=%d)", collection, h.ClientCount())

	// Keep-alive ticker
	ticker := time.NewTicker(h.keepAlive)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			log.Printf("SSE client disconnected (total=%d)", h.ClientCount()-1)
			return
		case msg, ok := <-client.ch:
			if !ok {
				return
			}
			_, _ = w.Write(msg)
			flusher.Flush()
		case <-ticker.C:
			// Keep-alive comment to prevent proxy timeouts
			_, _ = fmt.Fprintf(w, ": keepalive %d\n\n", time.Now().Unix())
			flusher.Flush()
		}
	}
}

// ClientCount returns the number of connected SSE clients.
func (h *SSEHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
