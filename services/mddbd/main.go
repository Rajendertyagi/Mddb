package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"mddb/internal/audit"
	"mddb/internal/binlog"
	"mddb/internal/cache"
	"mddb/internal/compression"
	"mddb/internal/delta"
	"mddb/internal/embedding"
	"mddb/internal/envconf"
	"mddb/internal/fts"
	"mddb/internal/geo"
	"mddb/internal/schema"
	"mddb/internal/sliceutil"
	"mddb/internal/spell"
	"mddb/internal/temporal"
	"mddb/internal/vector"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	json "github.com/goccy/go-json"
	bolt "go.etcd.io/bbolt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// VERSION is the current release version of the MDDB server.
const VERSION = "2.10.0"

// AccessMode defines the database access mode (read, write, or both).
type AccessMode string

// Access mode constants.
const (
	ModeRead  AccessMode = "read"
	ModeWrite AccessMode = "write"
	ModeRW    AccessMode = "wr"
)

// Server is the main MDDB server instance holding the database and all subsystems.
type Server struct {
	// restoreMu guards the swappable DB handle (and the in-place reload of the
	// caches/managers) against a replication snapshot restore (GO-004). All
	// production BoltDB access goes through DBView/DBUpdate, which hold a read
	// lock; the follower's restore holds the write lock for the whole swap, so
	// in-flight reads drain before the old *bolt.DB is closed and never observe
	// a half-swapped handle.
	restoreMu           sync.RWMutex
	DB                  *bolt.DB
	Path                string
	Mode                AccessMode
	Config              ServerConfig // protocol toggles & addresses
	Hooks               Hooks        // optional extensions
	BucketNames         BucketNames
	Cache               *cache.DocumentCache  // Read-through cache (legacy)
	LockFreeCache       *LockFreeCache        // Lock-free cache (extreme performance)
	IndexQueue          *IndexQueue           // Async metadata indexing
	WAL                 *WAL                  // Write-Ahead Log
	MVCC                *MVCC                 // Multi-Version Concurrency Control
	BloomFilters        *BloomFilterManager   // Bloom filters for negative lookups
	DeltaEncoder        *delta.DeltaEncoder   // Delta encoding for revisions
	AdaptiveIndex       *AdaptiveIndexManager // Adaptive indexing
	AsyncIO             *AsyncIO              // Async I/O
	ZeroCopy            *ZeroCopyManager      // Zero-copy I/O
	SIMD                *vector.SIMDProcessor // Vectorized operations
	ShardCluster        *ShardCluster         // Distributed sharding
	finalBatchProcessor *FinalBatchProcessor  // Final optimized batch processor
	UseExtreme          bool                  // Enable extreme performance features
	// Vector search
	VectorStore       *vector.VectorStore              // Persistent vector storage in BoltDB
	VectorIndex       *vector.VectorIndex              // In-memory flat vector index
	VectorSearchers   map[string]vector.VectorSearcher // algorithm name -> searcher (flat, hnsw, ivf, pq, sq, bq)
	QuantizedVecIndex *vector.QuantizedVectorIndex     // In-memory quantized vector index (int8/int4)
	EmbeddingWorker   *EmbeddingWorker                 // Background embedding processor
	Embedding         embedding.Provider               // Embedding generation provider
	// Geo search
	GeoStore     *geo.GeoStore     // Persistent geo points in BoltDB ("geo" bucket)
	GeoIndex     *geo.GeoIndex     // In-memory R-tree geo index (default)
	GeoHashIndex *geo.GeoHashIndex // Alternative geohash-prefix index
	// New features
	TTLManager         *TTLManager               // Document TTL / auto-expiry
	FTSIndex           *fts.FTSIndex             // Full-text search index
	WebhookManager     *WebhookManager           // Webhook subscriptions and delivery
	SchemaManager      *schema.SchemaManager     // Per-collection metadata schema validation
	Metrics            *Metrics                  // Prometheus-compatible telemetry
	AuthManager        *AuthManager              // Authentication and authorization
	AuditManager       *audit.AuditManager       // Audit log (ISO 27001 A.8.15, SOC 2 CC7.2)
	RateLimiter        *RateLimiter              // Cross-transport rate limiter (ISO 27001 A.5.30, SOC 2 CC6.6)
	Encryptor          *Encryptor                // At-rest AES-256-GCM encryption (ISO 27001 A.8.24, SOC 2 CC6.7)
	RotationManager    *RotationManager          // Background re-encryption after key rotation
	AuthFailureTracker *AuthFailureTracker       // Sliding-window auth failure counter → security.auth_failure_burst
	LagMonitor         *ReplicationLagMonitor    // Periodic replication-lag → ops.replication_lag_high
	DiskMonitor        *DiskUsageMonitor         // Periodic disk-usage → ops.disk_usage_high
	SynonymManager     *fts.SynonymManager       // Synonym dictionaries for FTS
	StopWordManager    *fts.StopWordManager      // Per-collection custom stop words for FTS
	AutomationManager  *AutomationManager        // Automation: triggers, crons, webhook targets
	AutomationLogStore *AutomationLogStore       // Automation execution logs
	CronScheduler      *CronScheduler            // Cron scheduler for automation
	CollectionManager  *CollectionManager        // Per-collection attributes (type, description, icon, etc.)
	CurationManager    *CurationManager          // FTS/Hybrid curation rules: pinned + hidden results per query
	TemporalManager    *temporal.TemporalManager // Document lifecycle event tracking (create/update/access)
	SpellManager       *spell.SpellManager       // Spell correction for FTS queries and document content
	SSEHub             *SSEHub                   // Server-Sent Events for real-time document change notifications
	BulkIngest         *BulkIngestManager        // Async bulk ingest job manager
	MCPInfo            MCPServerInfo             // Customizable MCP server profile
	MCPInstructions    string                    // System prompt for LLM — how to use this server
	mcpKeyStore        *mcpAPIKeyStore           // BoltDB-backed MCP API key store
	mcpAuth            *MCPAPIKeyMiddleware      // MCP API key middleware (for cache invalidation)
	// Replication
	Binlog          *binlog.Binlog     // Binary replication log
	ReplicationRole string             // "leader", "follower", or "" (standalone)
	NodeID          string             // Unique node identifier
	replServer      *ReplicationServer // Leader-side gRPC replication service
	replClient      *ReplicationClient // Follower-side replication client
	// Readiness
	Ready bool // Set to true after full initialization; health check returns "warming_up" until then
}

// BucketNames caches bucket name byte slices to avoid repeated allocations
type BucketNames struct {
	Docs    []byte
	IdxMeta []byte
	Rev     []byte
	ByKey   []byte
}

// Hooks holds optional post-action webhook and exec hooks.
type Hooks struct {
	PostAddWebhookURL    string   // e.g. http://localhost:9000/hook/add
	PostAddExec          []string // e.g. ["/usr/local/bin/on-add"]
	PostUpdateWebhookURL string
	PostUpdateExec       []string
}

// Doc represents a stored markdown document.
type Doc struct {
	ID        string              `json:"id"`        // generated
	Key       string              `json:"key"`       // e.g. "homepage"
	Lang      string              `json:"lang"`      // e.g. "en_GB"
	Meta      map[string][]string `json:"meta"`      // meta values (multi)
	ContentMD string              `json:"contentMd"` // raw markdown
	AddedAt   int64               `json:"addedAt"`
	UpdatedAt int64               `json:"updatedAt"`
	ExpiresAt int64               `json:"expiresAt,omitempty"` // unix timestamp; 0 = never
}

// AddRequest is the HTTP request body for adding or updating a document.
type AddRequest struct {
	Collection string              `json:"collection"`
	Key        string              `json:"key"`
	Lang       string              `json:"lang"`
	Meta       map[string][]string `json:"meta"`
	ContentMD  string              `json:"contentMd"`
	TTL        int64               `json:"ttl,omitempty"` // seconds; 0 = no expiry
}

// GetRequest is the HTTP request body for retrieving a document.
type GetRequest struct {
	Collection string            `json:"collection"`
	Key        string            `json:"key"`
	Lang       string            `json:"lang"`
	Env        map[string]string `json:"env"` // for templating
}

// SearchRequest is the HTTP request body for searching documents.
type SearchRequest struct {
	Collection string              `json:"collection"`
	FilterMeta map[string][]string `json:"filterMeta"` // AND over keys, OR over values
	Sort       string              `json:"sort"`       // addedAt|updatedAt|key
	Asc        bool                `json:"asc"`
	Limit      int                 `json:"limit"`
	Offset     int                 `json:"offset"`
}

// ExportRequest is the HTTP request body for exporting documents.
type ExportRequest struct {
	Collection string              `json:"collection"`
	FilterMeta map[string][]string `json:"filterMeta"`
	Format     string              `json:"format"` // ndjson|zip
}

// TruncateRequest is the HTTP request body for truncating a collection.
type TruncateRequest struct {
	Collection string `json:"collection"`
	KeepRevs   int    `json:"keepRevs"` // keep last N revisions per doc (0 = drop all history)
	DropCache  bool   `json:"dropCache"`
}

// DeleteRequest is the HTTP request body for deleting a single document.
type DeleteRequest struct {
	Collection string `json:"collection"`
	Key        string `json:"key"`
	Lang       string `json:"lang"`
}

// DeleteCollectionRequest is the HTTP request body for deleting an entire collection.
type DeleteCollectionRequest struct {
	Collection string `json:"collection"`
}

// getOptimizedBoltOptions returns optimized BoltDB options for performance
func getOptimizedBoltOptions() *bolt.Options {
	return &bolt.Options{
		Timeout:         2 * time.Second,
		NoFreelistSync:  true,                 // Don't sync freelist to disk on every commit (faster writes)
		FreelistType:    bolt.FreelistMapType, // Use hashmap for freelist (faster than array)
		NoGrowSync:      false,                // Sync after growing mmap (safer)
		InitialMmapSize: 100 * 1024 * 1024,    // 100MB initial mmap (reduce remapping)
	}
}

func main() {
	// Load server configuration (CLI flags > env vars > config file > defaults)
	srvCfg := loadServerConfig()

	dbPath := srvCfg.Database.Path
	mode := srvCfg.Database.Mode

	db, err := bolt.Open(dbPath, 0600, getOptimizedBoltOptions())
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}()

	// Extreme mode = HTTP/3 enabled (legacy MDDB_EXTREME mapped in config)
	useExtreme := srvCfg.HTTP3.Enabled

	s := &Server{
		DB:     db,
		Path:   dbPath,
		Mode:   mode,
		Config: srvCfg,
		BucketNames: BucketNames{
			Docs:    []byte("docs"),
			IdxMeta: []byte("idxmeta"),
			Rev:     []byte("rev"),
			ByKey:   []byte("bykey"),
		},
		Cache:         cache.NewDocumentCache(1000, 300), // 1000 docs, 5min TTL
		LockFreeCache: NewLockFreeCache(10000, 300),      // 10k docs, 5min TTL (lock-free)
		IndexQueue:    NewIndexQueue(nil, 4),             // 4 workers (will set server below)
		BloomFilters:  NewBloomFilterManager(),           // Bloom filters
		DeltaEncoder:  delta.NewDeltaEncoder(),           // Delta encoding
		AdaptiveIndex: NewAdaptiveIndexManager(),         // Adaptive indexing
		AsyncIO:       NewAsyncIO(),                      // Async I/O
		ZeroCopy:      NewZeroCopyManager(),              // Zero-copy I/O
		SIMD:          vector.NewSIMDProcessor(),         // Vectorized operations
		ShardCluster:  NewShardCluster(4, 2),             // 4 shards, 2x replication
		UseExtreme:    useExtreme,
	}
	s.IndexQueue.server = s // Set server reference

	// Start early health-only HTTP server so clients can poll readiness during init.
	// This server is shut down gracefully before the main HTTP server starts.
	var earlyServer *http.Server
	if srvCfg.HTTP.Enabled {
		earlyMux := http.NewServeMux()
		earlyMux.HandleFunc("/health", s.handleHealth)
		earlyMux.HandleFunc("/v1/health", s.handleHealth)
		earlyServer = &http.Server{
			Addr:              srvCfg.HTTP.Addr,
			Handler:           earlyMux,
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			if err := earlyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Early health server: %v", err)
			}
		}()
		log.Printf("Health endpoint available on %s (warming up...)", srvCfg.HTTP.Addr)
	}

	// Initialize extreme performance features
	if useExtreme {
		log.Println("🚀 Extreme Performance Mode ENABLED")

		// Initialize WAL
		wal, err := NewWAL(dbPath, SyncPeriodic)
		if err != nil {
			log.Fatalf("Failed to initialize WAL: %v", err)
		}
		s.WAL = wal
		log.Println("  ✓ WAL initialized (SyncPeriodic)")

		// Initialize MVCC
		s.MVCC = NewMVCC()
		log.Println("  ✓ MVCC initialized")

		log.Println("  ✓ Lock-Free Cache enabled")
		log.Println("  ✓ Bloom Filters enabled")
		log.Println("  ✓ Delta Encoding enabled")
		log.Println("  ✓ Adaptive Compression enabled (Snappy + Zstd)")
		log.Println("  ✓ Adaptive Indexing enabled")
		log.Println("  ✓ Async I/O enabled")
		log.Println("  ✓ Zero-Copy I/O enabled")
		log.Println("  ✓ Vectorized Operations (SIMD) enabled")
		log.Println("  ✓ Distributed Sharding enabled (4 shards, 2x replication)")
	}

	if err := s.ensureBuckets(); err != nil {
		log.Fatal(err)
	}

	// Initialize vector search
	s.VectorStore = vector.NewVectorStore(db)
	if err := s.VectorStore.EnsureBucket(); err != nil {
		log.Fatal(err)
	}
	s.VectorIndex = vector.NewVectorIndex()

	// Initialize geo search. Startup rebuild from BoltDB happens asynchronously
	// below so mddbd can start serving non-geo requests immediately.
	// Both indexes share the same "geo" bucket in BoltDB — they're two views
	// of the same underlying data, picked by the /v1/geo-search?algorithm=...
	// parameter at query time.
	s.GeoStore = geo.NewGeoStore(db)
	if err := s.GeoStore.EnsureBucket(); err != nil {
		log.Fatal(err)
	}
	s.GeoIndex = geo.NewGeoIndex()
	s.GeoHashIndex = geo.NewGeoHashIndex()
	bqRerank := srvCfg.Vector.BQRerankFactor
	if bqRerank <= 0 {
		bqRerank = 10
	}
	s.QuantizedVecIndex = vector.NewQuantizedVectorIndex(func(collection string) vector.QuantizationType {
		if s.CollectionManager == nil {
			return vector.QuantNone
		}
		cfg, ok := s.CollectionManager.Get(collection)
		if !ok || cfg.Quantization == "" {
			return vector.QuantNone
		}
		return vector.ParseQuantization(cfg.Quantization)
	})
	s.VectorSearchers = map[string]vector.VectorSearcher{
		"flat":      s.VectorIndex,
		"hnsw":      vector.NewHNSWIndex(16, 200, 100),
		"ivf":       vector.NewIVFIndex(10, 20),
		"pq":        vector.NewPQIndex(8, 256, 20),
		"opq":       vector.NewOPQIndex(8, 256, 20, 5),
		"sq":        vector.NewSQIndex(),
		"bq":        vector.NewBQIndex(bqRerank),
		"quantized": s.QuantizedVecIndex,
	}

	// Try to load embedding config from database first
	defaultConfig, err := s.GetDefaultEmbeddingConfig()
	if err == nil && defaultConfig != nil {
		// Use stored configuration
		s.InitializeEmbeddingFromConfig(defaultConfig)
		log.Printf("Vector search enabled from stored config (provider=%s, model=%s, dims=%d)",
			defaultConfig.Provider, defaultConfig.Model, defaultConfig.Dimensions)
	} else {
		// Fall back to environment variables
		s.Embedding = embedding.NewProvider()
		if s.Embedding != nil {
			s.EmbeddingWorker = NewEmbeddingWorker(s.Embedding, s.VectorStore, s.VectorIndex, 1000)
			s.EmbeddingWorker.Start(2)
			log.Printf("Vector search enabled from env vars (provider=%s, model=%s, dims=%d)",
				s.Embedding.Model(), s.Embedding.Model(), s.Embedding.Dimensions())
		} else {
			log.Println("Vector search: embedding provider not configured (set MDDB_EMBEDDING_PROVIDER or configure in panel)")
		}
	}

	// Load vectors into memory asynchronously
	go s.loadVectorIndex()

	// Load geo indexes from BoltDB asynchronously. Both R-tree and geohash
	// are populated from the same "geo" bucket so the /v1/geo-search
	// algorithm switch is a pure runtime decision — no per-algorithm
	// persistence layers. Handlers reject requests with 503 while !IsReady,
	// matching the vector-index startup contract.
	go func() {
		start := time.Now()
		n, err := s.GeoStore.Rebuild(s.GeoIndex, "")
		if err != nil {
			log.Printf("Geo index rebuild failed: %v", err)
		}
		s.GeoIndex.SetReady()
		log.Printf("Geo R-tree loaded: %d points across %d collections in %v",
			n, len(s.GeoIndex.Collections()), time.Since(start))

		start = time.Now()
		nh, err := s.GeoStore.RebuildHash(s.GeoHashIndex, "")
		if err != nil {
			log.Printf("Geohash index rebuild failed: %v", err)
		}
		s.GeoHashIndex.SetReady()
		log.Printf("Geohash index loaded: %d points in %v", nh, time.Since(start))
	}()

	// Initialize audit log (ISO 27001 A.8.15 / SOC 2 CC7.2)
	auditEnabled := env("MDDB_AUDIT_ENABLED", "false") == "true"
	auditRetention := 90
	if v := env("MDDB_AUDIT_RETENTION_DAYS", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			auditRetention = n
		}
	}
	s.AuditManager = audit.NewAuditManager(db, auditEnabled, auditRetention)
	if err := s.AuditManager.EnsureBuckets(); err != nil {
		log.Fatal(err)
	}
	s.AuditManager.Start()
	if auditEnabled {
		log.Printf("Audit log enabled (retention %d days)", auditRetention)

		// Wire optional external sinks (ISO 27001 A.8.15 / SOC 2 CC7.2 —
		// audit trail must be tamper-evident; pushing to an off-host SIEM
		// or syslog collector covers the case where the local DB is
		// compromised).
		exportBuf := 1024
		if v := env("MDDB_AUDIT_EXPORT_BUFFER", ""); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				exportBuf = n
			}
		}
		if u := env("MDDB_AUDIT_EXPORT_WEBHOOK_URL", ""); u != "" {
			insecure := env("MDDB_AUDIT_EXPORT_WEBHOOK_INSECURE_TLS", "") == "true"
			we, err := audit.NewWebhookExporter(u, env("MDDB_AUDIT_EXPORT_WEBHOOK_HEADER", ""), exportBuf, insecure)
			if err != nil {
				log.Printf("audit webhook exporter: %v", err)
			} else {
				s.AuditManager.AddExporter(we)
				log.Printf("Audit webhook exporter active → %s", u)
			}
		}
		if a := env("MDDB_AUDIT_EXPORT_SYSLOG_ADDR", ""); a != "" {
			fac := env("MDDB_AUDIT_EXPORT_SYSLOG_FACILITY", "local0")
			se, err := audit.NewSyslogExporter(a, fac, exportBuf)
			if err != nil {
				log.Printf("audit syslog exporter: %v", err)
			} else {
				s.AuditManager.AddExporter(se)
				log.Printf("Audit syslog exporter active → %s (facility %s)", a, fac)
			}
		}
	}

	// Initialize TTL manager
	s.TTLManager = NewTTLManager(db, s)
	if err := s.TTLManager.EnsureBuckets(); err != nil {
		log.Fatal(err)
	}
	s.TTLManager.StartCleanup(30 * time.Second)
	log.Println("TTL manager started (cleanup every 30s)")

	// Configure compression
	compression.ConfigureCompression(srvCfg.Compression.Enabled, srvCfg.Compression.SmallThreshold, srvCfg.Compression.MediumThreshold)
	if !srvCfg.Compression.Enabled {
		log.Println("Document compression disabled")
	}

	// Initialize FTS index
	s.FTSIndex = fts.NewFTSIndex(db)
	if err := s.FTSIndex.EnsureBuckets(); err != nil {
		log.Fatal(err)
	}

	// Initialize multi-language FTS support
	langReg := fts.NewLangRegistry(srvCfg.FTS.DefaultLang)
	if srvCfg.FTS.StemmingEnabled {
		fts.RegisterDefaultLanguages(langReg)
		s.FTSIndex.SetStemmer(fts.NewPorterStemmer())
		s.FTSIndex.SetLangRegistry(langReg)
		log.Printf("FTS stemming enabled — %d languages (default: %s)", len(langReg.Languages()), srvCfg.FTS.DefaultLang)
	}

	// Initialize synonym manager
	s.SynonymManager = fts.NewSynonymManager(db)
	if err := s.SynonymManager.EnsureBucket(); err != nil {
		log.Fatal(err)
	}
	if err := s.SynonymManager.LoadAll(); err != nil {
		log.Fatal(err)
	}
	if srvCfg.FTS.SynonymsEnabled {
		s.FTSIndex.SetSynonymManager(s.SynonymManager)
		log.Println("FTS synonyms enabled")
	}

	// Initialize stop word manager
	s.StopWordManager = fts.NewStopWordManager(db)
	if err := s.StopWordManager.EnsureBucket(); err != nil {
		log.Fatal(err)
	}
	if err := s.StopWordManager.LoadAll(); err != nil {
		log.Fatal(err)
	}
	s.FTSIndex.SetStopWordManager(s.StopWordManager)
	s.StopWordManager.SetLangRegistry(langReg)
	log.Println("Stop word manager initialized")

	// Initialize PMI data for PMISparse search
	s.FTSIndex.SetPMIData(fts.NewPMIData())

	log.Println("Full-text search index initialized")

	// Initialize webhook manager
	s.WebhookManager = NewWebhookManager(db)
	if err := s.WebhookManager.EnsureBucket(); err != nil {
		log.Fatal(err)
	}
	if err := s.WebhookManager.LoadAll(); err != nil {
		log.Fatal(err)
	}
	log.Printf("Webhook manager initialized (%d hooks loaded)", len(s.WebhookManager.List()))

	// Incident detectors — reuse the WebhookManager to deliver
	// security.* and ops.* events to whichever hooks subscribed.
	s.AuthFailureTracker = NewAuthFailureTracker(s.WebhookManager)
	s.DiskMonitor = NewDiskUsageMonitor(s.WebhookManager, dbPath)
	s.DiskMonitor.Start()

	// Initialize automation manager (triggers, crons, webhook targets)
	automationsEnabled := env("MDDB_AUTOMATIONS", "enable") != "disable"
	if automationsEnabled {
		s.AutomationManager = NewAutomationManager(db)
		s.AutomationManager.SetServer(s)
		if err := s.AutomationManager.EnsureBucket(); err != nil {
			log.Fatal(err)
		}
		if err := s.AutomationManager.LoadAll(); err != nil {
			log.Fatal(err)
		}
		log.Printf("Automation manager initialized (%d rules loaded)", len(s.AutomationManager.List("")))

		// Initialize automation log store
		if env("MDDB_AUTOMATION_LOGS", "enable") != "disable" {
			logTTLStr := env("MDDB_AUTOMATION_LOGS_TTL", "7d")
			logTTL, err := ParseDurationString(logTTLStr)
			if err != nil {
				log.Fatalf("Invalid MDDB_AUTOMATION_LOGS_TTL: %v", err)
			}
			s.AutomationLogStore = NewAutomationLogStore(db, logTTL)
			if err := s.AutomationLogStore.EnsureBucket(); err != nil {
				log.Fatal(err)
			}
			s.AutomationLogStore.StartCleanup(5 * time.Minute)
			s.AutomationManager.SetLogStore(s.AutomationLogStore)
			log.Printf("Automation logs enabled (TTL: %s)", logTTLStr)
		}
	}

	// Initialize schema manager
	s.SchemaManager = schema.NewSchemaManager(db)
	if err := s.SchemaManager.EnsureBucket(); err != nil {
		log.Fatal(err)
	}
	if err := s.SchemaManager.LoadAll(); err != nil {
		log.Fatal(err)
	}
	log.Printf("Schema manager initialized (%d schemas loaded)", len(s.SchemaManager.List()))

	// Initialize collection config manager
	s.CollectionManager = NewCollectionManager(db)
	if err := s.CollectionManager.EnsureBucket(); err != nil {
		log.Fatal(err)
	}
	if err := s.CollectionManager.LoadAll(); err != nil {
		log.Fatal(err)
	}
	log.Printf("Collection manager initialized (%d collections configured)", len(s.CollectionManager.ListAll()))

	// At-rest encryption (ISO 27001 A.8.24 / SOC 2 CC6.7). The encryptor
	// is a no-op when MDDB_ENCRYPTION_KEY is unset; a misconfigured key
	// fatals to avoid silently storing plaintext for a collection that
	// was explicitly opted in.
	enc, err := NewEncryptor()
	if err != nil {
		log.Fatalf("encryption init: %v", err)
	}
	s.Encryptor = enc
	SetGlobalEncryptor(enc)
	s.CollectionManager.SetEncryptor(enc)
	if enc.Enabled() {
		encCount := 0
		for _, cfg := range s.CollectionManager.ListAll() {
			if cfg != nil && cfg.Encrypted {
				encCount++
			}
		}
		log.Printf("At-rest encryption enabled (%d collection(s) opted in)", encCount)
		log.Printf("Encryption primary keyID=%d, previous=%d", enc.PrimaryKeyID(), len(enc.PreviousKeyIDs()))
		s.RotationManager = NewRotationManager(s, enc)
	} else {
		// If any collection is flagged as encrypted but we have no key,
		// refuse to start — writing plaintext into a supposedly encrypted
		// collection is a silent compliance failure.
		for name, cfg := range s.CollectionManager.ListAll() {
			if cfg != nil && cfg.Encrypted {
				log.Fatalf("collection %q has encrypted=true but MDDB_ENCRYPTION_KEY is not set", name)
			}
		}
	}

	// Curation manager: loads all pinning/hiding rules into memory so the
	// search hot path never hits disk.
	s.CurationManager = NewCurationManager(db)
	if err := s.CurationManager.EnsureBucket(); err != nil {
		log.Fatal(err)
	}
	if err := s.CurationManager.LoadAll(); err != nil {
		log.Fatal(err)
	}
	log.Printf("Curation manager initialized (%d rules loaded)", len(s.CurationManager.ListAll()))

	// Initialize temporal event tracking (disabled by default; set MDDB_TEMPORAL=true to enable)
	if env("MDDB_TEMPORAL", "false") == "true" {
		s.TemporalManager = temporal.NewTemporalManager(db)
		if err := s.TemporalManager.EnsureBuckets(); err != nil {
			log.Fatal(err)
		}
		s.TemporalManager.Start()
		log.Println("Temporal event tracking initialized")
	}

	// Initialize spell checker (disabled by default; set MDDB_SPELL=true to enable)
	if env("MDDB_SPELL", "false") == "true" {
		s.SpellManager = spell.NewSpellManager(db)
		if err := s.SpellManager.EnsureBucket(); err != nil {
			log.Fatal(err)
		}
		s.SpellManager.LoadAll() // async — sets ready flag when done
		log.Println("Spell manager initialized (loading dictionaries in background)")
	}

	// Initialize SSE hub (enabled by default, set MDDB_SSE_ENABLED=false to disable)
	sseEnabled := env("MDDB_SSE_ENABLED", "true") != "false"
	sseMaxClients := envconf.Int("MDDB_SSE_MAX_CLIENTS", 1000)
	sseMaxPerIP := envconf.Int("MDDB_SSE_MAX_PER_IP", 5)
	s.SSEHub = NewSSEHub(sseEnabled, sseMaxClients, sseMaxPerIP)
	if sseEnabled {
		log.Printf("SSE event stream enabled (max clients: %d, max per IP: %d)", sseMaxClients, sseMaxPerIP)
	}

	// Store MCP server info for handlers
	s.MCPInfo = srvCfg.MCP.ServerInfo
	s.MCPInstructions = srvCfg.MCP.Instructions

	// Initialize MCP API key store (BoltDB-backed, persists across restarts)
	s.mcpKeyStore = newMCPAPIKeyStore(s.DB)

	// Initialize metrics (enabled by default, set MDDB_METRICS=false to disable)
	metricsEnabled := env("MDDB_METRICS", "true") != "false"
	s.Metrics = NewMetrics(s, metricsEnabled)
	if metricsEnabled {
		log.Println("Prometheus metrics enabled (GET /metrics)")
	}

	// Wire metrics into subsystems
	if s.EmbeddingWorker != nil {
		s.EmbeddingWorker.metrics = s.Metrics
	}
	if s.WebhookManager != nil {
		s.WebhookManager.metrics = s.Metrics
	}

	// Initialize replication
	s.ReplicationRole = env("MDDB_REPLICATION_ROLE", "") // "leader", "follower", ""
	s.NodeID = env("MDDB_NODE_ID", "")
	if s.NodeID == "" && s.ReplicationRole != "" {
		s.NodeID = fmt.Sprintf("node-%d", time.Now().UnixNano()%100000)
	}

	// Force follower to read-only mode
	if s.ReplicationRole == "follower" {
		s.Mode = ModeRead
		log.Println("Replication: follower mode — forced read-only")
	}

	// Initialize binlog (auto-enabled for leader, opt-in for standalone)
	binlogEnabled := env("MDDB_BINLOG_ENABLED", "false") == "true" || s.ReplicationRole == "leader"
	if binlogEnabled {
		bl, err := binlog.NewBinlog(dbPath, binlog.BinlogConfig{
			Path:    env("MDDB_BINLOG_PATH", ""),
			MaxSize: 256 * 1024 * 1024, // 256MB
			MaxAge:  24 * time.Hour,
		})
		if err != nil {
			log.Fatalf("Failed to initialize binlog: %v", err)
		}
		s.Binlog = bl
		log.Printf("Binlog enabled (LSN=%d)", bl.CurrentLSN())
	}

	// Set binlog on all subsystems
	if s.Binlog != nil {
		s.VectorStore.SetBinlog(s.Binlog)
		s.GeoStore.SetBinlog(s.Binlog)
		s.FTSIndex.SetBinlog(s.Binlog)
		s.TTLManager.SetBinlog(s.Binlog)
		s.WebhookManager.SetBinlog(s.Binlog)
		s.SchemaManager.SetBinlog(s.Binlog)
		s.SynonymManager.SetBinlog(s.Binlog)
		s.StopWordManager.SetBinlog(s.Binlog)
		s.AutomationManager.SetBinlog(s.Binlog)
		s.CollectionManager.SetBinlog(s.Binlog)
		if s.TemporalManager != nil {
			s.TemporalManager.SetBinlog(s.Binlog)
		}
		if s.SpellManager != nil {
			s.SpellManager.SetBinlog(s.Binlog)
		}
	}

	// Follower: disable background writers (data comes from binlog)
	if s.ReplicationRole == "follower" {
		s.TTLManager.Stop() // don't run TTL cleanup, leader handles it
		s.TTLManager = NewTTLManager(db, s)
		_ = s.TTLManager.EnsureBuckets()
		// Don't start cleanup — deletes come from binlog

		// Don't start embedding worker — embeddings come from leader
		if s.EmbeddingWorker != nil {
			s.EmbeddingWorker.Stop()
			s.EmbeddingWorker = nil
		}
		log.Println("Replication: disabled TTL cleanup and embedding worker on follower")
	}

	// Initialize cron scheduler (if enabled)
	if automationsEnabled && env("MDDB_CRONS", "false") == "true" {
		s.CronScheduler = NewCronScheduler(s)
		s.CronScheduler.Start()
		s.CronScheduler.Reload()
		log.Println("Cron scheduler started")
	}

	// Initialize authentication (disabled by default)
	authEnabled := env("MDDB_AUTH_ENABLED", "false") == "true"
	if authEnabled {
		jwtSecret := env("MDDB_AUTH_JWT_SECRET", "")
		if jwtSecret == "" {
			log.Fatal("MDDB_AUTH_ENABLED=true requires MDDB_AUTH_JWT_SECRET to be set")
		}

		jwtExpiryStr := env("MDDB_AUTH_JWT_EXPIRY", "24h")
		jwtExpiry, err := time.ParseDuration(jwtExpiryStr)
		if err != nil {
			log.Fatalf("Invalid MDDB_AUTH_JWT_EXPIRY: %v", err)
		}

		s.AuthManager = NewAuthManager(db, AuthConfig{
			JWTSecret:     jwtSecret,
			JWTExpiry:     jwtExpiry,
			AdminUsername: env("MDDB_AUTH_ADMIN_USERNAME", "admin"),
			AdminPassword: env("MDDB_AUTH_ADMIN_PASSWORD", ""),
		})

		if err := s.AuthManager.EnsureBuckets(); err != nil {
			log.Fatal(err)
		}
		if err := s.AuthManager.LoadAll(); err != nil {
			log.Fatal(err)
		}
		if err := s.AuthManager.BootstrapAdmin(); err != nil {
			log.Fatal(err)
		}

		log.Println("✓ Authentication enabled")
	}
	if s.AuthManager != nil {
		s.AuthManager.SetServer(s)
	}

	// Initialize the shared HTTP+gRPC rate limiter (opt-in via
	// MDDB_RATE_LIMIT_ENABLED). Both transports consume the same
	// per-client budget. Wire the webhook publisher so rejections
	// surface as security.rate_limit_exceeded events.
	s.RateLimiter = NewRateLimiter()
	s.RateLimiter.SetWebhookManager(s.WebhookManager)

	// Enforce ISO 27001 / SOC 2 guardrails when MDDB_PRODUCTION=true,
	// or log a one-shot warning otherwise.
	EnforceProductionGuards(log.Printf, log.Fatalf)

	// MCP stdio mode — replaces normal HTTP/gRPC operation
	if srvCfg.MCP.Stdio {
		s.runMCPStdio()
		return
	}

	// Log protocol configuration
	log.Printf("Protocol config: HTTP=%v gRPC=%v MCP=%v HTTP3=%v", srvCfg.HTTP.Enabled, srvCfg.GRPC.Enabled, srvCfg.MCP.Enabled, srvCfg.HTTP3.Enabled)
	// Log per-protocol mode overrides
	if srvCfg.HTTP.Mode != "" {
		log.Printf("Per-protocol mode: API=%s (MDDB_API_MODE)", srvCfg.HTTP.Mode)
	}
	if srvCfg.GRPC.Mode != "" {
		log.Printf("Per-protocol mode: gRPC=%s (MDDB_GRPC_MODE)", srvCfg.GRPC.Mode)
	}
	if srvCfg.MCP.Mode != "" {
		log.Printf("Per-protocol mode: MCP=%s (MDDB_MCP_MODE)", srvCfg.MCP.Mode)
	}
	if srvCfg.HTTP3.Mode != "" {
		log.Printf("Per-protocol mode: HTTP3=%s (MDDB_HTTP3_MODE)", srvCfg.HTTP3.Mode)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/compliance-status", s.handleComplianceStatus)
	mux.HandleFunc("/v1/add", s.guardWrite(s.handleAdd))
	mux.HandleFunc("/v1/add-batch", s.guardWrite(s.handleAddBatch))
	mux.HandleFunc("/v1/bulk-ingest-job", s.guardWrite(s.handleBulkIngestSubmit))
	mux.HandleFunc("/v1/bulk-ingest-job/", s.handleBulkIngestStatus)
	mux.HandleFunc("/v1/bulk-ingest-jobs", s.handleBulkIngestList)
	mux.HandleFunc("/v1/ingest", s.guardWrite(s.handleIngest))
	mux.HandleFunc("/v1/get", s.handleGet)
	mux.HandleFunc("/v1/search", s.handleSearch)
	mux.HandleFunc("/v1/export", s.handleExport)
	mux.HandleFunc("/v1/backup", s.handleBackup)
	mux.HandleFunc("/v1/restore", s.guardWrite(s.handleRestore))
	mux.HandleFunc("/v1/truncate", s.guardWrite(s.handleTruncate))
	mux.HandleFunc("/v1/delete", s.guardWrite(s.handleDelete))
	mux.HandleFunc("/v1/delete-batch", s.guardWrite(s.handleDeleteBatch))
	mux.HandleFunc("/v1/delete-collection", s.guardWrite(s.handleDeleteCollection))
	mux.HandleFunc("/v1/stats", s.handleStats)
	mux.HandleFunc("/v1/vector-search", s.handleVectorSearch)
	mux.HandleFunc("/v1/vector-reindex", s.guardWrite(s.handleVectorReindex))
	mux.HandleFunc("/v1/vector-stats", s.handleVectorStats)
	mux.HandleFunc("/v1/geo-search", s.handleGeoSearch)
	mux.HandleFunc("/v1/geo-within", s.handleGeoWithin)
	mux.HandleFunc("/v1/geo-polygon", s.handleGeoPolygon)
	mux.HandleFunc("/v1/geo-reindex", s.guardWrite(s.handleGeoReindex))
	mux.HandleFunc("/v1/geo-stats", s.handleGeoStats)
	mux.HandleFunc("/v1/geo-encode", s.handleGeoEncode)
	mux.HandleFunc("/v1/geo-decode", s.handleGeoDecode)
	mux.HandleFunc("/v1/embedding-configs", s.handleEmbeddingConfigs)
	mux.HandleFunc("/v1/embedding-configs/", s.handleEmbeddingConfigDetail)
	mux.HandleFunc("/v1/embedding-configs/set-default", s.guardWrite(s.handleSetDefaultEmbeddingConfig))
	mux.HandleFunc("/v1/upload", s.guardWrite(s.handleUpload))
	mux.HandleFunc("/v1/import-url", s.guardWrite(s.handleImportURL))
	mux.HandleFunc("/v1/import-wiki", s.guardWrite(s.handleWikiImport))
	mux.HandleFunc("/v1/set-ttl", s.guardWrite(s.handleSetTTL))
	mux.HandleFunc("/v1/fts", s.handleFTS)
	mux.HandleFunc("/v1/fts-reindex", s.guardWrite(s.handleFTSReindex))
	mux.HandleFunc("/v1/fts-languages", s.handleFTSLanguages)
	mux.HandleFunc("/v1/autocomplete", s.handleAutocomplete)
	mux.HandleFunc("/v1/meta-keys", s.handleMetaKeys)
	mux.HandleFunc("/v1/checksum", s.handleChecksum)
	mux.HandleFunc("/v1/update", s.guardWrite(s.handleUpdate))
	mux.HandleFunc("/v1/doc-meta", s.handleDocMeta)
	mux.HandleFunc("/v1/classify", s.handleClassify)
	mux.HandleFunc("/v1/hybrid-search", s.handleHybridSearch)
	mux.HandleFunc("/v1/synonyms", s.handleSynonyms)
	mux.HandleFunc("/v1/stopwords", s.handleStopWords)
	mux.HandleFunc("/v1/audit", s.handleAudit)
	mux.HandleFunc("/v1/audit/exporters", s.handleAuditExporters)
	mux.HandleFunc("/v1/encryption/status", s.handleEncryptionStatus)
	mux.HandleFunc("/v1/encryption/rotate", s.handleEncryptionRotate)
	mux.HandleFunc("/v1/encryption/jobs", s.handleEncryptionJob)
	mux.HandleFunc("/v1/encryption/jobs/", s.handleEncryptionJob)
	mux.HandleFunc("/v1/webhooks", s.handleWebhooks)
	mux.HandleFunc("/v1/webhooks/delete", s.guardWrite(s.handleWebhookDelete))
	mux.HandleFunc("/v1/revisions", s.handleRevisions)
	mux.HandleFunc("/v1/revisions/restore", s.guardWrite(s.handleRevisionRestore))
	if s.AutomationManager != nil {
		mux.HandleFunc("/v1/automation", s.handleAutomation)
		mux.HandleFunc("/v1/automation/", s.handleAutomationDetail)
		if s.AutomationLogStore != nil {
			mux.HandleFunc("/v1/automation-logs", s.handleAutomationLogs)
		}
	}
	mux.HandleFunc("/v1/schema/set", s.guardWrite(s.handleSchemaSet))
	mux.HandleFunc("/v1/schema/get", s.handleSchemaGet)
	mux.HandleFunc("/v1/schema/delete", s.guardWrite(s.handleSchemaDelete))
	mux.HandleFunc("/v1/schema/list", s.handleSchemaList)
	mux.HandleFunc("/v1/validate", s.handleValidate)
	mux.HandleFunc("/v1/collection-config", s.handleCollectionConfig)
	mux.HandleFunc("/v1/collection-configs", s.handleCollectionConfigList)
	mux.HandleFunc("/v1/curation", s.handleCuration)
	mux.HandleFunc("/v1/events", s.handleSSE)
	// Memory RAG endpoints
	mux.HandleFunc("/v1/memory/session", s.guardWrite(s.handleMemorySessionCreate))
	mux.HandleFunc("/v1/memory/message", s.guardWrite(s.handleMemoryMessageAdd))
	mux.HandleFunc("/v1/memory/recall", s.handleMemoryRecall)
	mux.HandleFunc("/v1/memory/summarize", s.guardWrite(s.handleMemorySummarize))
	mux.HandleFunc("/v1/memory/sessions", s.handleMemorySessionsList)
	mux.HandleFunc("/v1/memory/history", s.handleMemoryHistory)

	mux.HandleFunc("/v1/cross-search", s.handleCrossSearch)
	mux.HandleFunc("/v1/find-duplicates", s.handleFindDuplicates)
	mux.HandleFunc("/v1/aggregate", s.handleAggregate)
	// Temporal event tracking
	if s.TemporalManager != nil {
		mux.HandleFunc("/v1/temporal/query", s.handleTemporalQuery)
		mux.HandleFunc("/v1/temporal/hot", s.handleTemporalHot)
		mux.HandleFunc("/v1/temporal/histogram", s.handleTemporalHistogram)
	}
	// Spell correction
	if s.SpellManager != nil {
		mux.HandleFunc("/v1/spell-suggest", s.handleSpellSuggest)
		mux.HandleFunc("/v1/spell-cleanup", s.handleSpellCleanup)
		mux.HandleFunc("/v1/spell-dictionary", s.handleSpellDictionary)
	}
	mux.HandleFunc("/metrics", s.Metrics.HandleMetrics)

	// pprof profiling endpoints (disabled by default, set MDDB_PPROF_ENABLED=true)
	if env("MDDB_PPROF_ENABLED", "false") == "true" {
		registerPprof(mux)
		log.Println("pprof profiling endpoints enabled at /debug/pprof/")
	}

	// Replication status endpoint
	mux.HandleFunc("/v1/replication/status", s.handleReplicationStatus)

	// System info and config endpoints
	mux.HandleFunc("/v1/system/info", s.handleSystemInfo)
	mux.HandleFunc("/v1/config", s.handleConfig)
	mux.HandleFunc("/v1/mcp/config", s.handleMCPConfig)
	mux.HandleFunc("/v1/mcp/keys", s.guardWrite(s.handleMCPAPIKeys))
	mux.HandleFunc("/v1/mcp/keys/disable", s.guardWrite(s.handleMCPAPIKeyDisable))
	mux.HandleFunc("/v1/endpoints", s.handleEndpoints)

	// Auth endpoints (if enabled)
	if authEnabled {
		mux.HandleFunc("/v1/auth/login", s.handleAuthLogin)
		mux.HandleFunc("/v1/auth/register", s.handleAuthRegister)
		mux.HandleFunc("/v1/auth/api-key", s.handleAuthAPIKey)
		mux.HandleFunc("/v1/auth/api-keys/", s.handleAuthAPIKeyDelete) // Note: trailing slash for DELETE /v1/auth/api-keys/:keyHash
		mux.HandleFunc("/v1/auth/api-keys", s.handleAuthAPIKeysList)
		mux.HandleFunc("/v1/auth/me", s.handleAuthMe)
		mux.HandleFunc("/v1/auth/permissions", s.handleAuthPermissions)
		mux.HandleFunc("/v1/auth/users/", s.handleAuthDeleteUser) // Note: trailing slash for DELETE /v1/auth/users/:username
		mux.HandleFunc("/v1/auth/users", s.handleAuthUsersList)
		mux.HandleFunc("/v1/auth/groups", s.handleAuthGroups)
		mux.HandleFunc("/v1/auth/groups/", s.handleAuthGroupDetail)
		mux.HandleFunc("/v1/auth/group-permissions", s.handleAuthGroupPermissions)
	}

	// GraphQL endpoint (disabled by default)
	graphqlEnabled := env("MDDB_GRAPHQL_ENABLED", "true") != "false"
	if graphqlEnabled {
		graphqlHandler := s.newGraphQLHandler()

		// Wrap with auth middleware if enabled
		if authEnabled && s.AuthManager != nil {
			graphqlHandler = s.GraphQLAuthMiddleware(graphqlHandler)
		}

		mux.Handle("/graphql", graphqlHandler)
		log.Printf("GraphQL endpoint enabled at /graphql")

		// GraphQL Playground (development tool)
		if env("MDDB_GRAPHQL_PLAYGROUND", "true") == "true" {
			mux.Handle("/playground", newGraphQLPlaygroundHandler("/graphql"))
			log.Printf("GraphQL Playground enabled at /playground")
		}
	}

	httpAddr := srvCfg.HTTP.Addr
	grpcAddr := srvCfg.GRPC.Addr

	// Panel mode: internal (default) = CORS enabled, external = CORS disabled (panel proxies)
	panelMode := env("MDDB_PANEL_MODE", "internal")

	// Wrap mux: CORS → panic-recover → rate limit → auth → metrics → max-body → JSON → routes
	handler := withJSON(mux)
	// SEC-005: cap request body size globally so a single oversized JSON body
	// can't OOM the process. Upload/import endpoints legitimately stream large
	// files and manage their own limits, so they are exempt.
	handler = withMaxBody(
		int64(envconf.Int("MDDB_MAX_BODY_BYTES", 32<<20)), // 32MB default
		map[string]bool{"/v1/upload": true, "/v1/import-wiki": true},
		handler,
	)
	handler = s.Metrics.Middleware(handler)
	if authEnabled && s.AuthManager != nil {
		handler = s.AuthManager.HTTPMiddleware(handler)
	}
	if s.RateLimiter.Enabled() {
		handler = s.RateLimiter.HTTPMiddleware(handler)
	}
	// Panic recovery is the outermost concern so every other
	// middleware is shielded — a crash in auth/rate-limit/etc.
	// becomes an ops.panic_recovered event, not a process kill.
	handler = PanicRecoveryMiddleware(s.WebhookManager, handler)
	if panelMode != "external" {
		handler = withCORS(handler)
		// SEC-008: a wildcard CORS policy lets any website read responses from a
		// user's browser. Warn so operators set MDDB_CORS_ORIGINS for non-public
		// deployments.
		if envCORSConfig().wildcard {
			if s.AuthManager != nil && s.AuthManager.enabled {
				log.Printf("⚠️  SECURITY (SEC-008): CORS is wildcard (*) with auth enabled — " +
					"any origin can attempt credentialed cross-origin reads. Set MDDB_CORS_ORIGINS to an allowlist.")
			} else {
				log.Printf("⚠️  SECURITY (SEC-008): CORS is wildcard (*) — any website can read this instance " +
					"from a user's browser. Set MDDB_CORS_ORIGINS to an allowlist for non-public deployments.")
			}
		}
	}
	if panelMode == "external" {
		log.Printf("Panel mode: external (CORS disabled, panel proxies requests)")
	}

	// Shut down early health server before starting the main one
	if earlyServer != nil {
		_ = earlyServer.Close()
		time.Sleep(50 * time.Millisecond) // brief pause to release port
	}

	// Start async bulk ingest manager — recovers orphan jobs from previous run
	// and spins up the single worker that drains the in-memory queue.
	s.BulkIngest = NewBulkIngestManager(s, 64)
	s.BulkIngest.Start()

	// Mark server as ready — health check will now return "healthy" instead of "warming_up"
	s.Ready = true
	log.Println("Server initialization complete — ready to serve")

	// Start HTTP server (with optional TLS). httpAddr may be a TCP host:port
	// or a Unix Domain Socket (unix:/path/to/sock) — see listen_addr.go.
	if srvCfg.HTTP.Enabled {
		go func() {
			server := &http.Server{
				Addr:              httpAddr,
				Handler:           handler,
				ReadHeaderTimeout: 10 * time.Second,
				ReadTimeout:       30 * time.Second,
				WriteTimeout:      30 * time.Second,
				IdleTimeout:       120 * time.Second,
			}
			lis, err := openListener(httpAddr)
			if err != nil {
				log.Fatal(err)
			}
			defer func() { _ = closeListener(lis, httpAddr) }()
			tlsOn := srvCfg.TLS.Enabled && srvCfg.TLS.CertFile != "" && srvCfg.TLS.KeyFile != ""
			if tlsOn && isUnixAddr(httpAddr) {
				log.Printf("mddb: TLS ignored on UDS listener %s (filesystem perms authenticate the peer)", httpAddr)
				tlsOn = false
			}
			if tlsOn {
				tlsCfg, terr := buildServerTLSConfig(srvCfg.TLS)
				if terr != nil {
					log.Fatalf("TLS config: %v", terr)
				}
				server.TLSConfig = tlsCfg
				mtls := ""
				if srvCfg.TLS.ClientCAFile != "" {
					mode := srvCfg.TLS.ClientAuth
					if mode == "" {
						mode = "require"
					}
					mtls = fmt.Sprintf(", mtls=on (clientAuth=%s)", mode)
				}
				log.Printf("mddb HTTPS listening on %s (mode=%s, db=%s, tls=on%s)", httpAddr, s.Mode, dbPath, mtls)
				if err := server.ServeTLS(lis, "", ""); err != nil && err != http.ErrServerClosed {
					log.Fatal(err)
				}
			} else {
				log.Printf("mddb HTTP listening on %s (mode=%s, db=%s)", httpAddr, s.Mode, dbPath)
				if err := server.Serve(lis); err != nil && err != http.ErrServerClosed {
					log.Fatal(err)
				}
			}
		}()
	} else {
		log.Println("HTTP server disabled")
	}

	// Start MCP HTTP server on its own port
	if srvCfg.MCP.Enabled {
		go func() {
			mcpMux := http.NewServeMux()
			mcpSrv := s.newMCPHTTPServer()
			mcpMux.HandleFunc("/resources", mcpSrv.handleResources)
			mcpMux.HandleFunc("/resources/read", mcpSrv.handleResourceRead)
			mcpMux.HandleFunc("/tools", mcpSrv.handleTools)
			mcpMux.HandleFunc("/tools/call", s.guardWrite(mcpSrv.handleToolCall))
			mcpMux.HandleFunc("/events", s.handleSSE)

			// MCP-over-SSE transport (legacy, 2024-11-05 spec)
			mcpSSEHandler := NewMCPHandlerWithConfig(NewDirectClient(s), loadMCPCustomTools(), srvCfg.MCP.ServerInfo, srvCfg.MCP.Instructions, s.Mode, srvCfg.MCP.Mode)
			mcpSSE := NewMCPSSETransport(mcpSSEHandler)
			mcpMux.HandleFunc("/sse", mcpSSE.HandleSSE)
			mcpMux.HandleFunc("/message", mcpSSE.HandleMessage)
			log.Println("MCP-over-SSE transport enabled at /sse + /message (legacy)")

			// Streamable HTTP transport (2025-11-25 spec)
			mcpStreamableHandler := NewMCPHandlerWithConfig(NewDirectClient(s), loadMCPCustomTools(), srvCfg.MCP.ServerInfo, srvCfg.MCP.Instructions, s.Mode, srvCfg.MCP.Mode)
			mcpStreamable := NewMCPStreamableTransport(mcpStreamableHandler)
			mcpMux.HandleFunc("/mcp", mcpStreamable.Handle)
			log.Println("MCP Streamable HTTP transport enabled at /mcp")

			// MCP middleware chain: CORS → API Key Auth → Rate Limit → Request Logging → JSON → Routes
			var mcpHandler http.Handler = mcpMux
			mcpHandler = withJSON(mcpHandler)

			mcpLogger := NewMCPRequestLogger()
			mcpHandler = mcpLogger.Wrap(mcpHandler)

			mcpRateLimiter := NewMCPRateLimiter()
			mcpHandler = mcpRateLimiter.Wrap(mcpHandler)

			mcpAuth := NewMCPAPIKeyMiddleware()
			mcpAuth.SetKeyStore(s.mcpKeyStore)
			s.mcpAuth = mcpAuth
			mcpHandler = mcpAuth.Wrap(mcpHandler)

			// SEC-002: tie MCP exposure to the main auth config. When
			// MDDB_AUTH_ENABLED=true and MCP has no key auth of its own, gate
			// the listener with the main AuthManager so it can't be an
			// anonymous full-R/W bypass; warn loudly if MCP is unauthenticated
			// and bound beyond loopback.
			mcpHandler = s.applyMCPAuth(mcpHandler, srvCfg.MCP.Addr)

			if panelMode != "external" {
				mcpHandler = withCORS(mcpHandler)
			}

			log.Printf("mddb MCP HTTP listening on %s", srvCfg.MCP.Addr)
			server := &http.Server{
				Addr:              srvCfg.MCP.Addr,
				Handler:           mcpHandler,
				ReadHeaderTimeout: 10 * time.Second,
				ReadTimeout:       30 * time.Second,
				WriteTimeout:      30 * time.Second,
				IdleTimeout:       120 * time.Second,
			}
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatal(err)
			}
		}()
	} else {
		log.Println("MCP server disabled")
	}

	// Start HTTP/3 server
	if srvCfg.HTTP3.Enabled {
		go func() {
			log.Printf("mddb HTTP/3 listening on %s", srvCfg.HTTP3.Addr)
			h3Server, err := NewHTTP3Server(srvCfg.HTTP3.Addr, HTTP3Middleware(handler))
			if err != nil {
				log.Printf("WARNING: Failed to start HTTP/3 server: %v", err)
				return
			}
			if err := h3Server.Start(); err != nil {
				log.Printf("WARNING: HTTP/3 server error: %v", err)
			}
		}()
	}

	// Start replication client (follower mode)
	var replClient *ReplicationClient
	if s.ReplicationRole == "follower" {
		leaderAddr := env("MDDB_REPLICATION_LEADER_ADDR", "")
		if leaderAddr == "" {
			log.Fatal("MDDB_REPLICATION_ROLE=follower requires MDDB_REPLICATION_LEADER_ADDR")
		}
		replClient = NewReplicationClient(s, ReplicationClientConfig{
			LeaderAddr: leaderAddr,
			FollowerID: s.NodeID,
		})
		s.replClient = replClient
		replClient.Start()
		defer replClient.Stop()
		log.Printf("Replication client started (leader=%s, follower=%s)", leaderAddr, s.NodeID)

		// Monitor replication lag and fire ops.replication_lag_high
		// when it crosses the threshold.
		s.LagMonitor = NewReplicationLagMonitor(s.WebhookManager, replClient)
		s.LagMonitor.Start()
		defer s.LagMonitor.Stop()
	}

	if s.ReplicationRole == "leader" {
		log.Printf("Replication leader started (node=%s, binlog LSN=%d)", s.NodeID, s.Binlog.CurrentLSN())
	}

	// Close binlog on shutdown
	if s.Binlog != nil {
		defer func() { _ = s.Binlog.Close() }()
	}

	// Stop automation log cleanup on shutdown
	if s.AutomationLogStore != nil {
		defer s.AutomationLogStore.Stop()
	}

	// Flush pending audit events on shutdown
	if s.AuditManager != nil {
		defer s.AuditManager.Stop()
	}

	// Stop the disk-usage monitor; LagMonitor is stopped adjacent to
	// the replication client in the follower branch above.
	if s.DiskMonitor != nil {
		defer s.DiskMonitor.Stop()
	}

	// Start gRPC server
	if srvCfg.GRPC.Enabled {
		var grpcOpts []grpc.ServerOption
		var unaryChain []grpc.UnaryServerInterceptor
		var streamChain []grpc.StreamServerInterceptor
		if s.RateLimiter.Enabled() {
			unaryChain = append(unaryChain, s.RateLimiter.UnaryInterceptor())
			streamChain = append(streamChain, s.RateLimiter.StreamInterceptor())
		}
		if authEnabled && s.AuthManager != nil {
			unaryChain = append(unaryChain, s.AuthManager.GRPCUnaryInterceptor())
			// SEC-003: streaming RPCs (Export, replication) must be
			// authenticated too — and claims injected so stream handlers'
			// CheckPermission sees them.
			streamChain = append(streamChain, s.AuthManager.GRPCStreamInterceptor())
		}
		if len(unaryChain) > 0 {
			grpcOpts = append(grpcOpts, grpc.ChainUnaryInterceptor(unaryChain...))
		}
		if len(streamChain) > 0 {
			grpcOpts = append(grpcOpts, grpc.ChainStreamInterceptor(streamChain...))
		}
		// TLS / mTLS — same buildServerTLSConfig as the HTTP listener.
		// Skipped on UDS (filesystem perms authenticate the local peer).
		grpcTLSLog := ""
		if !isUnixAddr(grpcAddr) {
			tlsCfg, terr := buildServerTLSConfig(srvCfg.TLS)
			if terr != nil {
				log.Fatalf("gRPC TLS config: %v", terr)
			}
			if tlsCfg != nil {
				grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsCfg)))
				grpcTLSLog = ", tls=on"
				if srvCfg.TLS.ClientCAFile != "" {
					mode := srvCfg.TLS.ClientAuth
					if mode == "" {
						mode = "require"
					}
					grpcTLSLog = fmt.Sprintf(", tls=on, mtls=on (clientAuth=%s)", mode)
				}
			}
		}
		go func() {
			log.Printf("mddb gRPC listening on %s (mode=%s, db=%s%s)", grpcAddr, s.Mode, dbPath, grpcTLSLog)
			if err := startGRPCServer(s, grpcAddr, grpcOpts...); err != nil {
				log.Fatal(err)
			}
		}()
	} else {
		log.Println("gRPC server disabled")
	}

	// Block until signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received %s, shutting down...", sig)

	if s.LockFreeCache != nil {
		s.LockFreeCache.Close()
	}
	// Stop the cache.DocumentCache cleanup goroutine (GO-006).
	if s.Cache != nil {
		s.Cache.Close()
	}
	// Stop the adaptive-index optimization worker goroutine (GO-007).
	if s.AdaptiveIndex != nil {
		s.AdaptiveIndex.Close()
	}
}

// --- helpers / buckets

func (s *Server) ensureBuckets() error {
	return s.DBUpdate(func(tx *bolt.Tx) error {
		_, _ = tx.CreateBucketIfNotExists(s.BucketNames.Docs)          // doc|collection|id -> json
		_, _ = tx.CreateBucketIfNotExists(s.BucketNames.IdxMeta)       // meta|collection|key|value|docID -> 1
		_, _ = tx.CreateBucketIfNotExists(s.BucketNames.Rev)           // rev|collection|docID|ts -> json
		_, _ = tx.CreateBucketIfNotExists(s.BucketNames.ByKey)         // bykey|collection|key|lang -> docID
		_, _ = tx.CreateBucketIfNotExists([]byte("embedding_configs")) // embedding model configurations
		_, _ = tx.CreateBucketIfNotExists(bucketBulkJobs)              // bulk ingest job tracking
		return nil
	})
}

func kDoc(coll, id string) []byte          { return []byte("doc|" + coll + "|" + id) }
func kByKey(coll, key, lang string) []byte { return []byte("bykey|" + coll + "|" + key + "|" + lang) }
func kRevPrefix(coll, id string) []byte    { return []byte("rev|" + coll + "|" + id + "|") }
func kMetaKeyPrefix(coll, mk, mv string) []byte {
	return []byte("meta|" + coll + "|" + mk + "|" + mv + "|")
}

// --- middleware

func withCORS(h http.Handler) http.Handler {
	// SEC-008: resolve the origin policy once. Prefer the MDDB_CORS_ORIGINS
	// allowlist over a wildcard so a hostile site can't read responses from a
	// user's local/intranet MDDB instance through their browser.
	cfg := envCORSConfig()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.applyOrigin(w, r.Header.Get("Origin"))
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		w.Header().Set("Access-Control-Expose-Headers", "X-Total-Count")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func withJSON(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		h.ServeHTTP(w, r)
	})
}

// withMaxBody caps the request body size (SEC-005). A declared Content-Length
// over the limit is rejected immediately with 413; otherwise the body is
// wrapped in http.MaxBytesReader so reads can never allocate more than `limit`
// even when Content-Length is absent or lies. Paths in `exempt` (large
// file uploads / wiki imports that stream from disk and enforce their own
// caps) are left untouched. Configurable via MDDB_MAX_BODY_BYTES.
func withMaxBody(limit int64, exempt map[string]bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if limit > 0 && !exempt[r.URL.Path] {
			if r.ContentLength > limit {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				_, _ = w.Write([]byte(`{"error":"request body too large"}`))
				return
			}
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) guardWrite(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mode := effectiveMode(s.Mode, s.Config.HTTP.Mode)
		if mode == ModeRead {
			http.Error(w, `{"error":"read-only mode"}`, http.StatusForbidden)
			return
		}
		if s.AuditManager == nil || !s.AuditManager.Enabled() {
			next(w, r)
			return
		}
		aw := &auditResponseWriter{ResponseWriter: w, status: 200}
		next(aw, r)
		result := "ok"
		if aw.status >= 400 {
			result = "fail"
		}
		actor := ""
		if claims, ok := r.Context().Value(authContextKey).(*JWTClaims); ok && claims != nil {
			actor = claims.Username
		}
		s.AuditManager.Record(audit.AuditEvent{
			Actor:     actor,
			Action:    "write." + r.URL.Path,
			Resource:  r.URL.Path,
			Result:    result,
			IP:        ClientIP(r),
			UserAgent: r.UserAgent(),
			Detail:    fmt.Sprintf("status=%d", aw.status),
		})
	}
}

// auditResponseWriter captures the first status code written so the
// guardWrite wrapper can classify the outcome as ok/fail.
type auditResponseWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func (a *auditResponseWriter) WriteHeader(code int) {
	if !a.written {
		a.status = code
		a.written = true
	}
	a.ResponseWriter.WriteHeader(code)
}

func (a *auditResponseWriter) Write(b []byte) (int, error) {
	if !a.written {
		a.written = true
	}
	return a.ResponseWriter.Write(b)
}

// effectiveMode returns the per-protocol mode if set, otherwise falls back to the global mode.
func effectiveMode(global, perProtocol AccessMode) AccessMode {
	if perProtocol != "" {
		return perProtocol
	}
	return global
}

// --- handlers

// addDocument is the shared internal method for adding/updating a document.
// Returns the saved document, whether it was newly created, and any error.
// addDocument is the single write path for one document, shared by every
// transport (HTTP, gRPC, MCP, GraphQL). It performs the in-transaction insert,
// metadata index, and — when saveRevision is true — the revision write plus
// MaxRevisions trimming, then runs the shared post-write side-effect pipeline.
//
// saveRevision lets the transport opt out of revision history (gRPC exposes a
// per-request SaveRevision flag); all other callers pass true to preserve the
// always-record-a-revision behaviour they have always had.
func (s *Server) addDocument(collection, key, lang string, meta map[string][]string, contentMD string, ttl int64, saveRevision bool) (Doc, bool, error) {
	// GO-003: validate in the single write path so EVERY transport is covered.
	// Previously only gRPC Add and HTTP handleAdd validated; MCP (DirectClient)
	// and GraphQL went straight to addDocument with no checks, and the batch
	// path skipped schema validation entirely. Schema validation is opt-in
	// (no-op unless a schema is registered for the collection), so this is safe
	// for internal callers (memory/upload/import) too.
	if collection == "" || key == "" || lang == "" {
		return Doc{}, false, errors.New("missing required field: collection, key and lang are required")
	}
	if s.SchemaManager != nil {
		if err := s.SchemaManager.Validate(collection, meta); err != nil {
			return Doc{}, false, err
		}
	}

	now := time.Now().Unix()
	docID := genID(collection, key, lang)

	var saved Doc
	var isNew bool
	var bo binlog.BinlogOps
	var cachedBuf []byte // marshaled doc, reused to refresh the read cache (GO-002)
	err := s.DBUpdate(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		bIdx := tx.Bucket([]byte("idxmeta"))
		bRev := tx.Bucket([]byte("rev"))
		bByK := tx.Bucket([]byte("bykey"))

		existing := Doc{}
		if v := bDocs.Get(kDoc(collection, docID)); v != nil {
			existingPtr, err := loadDoc(v)
			if err != nil {
				return err
			}
			existing = *existingPtr
		}
		added := existing.AddedAt
		if added == 0 {
			added = now
			isNew = true
		}

		doc := Doc{
			ID: docID, Key: key, Lang: lang, Meta: meta,
			ContentMD: contentMD, AddedAt: added, UpdatedAt: now,
		}
		if ttl > 0 {
			doc.ExpiresAt = now + ttl
		}

		buf, err := marshalAndEncrypt(&doc, collection)
		if err != nil {
			return err
		}
		cachedBuf = buf
		docKey := kDoc(collection, docID)
		if err := bDocs.Put(docKey, buf); err != nil {
			return err
		}
		bo.Put("docs", docKey, buf)

		byKeyK := kByKey(collection, key, lang)
		if err := bByK.Put(byKeyK, []byte(docID)); err != nil {
			return err
		}
		bo.Put("bykey", byKeyK, []byte(docID))

		if metadataChanged(existing.Meta, doc.Meta) {
			if existing.ID != "" && existing.Meta != nil {
				for mk, vals := range existing.Meta {
					for _, mv := range vals {
						prefix := append(kMetaKeyPrefix(collection, mk, mv), []byte(existing.ID)...)
						_ = bIdx.Delete(prefix)
						bo.Delete("idxmeta", prefix)
					}
				}
			}
			for mk, vals := range doc.Meta {
				for _, mv := range vals {
					mkey := append(kMetaKeyPrefix(collection, mk, mv), []byte(doc.ID)...)
					if err := bIdx.Put(mkey, []byte("1")); err != nil {
						return err
					}
					bo.Put("idxmeta", mkey, []byte("1"))
				}
			}
		}

		if saveRevision {
			rkey := append(kRevPrefix(collection, doc.ID), []byte(fmt.Sprintf("%020d", now))...)
			if err := bRev.Put(rkey, buf); err != nil {
				return err
			}
			bo.Put("rev", rkey, buf)

			// Enforce per-collection MaxRevisions: drop oldest revs over the cap so
			// history never grows unbounded on high-churn collections.
			if s.CollectionManager != nil {
				if cfg, found := s.CollectionManager.Get(collection); found && cfg.MaxRevisions > 0 {
					if err := trimRevisions(tx, &bo, collection, doc.ID, cfg.MaxRevisions); err != nil {
						return err
					}
				}
			}
		}

		saved = doc
		return nil
	})
	if err == nil {
		bo.FlushTo(s.Binlog)
	}
	if err != nil {
		return Doc{}, false, err
	}

	// Refresh the read cache so a subsequent gRPC Get (the only path that
	// consults it) sees the new value instead of a stale entry for up to the
	// 5-minute TTL (GO-002). Keyed identically to gRPC Add/Get via
	// cache.BuildCacheKey, so every transport stays coherent.
	if cachedBuf != nil {
		cacheKey := cache.BuildCacheKey(collection, key, lang)
		if s.UseExtreme && s.LockFreeCache != nil {
			s.LockFreeCache.Set(cacheKey, cachedBuf)
		} else if s.Cache != nil {
			s.Cache.Set(cacheKey, cachedBuf)
		}
	}

	// Side-effect pipeline shared by every write transport (GO-001).
	s.runPostWriteHooks(collection, saved, isNew)

	return saved, isNew, nil
}

// runPostWriteHooks runs the side-effect pipeline that must fire after a
// document write commits to BoltDB, regardless of the transport that produced
// it — HTTP, gRPC (single Add or AddBatch), MCP, or GraphQL. Centralising it
// here is what guarantees identical behaviour across transports (GO-001):
// async embedding, TTL bucket registration, FTS (content + positional +
// field/BM25F), geo (R-tree + geohash + GeoStore), temporal tracking,
// webhooks, SSE broadcast, and automation triggers. Every dependency is
// nil-guarded so partially-configured servers (and tests) are safe.
//
// Revision writing and MaxRevisions trimming are intentionally NOT here: they
// must happen inside the write transaction (see addDocument / the batch
// commits) so a crash can never leave a doc without its revision.
func (s *Server) runPostWriteHooks(collection string, saved Doc, isNew bool) {
	// Trigger async embedding
	if s.EmbeddingWorker != nil && saved.ContentMD != "" {
		s.EmbeddingWorker.Enqueue(EmbeddingJob{
			Collection: collection,
			DocID:      saved.ID,
			ContentMD:  saved.ContentMD,
		})
	}

	// TTL bucket entry
	if s.TTLManager != nil && saved.ExpiresAt > 0 {
		_ = s.TTLManager.Set(collection, saved.ID, saved.ExpiresAt)
	}

	// FTS indexing (language-aware)
	if s.FTSIndex != nil && saved.ContentMD != "" {
		_ = s.FTSIndex.IndexWithLang(collection, saved.ID, saved.ContentMD, saved.Lang)
		// Positional index for phrase/proximity search
		_ = s.FTSIndex.IndexPositionsWithLang(collection, saved.ID, saved.ContentMD, saved.Lang)
		// Field-level indexing for BM25F
		fields := map[string]string{"content": saved.ContentMD}
		for k, vals := range saved.Meta {
			if len(vals) > 0 {
				fields["meta."+k] = strings.Join(vals, " ")
			}
		}
		_ = s.FTSIndex.IndexFieldsWithLang(collection, saved.ID, fields, saved.Lang)
	}

	// Geo indexing: persist resolved lat/lng to BoltDB and mirror into
	// BOTH in-memory indexes (R-tree + geohash). Coordinates are extracted
	// from meta via AddFromMeta, which tries explicit geo_lat/geo_lng,
	// then geo_hash, then the optional postcode lookup — silent no-op if
	// the doc has none of those.
	if s.GeoIndex != nil && s.GeoStore != nil {
		if lat, lng, ok := s.GeoIndex.AddFromMeta(collection, saved.ID, saved.Meta); ok {
			_ = s.GeoStore.Put(collection, saved.ID, lat, lng)
			if s.GeoHashIndex != nil {
				s.GeoHashIndex.Add(collection, saved.ID, lat, lng)
			}
		} else if !isNew {
			// Update may have dropped the geo fields — remove any stale entry.
			s.GeoIndex.Remove(collection, saved.ID)
			if s.GeoHashIndex != nil {
				s.GeoHashIndex.Remove(collection, saved.ID)
			}
			_ = s.GeoStore.Delete(collection, saved.ID)
		}
	}

	// Temporal tracking
	if s.TemporalManager != nil {
		et := temporal.EventUpdate
		if isNew {
			et = temporal.EventCreate
		}
		s.TemporalManager.RecordAsync(collection, saved.ID, et, "")
	}

	// Webhooks + SSE
	event := "doc.updated"
	if isNew {
		event = "doc.added"
	}
	if s.WebhookManager != nil {
		s.WebhookManager.Fire(event, collection, saved.Key, saved.Lang, &saved)
	}
	if s.SSEHub != nil {
		s.SSEHub.BroadcastWithAuth(event, collection, saved.Key, saved.Lang, s.AuthManager)
	}

	// Automation triggers
	if s.AutomationManager != nil && env("MDDB_TRIGGERS", "false") == "true" {
		triggerEvent := "insert"
		if !isNew {
			triggerEvent = "update"
		}
		go s.AutomationManager.EvaluateTriggers(collection, saved, triggerEvent)
	}
}

// deleteDocumentInternal deletes a document and all its associated data.
func (s *Server) deleteDocumentInternal(collection, key, lang string) error {
	docID := genID(collection, key, lang)

	var bo binlog.BinlogOps
	var deletedDoc Doc // captured for trigger evaluation
	err := s.DBUpdate(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		bIdx := tx.Bucket([]byte("idxmeta"))
		bRev := tx.Bucket([]byte("rev"))
		bByK := tx.Bucket([]byte("bykey"))

		v := bDocs.Get(kDoc(collection, docID))
		if v == nil {
			return errors.New("document not found")
		}
		docPtr, err := loadDoc(v)
		if err != nil {
			return err
		}
		doc := *docPtr
		deletedDoc = doc

		docKey := kDoc(collection, docID)
		if err := bDocs.Delete(docKey); err != nil {
			return err
		}
		bo.Delete("docs", docKey)

		byKeyK := kByKey(collection, key, lang)
		if err := bByK.Delete(byKeyK); err != nil {
			return err
		}
		bo.Delete("bykey", byKeyK)

		c := bRev.Cursor()
		rp := kRevPrefix(collection, docID)
		for k, _ := c.Seek(rp); k != nil && bytes.HasPrefix(k, rp); k, _ = c.Next() {
			if err := bRev.Delete(k); err != nil {
				return err
			}
			bo.Delete("rev", k)
		}

		for mk, vals := range doc.Meta {
			for _, mv := range vals {
				mkey := append(kMetaKeyPrefix(collection, mk, mv), []byte(docID)...)
				if err := bIdx.Delete(mkey); err != nil {
					return err
				}
				bo.Delete("idxmeta", mkey)
			}
		}

		return nil
	})
	if err == nil {
		bo.FlushTo(s.Binlog)
	}
	if err != nil {
		return err
	}

	// Clean up vector embedding (legacy key + all chunk keys)
	if s.VectorStore != nil {
		_ = s.VectorStore.Delete(collection, docID)
		for _, searcher := range s.VectorSearchers {
			searcher.Remove(collection, docID)
			// Also remove chunk keys
			for i := 0; i < 100; i++ {
				chunkKey := docID + "#" + strconv.Itoa(i)
				if searcher.CollectionSize(collection) == 0 {
					break
				}
				searcher.Remove(collection, chunkKey)
			}
		}
	}

	// Clean up TTL entry
	if s.TTLManager != nil {
		_ = s.TTLManager.Remove(collection, docID)
	}

	// Automation triggers (before FTS cleanup so doc is still searchable)
	if s.AutomationManager != nil && env("MDDB_TRIGGERS", "false") == "true" {
		go s.AutomationManager.EvaluateTriggers(collection, deletedDoc, "delete")
	}

	// Invalidate the read cache so a gRPC Get can't serve the just-deleted doc
	// for up to the 5-minute TTL (GO-002). Same cache.BuildCacheKey as the write path.
	cacheKey := cache.BuildCacheKey(collection, key, lang)
	if s.Cache != nil {
		s.Cache.Delete(cacheKey)
	}
	if s.LockFreeCache != nil {
		s.LockFreeCache.Delete(cacheKey)
	}

	// Clean up FTS index
	if s.FTSIndex != nil {
		_ = s.FTSIndex.Remove(collection, docID)
	}

	// Clean up both geo indexes (no-op if this doc had no point).
	if s.GeoIndex != nil {
		s.GeoIndex.Remove(collection, docID)
	}
	if s.GeoHashIndex != nil {
		s.GeoHashIndex.Remove(collection, docID)
	}
	if s.GeoStore != nil {
		_ = s.GeoStore.Delete(collection, docID)
	}

	// Fire webhook + SSE
	if s.WebhookManager != nil {
		s.WebhookManager.Fire("doc.deleted", collection, key, lang, nil)
	}
	if s.SSEHub != nil {
		s.SSEHub.BroadcastWithAuth("doc.deleted", collection, key, lang, s.AuthManager)
	}

	return nil
}

func (s *Server) handleAdd(w http.ResponseWriter, r *http.Request) {
	var req AddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		bad(w, errors.New("missing fields"))
		return
	}

	// Check write permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermWrite); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	if err := s.SchemaManager.Validate(req.Collection, req.Meta); err != nil {
		bad(w, err)
		return
	}

	saved, _, err := s.addDocument(req.Collection, req.Key, req.Lang, req.Meta, req.ContentMD, req.TTL, true)
	if err != nil {
		bad(w, err)
		return
	}
	ok(w, saved)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	var req GetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		bad(w, errors.New("missing fields"))
		return
	}

	// Check read permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermRead); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	var doc Doc
	err := s.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		bByK := tx.Bucket([]byte("bykey"))
		docID := bByK.Get(kByKey(req.Collection, req.Key, req.Lang))
		if docID == nil {
			return errors.New("not found")
		}
		v := bDocs.Get(kDoc(req.Collection, string(docID)))
		if v == nil {
			return errors.New("not found")
		}
		docPtr, unmErr := loadDoc(v)
		if unmErr != nil {
			return unmErr
		}
		doc = *docPtr
		return nil
	})
	if err != nil {
		bad(w, err)
		return
	}

	// Check TTL expiry
	if doc.ExpiresAt > 0 && doc.ExpiresAt < time.Now().Unix() {
		bad(w, errors.New("not found"))
		return
	}

	// Temporal access tracking (gated on collection config)
	if s.TemporalManager != nil && s.CollectionManager != nil {
		if cfg, cfgOk := s.CollectionManager.Get(req.Collection); cfgOk && cfg.TrackAccess {
			actor := ""
			if claims, ok := GetClaimsFromContext(r.Context()); ok {
				actor = claims.Username
			}
			s.TemporalManager.RecordAsync(req.Collection, doc.ID, temporal.EventAccess, actor)
		}
	}

	// Templating via ENV: replace %%var%%
	if len(req.Env) > 0 && doc.ContentMD != "" {
		doc.ContentMD = applyEnv(doc.ContentMD, req.Env)
	}
	ok(w, doc)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Limit <= 0 {
		req.Limit = 50
	}

	// Check read permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermRead); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	type row struct{ Doc Doc }
	var rows []row

	err := s.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		bIdx := tx.Bucket([]byte("idxmeta"))
		seen := make(map[string]bool)

		// Jeśli brak filtra meta → pełny scan kolekcji (dla prostoty; można dodać bucket per collection)
		if len(req.FilterMeta) == 0 {
			c := bDocs.Cursor()
			prefix := []byte("doc|" + req.Collection + "|")
			for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
				d, err := loadDoc(v)
				if err != nil {
					return err
				}
				if d.ExpiresAt > 0 && d.ExpiresAt < time.Now().Unix() {
					continue
				}
				rows = append(rows, row{*d})
			}
		} else {
			// Intersect po meta kluczach
			var sets [][]string
			for mk, mvals := range req.FilterMeta {
				var ids []string
				for _, mv := range mvals {
					prefix := kMetaKeyPrefix(req.Collection, mk, mv)
					c := bIdx.Cursor()
					for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
						id := string(k[len(prefix):])
						ids = append(ids, id)
					}
				}
				ids = sliceutil.Unique(ids)
				sets = append(sets, ids)
			}
			ids := intersect(sets...)
			for _, id := range ids {
				if seen[id] {
					continue
				}
				seen[id] = true
				v := tx.Bucket([]byte("docs")).Get(kDoc(req.Collection, id))
				if v == nil {
					continue
				}
				d, err := loadDoc(v)
				if err != nil {
					return err
				}
				if d.ExpiresAt > 0 && d.ExpiresAt < time.Now().Unix() {
					continue
				}
				rows = append(rows, row{*d})
			}
		}
		return nil
	})
	if err != nil {
		bad(w, err)
		return
	}

	// sort
	switch req.Sort {
	case "addedAt":
		sort.Slice(rows, func(i, j int) bool {
			if req.Asc {
				return rows[i].Doc.AddedAt < rows[j].Doc.AddedAt
			}
			return rows[i].Doc.AddedAt > rows[j].Doc.AddedAt
		})
	case "updatedAt":
		sort.Slice(rows, func(i, j int) bool {
			if req.Asc {
				return rows[i].Doc.UpdatedAt < rows[j].Doc.UpdatedAt
			}
			return rows[i].Doc.UpdatedAt > rows[j].Doc.UpdatedAt
		})
	case "key":
		sort.Slice(rows, func(i, j int) bool {
			if req.Asc {
				return rows[i].Doc.Key < rows[j].Doc.Key
			}
			return rows[i].Doc.Key > rows[j].Doc.Key
		})
	}

	// paginate
	start := req.Offset
	if start > len(rows) {
		start = len(rows)
	}
	end := start + req.Limit
	if end > len(rows) {
		end = len(rows)
	}

	out := make([]Doc, 0, end-start)
	for _, r := range rows[start:end] {
		out = append(out, r.Doc)
	}
	w.Header().Set("X-Total-Count", fmt.Sprintf("%d", len(rows)))
	ok(w, out)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	var req ExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Format == "" {
		req.Format = "ndjson"
	}

	// Check read permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermRead); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	// Reużyj /search
	sr := SearchRequest{Collection: req.Collection, FilterMeta: req.FilterMeta, Limit: 1 << 30}
	buf := new(bytes.Buffer)

	switch req.Format {
	case "ndjson":
		// stream NDJSON
		res, _ := http.Post("http://localhost"+env("MDDB_ADDR", ":11023")+"/v1/search", "application/json", bytes.NewReader(mustJSON(sr)))
		defer func() { _ = res.Body.Close() }()
		var docs []Doc
		_ = json.NewDecoder(res.Body).Decode(&docs)
		for _, d := range docs {
			b, _ := json.Marshal(d)
			buf.Write(b)
			buf.WriteByte('\n')
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write(buf.Bytes())

	case "zip":
		// pack contentMd as files {key}.{lang}.md
		res, _ := http.Post("http://localhost"+env("MDDB_ADDR", ":11023")+"/v1/search", "application/json", bytes.NewReader(mustJSON(sr)))
		defer func() { _ = res.Body.Close() }()
		var docs []Doc
		_ = json.NewDecoder(res.Body).Decode(&docs)
		var z bytes.Buffer
		zw := zip.NewWriter(&z)
		for _, d := range docs {
			name := fmt.Sprintf("%s.%s.md", safe(d.Key), safe(d.Lang))
			f, _ := zw.Create(name)
			_, _ = io.WriteString(f, d.ContentMD)
		}
		_ = zw.Close()
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(z.Bytes())

	default:
		http.Error(w, `{"error":"unsupported format"}`, 400)
	}
}

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	// Check admin permission (database-wide operation)
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), "*", PermAdmin); err != nil {
			http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
			return
		}
	}

	// snapshot = copy pliku DB (najprościej)
	dst := r.URL.Query().Get("to")
	if dst == "" {
		dst = fmt.Sprintf("backup-%d.db", time.Now().Unix())
	}
	safeDst, err := safeBackupPath(dst, false)
	if err != nil {
		bad(w, err)
		return
	}
	if err := copyFile(s.Path, safeDst); err != nil {
		bad(w, err)
		return
	}
	ok(w, map[string]string{"backup": safeDst})
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From string `json:"from"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		bad(w, err)
		return
	}
	if body.From == "" {
		bad(w, errors.New("missing from"))
		return
	}

	// Check admin permission (database-wide operation)
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), "*", PermAdmin); err != nil {
			http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
			return
		}
	}

	safeFrom, err := safeBackupPath(body.From, true)
	if err != nil {
		bad(w, err)
		return
	}

	// zamknij db, podmień plik, otwórz ponownie
	_ = s.DB.Close()
	if err := copyFile(safeFrom, s.Path); err != nil {
		bad(w, err)
		return
	}

	db, err := bolt.Open(s.Path, 0600, getOptimizedBoltOptions())
	if err != nil {
		bad(w, err)
		return
	}
	s.DB = db

	// Reset binlog after restore — forces followers to re-snapshot
	if s.Binlog != nil {
		if err := s.Binlog.Rotate(0); err != nil {
			log.Printf("Warning: failed to reset binlog after restore: %v", err)
		}
	}

	ok(w, map[string]string{"restored": body.From})
}

func (s *Server) handleTruncate(w http.ResponseWriter, r *http.Request) {
	var req TruncateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" {
		bad(w, errors.New("missing collection"))
		return
	}

	// Check write permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermWrite); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	var bo binlog.BinlogOps
	err := s.DBUpdate(func(tx *bolt.Tx) error {
		bRev := tx.Bucket([]byte("rev"))
		bDocs := tx.Bucket([]byte("docs"))

		// Dla każdego dokumentu w kolekcji: utnij historię do N
		c := bDocs.Cursor()
		prefix := []byte("doc|" + req.Collection + "|")
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			dPtr, err := loadDoc(v)
			if err != nil {
				return err
			}
			d := *dPtr
			// Zbierz revety
			rc := bRev.Cursor()
			rp := kRevPrefix(req.Collection, d.ID)
			var revKeys [][]byte
			for rk, _ := rc.Seek(rp); rk != nil && bytes.HasPrefix(rk, rp); rk, _ = rc.Next() {
				cp := make([]byte, len(rk))
				copy(cp, rk)
				revKeys = append(revKeys, cp)
			}
			// jeśli trzeba ciąć
			if req.KeepRevs >= 0 && len(revKeys) > req.KeepRevs {
				// posortowane rosnąco po ts dzięki key; usuń najstarsze
				toDel := revKeys[:len(revKeys)-req.KeepRevs]
				for _, delk := range toDel {
					_ = bRev.Delete(delk)
					bo.Delete("rev", delk)
				}
			}
			// DropCache placeholder — jeśli trzymasz rendery, wyczyść je tutaj
			_ = req.DropCache
		}
		return nil
	})
	if err == nil {
		bo.FlushTo(s.Binlog)
	}
	if err != nil {
		bad(w, err)
		return
	}
	ok(w, map[string]string{"status": "truncated"})
}

// --- utils

func ok(w http.ResponseWriter, v any) {
	b, _ := json.Marshal(v)
	w.WriteHeader(200)
	_, _ = w.Write(b) // #nosec G705 -- response write to http.ResponseWriter
}
func bad(w http.ResponseWriter, err error) {
	w.WriteHeader(400)
	_, _ = fmt.Fprintf(w, `{"error":%q}`, err.Error()) // #nosec G705 -- response write to http.ResponseWriter
}

// handleHealth returns a simple health check response
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Check if server has finished initialization
	if !s.Ready {
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"status":"warming_up"}`))
		return
	}

	// Check if database is accessible
	err := s.DBView(func(tx *bolt.Tx) error {
		return nil
	})

	if err != nil {
		w.WriteHeader(503)
		_, _ = fmt.Fprintf(w, `{"status":"unhealthy","error":%q}`, err.Error())
		return
	}

	w.WriteHeader(200)
	_, _ = w.Write([]byte(`{"status":"healthy","mode":"` + string(s.Mode) + `"}`))
}

// handleComplianceStatus returns the ISO 27001 / SOC 2 production-guard
// state so operators (and the Panel) can see whether the server is
// running with the required security envelope.
func (s *Server) handleComplianceStatus(w http.ResponseWriter, r *http.Request) {
	missing := CheckProductionGuards()
	type missingEntry struct {
		EnvVar string `json:"envVar"`
		Want   string `json:"want"`
		Reason string `json:"reason"`
	}
	entries := make([]missingEntry, 0, len(missing))
	for _, m := range missing {
		entries = append(entries, missingEntry{EnvVar: m.EnvVar, Want: m.Want, Reason: m.Reason})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"production":   IsProduction(),
		"compliant":    len(missing) == 0,
		"missing":      entries,
		"missingCount": len(missing),
	})
}

func (s *Server) collectionChecksum(collection string) (string, int) {
	var checksum uint32
	var count int

	_ = s.DBView(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs == nil {
			return nil
		}
		prefix := []byte("doc|" + collection + "|")
		c := bDocs.Cursor()
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			count++
			// Hash key + first 64 bytes of value (contains updatedAt in serialized form)
			h := crc32.ChecksumIEEE(k)
			if len(v) > 64 {
				h ^= crc32.ChecksumIEEE(v[:64])
			} else {
				h ^= crc32.ChecksumIEEE(v)
			}
			checksum ^= h
		}
		return nil
	})

	return fmt.Sprintf("%08x", checksum), count
}

func (s *Server) handleChecksum(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	collection := r.URL.Query().Get("collection")
	if collection == "" {
		http.Error(w, `{"error":"collection is required"}`, http.StatusBadRequest)
		return
	}

	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), collection, PermRead); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	checksum, count := s.collectionChecksum(collection)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"collection":    collection,
		"checksum":      checksum,
		"documentCount": count,
	})
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Parse raw JSON to detect which fields are present
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		bad(w, err)
		return
	}

	// Required fields
	var collection, key, lang string
	if v, ok := raw["collection"]; ok {
		_ = json.Unmarshal(v, &collection)
	}
	if v, ok := raw["key"]; ok {
		_ = json.Unmarshal(v, &key)
	}
	if v, ok := raw["lang"]; ok {
		_ = json.Unmarshal(v, &lang)
	}

	if collection == "" || key == "" || lang == "" {
		bad(w, errors.New("missing required fields: collection, key, lang"))
		return
	}

	// Check which optional fields are present
	_, hasMeta := raw["meta"]
	_, hasContent := raw["contentMd"]
	_, hasTTL := raw["ttl"]

	if !hasMeta && !hasContent && !hasTTL {
		bad(w, errors.New("no fields to update"))
		return
	}

	// Parse optional fields
	var newMeta map[string][]string
	if hasMeta {
		_ = json.Unmarshal(raw["meta"], &newMeta)
	}
	var newContent string
	if hasContent {
		_ = json.Unmarshal(raw["contentMd"], &newContent)
	}
	var newTTL int64
	if hasTTL {
		_ = json.Unmarshal(raw["ttl"], &newTTL)
	}

	// Auth check
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), collection, PermWrite); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	// Schema validation for meta update
	if hasMeta {
		if err := s.SchemaManager.Validate(collection, newMeta); err != nil {
			bad(w, err)
			return
		}
	}

	// Load existing doc, apply partial changes, save
	now := time.Now().Unix()
	var saved Doc
	var bo binlog.BinlogOps
	var metaDidChange bool

	err := s.DBUpdate(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		bIdx := tx.Bucket([]byte("idxmeta"))
		bRev := tx.Bucket([]byte("rev"))
		bByK := tx.Bucket([]byte("bykey"))

		// Find existing doc
		docIDBytes := bByK.Get(kByKey(collection, key, lang))
		if docIDBytes == nil {
			return errors.New("not found")
		}

		v := bDocs.Get(kDoc(collection, string(docIDBytes)))
		if v == nil {
			return errors.New("not found")
		}

		existing, err := loadDoc(v)
		if err != nil {
			return err
		}

		// Check TTL expiry
		if existing.ExpiresAt > 0 && existing.ExpiresAt < now {
			return errors.New("not found")
		}

		// Apply partial updates
		doc := *existing
		doc.UpdatedAt = now

		if hasMeta {
			metaDidChange = metadataChanged(doc.Meta, newMeta)
			doc.Meta = newMeta
		}
		if hasContent {
			doc.ContentMD = newContent
		}
		if hasTTL {
			if newTTL > 0 {
				doc.ExpiresAt = now + newTTL
			} else {
				doc.ExpiresAt = 0
			}
		}

		// Persist
		buf, err := marshalAndEncrypt(&doc, collection)
		if err != nil {
			return err
		}

		docKey := kDoc(collection, doc.ID)
		if err := bDocs.Put(docKey, buf); err != nil {
			return err
		}
		bo.Put("docs", docKey, buf)

		// Reindex metadata if changed
		if metaDidChange {
			// Remove old meta index entries
			if existing.Meta != nil {
				for mk, vals := range existing.Meta {
					for _, mv := range vals {
						mkey := append(kMetaKeyPrefix(collection, mk, mv), []byte(doc.ID)...)
						_ = bIdx.Delete(mkey)
						bo.Delete("idxmeta", mkey)
					}
				}
			}
			// Add new meta index entries
			for mk, vals := range doc.Meta {
				for _, mv := range vals {
					mkey := append(kMetaKeyPrefix(collection, mk, mv), []byte(doc.ID)...)
					if err := bIdx.Put(mkey, []byte("1")); err != nil {
						return err
					}
					bo.Put("idxmeta", mkey, []byte("1"))
				}
			}
		}

		// Save revision
		rkey := append(kRevPrefix(collection, doc.ID), []byte(fmt.Sprintf("%020d", now))...)
		if err := bRev.Put(rkey, buf); err != nil {
			return err
		}
		bo.Put("rev", rkey, buf)

		if s.CollectionManager != nil {
			if cfg, found := s.CollectionManager.Get(collection); found && cfg.MaxRevisions > 0 {
				if err := trimRevisions(tx, &bo, collection, doc.ID, cfg.MaxRevisions); err != nil {
					return err
				}
			}
		}

		saved = doc
		return nil
	})
	if err == nil {
		bo.FlushTo(s.Binlog)
	}
	if err != nil {
		if err.Error() == "not found" {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		bad(w, err)
		return
	}

	// Post-update hooks
	if hasContent && s.EmbeddingWorker != nil && saved.ContentMD != "" {
		s.EmbeddingWorker.Enqueue(EmbeddingJob{
			Collection: collection,
			DocID:      saved.ID,
			ContentMD:  saved.ContentMD,
		})
	}

	if s.TTLManager != nil {
		if saved.ExpiresAt > 0 {
			_ = s.TTLManager.Set(collection, saved.ID, saved.ExpiresAt)
		} else if hasTTL {
			_ = s.TTLManager.Remove(collection, saved.ID)
		}
	}

	if hasContent && s.FTSIndex != nil && saved.ContentMD != "" {
		_ = s.FTSIndex.IndexWithLang(collection, saved.ID, saved.ContentMD, saved.Lang)
		_ = s.FTSIndex.IndexPositionsWithLang(collection, saved.ID, saved.ContentMD, saved.Lang)
		fields := map[string]string{"content": saved.ContentMD}
		for k, vals := range saved.Meta {
			if len(vals) > 0 {
				fields["meta."+k] = strings.Join(vals, " ")
			}
		}
		_ = s.FTSIndex.IndexFieldsWithLang(collection, saved.ID, fields, saved.Lang)
	}

	// Geo re-index on partial update. Mirrors the Add/Upsert path above:
	// if meta now contains a usable point, upsert it into both indexes;
	// otherwise drop any stale points.
	if s.GeoIndex != nil && s.GeoStore != nil {
		if lat, lng, ok := s.GeoIndex.AddFromMeta(collection, saved.ID, saved.Meta); ok {
			_ = s.GeoStore.Put(collection, saved.ID, lat, lng)
			if s.GeoHashIndex != nil {
				s.GeoHashIndex.Add(collection, saved.ID, lat, lng)
			}
		} else {
			s.GeoIndex.Remove(collection, saved.ID)
			if s.GeoHashIndex != nil {
				s.GeoHashIndex.Remove(collection, saved.ID)
			}
			_ = s.GeoStore.Delete(collection, saved.ID)
		}
	}

	if s.WebhookManager != nil {
		s.WebhookManager.Fire("doc.updated", collection, key, lang, &saved)
	}

	if s.AutomationManager != nil && env("MDDB_TRIGGERS", "false") == "true" {
		go s.AutomationManager.EvaluateTriggers(collection, saved, "update")
	}

	if s.Metrics != nil {
		s.Metrics.IncOp("doc_update")
	}

	ok(w, saved)
}

func (s *Server) handleDocMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	collection := r.URL.Query().Get("collection")
	key := r.URL.Query().Get("key")
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}

	if collection == "" || key == "" {
		http.Error(w, `{"error":"collection and key are required"}`, http.StatusBadRequest)
		return
	}

	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), collection, PermRead); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	var doc Doc
	err := s.DBView(func(tx *bolt.Tx) error {
		bByK := tx.Bucket([]byte("bykey"))
		bDocs := tx.Bucket([]byte("docs"))
		docID := bByK.Get(kByKey(collection, key, lang))
		if docID == nil {
			return errors.New("not found")
		}
		v := bDocs.Get(kDoc(collection, string(docID)))
		if v == nil {
			return errors.New("not found")
		}
		d, err := loadDoc(v)
		if err != nil {
			return err
		}
		doc = *d
		return nil
	})
	if err != nil {
		if err.Error() == "not found" {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		bad(w, err)
		return
	}

	if doc.ExpiresAt > 0 && doc.ExpiresAt < time.Now().Unix() {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	// Return metadata only (no contentMd)
	resp := map[string]interface{}{
		"key":       doc.Key,
		"lang":      doc.Lang,
		"meta":      doc.Meta,
		"addedAt":   doc.AddedAt,
		"updatedAt": doc.UpdatedAt,
	}
	if doc.ExpiresAt > 0 {
		resp["expiresAt"] = doc.ExpiresAt
	}

	if s.Metrics != nil {
		s.Metrics.IncOp("doc_meta")
	}

	ok(w, resp)
}

func (s *Server) handleMetaKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	collection := r.URL.Query().Get("collection")
	if collection == "" {
		http.Error(w, `{"error":"collection is required"}`, http.StatusBadRequest)
		return
	}

	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), collection, PermRead); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	meta := make(map[string][]string)

	_ = s.DBView(func(tx *bolt.Tx) error {
		bIdx := tx.Bucket([]byte("idxmeta"))
		if bIdx == nil {
			return nil
		}

		prefix := []byte("meta|" + collection + "|")
		c := bIdx.Cursor()
		seen := make(map[string]map[string]bool)

		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			rest := string(k[len(prefix):])
			parts := strings.SplitN(rest, "|", 3)
			if len(parts) < 2 {
				continue
			}
			mk, mv := parts[0], parts[1]
			if seen[mk] == nil {
				seen[mk] = make(map[string]bool)
			}
			if !seen[mk][mv] {
				seen[mk][mv] = true
				meta[mk] = append(meta[mk], mv)
			}
		}
		return nil
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	type CollectionStats struct {
		Name           string `json:"name"`
		DocumentCount  int    `json:"documentCount"`
		RevisionCount  int    `json:"revisionCount"`
		MetaIndexCount int    `json:"metaIndexCount"`
		Checksum       string `json:"checksum"`
		Type           string `json:"type,omitempty"`
		Description    string `json:"description,omitempty"`
		Icon           string `json:"icon,omitempty"`
		Color          string `json:"color,omitempty"`
	}

	// Check read permission (database-wide stats)
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), "*", PermRead); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	// IndexQueueStats surfaces async meta-indexing health (GO-010): how many
	// jobs were processed, failed, or had to be indexed synchronously because
	// the queue was full (fallbacks), plus the current queue depth.
	type IndexQueueStats struct {
		Processed uint64 `json:"processed"`
		Failed    uint64 `json:"failed"`
		Fallbacks uint64 `json:"fallbacks"`
		QueueLen  int    `json:"queueLen"`
	}

	type Stats struct {
		DatabasePath     string            `json:"databasePath"`
		DatabaseSize     int64             `json:"databaseSize"`
		Mode             string            `json:"mode"`
		Collections      []CollectionStats `json:"collections"`
		TotalDocuments   int               `json:"totalDocuments"`
		TotalRevisions   int               `json:"totalRevisions"`
		TotalMetaIndices int               `json:"totalMetaIndices"`
		IndexQueue       *IndexQueueStats  `json:"indexQueue,omitempty"`
		Uptime           string            `json:"uptime"`
	}

	stats := Stats{
		DatabasePath: s.Path,
		Mode:         string(s.Mode),
		Collections:  []CollectionStats{},
	}

	// Get database file size
	if info, err := os.Stat(s.Path); err == nil {
		stats.DatabaseSize = info.Size()
	}

	// Collect statistics per collection
	collectionMap := make(map[string]*CollectionStats)

	err := s.DBView(func(tx *bolt.Tx) error {
		// Count documents per collection
		bDocs := tx.Bucket([]byte("docs"))
		if bDocs != nil {
			c := bDocs.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				// key format: doc|collection|id
				parts := strings.Split(string(k), "|")
				if len(parts) >= 2 {
					coll := parts[1]
					if _, ok := collectionMap[coll]; !ok {
						collectionMap[coll] = &CollectionStats{Name: coll}
					}
					collectionMap[coll].DocumentCount++
					stats.TotalDocuments++
				}
			}
		}

		// Count revisions per collection
		bRev := tx.Bucket([]byte("rev"))
		if bRev != nil {
			c := bRev.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				// key format: rev|collection|docID|ts
				parts := strings.Split(string(k), "|")
				if len(parts) >= 2 {
					coll := parts[1]
					if _, ok := collectionMap[coll]; !ok {
						collectionMap[coll] = &CollectionStats{Name: coll}
					}
					collectionMap[coll].RevisionCount++
					stats.TotalRevisions++
				}
			}
		}

		// Count meta indices per collection
		bIdx := tx.Bucket([]byte("idxmeta"))
		if bIdx != nil {
			c := bIdx.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				// key format: meta|collection|key|value|docID
				parts := strings.Split(string(k), "|")
				if len(parts) >= 2 {
					coll := parts[1]
					if _, ok := collectionMap[coll]; !ok {
						collectionMap[coll] = &CollectionStats{Name: coll}
					}
					collectionMap[coll].MetaIndexCount++
					stats.TotalMetaIndices++
				}
			}
		}

		return nil
	})

	if err != nil {
		bad(w, err)
		return
	}

	// Compute checksums per collection
	for name, cs := range collectionMap {
		cs.Checksum, _ = s.collectionChecksum(name)
	}

	// Enrich with collection config attributes
	if s.CollectionManager != nil {
		for name, cs := range collectionMap {
			if cfg, ok := s.CollectionManager.Get(name); ok {
				cs.Type = cfg.Type
				cs.Description = cfg.Description
				cs.Icon = cfg.Icon
				cs.Color = cfg.Color
			}
		}
	}

	// Convert map to slice
	for _, cs := range collectionMap {
		stats.Collections = append(stats.Collections, *cs)
	}

	// Sort collections by name
	sort.Slice(stats.Collections, func(i, j int) bool {
		return stats.Collections[i].Name < stats.Collections[j].Name
	})

	// Async meta-indexing queue health
	if s.IndexQueue != nil {
		processed, failed, fallbacks, queueLen := s.IndexQueue.Stats()
		stats.IndexQueue = &IndexQueueStats{
			Processed: processed,
			Failed:    failed,
			Fallbacks: fallbacks,
			QueueLen:  queueLen,
		}
	}

	ok(w, stats)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
func genID(parts ...string) string {
	// Optimized ID generation without string allocations
	totalLen := 0
	for i, part := range parts {
		totalLen += len(part)
		if i < len(parts)-1 {
			totalLen++ // for '|'
		}
	}

	buf := make([]byte, 0, totalLen)
	for i, part := range parts {
		for j := 0; j < len(part); j++ {
			c := part[j]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			buf = append(buf, c)
		}
		if i < len(parts)-1 {
			buf = append(buf, '|')
		}
	}

	return string(buf)
}
func applyEnv(s string, env map[string]string) string {
	for k, v := range env {
		s = strings.ReplaceAll(s, "%%"+k+"%%", v)
	}
	return s
}
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, s)
}
func intersect(sets ...[]string) []string {
	if len(sets) == 0 {
		return nil
	}
	m := map[string]int{}
	for _, s := range sets {
		for _, id := range s {
			m[id]++
		}
	}
	out := []string{}
	for id, c := range m {
		if c == len(sets) {
			out = append(out, id)
		}
	}
	return out
}
func copyFile(src, dst string) error {
	// #nosec G304 -- Function intentionally copies provided path
	in, err := os.Open(filepath.Clean(src))
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	tmp := dst + ".tmp"
	// #nosec G304 -- Subpath created securely
	out, err := os.Create(filepath.Clean(tmp))
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst) // #nosec G703 -- paths are internally constructed
}
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
func sortDocs(docs []Doc, field string, asc bool) {
	sort.Slice(docs, func(i, j int) bool {
		var less bool
		switch field {
		case "addedAt":
			less = docs[i].AddedAt < docs[j].AddedAt
		case "updatedAt":
			less = docs[i].UpdatedAt < docs[j].UpdatedAt
		case "key":
			less = docs[i].Key < docs[j].Key
		default:
			less = docs[i].UpdatedAt < docs[j].UpdatedAt
		}
		if asc {
			return less
		}
		return !less
	})
}

// handleDelete deletes a single document from a collection
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	var req DeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" || req.Key == "" || req.Lang == "" {
		bad(w, errors.New("missing fields"))
		return
	}

	// Check write permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermWrite); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	if err := s.deleteDocumentInternal(req.Collection, req.Key, req.Lang); err != nil {
		bad(w, err)
		return
	}

	ok(w, map[string]interface{}{
		"status":     "deleted",
		"collection": req.Collection,
		"key":        req.Key,
		"lang":       req.Lang,
	})
}

// handleDeleteBatch deletes multiple documents in a single request.
func (s *Server) handleDeleteBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Collection string `json:"collection"`
		Documents  []struct {
			Key  string `json:"key"`
			Lang string `json:"lang"`
		} `json:"documents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" {
		bad(w, errors.New("missing collection"))
		return
	}
	if len(req.Documents) == 0 {
		bad(w, errors.New("missing documents"))
		return
	}

	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermWrite); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	var deleted, notFound, failed int
	var errs []string
	for _, d := range req.Documents {
		if d.Key == "" || d.Lang == "" {
			failed++
			errs = append(errs, "missing key or lang")
			continue
		}
		if err := s.deleteDocumentInternal(req.Collection, d.Key, d.Lang); err != nil {
			if strings.Contains(err.Error(), "not found") {
				notFound++
			} else {
				failed++
				errs = append(errs, err.Error())
			}
			continue
		}
		deleted++
	}

	ok(w, map[string]interface{}{
		"deleted":   deleted,
		"not_found": notFound,
		"failed":    failed,
		"errors":    errs,
	})
}

// handleDeleteCollection deletes all documents in a collection
func (s *Server) handleDeleteCollection(w http.ResponseWriter, r *http.Request) {
	var req DeleteCollectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		bad(w, err)
		return
	}
	if req.Collection == "" {
		bad(w, errors.New("missing collection"))
		return
	}

	// Check write permission
	if s.AuthManager != nil {
		if err := s.AuthManager.CheckPermission(r.Context(), req.Collection, PermWrite); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	var deletedCount int
	var bo binlog.BinlogOps

	err := s.DBUpdate(func(tx *bolt.Tx) error {
		bDocs := tx.Bucket([]byte("docs"))
		bIdx := tx.Bucket([]byte("idxmeta"))
		bRev := tx.Bucket([]byte("rev"))
		bByK := tx.Bucket([]byte("bykey"))

		// Delete all documents in collection
		c := bDocs.Cursor()
		prefix := []byte("doc|" + req.Collection + "|")
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			// Load document to get metadata for index cleanup
			docPtr, err := loadDoc(v)
			if err != nil {
				continue
			}
			doc := *docPtr

			// Delete document
			if err := bDocs.Delete(k); err != nil {
				return err
			}
			bo.Delete("docs", k)

			// Delete from bykey index
			bykKey := kByKey(req.Collection, doc.Key, doc.Lang)
			if err := bByK.Delete(bykKey); err != nil {
				return err
			}
			bo.Delete("bykey", bykKey)

			// Delete all revisions
			rc := bRev.Cursor()
			rp := kRevPrefix(req.Collection, doc.ID)
			for rk, _ := rc.Seek(rp); rk != nil && bytes.HasPrefix(rk, rp); rk, _ = rc.Next() {
				if err := bRev.Delete(rk); err != nil {
					return err
				}
				bo.Delete("rev", rk)
			}

			// Delete metadata indices
			for mk, vals := range doc.Meta {
				for _, mv := range vals {
					mkey := append(kMetaKeyPrefix(req.Collection, mk, mv), []byte(doc.ID)...)
					if err := bIdx.Delete(mkey); err != nil {
						return err
					}
					bo.Delete("idxmeta", mkey)
				}
			}

			deletedCount++
		}

		return nil
	})
	if err == nil {
		bo.FlushTo(s.Binlog)
	}

	if err != nil {
		bad(w, err)
		return
	}

	// Clean up collection config
	if s.CollectionManager != nil {
		_ = s.CollectionManager.Delete(req.Collection)
	}

	// Clean up both geo indexes and persisted geo points for this collection.
	if s.GeoIndex != nil {
		s.GeoIndex.Clear(req.Collection)
	}
	if s.GeoHashIndex != nil {
		s.GeoHashIndex.Clear(req.Collection)
	}
	if s.GeoStore != nil {
		_ = s.GeoStore.DeleteCollection(req.Collection)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "deleted",
		"collection":   req.Collection,
		"deletedCount": deletedCount,
	}); err != nil {
		log.Printf("Error encoding delete collection response: %v", err)
	}
}
