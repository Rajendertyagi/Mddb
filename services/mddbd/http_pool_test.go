package main

import (
	"net/http"
	"testing"
	"time"
)

func TestSharedHTTPClientExists(t *testing.T) {
	if SharedHTTPClient == nil {
		t.Fatal("SharedHTTPClient should be initialized via init()")
	}
	if SharedHTTPClient.Transport == nil {
		t.Fatal("SharedHTTPClient should have a transport")
	}
}

func TestNewPooledClientWithTimeout(t *testing.T) {
	client := NewPooledClientWithTimeout(5 * time.Second)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Timeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", client.Timeout)
	}
	// Should share the same transport
	if client.Transport != SharedHTTPClient.Transport {
		t.Error("pooled client should share transport with SharedHTTPClient")
	}
}

func TestPooledClientTransportConfig(t *testing.T) {
	transport, ok := SharedHTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("SharedHTTPClient.Transport should be *http.Transport")
	}
	if transport.MaxIdleConns <= 0 {
		t.Error("MaxIdleConns should be positive")
	}
	if transport.MaxIdleConnsPerHost <= 0 {
		t.Error("MaxIdleConnsPerHost should be positive")
	}
	if transport.IdleConnTimeout <= 0 {
		t.Error("IdleConnTimeout should be positive")
	}
	if !transport.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 should be true")
	}
}
