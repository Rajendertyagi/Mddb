# MDDB Features

Complete list of MDDB features organized by category.

## Core Database Features

### Document Management
- **Full CRUD Operations** - Add, retrieve, update, delete markdown documents
- **Rich Metadata** - Multi-value tags and structured metadata per document
- **Collections** - Organize documents into logical groups
- **Multi-language Support** - Store same document in multiple languages with same key
- **Template Variables** - Dynamic `{{variable}}` substitution on retrieval
- **Document TTL** - Auto-expiring documents with background cleanup (like Redis)
- **Schema Validation** - Optional per-collection JSON Schema validation for metadata

### Revision History
- **Complete Version Control** - Every update creates a new revision
- **Full Content Snapshots** - Each revision stores complete document content
- **Revision Queries** - Access any historical version through API
- **Truncation** - Remove old revisions to save space

### Search & Discovery

#### Metadata Search
- **Indexed Queries** - Fast searches using metadata tags
- **Multi-value Filters** - Query documents by multiple tag values
- **Sorting** - Sort by `addedAt`, `updatedAt`, or custom metadata
- **Pagination** - Limit and offset for large result sets

#### Vector Search (Semantic)
- **Auto-embeddings** - Documents embedded automatically in background
- **Multiple Providers** - OpenAI, Cohere, Voyage AI, Ollama
- **Similarity Search** - Find documents by meaning, not keywords
- **Multiple Algorithms** - Flat (exact), HNSW (approximate), IVF (clustered), PQ (compressed)
- **Query-time Algorithm Selection** - Choose algorithm per request via `algorithm` parameter
- **Threshold Filtering** - Minimum similarity score
- **Metadata Filtering** - Combine semantic + metadata filters
- **Configurable Models** - Switch providers/models without code changes
- **Background Indexing** - Non-blocking embedding generation
- **Automatic Fallback** - Falls back to flat if ANN index not ready

#### Full-Text Search
- **Built-in Inverted Index** - No external dependencies (Elasticsearch, etc.)
- **TF-IDF Scoring** - Classic term frequency-inverse document frequency ranking
- **BM25 Scoring** - Okapi BM25 with document length normalization (k1=1.2, b=0.75)
- **Query-time Algorithm Selection** - Choose TF-IDF or BM25 per request via `algorithm` parameter
- **Typo Tolerance** - Fuzzy matching with configurable edit distance (0-2) via `fuzzy` parameter
- **Stop Word Filtering** - Remove common words
- **Multi-field Search** - Search in content and metadata
- **Language-aware** - Per-language stop words

## APIs & Protocols

### HTTP/JSON REST API
- **RESTful Design** - Standard HTTP methods (GET, POST, DELETE)
- **JSON Payloads** - Easy to debug with curl, Postman
- **OpenAPI/Swagger** - Machine-readable spec + interactive docs
- **CORS Support** - Cross-origin requests
- **Content Negotiation** - Accept headers
- **Rate Limiting** - Configurable request limits

### gRPC/Protobuf API
- **High Performance** - 16x faster than HTTP/JSON
- **Binary Protocol** - 70% smaller payload size
- **HTTP/2 Multiplexing** - Concurrent requests on single connection
- **Streaming** - Bi-directional streaming support
- **gRPC Reflection** - Use grpcurl for debugging
- **Protocol Buffers** - Strongly typed contracts
- **Code Generation** - Auto-generate clients for Go, Python, Node.js, PHP

### GraphQL API
- **Flexible Queries** - Request exactly the fields you need
- **Schema Introspection** - Self-documenting API
- **GraphQL Playground** - Interactive development tool
- **Authentication Directives** - `@auth`, `@hasRole`, `@hasPermission`
- **Mutations** - Create, update, delete operations
- **Nested Queries** - Fetch related data in single request
- **Type Safety** - Compile-time validation
- **Query Complexity Limits** - Prevent expensive queries

### MCP Server (Model Context Protocol)
- **Dual Mode** - HTTP server + stdio mode for IDE integration
- **LLM Integration** - Windsurf, Claude Desktop, other AI tools
- **Custom Tools** - YAML-defined website-specific AI tools
- **Preconfigured Defaults** - Semantic search, document search, full-text search
- **Docker Ready** - Single image, mode selection via env var
- **gRPC Backend** - High-performance communication with MDDB
- **REST Fallback** - Automatic fallback if gRPC unavailable

## Security & Access Control

### Authentication
- **JWT Tokens** - JSON Web Tokens with expiry
- **API Keys** - Long-lived keys for scripts and automation
- **bcrypt Hashing** - Secure password storage
- **Token Refresh** - Renew tokens without re-authentication
- **Configurable Expiry** - Set token lifetime
- **Secure Headers** - `Authorization: Bearer <token>` or `X-API-Key: <key>`

### Authorization (RBAC)
- **Collection-level Permissions** - Read, Write, Admin per collection
- **User Roles** - Regular users and administrators
- **Group-based Permissions** - Organize users into groups
- **Inherited Permissions** - Groups grant permissions to all members
- **Admin Override** - Admins have full access to all collections
- **Permission Checks** - Enforced at every API call

### User Management
- **Multi-user Support** - Unlimited users
- **Admin Accounts** - Super users with full access
- **User CRUD** - Create, read, update, delete users (admin only)
- **Password Changes** - Users can change own password
- **User Listing** - Admins can list all users
- **API Key Management** - Users manage their own API keys

### Group Management
- **Group Creation** - Organize users into logical groups
- **Permission Assignment** - Grant collection permissions to groups
- **User Assignment** - Add/remove users from groups
- **Multiple Groups** - Users can belong to multiple groups
- **Cumulative Permissions** - Users get permissions from all their groups

## Integration & Automation

### Webhooks
- **Event Triggers** - `doc.added`, `doc.updated`, `doc.deleted`
- **HTTP Callbacks** - POST JSON payload to URL
- **Retry Logic** - 3 retries with exponential backoff
- **Collection Filtering** - Listen to specific collections only
- **Payload** - Event type, collection, key, lang, timestamp, document
- **Webhook Management** - Register, list, delete webhooks
- **Error Logging** - Failed deliveries logged

### Import from URL
- **Fetch Markdown** - Download from any HTTP/HTTPS URL
- **Frontmatter Parsing** - Automatic YAML frontmatter extraction
- **Key Derivation** - Auto-generate key from filename if not provided
- **Metadata Extraction** - Frontmatter becomes document metadata
- **Content Cleaning** - Strip frontmatter from content
- **Error Handling** - Graceful handling of HTTP errors, invalid markdown

### Telemetry (Prometheus)
- **Metrics Endpoint** - `GET /metrics` (Prometheus-compatible)
- **Request Counters** - Total requests by method, path, status
- **Latency Histograms** - Request duration distribution
- **Database Stats** - Document count, revision count, DB size
- **Vector Stats** - Embedding count, queue size
- **Go Runtime** - Goroutines, memory, GC stats
- **Configurable** - Disable with `MDDB_METRICS=false`

### Bulk Import
- **Folder Scanning** - Load all `.md` files from directory
- **Recursive** - Optional recursive subdirectory scanning
- **Frontmatter Support** - Extract metadata from YAML frontmatter
- **Custom Metadata** - Add metadata to all imported files
- **Progress Tracking** - Show import progress
- **Dry Run** - Preview what would be imported
- **Error Handling** - Continue on errors, report failures

## Developer Tools

### CLI Client
- **Full-featured** - All server operations available
- **File Input** - Read markdown from files
- **Piping Support** - Works with Unix pipes
- **Man Page** - Complete Unix-style manual
- **GraphQL Support** - Use GraphQL instead of REST with `--graphql`
- **Interactive Playground** - `mddb-cli playground` opens browser
- **Authentication** - JWT and API key support
- **Output Formatting** - JSON, pretty-print, or raw

### Web Admin Panel
- **Modern React UI** - Fast, responsive interface
- **Server Statistics** - Real-time metrics dashboard
- **Collection Browser** - Browse all collections and documents
- **Document Viewer** - View markdown with syntax highlighting
- **Document Editor** - Edit with live preview
- **Split-view** - Edit/Preview/Both modes
- **Markdown Toolbar** - Formatting buttons
- **Templates** - Pre-built templates (blog, docs, README, API)
- **Advanced Filtering** - Filter by metadata
- **Pagination** - Offset-based pagination with total count for metadata search
- **Result Counts** - Shows number of results for FTS and vector search
- **Resizable Sidebar** - Drag to resize, collapsible with persist to localStorage
- **API Mode Toggle** - Switch between REST and GraphQL
- **Authentication** - Login with username/password
- **User Management** - Manage users and groups (admin only)
- **Conditional UI** - Users & Groups hidden when auth is disabled
- **Vector Search** - Semantic search interface with auto-retry on index loading

### Docker Support
- **Multi-arch Images** - amd64, arm64, armv7
- **Alpine Linux** - Minimal image size (~15MB)
- **Health Checks** - Built-in health check endpoint
- **Volume Persistence** - Mount `/data` for database
- **Environment Config** - All settings via env vars
- **Docker Compose** - Development and production configs
- **Auto-restart** - Crash recovery

### Development Mode
- **Hot Reload** - Auto-restart on code changes (with air)
- **Development Logging** - Verbose debug output
- **CORS** - Enabled for local development
- **Source Maps** - JavaScript debugging
- **Fast Builds** - Optimized for iteration

## Horizontal Scaling

### Leader-Follower Replication
- **Single-leader Replication** - One write node, multiple read-only followers
- **Binary Replication Log** - Compact binlog with LSN-based change tracking
- **Real-time Streaming** - gRPC-based binlog streaming with sub-100ms lag
- **Full Snapshot Sync** - Automatic snapshot transfer for new followers
- **Automatic Reconnection** - Followers reconnect and catch up after disconnects
- **Subsystem Replication** - Documents, vectors, FTS, webhooks, schemas, auth all replicated
- **Cluster Dashboard** - Web panel shows node status, lag, and follower health
- **Prometheus Metrics** - `mddb_replication_lsn`, `mddb_binlog_size_bytes`, follower count
- **Zero Config Standalone** - Replication disabled by default, opt-in via `MDDB_REPLICATION_ROLE`

See [Replication Guide](REPLICATION.md) for setup instructions and examples.

## Operations & Management

### Export & Backup
- **NDJSON Export** - Newline-delimited JSON format
- **ZIP Export** - Compressed archive with metadata
- **Filtered Export** - Export specific collections or metadata filters
- **Full Backup** - Complete database backup (BoltDB file)
- **Streaming** - Memory-efficient for large datasets
- **Restore** - Restore from backup file

### Database Maintenance
- **Truncate Revisions** - Remove old revisions to save space
- **Keep N Revisions** - Configurable retention policy
- **Cache Invalidation** - Drop cache after truncate
- **Vacuum** - BoltDB compaction (manual)
- **Statistics** - Real-time server and DB metrics

### Access Modes
- **Read-only** - Prevent all writes
- **Write-only** - Prevent reads (rare use case)
- **Read-write** - Default mode
- **Environment Config** - `MDDB_ACCESS_MODE=read-only`

### Monitoring
- **Health Check** - `GET /health` endpoint
- **Statistics** - `GET /v1/stats` for server info
- **Prometheus Metrics** - Full observability
- **Grafana Dashboards** - Pre-built visualizations
- **Alerting** - Prometheus alertmanager rules
- **Logging** - Structured JSON logs

## Performance Features

### Storage Optimizations
- **BoltDB** - Embedded ACID key-value store
- **Single File** - Entire database in one file
- **B+Tree Index** - Fast lookups and range scans
- **MVCC** - Multi-version concurrency control
- **Prefix Indices** - Composite keys for fast metadata queries
- **NoFreelistSync** - Faster writes (configurable)
- **Initial mmap** - Pre-allocate 100MB for performance

### Caching
- **Lock-free Cache** - 16 shards for concurrent reads
- **LRU Eviction** - Configurable cache size
- **Metadata Cache** - Index metadata in memory
- **Query Cache** - Cache search results
- **TTL Support** - Auto-expire cached entries

### Batch Operations
- **Batch Add** - Add multiple documents in single transaction
- **Parallel Processing** - Concurrent batch processing
- **Single Transaction** - ACID guarantees for batch
- **Error Handling** - Partial success reporting

### Async Processing
- **Background Embedding** - Non-blocking vector indexing
- **TTL Cleanup** - Async expired document removal
- **Webhook Delivery** - Async HTTP callbacks
- **Queue Management** - Configurable queue sizes

### Advanced (Extreme Mode)
Enable with `MDDB_EXTREME=true`:
- **Write-Ahead Log** - WAL with periodic sync
- **Adaptive Compression** - Snappy/Zstd based on size
- **Delta Encoding** - 5-10x smaller revision storage
- **Bloom Filters** - Fast negative lookups (1% FP)
- **Zero-Copy I/O** - Direct memory access
- **Vectorized Operations** - SIMD instructions
- **HTTP/3 + QUIC** - Modern transport protocol

## Extensibility

### Protocol Buffers
- **Shared Definitions** - Single source of truth
- **Code Generation** - Auto-generate clients
- **Version Control** - API contract versioning
- **Type Safety** - Compile-time validation
- **Multi-language** - Go, Python, Node.js, PHP, Java, C++

### Client Libraries
- **Go** - Native client library
- **Python** - Zero-dependency single file
- **PHP** - Zero-dependency single file (PHP 8.0+)
- **Node.js** - npm package
- **Auto-generated** - From protobuf definitions

### Custom MCP Tools
- **YAML Definitions** - Define website-specific tools
- **Preconfigured Defaults** - Semantic search, document search, FTS
- **Prompt Templates** - Customize AI tool descriptions
- **Parameter Validation** - JSON schema validation
- **Dynamic Loading** - Hot reload tool definitions

## Platform Support

### Operating Systems
- **Linux** - Ubuntu, Debian, RHEL, CentOS, Fedora, Arch
- **macOS** - Intel and Apple Silicon
- **FreeBSD** - amd64
- **Windows** - WSL2 recommended
- **Docker** - Any platform with Docker

### Architectures
- **amd64** - Intel/AMD 64-bit
- **arm64** - ARM 64-bit (Apple Silicon, Raspberry Pi 4)
- **armv7** - ARM 32-bit (Raspberry Pi 3)

### Package Formats
- **DEB** - Debian/Ubuntu packages
- **RPM** - RHEL/CentOS/Fedora packages
- **Tarball** - Standalone binaries
- **Docker** - Container images
- **Homebrew** - macOS (coming soon)

## Configuration

### Environment Variables
- `MDDB_DB_PATH` - Database file path
- `MDDB_HTTP_PORT` - HTTP API port (default: 11023)
- `MDDB_GRPC_PORT` - gRPC API port (default: 11024)
- `MDDB_ACCESS_MODE` - read-only, write-only, read-write
- `MDDB_EXTREME` - Enable extreme performance mode
- `MDDB_GRAPHQL_ENABLED` - Enable GraphQL endpoint
- `MDDB_GRAPHQL_PLAYGROUND` - Enable GraphQL Playground
- `MDDB_METRICS` - Enable Prometheus metrics
- `MDDB_EMBEDDING_PROVIDER` - openai, cohere, voyage, ollama
- `MDDB_EMBEDDING_API_KEY` - API key for embedding provider
- `MDDB_EMBEDDING_MODEL` - Model name
- `MDDB_EMBEDDING_DIMENSIONS` - Vector dimensions
- `MDDB_AUTH_ENABLED` - Enable authentication
- `MDDB_JWT_SECRET` - JWT signing secret
- `MDDB_REPLICATION_ROLE` - leader, follower, or empty (standalone)
- `MDDB_REPLICATION_LEADER_ADDR` - Follower: gRPC address of the leader
- `MDDB_BINLOG_ENABLED` - Enable binlog (auto-enabled for leader)
- `MDDB_NODE_ID` - Unique node identifier

### CLI Flags
- `--db` - Database file path
- `--http-port` - HTTP API port
- `--grpc-port` - gRPC API port
- `--access-mode` - Access mode
- `--graphql` - Enable GraphQL
- `--extreme` - Enable extreme mode
- `--help` - Show help

**[← Back to README](../README.md)**
