package main

import (
	"strings"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"
)

func TestSSEHubBroadcast(t *testing.T) {
	hub := NewSSEHub(true, 100)

	client := &sseClient{
		ch:         make(chan []byte, 10),
		collection: "",
		mode:       "read",
	}
	hub.mu.Lock()
	hub.clients[client] = true
	hub.mu.Unlock()

	hub.Broadcast("doc.added", "blog", "post1", "en")

	select {
	case msg := <-client.ch:
		s := string(msg)
		if !strings.Contains(s, "event: doc.added") {
			t.Errorf("expected event: doc.added, got: %s", s)
		}
		if !strings.Contains(s, `"collection":"blog"`) {
			t.Errorf("expected collection blog in data, got: %s", s)
		}
		if !strings.Contains(s, `"key":"post1"`) {
			t.Errorf("expected key post1 in data, got: %s", s)
		}
		if !strings.Contains(s, `"readOnly":true`) {
			t.Errorf("expected readOnly:true for unauthenticated broadcast, got: %s", s)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestSSEHubBroadcastWithAuthReadWrite(t *testing.T) {
	hub := NewSSEHub(true, 100)

	// Client with readwrite mode
	client := &sseClient{
		ch:         make(chan []byte, 10),
		collection: "",
		mode:       "readwrite",
	}
	hub.mu.Lock()
	hub.clients[client] = true
	hub.mu.Unlock()

	// No auth manager = no permission checks, but mode is readwrite
	hub.BroadcastWithAuth("doc.updated", "blog", "post1", "en", nil)

	select {
	case msg := <-client.ch:
		s := string(msg)
		if !strings.Contains(s, `"readOnly":false`) {
			t.Errorf("expected readOnly:false for readwrite client, got: %s", s)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestSSEHubCollectionFilter(t *testing.T) {
	hub := NewSSEHub(true, 100)

	blogClient := &sseClient{ch: make(chan []byte, 10), collection: "blog", mode: "read"}
	allClient := &sseClient{ch: make(chan []byte, 10), collection: "", mode: "read"}

	hub.mu.Lock()
	hub.clients[blogClient] = true
	hub.clients[allClient] = true
	hub.mu.Unlock()

	hub.Broadcast("doc.added", "docs", "readme", "en")

	// Blog client should NOT receive docs event
	select {
	case <-blogClient.ch:
		t.Error("blog-filtered client should not receive docs event")
	case <-time.After(50 * time.Millisecond):
		// expected
	}

	// All-collection client SHOULD receive it
	select {
	case msg := <-allClient.ch:
		if !strings.Contains(string(msg), "docs") {
			t.Error("all-collection client should receive docs event")
		}
	case <-time.After(time.Second):
		t.Fatal("all-collection client should have received event")
	}
}

func TestSSEHubDisabled(t *testing.T) {
	hub := NewSSEHub(false, 100)

	// Broadcast should be no-op
	hub.Broadcast("doc.added", "blog", "post1", "en")

	// ServeHTTP should return 503
	req := httptest.NewRequest("GET", "/v1/events", nil)
	w := httptest.NewRecorder()
	hub.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when disabled, got %d", w.Code)
	}
}

func TestSSEHubMaxClients(t *testing.T) {
	hub := NewSSEHub(true, 2)

	for i := 0; i < 2; i++ {
		c := &sseClient{ch: make(chan []byte, 1), collection: "", mode: "read"}
		hub.mu.Lock()
		hub.clients[c] = true
		hub.mu.Unlock()
	}

	// handleSSE on server would check this, but we test hub directly
	if hub.ClientCount() != 2 {
		t.Errorf("expected 2 clients, got %d", hub.ClientCount())
	}
}

func TestSSEHubClientCount(t *testing.T) {
	hub := NewSSEHub(true, 100)

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", hub.ClientCount())
	}

	c := &sseClient{ch: make(chan []byte, 1), collection: "", mode: "read"}
	hub.mu.Lock()
	hub.clients[c] = true
	hub.mu.Unlock()

	if hub.ClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", hub.ClientCount())
	}
}

func TestSSEHandleNoAuthServer(t *testing.T) {
	// Server without auth — SSE should work for everyone
	s := &Server{
		SSEHub: NewSSEHub(true, 100),
	}

	server := httptest.NewServer(http.HandlerFunc(s.handleSSE))
	defer server.Close()

	resp, err := http.Get(server.URL + "?collection=blog")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}
}

func TestSSEHandleAuthRequiresToken(t *testing.T) {
	// Server with auth enabled — no token → 401
	am := &AuthManager{enabled: true}
	s := &Server{
		SSEHub:      NewSSEHub(true, 100),
		AuthManager: am,
	}

	req := httptest.NewRequest("GET", "/v1/events", nil)
	w := httptest.NewRecorder()
	s.handleSSE(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when auth enabled but no token, got %d", w.Code)
	}
}
