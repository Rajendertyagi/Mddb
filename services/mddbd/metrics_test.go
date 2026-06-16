package main

import (
	"mddb/internal/schema"
	"mddb/internal/vector"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func newTestServerForMetrics(t *testing.T) (*Server, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "mddb-metrics-*.db")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}
	s := &Server{
		DB:   db,
		Path: f.Name(),
		Mode: ModeRW,
		BucketNames: BucketNames{
			Docs:    []byte("docs"),
			IdxMeta: []byte("idxmeta"),
			Rev:     []byte("rev"),
			ByKey:   []byte("bykey"),
		},
	}
	if err := s.ensureBuckets(); err != nil {
		_ = db.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}
	s.VectorStore = vector.NewVectorStore(db)
	_ = s.VectorStore.EnsureBucket()
	s.VectorIndex = vector.NewVectorIndex()
	s.VectorSearchers = map[string]vector.VectorSearcher{
		"flat": s.VectorIndex,
	}
	s.WebhookManager = NewWebhookManager(db)
	_ = s.WebhookManager.EnsureBucket()
	_ = s.WebhookManager.LoadAll()
	s.SchemaManager = schema.NewSchemaManager(db)
	_ = s.SchemaManager.EnsureBucket()
	_ = s.SchemaManager.LoadAll()

	cleanup := func() {
		_ = db.Close()
		_ = os.Remove(f.Name())
	}
	return s, cleanup
}

func TestMetricsEndpointReturnsPrometheusFormat(t *testing.T) {
	s, cleanup := newTestServerForMetrics(t)
	defer cleanup()

	s.Metrics = NewMetrics(s, true)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	s.Metrics.HandleMetrics(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Fatalf("expected text/plain content-type, got %q", ct)
	}

	body := rec.Body.String()

	// Check required metrics are present
	required := []string{
		"mddb_info",
		"mddb_uptime_seconds",
		"mddb_database_size_bytes",
		"mddb_documents_total",
		"mddb_vector_index_ready",
		"mddb_embedding_provider_configured",
		"mddb_webhooks_total",
		"mddb_schemas_total",
		"go_goroutines",
		"go_memstats_alloc_bytes",
		"go_memstats_sys_bytes",
		"go_gc_completed_total",
	}
	for _, m := range required {
		if !strings.Contains(body, m) {
			t.Errorf("missing metric: %s", m)
		}
	}
}

func TestMetricsMiddlewareRecordsRequests(t *testing.T) {
	s, cleanup := newTestServerForMetrics(t)
	defer cleanup()

	s.Metrics = NewMetrics(s, true)

	// Create a simple handler to wrap
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	})

	wrapped := s.Metrics.Middleware(inner)

	// Simulate some requests
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", "/v1/add", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
	}
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/v1/search", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
	}

	// Now check /metrics output
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	s.Metrics.HandleMetrics(rec, req)
	body := rec.Body.String()

	if !strings.Contains(body, `mddb_http_requests_total{method="POST",path="/v1/add",status="200"} 5`) {
		t.Error("expected 5 requests to /v1/add")
		t.Log(body)
	}
	if !strings.Contains(body, `mddb_http_requests_total{method="POST",path="/v1/search",status="200"} 3`) {
		t.Error("expected 3 requests to /v1/search")
	}

	// Check histogram is present
	if !strings.Contains(body, "mddb_http_request_duration_seconds_bucket") {
		t.Error("expected histogram buckets")
	}
	if !strings.Contains(body, "mddb_http_request_duration_seconds_sum") {
		t.Error("expected histogram sum")
	}
	if !strings.Contains(body, "mddb_http_request_duration_seconds_count") {
		t.Error("expected histogram count")
	}
}

func TestMetricsDisabled(t *testing.T) {
	s, cleanup := newTestServerForMetrics(t)
	defer cleanup()

	s.Metrics = NewMetrics(s, false)

	// Handler should return 404 when disabled
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	s.Metrics.HandleMetrics(rec, req)
	if rec.Code != 404 {
		t.Fatalf("expected 404 when metrics disabled, got %d", rec.Code)
	}

	// Middleware should pass through without recording
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	wrapped := s.Metrics.Middleware(inner)
	req = httptest.NewRequest("GET", "/health", nil)
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected passthrough 200, got %d", rec.Code)
	}
}

func TestMetricsDBStatsCache(t *testing.T) {
	s, cleanup := newTestServerForMetrics(t)
	defer cleanup()

	m := NewMetrics(s, true)

	// First call should populate cache
	ds1 := m.getDBStats()
	if ds1 == nil {
		t.Fatal("expected non-nil dbStats")
	}

	// Second call within 15s should return same pointer (cached)
	ds2 := m.getDBStats()
	if ds1 != ds2 {
		t.Error("expected cached result, got different pointer")
	}

	// Force cache expiry
	m.cacheMu.Lock()
	m.cacheStamp = time.Now().Add(-20 * time.Second)
	m.cacheMu.Unlock()

	ds3 := m.getDBStats()
	if ds1 == ds3 {
		t.Error("expected fresh result after cache expiry")
	}
}

func TestHistogramObserve(t *testing.T) {
	h := newHistogram()

	// Observe values in different buckets
	h.observe(0.0005) // bucket 0.001
	h.observe(0.003)  // bucket 0.005
	h.observe(0.02)   // bucket 0.025
	h.observe(0.5)    // bucket 0.5
	h.observe(15.0)   // exceeds all buckets (+Inf only)

	if h.total != 5 {
		t.Errorf("expected total=5, got %d", h.total)
	}

	// Verify per-bucket non-cumulative counts
	expected := map[float64]int64{
		0.001: 1, 0.005: 1, 0.025: 1, 0.5: 1,
	}
	for i, b := range h.buckets {
		if exp, ok := expected[b]; ok {
			if h.counts[i] != exp {
				t.Errorf("bucket le=%.3f: expected %d, got %d", b, exp, h.counts[i])
			}
		} else {
			if h.counts[i] != 0 {
				t.Errorf("bucket le=%.3f: expected 0, got %d", b, h.counts[i])
			}
		}
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"/v1/add", "/v1/add"},
		{"/v1/search", "/v1/search"},
		{"/health", "/health"},
		{"/metrics", "/metrics"},
		{"/unknown", "/other"},
		{"/foo/bar", "/other"},
	}
	for _, tt := range tests {
		got := normalizePath(tt.input)
		if got != tt.expected {
			t.Errorf("normalizePath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestExtractCollection(t *testing.T) {
	tests := []struct {
		key      string
		expected string
	}{
		{"doc|blog|post1", "blog"},
		{"rev|articles|id|ts", "articles"},
		{"meta|faq|tag|go|doc1", "faq"},
		{"nopipe", ""},
		{"single|rest", "rest"},
	}
	for _, tt := range tests {
		got := extractCollection([]byte(tt.key))
		if got != tt.expected {
			t.Errorf("extractCollection(%q) = %q, want %q", tt.key, got, tt.expected)
		}
	}
}

func BenchmarkMetricsHandler(b *testing.B) {
	f, err := os.CreateTemp("", "mddb-metrics-bench-*.db")
	if err != nil {
		b.Fatal(err)
	}
	_ = f.Close()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		_ = db.Close()
		_ = os.Remove(f.Name())
	}()

	s := &Server{
		DB:   db,
		Path: f.Name(),
		Mode: ModeRW,
		BucketNames: BucketNames{
			Docs:    []byte("docs"),
			IdxMeta: []byte("idxmeta"),
			Rev:     []byte("rev"),
			ByKey:   []byte("bykey"),
		},
	}
	if err := s.ensureBuckets(); err != nil {
		b.Fatal(err)
	}
	s.VectorStore = vector.NewVectorStore(db)
	_ = s.VectorStore.EnsureBucket()
	s.VectorIndex = vector.NewVectorIndex()
	s.VectorSearchers = map[string]vector.VectorSearcher{
		"flat": s.VectorIndex,
	}
	s.WebhookManager = NewWebhookManager(db)
	_ = s.WebhookManager.EnsureBucket()
	_ = s.WebhookManager.LoadAll()
	s.SchemaManager = schema.NewSchemaManager(db)
	_ = s.SchemaManager.EnsureBucket()
	_ = s.SchemaManager.LoadAll()
	s.Metrics = NewMetrics(s, true)

	// Simulate some traffic
	for i := 0; i < 100; i++ {
		s.Metrics.mu.Lock()
		s.Metrics.reqCount["POST|/v1/add|200"] += 10
		h, ok := s.Metrics.histograms["POST|/v1/add"]
		if !ok {
			h = newHistogram()
			s.Metrics.histograms["POST|/v1/add"] = h
		}
		h.observe(0.005)
		s.Metrics.mu.Unlock()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/metrics", nil)
		rec := httptest.NewRecorder()
		s.Metrics.HandleMetrics(rec, req)
	}
}
