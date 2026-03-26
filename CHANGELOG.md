# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.9.4] - 2026-03-26

### Added
- **MCP API Key Authentication** ([#29](https://github.com/tradik/mddb/issues/29)) — Protect MCP endpoints with API keys. Enable with `MDDB_MCP_API_KEY_ENABLED=true`, define keys in `MDDB_MCP_API_KEYS=key1:name1,key2:name2`. Supports `X-API-Key` header, `Authorization: Bearer`, and `?api_key=` query param (for SSE).
- **MCP Request Logging / Audit Trail** ([#30](https://github.com/tradik/mddb/issues/30)) — Structured JSON audit logs for all MCP requests. Enable with `MDDB_MCP_LOGGING_ENABLED=true`. Logs method, path, status, duration, client IP, API key name, session ID, and user agent.
- **MCP Rate Limiting** ([#31](https://github.com/tradik/mddb/issues/31)) — Per-client rate limiting for MCP endpoints. Enable with `MDDB_MCP_RATE_LIMIT_ENABLED=true`. Configurable via `MDDB_MCP_RATE_LIMIT_REQUESTS` (default: 100), `MDDB_MCP_RATE_LIMIT_WINDOW` (default: 60s), `MDDB_MCP_RATE_LIMIT_BURST` (default: 20), `MDDB_MCP_RATE_LIMIT_BY` (ip/api_key/session). Returns `X-RateLimit-*` headers and `Retry-After` on 429.
- **Dynamic MCP API Key Management** ([#33](https://github.com/tradik/mddb/issues/33)) — REST API for creating, listing, disabling, and deleting MCP API keys stored in BoltDB. Keys persist across restarts. Supports TTL expiry and disable-without-delete. Endpoints: `POST/GET/DELETE /v1/mcp/keys`, `POST /v1/mcp/keys/disable`. Requires admin auth. Cache with configurable TTL (`MDDB_MCP_API_KEY_CACHE_TTL`).
- **Panel: MCP API Keys tab** — New "API Keys" tab in LLM Connections for creating, viewing, and managing MCP API keys from the web panel.
- **Panel: Metadata filter scroll fix** ([#14](https://github.com/tradik/mddb/issues/14)) — Filter panel now uses `max-h-[70vh]` instead of `max-h-60` for proper scrolling with many metadata fields.

## [2.9.3] - 2026-03-25

### Added
- **MCP Protocol 2025-11-25** — Upgraded from 2024-11-05 to the latest MCP specification
- **Streamable HTTP transport** (`/mcp`) — New standard transport supporting POST (JSON-RPC), GET (SSE stream), DELETE (session termination), `MCP-Session-Id` header. Legacy SSE transport (`/sse` + `/message`) preserved for backward compatibility
- **Tool annotations** — All 52+ tools annotated with `readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint` hints per MCP spec. Enables AI clients (Claude, Cursor) to auto-approve safe tools
- **Structured output schemas** — `outputSchema` on 9 key tools: `get_stats`, `search_documents`, `semantic_search`, `full_text_search`, `hybrid_search`, `vector_stats`, `get_checksum`, `classify_document`, `aggregate`
- **5 MCP Prompts** — Built-in prompt templates: `analyze-collection`, `search-help`, `summarize-collection`, `import-guide`, `rag-pipeline`
- **Completion/autocomplete** — `completion/complete` for collection names and prompt arguments (source, model, algorithm)
- **MCP Logging** — `logging/setLevel` method + `notifications/message` support with RFC 5424 severity levels (debug through emergency)
- **Notification handling** — Server accepts `notifications/initialized`, `notifications/cancelled`, `notifications/roots/list_changed` without error
- **Progress token infrastructure** — `notifications/progress` support for long-running tools (vector_reindex, ingest_documents, fts_reindex, create_backup, etc.)
- **Cursor-based pagination** — `tools/list` and `resources/list` support cursor parameter per spec
- **Per-protocol access modes** — Independent read/write control per protocol via `MDDB_API_MODE`, `MDDB_GRPC_MODE`, `MDDB_MCP_MODE`, `MDDB_HTTP3_MODE`. Each overrides the global `MDDB_MODE` for its protocol. Example: `MDDB_MCP_MODE=read` makes MCP read-only while API remains read-write.
- **`MDDB_MCP_BUILTIN_TOOLS=false`** — Disable all 54 built-in MCP tools, exposing only custom YAML tools. Useful for restricting AI agents to domain-specific tools only.
- **39 new tests** — MCP protocol (29), per-protocol mode enforcement (10) including follower/global mode scenarios

### Security
- **MCP now respects global read-only mode** — `MDDB_MODE=read` and follower replication role correctly block MCP write tools. Previously MCP DirectClient bypassed the mode check entirely.
- **globalMode propagation** — Server's access mode (including follower-forced read-only) flows through the full MCP chain: Server → MCPHandler → MCPToolServer → write guard

### Fixed
- **Custom MCP tools work in read-only/follower mode** ([#27](https://github.com/tradik/mddb/issues/27)) — Custom tools with read-only actions (`full_text_search`, `search_documents`, `semantic_search`, `fts_languages`) are now allowed in read-only mode. Previously all custom tools were incorrectly blocked because they had no entries in the annotation map.

### Changed
- Tool call errors now return `isError: true` in result content (per spec) instead of JSON-RPC error codes
- `ping` response is now an empty object `{}` per MCP spec (was `{"result":"pong"}`)
- Resource read errors use code `-32002` (resource not found) instead of generic `-32000`
- Capabilities now advertise `prompts`, `logging`, `completions`, and `listChanged: true` for tools and resources

## [2.9.2] - 2026-03-25

### Added
- **MCP-over-SSE transport** — Spec-compliant MCP transport over Server-Sent Events at `/sse` + `/message` on MCP port (9000). Enables remote MCP connections without stdio for web-based agents and remote servers.
- **Panel SSE integration** — Real-time toast notifications for document changes, auto-refresh document list, Live/Offline connection indicator with auto-reconnect and exponential backoff
- **7 new MCP-over-SSE tests** — endpoint, full message flow, session validation, error handling

### Changed
- Version bump to 2.9.2 across all files

## [2.9.1] - 2026-03-25

### Fixed
- **SSE auth enforcement** — when auth is enabled, SSE now requires JWT/API key (returns 401 without token). Previously SSE was open to unauthenticated users.
- **SSE RBAC filtering** — events are only sent to clients with `PermRead` on the collection. `readOnly` field in each event indicates if client has `PermWrite`.

### Added
- **SSE per-IP rate limiting** — max concurrent SSE connections per IP address (default: 5). Prevents resource exhaustion. Configurable via `MDDB_SSE_MAX_PER_IP`.
- **SSE on MCP port** — `/events` endpoint available on MCP HTTP server (port 9000)
- **SSE connected event** includes `mode` ("read" or "readwrite") and `user` fields

## [2.9.0] - 2026-03-24

### Added
- **Per-Collection Vector Quantization** — Each collection can now configure its own vector quantization level for both storage and in-memory search. Supported formats:
  - `float32` (default) — Full precision, no compression
  - `int8` — 4x compression with ~1% recall drop, recommended for most use cases
  - `int4` — 8x compression with ~2-3% recall drop, ideal for large collections
- **Quantized Vector Index** (`vector_index_quantized.go`) — In-memory flat index that stores and searches vectors directly in int8/int4 format without dequantization
- **Quantized storage format (v2)** — New binary serialization with version byte prefix, backward-compatible with existing float32 records
- **Auto-select quantized searcher** — Vector search automatically uses the quantized index when the collection has quantization configured
- **`quantization` field in Collection Config API** — `PUT /v1/collection-config` now accepts `quantization` field (`"float32"`, `"int8"`, `"int4"`)
- **Quantization info in vector stats** — `GET /v1/vector-stats` now shows `quantization` per collection
- **Panel: Vector Quantization selector** — Collection Settings modal now includes Vector Quantization dropdown
- **New documentation** — `docs/QUANTIZATION.md` with full guide, examples, storage savings table, and technical details
- **17 new tests** — Round-trip quantization, similarity accuracy, storage integration, compression ratio verification

- **Server-Sent Events (SSE)** — Real-time document change notifications via `GET /v1/events`. Broadcasts `doc.added`, `doc.updated`, `doc.deleted` events. Per-collection filtering via `?collection=X`. Default enabled, configurable via `MDDB_SSE_ENABLED=false`. Keep-alive heartbeat every 30s.
- **pprof profiling endpoints** — Runtime CPU/memory profiling at `/debug/pprof/` (heap, goroutine, CPU profile, trace, allocs, block, mutex). Disabled by default, enable via `MDDB_PPROF_ENABLED=true`.
- **HTTP connection pooling** — Shared `http.Transport` with keep-alive for all outbound requests (webhooks, triggers, crons, import-url). Configurable via `MDDB_HTTP_POOL_MAX_IDLE`, `MDDB_HTTP_POOL_MAX_PER_HOST`, `MDDB_HTTP_POOL_IDLE_TIMEOUT`.
- **Built-in TLS/HTTPS** — Native TLS support without reverse proxy. Configure via `MDDB_TLS_ENABLED=true`, `MDDB_TLS_CERT`, `MDDB_TLS_KEY` or YAML config. Works for HTTP API server.

### Fixed
- **quantization.go**: Add bounds validation in `dequantizeInt8`/`dequantizeInt4` — prevents panic on corrupted or truncated quantized vector data
- **vector_store.go**: Return error instead of nil vector when dequantization fails due to data length mismatch
- **lockfree_cache.go**: Fix incorrect `size` tracking when updating an existing cache key — eviction was triggered unnecessarily and size counter drifted
- **lockfree_cache.go**: Fix goroutine leak — `cleanup()` goroutine now stops via `Close()` method; called on server shutdown
- **lockfree_cache.go**: Fix misleading "FIFO eviction" comment — Go map iteration is random, not ordered
- **mvcc.go**: Fix race condition in `Delete` — replaced `Load`+`Store` with `LoadOrStore` to prevent concurrent writes from being silently lost
- **mvcc.go**: Fix memory leak in GC — uncommitted versions from abandoned transactions were never cleaned up; GC now skips only versions belonging to active transactions
- **mvcc.go**: Optimize `Commit`/`Rollback` from O(all keys) to O(affected keys) via `txnKeys` tracking map
- **vector_store.go**: Harden `CountByCollection` chunk suffix stripping with `n >= 0` guard to reduce false deduplication risk

### Changed
- **Use Cases section linked to guides** — `docs/index.html` Use Cases section now links to step-by-step guides (`uses/website-chat.md`, `uses/wordpress-analyzer.md`, `uses/youtube-transcribe.md`); added "Use Cases" nav link
- Updated docs: README, CHANGELOG, openapi.yaml, docs/index.html, man page
- `vector_store.go` — `PutQuantized`, `PutChunksQuantized`, `LoadCollectionQuantized` methods, auto-detect v1/v2 format on read
- `vector_handlers.go` — Quantization-aware reindex, auto-algorithm selection, stats enrichment

## [2.8.0] - 2026-03-15

### Added
- **Per-Collection Storage Backends** — Each collection can now use its own storage backend instead of the server-wide default. Supported backends:
  - `boltdb` (default) — Embedded BoltDB, same as before
  - `memory` — In-memory ephemeral storage, data lost on restart. Ideal for scratch/cache collections
  - `s3` — S3-compatible object storage (AWS S3, MinIO, Cloudflare R2, etc.) with configurable endpoint, bucket, region, credentials, and prefix
- **Storage backend configuration via API** — `PUT /v1/collection-config` now accepts `storageBackend` and `storageConfig` fields
- **Storage backend configuration via Panel** — Collection Settings modal now includes Storage Backend selector with S3 configuration form
- **StorageBackend interface** (`storage_backend.go`) — Pluggable storage abstraction with `BackendRegistry` for per-collection routing
- **MemoryBackend** (`storage_memory.go`) — Thread-safe in-memory document store
- **S3Backend** (`storage_s3.go`) — S3-compatible storage using `minio-go/v7`, with auto bucket creation
- **Aggregations** — New `POST /v1/aggregate` endpoint for metadata facets and date histograms
  - Facets: count distinct values per metadata key with `count` or `value` ordering
  - Histograms: group documents by `addedAt`/`updatedAt` with `day`/`week`/`month`/`year` intervals
  - Optional `filterMeta` pre-filtering (same as `/v1/search`)
  - MCP tool: `aggregate`
- **Advanced Full-Text Search Modes** — The `/v1/fts` endpoint now supports 8 search types:
  - Boolean search: `AND`, `OR`, `NOT` operators and `+`/`-` prefix notation
  - Phrase search: exact consecutive phrase matching via positional index
  - Wildcard search: `*` (any chars) and `?` (single char) pattern matching
  - Proximity search: terms within N words of each other (`"rust performance"~5`)
  - Range search: numeric/date range filtering on metadata and timestamps (`rangeMeta`)
  - Auto-detect mode: automatically selects search type from query syntax
  - New `mode` parameter: `"auto"`, `"simple"`, `"boolean"`, `"phrase"`, `"wildcard"`, `"proximity"`
  - Positional index for phrase/proximity search (new `ftsp` bucket)
  - Panel: search mode selector, proximity distance slider, syntax hints
- **Multi-Language Full-Text Search** — Language-aware stemming and stop word filtering for 18 languages (English, Polish, German, French, Spanish, Italian, Portuguese, Dutch, Russian, Swedish, Norwegian, Danish, Finnish, Hungarian, Romanian, Turkish, Arabic, Tamil). Each document's `lang` field determines the FTS pipeline. New endpoints: `GET /v1/fts-languages`, `POST /v1/fts-reindex`. New config: `MDDB_FTS_DEFAULT_LANG`. New `lang` parameter in FTS search requests.
- **Chat Service Improvements** — Anthropic Claude LLM provider, tool-use support (search, get document), improved widget UI with typing indicators and error handling
- **File Upload Enhancements** — Extended `POST /v1/upload` with support for PDF, DOCX, HTML, ODT, RTF, TeX, YAML, and plain text file formats alongside Markdown

### Changed
- Updated docs: README, openapi.yaml, docs/index.html, API.md, SEARCH.md, man page
- Updated OpenAPI spec with FTS `mode`, `distance`, and `rangeMeta` schema fields

### Fixed
- **golint** — Fixed all 190 warnings across the codebase (added doc comments to all exported types, methods, constants)
- **gosec** — Fixed all 108 security issues in non-generated code:
  - G115: Added `safeInt32()` / `safeUint16()` helpers for overflow-safe integer conversions
  - G706/G704/G703/G705/G117/G404: Added targeted `#nosec` annotations with justification comments

## [2.7.1] - 2026-03-10

### Added
- **`POST /v1/delete-batch` endpoint** — Batch delete documents via REST API (previously only available in MCP/gRPC)
- **MCP tools**: `list_revisions` and `restore_revision` added to builtin tool schemas and validation map
- **MCP endpoint list**: Added missing `ingest_documents` and `upload_file` to `/v1/endpoints` MCP tools list
- **OpenAPI spec**: Added `/v1/delete-batch` endpoint definition with full request/response schema

### Fixed
- **Panel proxy 404 fix** — `server.js` mounted proxy at `/v1` which caused Express to strip the prefix; switched to root mount with `pathFilter: '/v1/**'` ([#17](https://github.com/tradik/mddb/issues/17))
- Panel endpoint counts now correct: HTTP(78), gRPC(54), MCP(53) — previously showed HTTP(76), MCP(51)
- MCP tool count updated from 52 to 53 across README, LLM_CONNECTIONS.md, and all documentation
- API.md expanded with ~35 missing endpoint docs (delete-batch, delete-collection, hybrid-search, cross-search, find-duplicates, collection-config, webhooks, revisions, automation, auth endpoints, system endpoints)

## [2.7.0] - 2026-03-06

### Added
- **Search Stats** — All search endpoints (`/v1/fts`, `/v1/vector-search`, `/v1/hybrid-search`) now return `searchStats` object with `durationMs`, `queryTerms`, `totalTokens`, and `indexSize`. Controlled by `MDDB_SEARCH_STATS` env var (default: enabled)
- **Distance Metrics**: Configurable distance metric for vector and hybrid search (cosine, dot_product, euclidean) via `distanceMetric` parameter
- **Document Revision History** — New `POST /v1/revisions` endpoint lists all revisions of a document. New `POST /v1/revisions/restore` restores a document to a previous revision
- **Collection attributes**: Per-collection type, description, icon, color, and custom metadata (`/v1/collection-config`, `/v1/collection-configs`)
- **Cross-collection search**: Search across multiple collections using a source document's embedding or text query (`/v1/cross-search`)
- **Duplicate detection**: Find exact and similar documents within a collection using content hashes and embedding similarity (`/v1/find-duplicates`, `find_duplicates` MCP tool)
- **4 new MCP tools**: `get_collection_config`, `set_collection_config`, `list_collection_configs`, `cross_search`
- **MCP tools**: `list_revisions`, `restore_revision`
- Panel: Full-page document editor (replaces constrained modal)
- Panel: Edit button moved to document header for easier access
- Panel: Document revision history viewer with restore capability
- Panel: Search stats display (duration, tokens, query terms) in all search panels

### Fixed
- Panel: Document content now refreshes after save, so re-opening editor shows updated content

### Changed
- Version bumped to 2.7.0 across all services and documentation

## [2.6.9] - 2026-03-05

### Added
- **Partial Document Update** — `PATCH /v1/update` for updating metadata and/or content independently
  - Meta only: `{"collection":"blog","key":"p1","lang":"en","meta":{"tag":["go"]}}`
  - Content only: `{"collection":"blog","key":"p1","lang":"en","contentMd":"new content"}`
  - Both: include both fields. Clear meta: `{"meta":{}}`
  - gRPC: `UpdateDocument` RPC. MCP: `update_document` tool
- **Document Metadata Read** — `GET /v1/doc-meta` returns metadata without content (lightweight)
  - gRPC: `GetDocumentMeta` RPC. MCP: `get_document_meta` tool
- **Zero-Shot Classification** — `POST /v1/classify` classifies documents against candidate labels using embedding similarity
  - By reference: provide `collection`, `key`, `lang` (reuses existing embedding if available)
  - By raw text: provide `text` field (embeds on the fly)
  - Labels embedded in a single batch call for efficiency
  - Parameters: `topK`, `multi` (return all above threshold), `threshold`
  - gRPC: `Classify` RPC. MCP: `classify_document` tool
- Panel: `updateDocument`, `getDocumentMeta`, and `classify` client methods

## [2.6.8] - 2026-03-05

### Added
- **Metadata Tag Filtering in Search** — Select metadata tags to filter FTS, vector, and hybrid search results in the panel. Dynamically loads available tags from collection. Multi-select with AND across keys, OR within values. New `MetaFilterBar` component.
- **`GET /v1/meta-keys` Endpoint** — List unique metadata keys and values for a collection. Powers the tag filter UI in the panel.
- **`GET /v1/checksum` Endpoint** — Lightweight CRC32-based collection checksum that changes on any document add/update/delete. Enables cache invalidation without downloading all documents. Also included in `/v1/stats` response per collection.
- **FTS `filterMeta` Support** — Full-text search now accepts `filterMeta` parameter for metadata pre-filtering (already supported in vector and hybrid search).

### Changed
- Version bumped to 2.6.8 across all services and documentation

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
- Updated golangci-lint to v2.6.1 with Go 1.26 support
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
- Go 1.26+ required
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
- Go 1.26 or later
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
