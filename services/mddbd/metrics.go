package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Prometheus-compatible metrics in text exposition format.
// Zero external dependencies – outputs plain text that any Prometheus-compatible
// scraper (Prometheus, Grafana Agent, Datadog Agent, Victoria Metrics, etc.)
// can consume directly.

// defaultBuckets defines histogram bucket boundaries for request duration (seconds).
var defaultBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0}

// Metrics collects server telemetry and exposes a /metrics endpoint.
type Metrics struct {
	enabled   bool
	startTime time.Time
	server    *Server

	mu         sync.Mutex
	reqCount   map[string]int64      // "method|path|status" -> count
	histograms map[string]*histogram // "method|path" -> histogram

	// Cached DB stats (refreshed at most every 15s)
	cacheMu    sync.Mutex
	cachedDB   *dbStats
	cacheStamp time.Time
}

type histogram struct {
	buckets []float64
	counts  []int64 // per-bucket counts (non-cumulative)
	sum     float64
	total   int64
}

type dbStats struct {
	sizeBytes    int64
	collections  map[string]collectionMetrics
	webhookCount int
	schemaCount  int
}

type collectionMetrics struct {
	documents  int
	revisions  int
	metaIndex  int
	embeddings int
}

// NewMetrics creates a new Metrics collector.
func NewMetrics(s *Server, enabled bool) *Metrics {
	return &Metrics{
		enabled:    enabled,
		startTime:  time.Now(),
		server:     s,
		reqCount:   make(map[string]int64),
		histograms: make(map[string]*histogram),
	}
}

func newHistogram() *histogram {
	return &histogram{
		buckets: defaultBuckets,
		counts:  make([]int64, len(defaultBuckets)),
	}
}

func (h *histogram) observe(v float64) {
	h.sum += v
	h.total++
	for i, b := range h.buckets {
		if v <= b {
			h.counts[i]++
			return
		}
	}
	// Value exceeds all buckets – only counted in +Inf.
}

// clone returns a snapshot safe to read without holding the lock.
func (h *histogram) clone() *histogram {
	c := &histogram{
		buckets: h.buckets,
		counts:  make([]int64, len(h.counts)),
		sum:     h.sum,
		total:   h.total,
	}
	copy(c.counts, h.counts)
	return c
}

// ---- HTTP middleware --------------------------------------------------------

// statusRecorder captures the HTTP response status code.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.written {
		r.status = code
		r.written = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.written {
		r.written = true
	}
	return r.ResponseWriter.Write(b)
}

// Middleware wraps an http.Handler, recording request count and latency.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	if !m.enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		dur := time.Since(start).Seconds()

		path := normalizePath(r.URL.Path)
		method := r.Method
		status := fmt.Sprintf("%d", rec.status)

		m.mu.Lock()
		m.reqCount[method+"|"+path+"|"+status]++
		key := method + "|" + path
		h, ok := m.histograms[key]
		if !ok {
			h = newHistogram()
			m.histograms[key] = h
		}
		h.observe(dur)
		m.mu.Unlock()
	})
}

func normalizePath(p string) string {
	if strings.HasPrefix(p, "/v1/") || p == "/health" || p == "/metrics" {
		return p
	}
	return "/other"
}

// ---- /metrics handler -------------------------------------------------------

// HandleMetrics serves GET /metrics in Prometheus text exposition format.
func (m *Metrics) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	if !m.enabled {
		http.Error(w, "metrics disabled", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	var buf strings.Builder

	// --- Server info ---
	writef(&buf, "# HELP mddb_info MDDB server information.\n")
	writef(&buf, "# TYPE mddb_info gauge\n")
	writef(&buf, "mddb_info{mode=%q} 1\n\n", string(m.server.Mode))

	// --- Uptime ---
	writef(&buf, "# HELP mddb_uptime_seconds Time since server start in seconds.\n")
	writef(&buf, "# TYPE mddb_uptime_seconds gauge\n")
	writef(&buf, "mddb_uptime_seconds %.1f\n\n", time.Since(m.startTime).Seconds())

	// --- HTTP request counter ---
	m.mu.Lock()
	reqSnapshot := make(map[string]int64, len(m.reqCount))
	for k, v := range m.reqCount {
		reqSnapshot[k] = v
	}
	histSnapshot := make(map[string]*histogram, len(m.histograms))
	for k, v := range m.histograms {
		histSnapshot[k] = v.clone()
	}
	m.mu.Unlock()

	writef(&buf, "# HELP mddb_http_requests_total Total number of HTTP requests.\n")
	writef(&buf, "# TYPE mddb_http_requests_total counter\n")
	for _, k := range sortedMapKeys(reqSnapshot) {
		parts := strings.SplitN(k, "|", 3)
		writef(&buf, "mddb_http_requests_total{method=%q,path=%q,status=%q} %d\n",
			parts[0], parts[1], parts[2], reqSnapshot[k])
	}
	buf.WriteString("\n")

	// --- HTTP request duration histogram ---
	writef(&buf, "# HELP mddb_http_request_duration_seconds HTTP request duration in seconds.\n")
	writef(&buf, "# TYPE mddb_http_request_duration_seconds histogram\n")
	for _, k := range sortedMapKeys(histSnapshot) {
		h := histSnapshot[k]
		parts := strings.SplitN(k, "|", 2)
		labels := fmt.Sprintf("method=%q,path=%q", parts[0], parts[1])
		cumulative := int64(0)
		for i, bound := range h.buckets {
			cumulative += h.counts[i]
			writef(&buf, "mddb_http_request_duration_seconds_bucket{%s,le=\"%.3f\"} %d\n",
				labels, bound, cumulative)
		}
		writef(&buf, "mddb_http_request_duration_seconds_bucket{%s,le=\"+Inf\"} %d\n", labels, h.total)
		writef(&buf, "mddb_http_request_duration_seconds_sum{%s} %.6f\n", labels, h.sum)
		writef(&buf, "mddb_http_request_duration_seconds_count{%s} %d\n", labels, h.total)
	}
	buf.WriteString("\n")

	// --- Database metrics (cached) ---
	ds := m.getDBStats()
	writef(&buf, "# HELP mddb_database_size_bytes Database file size in bytes.\n")
	writef(&buf, "# TYPE mddb_database_size_bytes gauge\n")
	writef(&buf, "mddb_database_size_bytes %d\n\n", ds.sizeBytes)

	writef(&buf, "# HELP mddb_documents_total Total documents per collection.\n")
	writef(&buf, "# TYPE mddb_documents_total gauge\n")
	for _, coll := range sortedCollections(ds.collections) {
		writef(&buf, "mddb_documents_total{collection=%q} %d\n", coll, ds.collections[coll].documents)
	}
	buf.WriteString("\n")

	writef(&buf, "# HELP mddb_revisions_total Total revisions per collection.\n")
	writef(&buf, "# TYPE mddb_revisions_total gauge\n")
	for _, coll := range sortedCollections(ds.collections) {
		writef(&buf, "mddb_revisions_total{collection=%q} %d\n", coll, ds.collections[coll].revisions)
	}
	buf.WriteString("\n")

	writef(&buf, "# HELP mddb_meta_indices_total Total metadata index entries per collection.\n")
	writef(&buf, "# TYPE mddb_meta_indices_total gauge\n")
	for _, coll := range sortedCollections(ds.collections) {
		writef(&buf, "mddb_meta_indices_total{collection=%q} %d\n", coll, ds.collections[coll].metaIndex)
	}
	buf.WriteString("\n")

	writef(&buf, "# HELP mddb_vector_embeddings_total Embedded documents per collection.\n")
	writef(&buf, "# TYPE mddb_vector_embeddings_total gauge\n")
	for _, coll := range sortedCollections(ds.collections) {
		if ds.collections[coll].embeddings > 0 {
			writef(&buf, "mddb_vector_embeddings_total{collection=%q} %d\n", coll, ds.collections[coll].embeddings)
		}
	}
	buf.WriteString("\n")

	// --- Vector search status ---
	writef(&buf, "# HELP mddb_vector_index_ready Whether the vector index is loaded (1=ready, 0=loading).\n")
	writef(&buf, "# TYPE mddb_vector_index_ready gauge\n")
	if m.server.VectorIndex != nil && m.server.VectorIndex.IsReady() {
		buf.WriteString("mddb_vector_index_ready 1\n")
	} else {
		buf.WriteString("mddb_vector_index_ready 0\n")
	}
	buf.WriteString("\n")

	writef(&buf, "# HELP mddb_embedding_provider_configured Whether an embedding provider is configured (1=yes, 0=no).\n")
	writef(&buf, "# TYPE mddb_embedding_provider_configured gauge\n")
	if m.server.Embedding != nil {
		buf.WriteString("mddb_embedding_provider_configured 1\n")
	} else {
		buf.WriteString("mddb_embedding_provider_configured 0\n")
	}
	buf.WriteString("\n")

	// Embedding queue size
	if m.server.EmbeddingWorker != nil {
		writef(&buf, "# HELP mddb_embedding_queue_size Current number of pending embedding jobs.\n")
		writef(&buf, "# TYPE mddb_embedding_queue_size gauge\n")
		writef(&buf, "mddb_embedding_queue_size %d\n\n", len(m.server.EmbeddingWorker.jobs))
	}

	// --- Webhooks & schemas ---
	writef(&buf, "# HELP mddb_webhooks_total Number of registered webhooks.\n")
	writef(&buf, "# TYPE mddb_webhooks_total gauge\n")
	writef(&buf, "mddb_webhooks_total %d\n\n", ds.webhookCount)

	writef(&buf, "# HELP mddb_schemas_total Number of registered metadata schemas.\n")
	writef(&buf, "# TYPE mddb_schemas_total gauge\n")
	writef(&buf, "mddb_schemas_total %d\n\n", ds.schemaCount)

	// --- Replication ---
	writef(&buf, "# HELP mddb_replication_role Replication role (0=standalone, 1=leader, 2=follower).\n")
	writef(&buf, "# TYPE mddb_replication_role gauge\n")
	switch m.server.ReplicationRole {
	case "leader":
		buf.WriteString("mddb_replication_role 1\n")
	case "follower":
		buf.WriteString("mddb_replication_role 2\n")
	default:
		buf.WriteString("mddb_replication_role 0\n")
	}
	buf.WriteString("\n")

	if m.server.Binlog != nil {
		bstats := m.server.Binlog.Stats()
		writef(&buf, "# HELP mddb_replication_lsn Current Log Sequence Number.\n")
		writef(&buf, "# TYPE mddb_replication_lsn gauge\n")
		writef(&buf, "mddb_replication_lsn %d\n\n", bstats.CurrentLSN)

		writef(&buf, "# HELP mddb_binlog_size_bytes Binlog file size in bytes.\n")
		writef(&buf, "# TYPE mddb_binlog_size_bytes gauge\n")
		writef(&buf, "mddb_binlog_size_bytes %d\n\n", bstats.FileSize)

		writef(&buf, "# HELP mddb_binlog_oldest_lsn Oldest LSN still in the binlog.\n")
		writef(&buf, "# TYPE mddb_binlog_oldest_lsn gauge\n")
		writef(&buf, "mddb_binlog_oldest_lsn %d\n\n", bstats.OldestLSN)

		writef(&buf, "# HELP mddb_replication_followers_connected Number of connected followers.\n")
		writef(&buf, "# TYPE mddb_replication_followers_connected gauge\n")
		writef(&buf, "mddb_replication_followers_connected %d\n\n", bstats.Subscribers)
	}

	// --- Go runtime ---
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	writef(&buf, "# HELP go_goroutines Number of goroutines.\n")
	writef(&buf, "# TYPE go_goroutines gauge\n")
	writef(&buf, "go_goroutines %d\n\n", runtime.NumGoroutine())

	writef(&buf, "# HELP go_memstats_alloc_bytes Number of bytes allocated and in use.\n")
	writef(&buf, "# TYPE go_memstats_alloc_bytes gauge\n")
	writef(&buf, "go_memstats_alloc_bytes %d\n\n", mem.Alloc)

	writef(&buf, "# HELP go_memstats_sys_bytes Total bytes of memory obtained from the OS.\n")
	writef(&buf, "# TYPE go_memstats_sys_bytes gauge\n")
	writef(&buf, "go_memstats_sys_bytes %d\n\n", mem.Sys)

	writef(&buf, "# HELP go_memstats_heap_inuse_bytes Bytes in in-use heap spans.\n")
	writef(&buf, "# TYPE go_memstats_heap_inuse_bytes gauge\n")
	writef(&buf, "go_memstats_heap_inuse_bytes %d\n\n", mem.HeapInuse)

	writef(&buf, "# HELP go_gc_completed_total Total number of completed GC cycles.\n")
	writef(&buf, "# TYPE go_gc_completed_total counter\n")
	writef(&buf, "go_gc_completed_total %d\n\n", mem.NumGC)

	writef(&buf, "# HELP go_gc_pause_seconds_total Total GC pause time in seconds.\n")
	writef(&buf, "# TYPE go_gc_pause_seconds_total counter\n")
	writef(&buf, "go_gc_pause_seconds_total %.6f\n", float64(mem.PauseTotalNs)/1e9)

	_, _ = w.Write([]byte(buf.String()))
}

// getDBStats returns cached database statistics (refreshed every 15s).
func (m *Metrics) getDBStats() *dbStats {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	if m.cachedDB != nil && time.Since(m.cacheStamp) < 15*time.Second {
		return m.cachedDB
	}

	ds := &dbStats{
		collections: make(map[string]collectionMetrics),
	}

	// File size
	if info, err := os.Stat(m.server.Path); err == nil {
		ds.sizeBytes = info.Size()
	}

	// Collection-level stats from BoltDB
	_ = m.server.DB.View(func(tx *bolt.Tx) error {
		// Documents
		if b := tx.Bucket([]byte("docs")); b != nil {
			c := b.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				if coll := extractCollection(k); coll != "" {
					cm := ds.collections[coll]
					cm.documents++
					ds.collections[coll] = cm
				}
			}
		}
		// Revisions
		if b := tx.Bucket([]byte("rev")); b != nil {
			c := b.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				if coll := extractCollection(k); coll != "" {
					cm := ds.collections[coll]
					cm.revisions++
					ds.collections[coll] = cm
				}
			}
		}
		// Meta index
		if b := tx.Bucket([]byte("idxmeta")); b != nil {
			c := b.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				if coll := extractCollection(k); coll != "" {
					cm := ds.collections[coll]
					cm.metaIndex++
					ds.collections[coll] = cm
				}
			}
		}
		return nil
	})

	// Vector embedding counts
	if m.server.VectorStore != nil {
		if counts, err := m.server.VectorStore.CountByCollection(); err == nil {
			for coll, n := range counts {
				cm := ds.collections[coll]
				cm.embeddings = n
				ds.collections[coll] = cm
			}
		}
	}

	// Webhook + schema counts
	if m.server.WebhookManager != nil {
		ds.webhookCount = len(m.server.WebhookManager.List())
	}
	if m.server.SchemaManager != nil {
		ds.schemaCount = len(m.server.SchemaManager.List())
	}

	m.cachedDB = ds
	m.cacheStamp = time.Now()
	return ds
}

// extractCollection pulls the collection name from a BoltDB key like "doc|blog|id".
func extractCollection(key []byte) string {
	i := bytes.IndexByte(key, '|')
	if i < 0 {
		return ""
	}
	rest := key[i+1:]
	j := bytes.IndexByte(rest, '|')
	if j < 0 {
		return string(rest)
	}
	return string(rest[:j])
}

// ---- helpers ----------------------------------------------------------------

func writef(b *strings.Builder, format string, args ...any) {
	fmt.Fprintf(b, format, args...)
}

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedCollections(m map[string]collectionMetrics) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
