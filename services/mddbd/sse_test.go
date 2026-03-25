package main

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSSEHubBroadcast(t *testing.T) {
	hub := NewSSEHub(true, 100)

	// Create a fake client
	client := &sseClient{
		ch:         make(chan []byte, 10),
		collection: "",
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
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestSSEHubCollectionFilter(t *testing.T) {
	hub := NewSSEHub(true, 100)

	blogClient := &sseClient{ch: make(chan []byte, 10), collection: "blog"}
	allClient := &sseClient{ch: make(chan []byte, 10), collection: ""}

	hub.mu.Lock()
	hub.clients[blogClient] = true
	hub.clients[allClient] = true
	hub.mu.Unlock()

	// Broadcast to "docs" collection
	hub.Broadcast("doc.added", "docs", "readme", "en")

	// Blog client should NOT receive it
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

	// Fill up clients
	for i := 0; i < 2; i++ {
		c := &sseClient{ch: make(chan []byte, 1), collection: ""}
		hub.mu.Lock()
		hub.clients[c] = true
		hub.mu.Unlock()
	}

	// Third connection should be rejected
	req := httptest.NewRequest("GET", "/v1/events", nil)
	w := httptest.NewRecorder()
	hub.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when max clients reached, got %d", w.Code)
	}
}

func TestSSEHubClientCount(t *testing.T) {
	hub := NewSSEHub(true, 100)

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", hub.ClientCount())
	}

	c := &sseClient{ch: make(chan []byte, 1), collection: ""}
	hub.mu.Lock()
	hub.clients[c] = true
	hub.mu.Unlock()

	if hub.ClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", hub.ClientCount())
	}
}

func TestSSEHubHTTPStream(t *testing.T) {
	hub := NewSSEHub(true, 100)

	// Start server
	server := httptest.NewServer(http.HandlerFunc(hub.ServeHTTP))
	defer server.Close()

	// Connect SSE client
	resp, err := http.Get(server.URL + "?collection=blog")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	// Read initial connected event
	scanner := bufio.NewScanner(resp.Body)
	foundConnected := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "event: connected") {
			foundConnected = true
			break
		}
	}
	if !foundConnected {
		t.Error("expected initial 'connected' event")
	}

	// Broadcast an event
	hub.Broadcast("doc.added", "blog", "test-post", "en")

	// Read the broadcasted event
	foundEvent := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "event: doc.added") {
			foundEvent = true
		}
		if strings.Contains(line, "test-post") {
			break
		}
	}
	if !foundEvent {
		t.Error("expected doc.added event in stream")
	}
}
