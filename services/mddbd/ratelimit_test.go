package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func setupRateLimiter(t *testing.T, limit, burst, windowSec int, by string) *RateLimiter {
	t.Helper()
	t.Setenv("MDDB_RATE_LIMIT_ENABLED", "true")
	t.Setenv("MDDB_RATE_LIMIT_REQUESTS", strconv.Itoa(limit))
	t.Setenv("MDDB_RATE_LIMIT_BURST", strconv.Itoa(burst))
	t.Setenv("MDDB_RATE_LIMIT_WINDOW", strconv.Itoa(windowSec))
	t.Setenv("MDDB_RATE_LIMIT_BY", by)
	return NewRateLimiter()
}

func TestRateLimiterDisabledByDefault(t *testing.T) {
	t.Setenv("MDDB_RATE_LIMIT_ENABLED", "")
	rl := NewRateLimiter()
	if rl.Enabled() {
		t.Fatal("limiter should be disabled when env unset")
	}
	// HTTP and gRPC wrappers must be passthrough.
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	w := httptest.NewRecorder()
	rl.HTTPMiddleware(next).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

func TestRateLimiterHTTPBurstAndBlock(t *testing.T) {
	rl := setupRateLimiter(t, 3, 1, 60, "ip")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := rl.HTTPMiddleware(next)

	hit := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/v1/fts", nil)
		r.RemoteAddr = "203.0.113.5:1234"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	// limit=3 + burst=1 => 4 allowed, 5th blocked.
	for i := 0; i < 4; i++ {
		if w := hit(); w.Code != 200 {
			t.Fatalf("req %d want 200, got %d", i+1, w.Code)
		}
	}
	w := hit()
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("5th want 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("missing Retry-After")
	}
	if !strings.Contains(w.Body.String(), "rate limit exceeded") {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestRateLimiterHTTPExemptions(t *testing.T) {
	rl := setupRateLimiter(t, 1, 0, 60, "ip")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := rl.HTTPMiddleware(next)

	// Burn the budget.
	r := httptest.NewRequest("GET", "/v1/fts", nil)
	r.RemoteAddr = "1.1.1.1:1"
	h.ServeHTTP(httptest.NewRecorder(), r)

	// /health must still pass.
	for _, path := range []string{"/health", "/v1/health", "/metrics"} {
		r := httptest.NewRequest("GET", path, nil)
		r.RemoteAddr = "1.1.1.1:1"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s want 200, got %d", path, w.Code)
		}
	}
}

func TestRateLimiterHTTPPerIP(t *testing.T) {
	rl := setupRateLimiter(t, 1, 0, 60, "ip")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := rl.HTTPMiddleware(next)

	// Client A burns budget.
	a := httptest.NewRequest("GET", "/v1/fts", nil)
	a.RemoteAddr = "1.1.1.1:1"
	h.ServeHTTP(httptest.NewRecorder(), a)
	a2 := httptest.NewRequest("GET", "/v1/fts", nil)
	a2.RemoteAddr = "1.1.1.1:1"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, a2)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("A second hit want 429, got %d", w.Code)
	}
	// Client B unaffected.
	b := httptest.NewRequest("GET", "/v1/fts", nil)
	b.RemoteAddr = "2.2.2.2:1"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, b)
	if w.Code != 200 {
		t.Fatalf("B first hit want 200, got %d", w.Code)
	}
}

func TestRateLimiterHTTPByUser(t *testing.T) {
	rl := setupRateLimiter(t, 1, 0, 60, "user")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := rl.HTTPMiddleware(next)

	mk := func(user, ip string) *http.Request {
		r := httptest.NewRequest("GET", "/v1/fts", nil)
		r.RemoteAddr = ip + ":1"
		if user != "" {
			r = r.WithContext(context.WithValue(r.Context(), authContextKey, &JWTClaims{Username: user}))
		}
		return r
	}
	// Same user from different IPs => shares budget.
	h.ServeHTTP(httptest.NewRecorder(), mk("alice", "1.1.1.1"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, mk("alice", "2.2.2.2"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("alice 2nd want 429, got %d", w.Code)
	}
	// Different user has own budget.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, mk("bob", "3.3.3.3"))
	if w.Code != 200 {
		t.Fatalf("bob first want 200, got %d", w.Code)
	}
	// Anonymous falls back to IP keying.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, mk("", "4.4.4.4"))
	if w.Code != 200 {
		t.Fatalf("anon first want 200, got %d", w.Code)
	}
}

func TestRateLimiterHeaders(t *testing.T) {
	rl := setupRateLimiter(t, 2, 0, 60, "ip")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := rl.HTTPMiddleware(next)

	r := httptest.NewRequest("GET", "/v1/fts", nil)
	r.RemoteAddr = "9.9.9.9:1"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if got := w.Header().Get("X-RateLimit-Limit"); got != "2" {
		t.Errorf("X-RateLimit-Limit: want 2, got %q", got)
	}
	if got := w.Header().Get("X-RateLimit-Remaining"); got != "1" {
		t.Errorf("X-RateLimit-Remaining: want 1, got %q", got)
	}
	if w.Header().Get("X-RateLimit-Reset") == "" {
		t.Error("X-RateLimit-Reset missing")
	}
}

func TestRateLimiterGRPCUnary(t *testing.T) {
	rl := setupRateLimiter(t, 1, 0, 60, "ip")
	interceptor := rl.UnaryInterceptor()

	ctx := peer.NewContext(context.Background(), &peer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 999},
	})
	h := func(ctx context.Context, req interface{}) (interface{}, error) { return "ok", nil }

	if _, err := interceptor(ctx, nil, nil, h); err != nil {
		t.Fatalf("first call: %v", err)
	}
	_, err := interceptor(ctx, nil, nil, h)
	if err == nil {
		t.Fatal("second call should be rejected")
	}
	if s, _ := status.FromError(err); s.Code() != codes.ResourceExhausted {
		t.Fatalf("want ResourceExhausted, got %v", s.Code())
	}
}

func TestRateLimiterGRPCUnaryDisabled(t *testing.T) {
	t.Setenv("MDDB_RATE_LIMIT_ENABLED", "")
	rl := NewRateLimiter()
	interceptor := rl.UnaryInterceptor()
	called := 0
	h := func(ctx context.Context, req interface{}) (interface{}, error) { called++; return nil, nil }
	for i := 0; i < 5; i++ {
		_, _ = interceptor(context.Background(), nil, nil, h)
	}
	if called != 5 {
		t.Fatalf("disabled limiter must be passthrough, called=%d", called)
	}
}

// mockServerStream is a minimal grpc.ServerStream stub for interceptor testing.
type mockServerStream struct {
	ctx context.Context
}

func (m *mockServerStream) SetHeader(metadata.MD) error   { return nil }
func (m *mockServerStream) SendHeader(metadata.MD) error  { return nil }
func (m *mockServerStream) SetTrailer(metadata.MD)        {}
func (m *mockServerStream) Context() context.Context      { return m.ctx }
func (m *mockServerStream) SendMsg(msg interface{}) error { return nil }
func (m *mockServerStream) RecvMsg(msg interface{}) error { return nil }

func TestRateLimiterGRPCStream(t *testing.T) {
	rl := setupRateLimiter(t, 1, 0, 60, "ip")
	interceptor := rl.StreamInterceptor()
	ctx := peer.NewContext(context.Background(), &peer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP("5.5.5.5"), Port: 7},
	})
	ss := &mockServerStream{ctx: ctx}
	noop := grpc.StreamHandler(func(srv interface{}, stream grpc.ServerStream) error { return nil })
	if err := interceptor(nil, ss, nil, noop); err != nil {
		t.Fatalf("first: %v", err)
	}
	err := interceptor(nil, ss, nil, noop)
	if err == nil {
		t.Fatal("second should reject")
	}
	if s, _ := status.FromError(err); s.Code() != codes.ResourceExhausted {
		t.Fatalf("want ResourceExhausted, got %v", s.Code())
	}
}

func TestRateLimiterGRPCClientIDFallback(t *testing.T) {
	rl := setupRateLimiter(t, 2, 0, 60, "ip")
	interceptor := rl.UnaryInterceptor()
	h := func(ctx context.Context, req interface{}) (interface{}, error) { return nil, nil }
	// No peer info in context — should still resolve to an "unknown" id and pass.
	if _, err := interceptor(context.Background(), nil, nil, h); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestRateLimiterUserFallbackGRPC(t *testing.T) {
	rl := setupRateLimiter(t, 1, 0, 60, "user")
	interceptor := rl.UnaryInterceptor()
	ctx := peer.NewContext(context.Background(), &peer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP("6.6.6.6"), Port: 1},
	})
	ctx = context.WithValue(ctx, authContextKey, &JWTClaims{Username: "alice"})
	h := func(ctx context.Context, req interface{}) (interface{}, error) { return nil, nil }
	if _, err := interceptor(ctx, nil, nil, h); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := interceptor(ctx, nil, nil, h); err == nil {
		t.Fatal("second should reject")
	}
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	rl := setupRateLimiter(t, 1, 0, 1, "ip") // 1-second window
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := rl.HTTPMiddleware(next)
	r := httptest.NewRequest("GET", "/v1/fts", nil)
	r.RemoteAddr = "7.7.7.7:1"
	h.ServeHTTP(httptest.NewRecorder(), r)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("2nd want 429, got %d", w.Code)
	}
	time.Sleep(1100 * time.Millisecond)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("after window want 200, got %d", w.Code)
	}
}

func TestGrpcPeerIPUnixSocket(t *testing.T) {
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.UnixAddr{Name: "/tmp/test.sock", Net: "unix"}})
	got := grpcPeerIP(ctx)
	if got != "/tmp/test.sock" {
		t.Errorf("unix: got %q", got)
	}
}

func TestGrpcPeerIPNoPeer(t *testing.T) {
	if got := grpcPeerIP(context.Background()); got != "unknown" {
		t.Errorf("want unknown, got %q", got)
	}
}

func TestNewRateLimiterBadConfigFallbacks(t *testing.T) {
	t.Setenv("MDDB_RATE_LIMIT_ENABLED", "true")
	t.Setenv("MDDB_RATE_LIMIT_REQUESTS", "-1")
	t.Setenv("MDDB_RATE_LIMIT_WINDOW", "0")
	t.Setenv("MDDB_RATE_LIMIT_BURST", "")
	t.Setenv("MDDB_RATE_LIMIT_BY", "weird")
	rl := NewRateLimiter()
	if rl.limit != 100 || rl.window != 60*time.Second || rl.burst != 50 || rl.by != "ip" {
		t.Fatalf("fallbacks wrong: limit=%d window=%v burst=%d by=%s", rl.limit, rl.window, rl.burst, rl.by)
	}
}
