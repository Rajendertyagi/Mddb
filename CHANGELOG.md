# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.6.7] - 2026-03-05

### Added
- **PMISparse Search Algorithm** - Two-phase sparse retrieval with PMI query expansion (invented by Tradik Limited)
  - BM25 scoring for direct term matches + automatic PPMI-based query expansion from corpus co-occurrence statistics
  - Lazy per-collection training with sliding-window co-occurrence matrix, automatic invalidation on document changes
  - Fuzzy variant combining edit-distance tolerance with PMI expansion for maximum recall
  - Works standalone (`algorithm: "pmisparse"`) and as the FTS component in hybrid search
  - Configurable parameters: k1, b, alpha, expansionK, windowSize, minCount, topK
  - Expansion matches marked with `~` prefix in `matchedTerms` for transparency
  - Dedicated documentation: `docs/PMISPARSE.md`
- **Sentiment Analysis for Triggers** - Keyword-based sentiment scoring for automation triggers
  - `AnalyzeSentiment()` returns score from -1.0 (negative) to +1.0 (positive) using built-in lexicon (~100 positive, ~100 negative words)
  - Optional `sentimentEnabled` condition on triggers with configurable min/max range
  - AND/OR logic (`conditionLogic`) when combining sentiment with search conditions
  - Markdown-aware text stripping before analysis
- **Automation Execution Logs** - Track webhook execution history
  - `GET /v1/automation-logs` endpoint with cursor-based pagination
  - Filter by `ruleId` and `status` (success, error, skipped)
  - TTL-based automatic cleanup with configurable retention (`MDDB_AUTOMATION_LOGS_TTL`, default: 7d)
  - Panel Logs tab with auto-refresh toggle and status filter
- **`MDDB_AUTOMATIONS` env var** - Single toggle to enable/disable entire automation system
  - `MDDB_AUTOMATION_LOGS` - Enable/disable automation execution logging
  - `MDDB_AUTOMATION_LOGS_TTL` - Log retention period (default: 7d)
- **Webhook Template Variables** - Dynamic `{{variable}}` substitution in webhook URLs and custom headers
  - Trigger variables: `{{doc.id}}`, `{{doc.key}}`, `{{doc.lang}}`, `{{doc.meta.FIELD}}`, `{{collection}}`, `{{score}}`, `{{sentiment}}`, `{{trigger.id}}`, `{{trigger.name}}`, `{{timestamp}}`, `{{webhook.id}}`, `{{event}}`
  - Cron variables: `{{cron.id}}`, `{{cron.name}}`, `{{timestamp}}`, `{{webhook.id}}`, `{{event}}`
  - Panel: collapsible help section listing available variables in webhook form

### Changed
- Version bumped to 2.6.7 across all services and documentation

## [2.6.6] - 2026-03-04

### Added
- **Automation System** - Triggers, Crons, and Webhook Targets for automated workflows
  - **Triggers**: Fire webhooks when new documents match search criteria (FTS/vector/hybrid) above threshold
  - **Crons**: Schedule periodic trigger execution using cron expressions (`robfig/cron/v3`)
  - **Webhook Targets**: Named HTTP endpoints with custom headers and configurable methods
  - Unified storage in single `automation` BoltDB bucket with `type` field
  - HTTP API: `GET/POST /v1/automation`, `GET/PUT/DELETE /v1/automation/{id}`, `POST /v1/automation/{id}/test`
  - gRPC RPCs: `ListAutomation`, `CreateAutomation`, `UpdateAutomation`, `DeleteAutomation`, `TestAutomation`
  - MCP tools: `list_automation`, `create_automation`, `update_automation`, `delete_automation`, `test_automation`
  - Env vars: `MDDB_TRIGGERS`, `MDDB_CRONS`, `MDDB_WEBHOOKS` (all default: false)
  - Webhook payload with retry backoff (0s, 1s, 5s, 15s) and custom X-MDDB headers
- **Panel: Automation Tab** - Full automation management UI
  - Type filter tabs (All/Webhooks/Triggers/Crons) with icons (Webhook/Zap/Clock)
  - Dynamic forms per type with collection/webhook/trigger dropdowns
  - Enable/disable toggle, test button (dry run with matching docs and scores)
- **Automation Tests** - 15 unit tests covering CRUD, validation, and edge cases

### Changed
- Version bumped to 2.6.6 across all services and documentation

## [2.6.5] - 2026-03-04

### Added
- **Hybrid Search** (`/v1/hybrid-search`) - New endpoint combining BM25/BM25F keyword search with vector semantic search
  - Alpha Blending strategy: weighted linear interpolation `combined = (1-α) * BM25 + α * vector`
  - RRF (Reciprocal Rank Fusion) strategy: rank-based fusion robust to different score distributions
  - Configurable: `strategy`, `alpha`, `rrfK`, `algorithm`, `vectorAlgorithm`
  - gRPC `HybridSearch` RPC and `hybrid_search` MCP tool
- **In-Graph FTS Filtering** - `filterMeta` parameter on full-text search endpoint
  - Pre-filters candidate documents by metadata before BM25 scoring
  - Supports OR within key, AND across keys
  - Added `filter_meta` field to FTS proto message
- **Panel: Hybrid Search Mode** - New search mode with strategy/alpha/algorithm controls
- **Panel: Command Modal** - Copy-ready API examples in curl, PHP, Python, and JavaScript for all search operations
- **Panel: System Info Default** - Default to System Information view after login

### Fixed
- Stale gRPC/MCP entries in endpoint documentation (removed non-existent `Delete`/`DeleteCollection` gRPC methods)

## [2.6.4] - 2026-03-04

### Added
- **BM25F Field-Weighted Search** - New full-text search algorithm
  - Weights matches in different document fields (title, tags, body) independently
  - Default weights: meta.title=3.0, meta.tags=2.0, meta.category=2.0, meta.description=1.5, content=1.0
  - Custom per-query field weights via `fieldWeights` parameter
  - Supports fuzzy matching with field weights
  - Field-level inverted index stored in dedicated BoltDB buckets
  - Algorithm option: `"bm25f"` in FTS API
- **Panel BM25F UI** - Field weights configuration panel with collapsible field weight editor, custom field support
- **Optimized Docker Pipeline** - Pre-built Go binaries instead of compiling in Docker
- **Scalar Quantization (SQ)** - New vector search algorithm
  - Quantizes float32 vectors to uint8 (0-255) using per-dimension min/max scaling
  - ADC-style search with precomputed distance tables + exact cosine re-ranking
  - ~75% memory reduction vs flat index
  - Algorithm option: `"sq"` in vector search API
- **Binary Quantization (BQ)** - New vector search algorithm
  - Reduces each float32 to 1 bit (sign bit), packed into uint64 words
  - Hamming distance for ultra-fast coarse ranking via `math/bits.OnesCount64`
  - Re-ranks top candidates with exact cosine similarity
  - ~97% memory reduction vs flat index
  - Algorithm option: `"bq"` in vector search API
- **Porter Stemming** for Full-Text Search
  - Pure Go Porter Stemmer implementation (no external deps)
  - Stems indexed terms and query terms for better recall
  - Configurable: `MDDB_FTS_STEMMING` (default: true)
  - Per-query disable via `disableStem` request field
- **Synonym Support** for Full-Text Search
  - Per-collection synonym dictionaries stored in BoltDB
  - HTTP endpoints: `POST/GET/DELETE /v1/synonyms`
  - Built-in default synonym groups (10 English groups)
  - Bidirectional query-time expansion
  - Configurable: `MDDB_FTS_SYNONYMS` (default: true)
  - Per-query disable via `disableSynonyms` request field
- **Compression Configuration**
  - `MDDB_COMPRESSION_ENABLED` - enable/disable adaptive compression
  - `MDDB_COMPRESSION_SMALL_THRESHOLD` - Snappy threshold (default: 1024)
  - `MDDB_COMPRESSION_MEDIUM_THRESHOLD` - Zstd threshold (default: 10240)
- **Extended Configuration**
  - New config sections: `fts`, `compression`, `vector` in YAML config file
  - All new features configurable via env vars, YAML, or CLI flags
  - FTS response includes `stemmingActive` and `synonymsActive` status

### Changed
- Panel VectorSearchPanel: added SQ and BQ to algorithm dropdown
- Panel FTSSearchPanel: added stemming/synonyms toggles
- Panel mddb-client: added synonym CRUD methods
- Version bumped to 2.6.4

## [2.6.2] - 2026-03-04

### Added
- **Embedding Chunking** - Auto-split long documents into chunks before embedding
  - Paragraph-based splitting with sentence and hard-split fallbacks
  - Multi-key chunk storage: `vec|collection|docID#0`, `vec|collection|docID#1`, etc.
  - Chunk deduplication in vector search: best-chunk-score per document
  - Oversampling (topK * 3) for accurate top-K after deduplication
  - Configurable via `MDDB_EMBEDDING_CHUNK_SIZE` (default 1500) and `MDDB_EMBEDDING_CHUNK_ENABLED` (default true)
  - Backward-compatible with existing non-chunked embeddings
  - Chunk stats in `/v1/vector-stats` and `/v1/vector-reindex` responses
- **Panel Mode** - `MDDB_PANEL_MODE` environment variable
  - `internal` (default): CORS enabled, browser accesses API directly
  - `external`: CORS disabled, panel reverse-proxies all `/v1/*` requests
  - Express production server (`server.js`) with `http-proxy-middleware`
  - Panel always uses relative `/v1` URLs (works in both modes)

### Changed
- Panel Dockerfile uses Express server instead of Vite preview for production
- Panel `mddb-client.js` simplified to always use relative `/v1` API base
- Vector search handlers (HTTP, gRPC, MCP) use oversampling + chunk deduplication
- All 4 vector searchers (flat, HNSW, IVF, PQ) handle chunk keys in filter matching

## [2.3.3] - 2026-02-28

### Added
- **Custom MCP Tools** - YAML-defined website-specific AI tools for mddb-mcp
  - Define custom tools in `config.yaml` under `custom_tools:` key
  - 3 supported actions: `semantic_search`, `search_documents`, `full_text_search`
  - Preconfigured defaults merged with user arguments at runtime
  - Custom tools appear alongside 23 built-in tools in `tools/list`
  - Startup validation: name conflicts, valid actions, valid param types
  - Works on both transports: stdio (Claude Desktop) and HTTP
  - Deduplicated tool definitions (~420 lines removed, shared `builtinTools()`)
  - Dedicated documentation: `docs/CUSTOM-TOOLS.md`

## [2.3.2] - 2026-02-28

### Added
- **Telemetry** - Prometheus-compatible `/metrics` endpoint
  - HTTP request counters with method, path, and status labels
  - Request duration histograms (12 buckets from 1ms to 10s)
  - Database metrics: file size, documents, revisions, meta indices per collection
  - Vector search metrics: embeddings count, index readiness, queue size
  - Webhook and schema counts
  - Go runtime: goroutines, memory stats, GC metrics
  - Zero external dependencies (pure Go text exposition format)
  - DB stats cached for 15s to minimize scan overhead
  - Configurable via `MDDB_METRICS` env var (enabled by default)
  - Dedicated documentation: `docs/TELEMETRY.md` with Grafana queries and alerting rules

## [2.3.1] - 2026-02-28

### Added
- **Schema Validation** - JSON Schema validation for document metadata
  - Per-collection schemas (opt-in, disabled by default)
  - HTTP endpoints: `/v1/schema/set`, `/v1/schema/get`, `/v1/schema/delete`, `/v1/schema/list`, `/v1/validate`
  - gRPC RPCs: `SetSchema`, `GetSchema`, `DeleteSchema`, `ListSchemas`, `ValidateDocument`
  - Supported rules: `required`, `properties` (types), `enum`, `pattern`, `minItems`/`maxItems`
  - Automatic validation on document add/update when schema is set
  - CLI commands: `schema set/get/delete/list`, `validate`
  - MCP tools: `set_schema`, `get_schema`, `delete_schema`, `list_schemas`, `validate_document`
  - PHP and Python extension support
  - Dedicated documentation: `docs/SCHEMA-VALIDATION.md`
- **SECURITY.md** - Security policy and vulnerability reporting process
- **CONTRIBUTING.md** - Contribution guidelines with setup instructions

## [2.1.0] - 2025-01-09

### Added
- **Health check endpoints** - `/health` and `/v1/health` for monitoring
  - Simple JSON response with status and mode
  - Database connectivity verification
  - HTTP 200 for healthy, 503 for unhealthy
- **OpenAPI/Swagger documentation** - Complete API specification
  - OpenAPI 3.0.3 specification in `docs/openapi.yaml`
  - Interactive Swagger UI in `docs/swagger.html`
  - Machine-readable API documentation
  - Try-it-out functionality for all endpoints
- **Health check documentation** - Comprehensive guide in `docs/HEALTHCHECK.md`
  - Docker and Docker Compose examples
  - Kubernetes liveness and readiness probes
  - Load balancer configuration (Nginx, HAProxy, Traefik)
  - Manual health check methods (curl, wget, httpie)
  - Monitoring integration examples
  - Troubleshooting guide

### Changed
- Updated Docker health checks to use `/health` endpoint
- Updated docker-compose.yml with proper health check configuration
- Updated Dockerfile with health check using wget
- Simplified performance claims in documentation (more pragmatic, less boastful)
- Removed Polish documentation files (English only)
- Fixed license badge and references (BSD-3-Clause)

## [2.0.3] - 2025-11-07

### Added
- **Document deletion functionality** - Delete documents with confirmation dialog
  - Delete button in document list items
  - Delete button in document viewer header
  - Confirmation modal with document details
  - Automatic list refresh after deletion
- **Error handling improvements** - Better error boundaries and fallbacks
  - ReactMarkdown error boundary with raw content fallback
  - Progressive document loading (immediate display + background fetch)
  - User-friendly error messages with recovery options
- **Docker image for mddb-panel** - Complete containerization
  - Multi-stage Docker build for production
  - Development Docker configuration
  - Docker Compose integration
  - Makefile targets for panel Docker operations

### Fixed
- **Blank document viewer issue** - Documents now display immediately with content
- **Document content loading** - Fixed API integration for full document fetching
- **ReactMarkdown compatibility** - Removed deprecated className prop
- **Content overflow issues** - Fixed margin and layout problems in document viewer
- **UI responsiveness** - Better loading states and user feedback
- **golangci-lint errors** - Fixed unchecked error returns in JSON encoding

### Improved
- **Document viewer layout** - Better container constraints and overflow handling
- **User experience** - Immediate feedback when clicking documents
- **Error recovery** - Users can always access document content in some form
- **Development workflow** - Added panel to development Docker setup

## [Unreleased]

### Added
- **Web Admin Panel (mddb-panel)** - Modern React-based admin interface
  - Server statistics dashboard
  - Collection browser with document count
  - Document list with metadata preview
  - Document viewer with full content and metadata
  - **Document editor** - Edit markdown content and metadata
  - **New document creation** - Create documents directly from UI
  - **Markdown editor with live preview** - Split view with real-time rendering
  - **Markdown toolbar** - Quick formatting buttons (bold, italic, headings, lists, etc.)
  - **Syntax highlighting** - Code blocks with language-specific highlighting
  - **Markdown templates** - Pre-built templates (blog, docs, README, API, changelog)
  - Advanced filtering by metadata fields
  - Sort by date, key, or custom fields
  - Copy document content to clipboard
  - Modern UI with TailwindCSS and Lucide icons
  - Built with React 19, Vite 6, and Zustand 5
  - Docker support with multi-stage builds
  - Proxy configuration for API requests
- Bulk import script for loading markdown files from folders
- `load-md-folder.sh` script with features:
  - Automatic key generation from filenames
  - YAML frontmatter metadata extraction
  - Recursive folder scanning
  - Progress tracking with statistics
  - Dry run mode for preview
  - Custom metadata support
  - Multi-language support
- Makefile targets for folder import:
  - `import-folder` - Import markdown files
  - `import-folder-dry` - Preview import without executing
  - `import-folder-recursive` - Import recursively
- Makefile targets for panel:
  - `panel-install` - Install panel dependencies
  - `panel-dev` - Run panel in development mode
  - `panel-build` - Build panel for production
  - `panel-preview` - Preview production build
- Comprehensive bulk import documentation (BULK-IMPORT.md)
- README section for bulk import usage
- README section for web admin panel
- Docker Compose configuration for panel service

## [2.0.2] - 2025-11-07

### Changed
- Updated quic-go to v0.55.0 (HTTP/3 improvements)
- Updated Alpine base image to 3.22 (security updates)
- Updated Go dependencies (crypto, net, sys, mod, text, tools)
- Disabled automatic workflow triggers (manual only)
- Removed Docker buildcache (not needed for users)
- Removed dev Docker images (production only)

### Fixed
- Docker build context issues
- Docker Hub description update (now manual)

## [2.0.1] - 2025-11-07

### Fixed
- Fixed all golangci-lint issues (18 total)
  - errcheck: Added error checking for binary.Write, file.Close, res.Body.Close
  - staticcheck: Removed redundant nil check, optimized fmt usage, fixed pointer usage
  - unused: Removed unused struct fields (mu, workerPool, current)
- Fixed proto definitions for UpdateDocument, DeleteDocument, and batch responses
- Added missing SaveRevision and NotFound fields to proto messages

### Added
- Docker Hub integration with automated builds
- Multi-platform Docker images (AMD64 + ARM64)
- Comprehensive Docker Hub documentation
- GitHub Actions workflow for Docker builds and pushes
- Docker Compose configuration for production deployment

### Changed
- Updated golangci-lint to v2.6.1 with Go 1.25 support
- Improved error handling across codebase
- Optimized buffer pool usage to avoid allocations

## [2.0.0] - 2025-11-07

### Major Performance Release

**Significant performance improvements through multiple optimization strategies**

#### Performance Enhancements
- Protobuf serialization for smaller payloads
- BoltDB tuning (NoFreelistSync, FreelistMapType, optimized mmap)
- Conditional metadata reindexing
- Batch processing with single transactions
- Parallel processing with worker pools
- Connection pooling for gRPC
- Bucket caching
- Optional revision history
- Single transaction search
- Lazy indexing with async queue
- Read-through document cache
- Batch delete and update operations

#### Advanced Features (Extreme Mode)
Enable with `MDDB_EXTREME=true` environment variable:
- Write-Ahead Log (WAL) with periodic sync
- Lock-free cache with 16 shards
- MVCC snapshot isolation
- Bloom filters for fast lookups
- Delta encoding for smaller revisions
- Adaptive compression (Snappy/Zstd)
- HTTP/3 + QUIC support
- Adaptive indexing
- Async I/O operations
- Zero-copy I/O
- Vectorized operations (SIMD)
- Distributed sharding

### Benchmark Results

Tested with 3000 documents:
- MDDB (Batch API): 29,810 docs/s, 34µs avg latency
- MongoDB: 5,176 docs/s, 192µs avg latency
- PostgreSQL: 4,324 docs/s, 231µs avg latency
- MySQL: 1,214 docs/s, 822µs avg latency
- CouchDB: 312 docs/s, 3,185µs avg latency

### Added
- HTTP/3 server on port 11443 (Extreme Mode)
- Comprehensive performance benchmarking suite
- Comparison tests with MongoDB, PostgreSQL, MySQL, CouchDB

## [1.0.0] - Initial Release

### Added
- Initial release of MDDB
- RESTful API for markdown document management
- **gRPC API** - High-performance binary protocol (70% smaller payload than JSON)
- **Dual protocol support** - HTTP (port 11023) and gRPC (port 11024) run simultaneously
- **Docker images** - Optimized Alpine Linux images (~15 MB)
- **Docker Compose** - Production and development configurations
- **Shared Protobuf** - Monorepo structure with centralized proto definitions
- **Multi-language clients** - Generated code for Go, Python, Node.js, PHP
- BoltDB-based storage engine
- Document versioning and revision history
- Metadata indexing and search
- Multi-language support
- Template variable substitution
- Export functionality (NDJSON and ZIP formats)
- Backup and restore capabilities
- Revision truncation for database maintenance
- Access mode control (read, write, read-write)
- **Statistics endpoint** - `/v1/stats` for server and database monitoring
- **Command-line client (mddb-cli)** - Full-featured CLI similar to mysql-client
- **Unix man page** - Complete manual page for CLI
- Comprehensive documentation
- Makefile with development and build targets
- Systemd service configuration

### Features

#### Core Functionality
- Add/update markdown documents with metadata
- Retrieve documents by key and language
- Search with metadata filtering
- Sort by addedAt, updatedAt, or key
- Pagination support
- Template variable substitution (%%var%% syntax)

#### Storage
- BoltDB embedded database
- Automatic metadata indexing
- Revision history tracking
- Efficient prefix-based indices
- ACID transactions

#### API Endpoints
- `POST /v1/add` - Add or update documents
- `POST /v1/get` - Retrieve documents
- `POST /v1/search` - Search with filters
- `POST /v1/export` - Export as NDJSON or ZIP
- `GET /v1/backup` - Create backup
- `POST /v1/restore` - Restore from backup
- `POST /v1/truncate` - Truncate revision history
- `GET /v1/stats` - Server and database statistics

#### Developer Experience
- Comprehensive Makefile with colored output
- Hot reload support with Air
- Cross-platform builds (Linux, Windows, macOS)
- Test coverage reporting
- Code formatting and linting targets
- Development and production modes

#### Command-Line Client
- `mddb-cli` - Full-featured CLI client
- Unix-style commands (add, get, search, export, backup, restore, truncate, stats)
- Man page documentation (`man mddb-cli`)
- JSON and human-readable output formats
- Pipe-friendly content-only mode
- Metadata filtering and search
- Template variable support
- Batch operation support
- Server statistics display

#### Documentation
- Quick start guide
- Complete API documentation
- Usage examples with multiple languages
- Architecture overview with diagrams
- Production deployment guide
- Docker and systemd configurations

### Technical Details
- Go 1.25+ required
- BoltDB for storage
- HTTP/JSON API
- Single binary deployment
- No external dependencies

## [0.1.0] - 2024-11-06

### Added
- Initial project structure
- Basic MDDB server implementation
- Core API endpoints
- Documentation suite
- Build system with Makefile
- Docker support

---

## Release Notes

### Version 0.1.0 (Initial Release)

This is the first release of MDDB - a lightweight markdown database with a RESTful API.

**Key Features:**
- Store and manage markdown documents with metadata
- Full revision history
- Fast metadata-based search
- Multi-language support
- Export capabilities
- Easy backup and restore

**Getting Started:**
```bash
make build
make run
```

See the [Quick Start Guide](docs/QUICKSTART.md) for detailed instructions.

**Requirements:**
- Go 1.25 or later
- 512 MB RAM minimum
- Linux, macOS, or Windows

**Known Limitations:**
- Single-writer (BoltDB limitation)
- No built-in authentication
- No full-text search (planned for future release)

**Future Plans:**
- Full-text search integration
- Built-in authentication
- GraphQL API
- Replication support
- Plugin system

---

## Contributing

When contributing, please:
1. Update this CHANGELOG with your changes
2. Follow [Keep a Changelog](https://keepachangelog.com/) format
3. Add entries under `[Unreleased]` section
4. Use these categories: Added, Changed, Deprecated, Removed, Fixed, Security

## Links

- [Repository](https://github.com/tradik/mddb)
- [Documentation](docs/)
- [Issues](https://github.com/tradik/mddb/issues)
