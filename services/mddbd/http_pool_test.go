package main

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// trackingBody records how many bytes were read and whether Close was called,
// so the drainAndClose tests can assert the body is both drained and closed.
type trackingBody struct {
	io.Reader
	read   int
	closed bool
}

func (b *trackingBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}

// TestDrainAndClose_NilBody documents that a nil body is a safe no-op (GO-014).
func TestDrainAndClose_NilBody(t *testing.T) {
	drainAndClose(nil) // must not panic
}

// TestDrainAndClose_ReadsAndCloses verifies the helper both drains and closes
// the body so the pooled connection can be reused.
func TestDrainAndClose_ReadsAndCloses(t *testing.T) {
	rc := &trackingBody{Reader: strings.NewReader("hello world payload")}
	drainAndClose(rc)
	if !rc.closed {
		t.Error("expected body to be closed")
	}
	if rc.read == 0 {
		t.Error("expected body to be drained (read > 0)")
	}
}

// TestDrainAndClose_CapsAtLimit ensures a pathological oversize body is not read
// in full — drainAndClose stops at drainBodyLimit and still closes.
func TestDrainAndClose_CapsAtLimit(t *testing.T) {
	big := bytes.Repeat([]byte("x"), drainBodyLimit*2)
	rc := &trackingBody{Reader: bytes.NewReader(big)}
	drainAndClose(rc)
	if rc.read > drainBodyLimit {
		t.Errorf("expected at most %d bytes read, got %d", drainBodyLimit, rc.read)
	}
	if !rc.closed {
		t.Error("expected body to be closed")
	}
}

// TestDrainAndClose_EnablesConnReuse is the behavioral proof of GO-014: with
// the body drained before Close, the pooled transport reuses the single TCP
// connection across repeated deliveries instead of dialing each time.
func TestDrainAndClose_EnablesConnReuse(t *testing.T) {
	t.Setenv("MDDB_OUTBOUND_ALLOW_PRIVATE", "true")

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("payload ", 64))
	}))
	var newConns int32
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			atomic.AddInt32(&newConns, 1)
		}
	}
	srv.Start()
	defer srv.Close()

	client := NewPooledClientWithTimeout(5 * time.Second)
	for i := 0; i < 4; i++ {
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		drainAndClose(resp.Body)
	}
	if got := atomic.LoadInt32(&newConns); got != 1 {
		t.Errorf("expected a single reused connection, got %d new connections", got)
	}
}

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
