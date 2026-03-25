package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	json "github.com/goccy/go-json"
	bolt "go.etcd.io/bbolt"
	"google.golang.org/grpc"
)

// VERSION is the current release version of the MDDB server.
const VERSION = "2.9.3"

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
	DB                  *bolt.DB
	Path                string
	Mode                AccessMode
	Config              ServerConfig // protocol toggles & addresses
	Hooks               Hooks        // optional extensions
	BucketNames         BucketNames
	Cache               *DocumentCache        // Read-through cache (legacy)
	LockFreeCache       *LockFreeCache        // Lock-free cache (extreme performance)
	IndexQueue          *IndexQueue           // Async metadata indexing
	WAL                 *WAL                  // Write-Ahead Log
	MVCC                *MVCC                 // Multi-Version Concurrency Control
	BloomFilters        *BloomFilterManager   // Bloom filters for negative lookups
	DeltaEncoder        *DeltaEncoder         // Delta encoding for revisions
	AdaptiveIndex       *AdaptiveIndexManager // Adaptive indexing
	AsyncIO             *AsyncIO              // Async I/O
	ZeroCopy            *ZeroCopyManager      // Zero-copy I/O
	SIMD                *SIMDProcessor        // Vectorized operations
	ShardCluster        *ShardCluster         // Distributed sharding
	finalBatchProcessor *FinalBatchProcessor  // Final optimized batch processor
	UseExtreme          bool                  // Enable extreme performance features
	// Vector search
	VectorStore       *VectorStore              // Persistent vector storage in BoltDB
	VectorIndex       *VectorIndex              // In-memory flat vector index
	VectorSearchers   map[string]VectorSearcher // algorithm name -> searcher (flat, hnsw, ivf, pq, sq, bq)
	QuantizedVecIndex *QuantizedVectorIndex     // In-memory quantized vector index (int8/int4)
	EmbeddingWorker   *EmbeddingWorker          // Background embedding processor
	Embedding         EmbeddingProvider         // Embedding generation provider
	// New features
	TTLManager         *TTLManager         // Document TTL / auto-expiry
	FTSIndex           *FTSIndex           // Full-text search index
	WebhookManager     *WebhookManager     // Webhook subscriptions and delivery
	SchemaManager      *SchemaManager      // Per-collection metadata schema validation
	Metrics            *Metrics            // Prometheus-compatible telemetry
	AuthManager        *AuthManager        // Authentication and authorization
	SynonymManager     *SynonymManager     // Synonym dictionaries for FTS
	StopWordManager    *StopWordManager    // Per-collection custom stop words for FTS
	AutomationManager  *AutomationManager  // Automation: triggers, crons, webhook targets
	AutomationLogStore *AutomationLogStore // Automation execution logs
	CronScheduler      *CronScheduler      // Cron scheduler for automation
	CollectionManager  *CollectionManager  // Per-collection attributes (type, description, icon, etc.)
	SSEHub             *SSEHub             // Server-Sent Events for real-time document change notifications
	MCPInfo            MCPServerInfo       // Customizable MCP server profile
	MCPInstructions    string              // System prompt for LLM — how to use this server
	// Replication
	Binlog          *Binlog            // Binary replication log
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

	dbPath := env("MDDB_PATH", "mddb.db")
	mode := AccessMode(env("MDDB_MODE", "wr")) // read|write|wr

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
		Cache:         NewDocumentCache(1000, 300),  // 1000 docs, 5min TTL
		LockFreeCache: NewLockFreeCache(10000, 300), // 10k docs, 5min TTL (lock-free)
		IndexQueue:    NewIndexQueue(nil, 4),        // 4 workers (will set server below)
		BloomFilters:  NewBloomFilterManager(),      // Bloom filters
		DeltaEncoder:  NewDeltaEncoder(),            // Delta encoding
		AdaptiveIndex: NewAdaptiveIndexManager(),    // Adaptive indexing
		AsyncIO:       NewAsyncIO(),                 // Async I/O
		ZeroCopy:      NewZeroCopyManager(),         // Zero-copy I/O
		SIMD:          NewSIMDProcessor(),           // Vectorized operations
		ShardCluster:  NewShardCluster(4, 2),        // 4 shards, 2x replication
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
	s.VectorStore = NewVectorStore(db)
	if err := s.VectorStore.EnsureBucket(); err != nil {
		log.Fatal(err)
	}
	s.VectorIndex = NewVectorIndex()
	bqRerank := srvCfg.Vector.BQRerankFactor
	if bqRerank <= 0 {
		bqRerank = 10
	}
	s.QuantizedVecIndex = NewQuantizedVectorIndex(func(collection string) QuantizationType {
		if s.CollectionManager == nil {
			return QuantNone
		}
		cfg, ok := s.CollectionManager.Get(collection)
		if !ok || cfg.Quantization == "" {
			return QuantNone
		}
		return ParseQuantization(cfg.Quantization)
	})
	s.VectorSearchers = map[string]VectorSearcher{
		"flat":      s.VectorIndex,
		"hnsw":      NewHNSWIndex(16, 200, 100),
		"ivf":       NewIVFIndex(10, 20),
		"pq":        NewPQIndex(8, 256, 20),
		"sq":        NewSQIndex(),
		"bq":        NewBQIndex(bqRerank),
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
		s.Embedding = NewEmbeddingProvider()
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

	// Initialize TTL manager
	s.TTLManager = NewTTLManager(db, s)
	if err := s.TTLManager.EnsureBuckets(); err != nil {
		log.Fatal(err)
	}
	s.TTLManager.StartCleanup(30 * time.Second)
	log.Println("TTL manager started (cleanup every 30s)")

	// Configure compression
	ConfigureCompression(srvCfg.Compression.Enabled, srvCfg.Compression.SmallThreshold, srvCfg.Compression.MediumThreshold)
	if !srvCfg.Compression.Enabled {
		log.Println("Document compression disabled")
	}

	// Initialize FTS index
	s.FTSIndex = NewFTSIndex(db)
	if err := s.FTSIndex.EnsureBuckets(); err != nil {
		log.Fatal(err)
	}

	// Initialize multi-language FTS support
	langReg := NewLangRegistry(srvCfg.FTS.DefaultLang)
	if srvCfg.FTS.StemmingEnabled {
		RegisterDefaultLanguages(langReg)
		s.FTSIndex.SetStemmer(NewPorterStemmer())
		s.FTSIndex.SetLangRegistry(langReg)
		log.Printf("FTS stemming enabled — %d languages (default: %s)", len(langReg.Languages()), srvCfg.FTS.DefaultLang)
	}

	// Initialize synonym manager
	s.SynonymManager = NewSynonymManager(db)
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
	s.StopWordManager = NewStopWordManager(db)
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
	s.FTSIndex.SetPMIData(NewPMIData())

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
	s.SchemaManager = NewSchemaManager(db)
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

	// Initialize SSE hub (enabled by default, set MDDB_SSE_ENABLED=false to disable)
	sseEnabled := env("MDDB_SSE_ENABLED", "true") != "false"
	sseMaxClients := envDefaultInt("MDDB_SSE_MAX_CLIENTS", 1000)
	sseMaxPerIP := envDefaultInt("MDDB_SSE_MAX_PER_IP", 5)
	s.SSEHub = NewSSEHub(sseEnabled, sseMaxClients, sseMaxPerIP)
	if sseEnabled {
		log.Printf("SSE event stream enabled (max clients: %d, max per IP: %d)", sseMaxClients, sseMaxPerIP)
	}

	// Store MCP server info for handlers
	s.MCPInfo = srvCfg.MCP.ServerInfo
	s.MCPInstructions = srvCfg.MCP.Instructions

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
		bl, err := NewBinlog(dbPath, BinlogConfig{
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
		s.FTSIndex.SetBinlog(s.Binlog)
		s.TTLManager.SetBinlog(s.Binlog)
		s.WebhookManager.SetBinlog(s.Binlog)
		s.SchemaManager.SetBinlog(s.Binlog)
		s.SynonymManager.SetBinlog(s.Binlog)
		s.StopWordManager.SetBinlog(s.Binlog)
		s.AutomationManager.SetBinlog(s.Binlog)
		s.CollectionManager.SetBinlog(s.Binlog)
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
	mux.HandleFunc("/v1/add", s.guardWrite(s.handleAdd))
	mux.HandleFunc("/v1/add-batch", s.guardWrite(s.handleAddBatch))
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
	mux.HandleFunc("/v1/embedding-configs", s.handleEmbeddingConfigs)
	mux.HandleFunc("/v1/embedding-configs/", s.handleEmbeddingConfigDetail)
	mux.HandleFunc("/v1/embedding-configs/set-default", s.guardWrite(s.handleSetDefaultEmbeddingConfig))
	mux.HandleFunc("/v1/upload", s.guardWrite(s.handleUpload))
	mux.HandleFunc("/v1/import-url", s.guardWrite(s.handleImportURL))
	mux.HandleFunc("/v1/set-ttl", s.guardWrite(s.handleSetTTL))
	mux.HandleFunc("/v1/fts", s.handleFTS)
	mux.HandleFunc("/v1/fts-reindex", s.guardWrite(s.handleFTSReindex))
	mux.HandleFunc("/v1/fts-languages", s.handleFTSLanguages)
	mux.HandleFunc("/v1/meta-keys", s.handleMetaKeys)
	mux.HandleFunc("/v1/checksum", s.handleChecksum)
	mux.HandleFunc("/v1/update", s.guardWrite(s.handleUpdate))
	mux.HandleFunc("/v1/doc-meta", s.handleDocMeta)
	mux.HandleFunc("/v1/classify", s.handleClassify)
	mux.HandleFunc("/v1/hybrid-search", s.handleHybridSearch)
	mux.HandleFunc("/v1/synonyms", s.handleSynonyms)
	mux.HandleFunc("/v1/stopwords", s.handleStopWords)
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
	mux.HandleFunc("/v1/events", s.handleSSE)
	mux.HandleFunc("/v1/cross-search", s.handleCrossSearch)
	mux.HandleFunc("/v1/find-duplicates", s.handleFindDuplicates)
	mux.HandleFunc("/v1/aggregate", s.handleAggregate)
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
	graphqlEnabled := env("MDDB_GRAPHQL_ENABLED", "false") == "true"
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

	// Wrap mux: CORS → auth middleware → metrics middleware → JSON content type → routes
	handler := withJSON(mux)
	handler = s.Metrics.Middleware(handler)
	if authEnabled && s.AuthManager != nil {
		handler = s.AuthManager.HTTPMiddleware(handler)
	}
	if panelMode != "external" {
		handler = withCORS(handler)
	}
	if panelMode == "external" {
		log.Printf("Panel mode: external (CORS disabled, panel proxies requests)")
	}

	// Shut down early health server before starting the main one
	if earlyServer != nil {
		_ = earlyServer.Close()
		time.Sleep(50 * time.Millisecond) // brief pause to release port
	}

	// Mark server as ready — health check will now return "healthy" instead of "warming_up"
	s.Ready = true
	log.Println("Server initialization complete — ready to serve")

	// Start HTTP server (with optional TLS)
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
			if srvCfg.TLS.Enabled && srvCfg.TLS.CertFile != "" && srvCfg.TLS.KeyFile != "" {
				log.Printf("mddb HTTPS listening on %s (mode=%s, db=%s, tls=on)", httpAddr, s.Mode, dbPath)
				if err := server.ListenAndServeTLS(srvCfg.TLS.CertFile, srvCfg.TLS.KeyFile); err != nil && err != http.ErrServerClosed {
					log.Fatal(err)
				}
			} else {
				log.Printf("mddb HTTP listening on %s (mode=%s, db=%s)", httpAddr, s.Mode, dbPath)
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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

			mcpHandler := withJSON(mcpMux)
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

	// Start gRPC server
	if srvCfg.GRPC.Enabled {
		var grpcOpts []grpc.ServerOption
		if authEnabled && s.AuthManager != nil {
			grpcOpts = append(grpcOpts, grpc.UnaryInterceptor(s.AuthManager.GRPCUnaryInterceptor()))
		}
		go func() {
			log.Printf("mddb gRPC listening on %s (mode=%s, db=%s)", grpcAddr, s.Mode, dbPath)
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
}

// --- helpers / buckets

func (s *Server) ensureBuckets() error {
	return s.DB.Update(func(tx *bolt.Tx) error {
		_, _ = tx.CreateBucketIfNotExists(s.BucketNames.Docs)          // doc|collection|id -> json
		_, _ = tx.CreateBucketIfNotExists(s.BucketNames.IdxMeta)       // meta|collection|key|value|docID -> 1
		_, _ = tx.CreateBucketIfNotExists(s.BucketNames.Rev)           // rev|collection|docID|ts -> json
		_, _ = tx.CreateBucketIfNotExists(s.BucketNames.ByKey)         // bykey|collection|key|lang -> docID
		_, _ = tx.CreateBucketIfNotExists([]byte("embedding_configs")) // embedding model configurations
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
	origin := env("MDDB_CORS_ORIGIN", "*")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
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

func (s *Server) guardWrite(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mode := effectiveMode(s.Mode, s.Config.HTTP.Mode)
		if mode == ModeRead {
			http.Error(w, `{"error":"read-only mode"}`, http.StatusForbidden)
			return
		}
		next(w, r)
	}
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
func (s *Server) addDocument(collection, key, lang string, meta map[string][]string, contentMD string, ttl int64) (Doc, bool, error) {
	now := time.Now().Unix()
	docID := genID(collection, key, lang)

	var saved Doc
	var isNew bool
	var bo BinlogOps
	err := s.DB.Update(func(tx *bolt.Tx) error {
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

		buf, err := marshalDoc(&doc)
		if err != nil {
			return err
		}
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

		rkey := append(kRevPrefix(collection, doc.ID), []byte(fmt.Sprintf("%020d", now))...)
		if err := bRev.Put(rkey, buf); err != nil {
			return err
		}
		bo.Put("rev", rkey, buf)

		saved = doc
		return nil
	})
	if err == nil {
		bo.FlushTo(s.Binlog)
	}
	if err != nil {
		return Doc{}, false, err
	}

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

	// Webhooks + SSE
	event := "doc.updated"
	if isNew {
		event = "doc.added"
	}
	if s.WebhookManager != nil {
		s.WebhookManager.Fire(event, collection, key, lang, &saved)
	}
	if s.SSEHub != nil {
		s.SSEHub.BroadcastWithAuth(event, collection, key, lang, s.AuthManager)
	}

	// Automation triggers
	if s.AutomationManager != nil && env("MDDB_TRIGGERS", "false") == "true" {
		triggerEvent := "insert"
		if !isNew {
			triggerEvent = "update"
		}
		go s.AutomationManager.EvaluateTriggers(collection, saved, triggerEvent)
	}

	return saved, isNew, nil
}

// deleteDocumentInternal deletes a document and all its associated data.
func (s *Server) deleteDocumentInternal(collection, key, lang string) error {
	docID := genID(collection, key, lang)

	var bo BinlogOps
	var deletedDoc Doc // captured for trigger evaluation
	err := s.DB.Update(func(tx *bolt.Tx) error {
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

	// Clean up FTS index
	if s.FTSIndex != nil {
		_ = s.FTSIndex.Remove(collection, docID)
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

	saved, _, err := s.addDocument(req.Collection, req.Key, req.Lang, req.Meta, req.ContentMD, req.TTL)
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
	err := s.DB.View(func(tx *bolt.Tx) error {
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

	err := s.DB.View(func(tx *bolt.Tx) error {
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
				ids = unique(ids)
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
	if err := copyFile(s.Path, dst); err != nil {
		bad(w, err)
		return
	}
	ok(w, map[string]string{"backup": dst})
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

	// zamknij db, podmień plik, otwórz ponownie
	_ = s.DB.Close()
	if err := copyFile(body.From, s.Path); err != nil {
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

	var bo BinlogOps
	err := s.DB.Update(func(tx *bolt.Tx) error {
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
	err := s.DB.View(func(tx *bolt.Tx) error {
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

func (s *Server) collectionChecksum(collection string) (string, int) {
	var checksum uint32
	var count int

	_ = s.DB.View(func(tx *bolt.Tx) error {
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
	var bo BinlogOps
	var metaDidChange bool

	err := s.DB.Update(func(tx *bolt.Tx) error {
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
		buf, err := marshalDoc(&doc)
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
	err := s.DB.View(func(tx *bolt.Tx) error {
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

	_ = s.DB.View(func(tx *bolt.Tx) error {
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

	type Stats struct {
		DatabasePath     string            `json:"databasePath"`
		DatabaseSize     int64             `json:"databaseSize"`
		Mode             string            `json:"mode"`
		Collections      []CollectionStats `json:"collections"`
		TotalDocuments   int               `json:"totalDocuments"`
		TotalRevisions   int               `json:"totalRevisions"`
		TotalMetaIndices int               `json:"totalMetaIndices"`
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

	err := s.DB.View(func(tx *bolt.Tx) error {
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
func unique(in []string) []string {
	m := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, x := range in {
		if _, ok := m[x]; !ok {
			m[x] = struct{}{}
			out = append(out, x)
		}
	}
	return out
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
	var bo BinlogOps

	err := s.DB.Update(func(tx *bolt.Tx) error {
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

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "deleted",
		"collection":   req.Collection,
		"deletedCount": deletedCount,
	}); err != nil {
		log.Printf("Error encoding delete collection response: %v", err)
	}
}
