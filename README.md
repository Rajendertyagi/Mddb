# MDDB - Markdown Database

[![Go Version](https://img.shields.io/badge/Go-1.26-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-green.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/tradik/mddb)](https://github.com/tradik/mddb/releases)
[![Docker](https://img.shields.io/docker/v/tradik/mddb?label=docker)](https://hub.docker.com/r/tradik/mddb)
[![Docker Pulls](https://img.shields.io/docker/pulls/tradik/mddb)](https://hub.docker.com/r/tradik/mddb)
[![Tests](https://github.com/tradik/mddb/workflows/Tests/badge.svg)](https://github.com/tradik/mddb/actions)
[![Performance](https://img.shields.io/badge/performance-optimized-brightgreen.svg)]()
[![gRPC](https://img.shields.io/badge/gRPC-enabled-blue.svg)](https://grpc.io)
[![Protocol Buffers](https://img.shields.io/badge/protobuf-3-blue.svg)](https://protobuf.dev)
[![MCP](https://img.shields.io/badge/MCP-enabled-purple.svg)](https://modelcontextprotocol.io)

**A high-performance, version-controlled markdown database with vector search, full-text search, webhooks, and dual protocol support (HTTP/JSON + gRPC/Protobuf)**

MDDB is a lightweight, embedded database specifically designed for storing and managing markdown documents with rich metadata. Built with Go and BoltDB, it provides blazing-fast document operations with full revision history, semantic vector search, full-text search, document TTL, webhooks, and URL import — making it perfect for content management systems, documentation platforms, knowledge bases, and RAG pipelines.

## 🎯 What is MDDB?

MDDB (Markdown Database) is a specialized database server that treats markdown documents as first-class citizens. Unlike traditional databases that store markdown as plain text, MDDB provides:

- **Native Markdown Support** - Store, version, and query markdown documents with their metadata
- **Dual Protocol APIs** - Choose between HTTP/JSON (easy debugging) or gRPC/Protobuf (high performance)
- **Full Revision History** - Every document update creates a new revision with complete content snapshot
- **Rich Metadata Indexing** - Fast searches using multi-value metadata tags
- **Vector Search** - Semantic similarity search powered by embeddings (OpenAI, Cohere, Voyage AI, Ollama)
- **Full-Text Search** - Built-in inverted index with TF scoring, no external dependencies
- **Document TTL** - Auto-expiring documents with background cleanup (like Redis)
- **Webhooks** - HTTP callbacks on document events with retry logic
- **Import from URL** - Fetch and store markdown from any URL with frontmatter parsing
- **Template Variables** - Dynamic content with variable substitution
- **Multi-language Support** - Store documents in multiple languages with the same key
- **Zero Configuration** - Single binary, embedded database, no external dependencies

## 🚀 Why MDDB?

MDDB is purpose-built for markdown document management. Here's how it compares to alternatives:

### vs Traditional Databases (PostgreSQL, MySQL)
- **Specialized for Markdown** - Native support with metadata indexing
- **Embedded** - No separate database server to manage
- **Built-in Versioning** - Automatic revision history
- **Simpler Deployment** - Single binary, ~15MB Docker image
- **Lower Resource Usage** - Minimal memory footprint

### vs Document Databases (MongoDB, CouchDB)
- **Markdown-First Design** - Purpose-built for markdown workflows
- **Dual Protocol** - HTTP/JSON for debugging, gRPC for performance
- **Smaller Footprint** - Embedded BoltDB, no separate server
- **Type-Safe gRPC** - Compile-time validation
- **Simpler Operations** - No sharding or replication complexity

### vs File-Based Systems (Git, Filesystem)
- **Instant Queries** - Indexed metadata searches
- **API Access** - REST + gRPC interfaces
- **Concurrent Access** - ACID transactions
- **Rich Metadata** - Structured tags and filtering
- **Better Performance** - Optimized for document operations

### vs CMS Platforms (WordPress, Strapi)
- **Lightweight** - Minimal installation size
- **API-First** - No admin UI overhead (optional web panel available)
- **Version Control** - Built-in revision tracking
- **Developer-Friendly** - Simple, well-documented API

## 💡 Use Cases

### 1. **Documentation Platforms**
```bash
# Store API documentation with versioning
mddb-cli add api-docs authentication en_US -f auth.md -m "version=2.0,status=published"
mddb-cli search api-docs -f "status=published" --sort updatedAt
```
**Perfect for**: Technical documentation, API references, knowledge bases

### 2. **Content Management Systems**
```bash
# Multi-language blog posts with metadata
mddb-cli add blog "getting-started" en_US -f post-en.md -m "author=John,tags=tutorial|beginner"
mddb-cli add blog "getting-started" pl_PL -f post-pl.md -m "author=John,tags=tutorial|beginner"
```
**Perfect for**: Blogs, news sites, multi-language content

### 3. **Configuration Management**
```bash
# Store configuration templates with variables
mddb-cli add configs nginx-prod en_US -f nginx.conf.md -m "env=production,service=web"
# Variables like {{domain}} are substituted on retrieval
```
**Perfect for**: Infrastructure configs, deployment templates

### 4. **Knowledge Bases**
```bash
# Searchable documentation with rich metadata
mddb-cli add kb troubleshooting en_US -f guide.md -m "category=support,difficulty=advanced,tags=network|vpn"
mddb-cli search kb -f "category=support,difficulty=beginner"
```
**Perfect for**: Internal wikis, support documentation, FAQs

### 5. **Microservices Communication**
```go
// High-performance gRPC for service-to-service communication
client := mddb.NewMDDBClient(conn)
doc, _ := client.Get(ctx, &mddb.GetRequest{
    Collection: "templates",
    Key: "email-welcome",
    Lang: "en_US",
})
```
**Perfect for**: Template storage, shared content, configuration distribution

### 6. **RAG / AI Knowledge Base**
```bash
# Documents are automatically embedded in the background
mddb-cli add kb faq-billing en_US -f billing.md -m "category=billing"

# Import knowledge directly from URLs
curl -X POST http://localhost:11023/v1/import-url \
  -d '{"collection": "kb", "url": "https://example.com/docs/faq.md", "lang": "en_US"}'

# Semantic search - find relevant docs by meaning, not keywords
curl -X POST http://localhost:11023/v1/vector-search \
  -d '{"collection": "kb", "query": "how do I cancel my subscription?", "topK": 5, "includeContent": true}'

# Full-text search - keyword-based search with TF scoring
curl -X POST http://localhost:11023/v1/fts \
  -d '{"collection": "kb", "query": "billing refund policy", "limit": 10}'
```
**Perfect for**: RAG pipelines, AI assistants, semantic search, chatbot knowledge bases

### 8. **Event-Driven Architecture with Webhooks**
```bash
# Register webhook for new documents
curl -X POST http://localhost:11023/v1/webhooks \
  -d '{"url": "https://your-app.com/on-doc-change", "events": ["doc.added", "doc.updated"], "collection": "blog"}'

# Every add/update/delete triggers HTTP callback with retry
# Payload: { event, collection, key, lang, timestamp, document }
```
**Perfect for**: Real-time notifications, search index sync, CDN invalidation

### 9. **Temporary/Cached Content with TTL**
```bash
# Add a document that expires in 1 hour (3600 seconds)
curl -X POST http://localhost:11023/v1/add \
  -d '{"collection": "cache", "key": "session-data", "lang": "en", "ttl": 3600, "contentMd": "# Temporary content"}'

# Set or update TTL on existing document (0 = remove TTL)
curl -X POST http://localhost:11023/v1/set-ttl \
  -d '{"collection": "cache", "key": "session-data", "lang": "en", "ttl": 7200}'
```
**Perfect for**: Cache layers, session data, temporary content, auto-cleanup

### 7. **Version-Controlled Content**
```bash
# Track all changes with full history
mddb-cli add docs readme en_US -f README.md -m "version=1.0"
# Update creates new revision
mddb-cli add docs readme en_US -f README-v2.md -m "version=2.0"
# Access any revision through API
```
**Perfect for**: Legal documents, compliance, audit trails

### 10. **GraphQL API**

MDDB provides a GraphQL API alongside REST for flexible, modern data fetching.

**Enabling GraphQL:**
```bash
# Via environment variable
export MDDB_GRAPHQL_ENABLED=true
mddbd

# Via CLI flag
mddbd --graphql

# With Docker
docker run -e MDDB_GRAPHQL_ENABLED=true tradik/mddb
```

**Endpoints:**
- GraphQL API: `POST /graphql`
- GraphQL Playground: `GET /playground` (interactive development tool)

**Example Query:**
```graphql
query {
  document(collection: "blog", key: "hello-world", lang: "en") {
    id
    key
    contentMd
    meta
    addedAt
  }
}
```

**Example Mutation:**
```graphql
mutation {
  addDocument(input: {
    collection: "blog"
    key: "new-post"
    lang: "en"
    meta: [
      { key: "author", values: ["John"] }
      { key: "tags", values: ["tutorial", "graphql"] }
    ]
    contentMd: "# Hello GraphQL\n\nModern API for flexible queries."
  }) {
    id
    addedAt
  }
}
```

**Vector Search via GraphQL:**
```graphql
query {
  vectorSearch(input: {
    collection: "kb"
    query: "how to configure authentication?"
    topK: 5
    includeContent: true
  }) {
    results {
      document { key contentMd }
      score
      rank
    }
  }
}
```

**Authentication:**
```bash
# Login and get JWT token
curl -X POST http://localhost:11023/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "mutation { login(username: \"admin\", password: \"secret\") { token expiresAt } }"}'

# Query with authentication
curl -X POST http://localhost:11023/graphql \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ me { username admin } }"}'
```

**GraphQL vs REST:**
- **Use GraphQL** for: flexible field selection, combining multiple queries, modern frontends (React, Vue)
- **Use REST** for: simple curl/wget access, scripts, streaming responses (export/backup)

**Perfect for**: Modern web applications, mobile apps, flexible data requirements

### API Key Management

MDDB supports API keys for programmatic access alongside JWT tokens. API keys are ideal for:
- **Long-running scripts** - No token expiry management needed
- **CI/CD pipelines** - Stable credentials for automation
- **Third-party integrations** - Distribute without sharing passwords
- **Service accounts** - Non-interactive authentication

**Creating an API Key (CLI):**
```bash
# Login first to get JWT token
mddb-cli login admin secret

# Create an API key (requires JWT authentication)
mddb-cli api-key create --description "Production deployment" --token $TOKEN

# Output:
# ✓ API Key created successfully
# Key:         mddb_live_abc123...
# Description: Production deployment
# Created:     2026-03-02T15:30:00Z
# Expires:     Never
#
# ⚠️  IMPORTANT: Save this key now! You won't be able to see it again.
# Use with: mddb-cli --api-key mddb_live_abc123... <command>
```

**Creating an API Key (HTTP):**
```bash
# Login and get JWT token
TOKEN=$(curl -s -X POST http://localhost:11023/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"secret"}' | jq -r .token)

# Create API key with JWT
curl -X POST http://localhost:11023/v1/auth/api-key \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"description":"CI/CD pipeline","expiresAt":0}'

# Response:
# {
#   "key": "mddb_live_xyz789...",
#   "description": "CI/CD pipeline",
#   "createdAt": 1709394600,
#   "expiresAt": 0
# }
```

**Using an API Key:**
```bash
# With CLI
mddb-cli --api-key mddb_live_xyz789... search blog -f "status=published"

# With HTTP
curl -H "X-API-Key: mddb_live_xyz789..." http://localhost:11023/v1/search \
  -H "Content-Type: application/json" \
  -d '{"collection":"blog","filterMeta":{"status":["published"]}}'
```

**Listing API Keys (CLI):**
```bash
mddb-cli api-key list --token $TOKEN

# Output:
# Your API Keys (2 total)
# ═══════════════════════════════════════
#
# 1. Key Hash: abc123...
#    Description: Production deployment
#    Created: 2026-03-02T15:30:00Z
#    Expires: Never
#    Delete with: mddb-cli api-key delete abc123... --token <token>
#
# 2. Key Hash: xyz789...
#    Description: CI/CD pipeline
#    Created: 2026-03-02T16:45:00Z
#    Expires: Never
```

**Listing API Keys (HTTP):**
```bash
curl -H "Authorization: Bearer $TOKEN" http://localhost:11023/v1/auth/api-keys

# Response:
# {
#   "keys": [
#     {
#       "keyHash": "abc123...",
#       "description": "Production deployment",
#       "createdAt": 1709394600,
#       "expiresAt": 0
#     }
#   ]
# }
```

**Deleting an API Key (CLI):**
```bash
mddb-cli api-key delete abc123... --token $TOKEN

# Output:
# ✓ API key deleted: abc123...
```

**Deleting an API Key (HTTP):**
```bash
curl -X DELETE -H "Authorization: Bearer $TOKEN" \
  http://localhost:11023/v1/auth/api-keys/abc123...

# Response:
# {"status":"deleted"}
```

**API Key Security:**
- API keys are hashed with SHA256 before storage (only you see the full key once)
- Users can only list/delete their own API keys
- API keys work with all HTTP and gRPC endpoints
- Optional expiry timestamp for temporary keys
- Revoke access instantly by deleting the key

## ⚡ Performance

MDDB is designed for high-performance document operations with multiple optimization strategies:

**Benchmark Results (3000 documents):**

| Database | Throughput | Avg Latency |
|----------|------------|-------------|
| **MDDB (Batch API)** | **29,810 docs/s** | **34µs** |
| MongoDB | 5,176 docs/s | 192µs |
| PostgreSQL | 4,324 docs/s | 231µs |
| MySQL | 1,214 docs/s | 822µs |
| CouchDB | 312 docs/s | 3,185µs |

### Key Performance Features

**Protocol & Storage:**
- Binary protocol (Protobuf) reduces payload size by ~70%
- Embedded BoltDB eliminates network overhead
- Batch operations use single transactions
- HTTP/2 multiplexing via gRPC

**Optimization Techniques:**
- Lock-free concurrent reads with sharded cache
- Optional revision history (save only when needed)
- Lazy metadata indexing with async queue
- Bloom filters for fast negative lookups
- Delta encoding for smaller revision storage
- Adaptive compression (Snappy/Zstd)

**Advanced Features (Extreme Mode):**
Enable with `MDDB_EXTREME=true` environment variable:
- Write-Ahead Log (WAL) with periodic sync
- MVCC snapshot isolation
- HTTP/3 + QUIC support
- Zero-copy I/O operations
- Vectorized operations (SIMD)

See [Performance Tests](test/README.md) for detailed benchmarks.

## 🎯 Key Features

### Core Functionality
- **Document Management** - Add, update, retrieve markdown with metadata
- **Revision History** - Every update creates a new revision with full content
- **Metadata Search** - Fast indexed search with multi-value tags
- **Vector Search** - Semantic similarity search with auto-generated embeddings (OpenAI, Ollama, Voyage AI)
- **Full-Text Search** - Built-in inverted index with TF scoring and stop word filtering
- **Document TTL** - Time-to-live with automatic cleanup (0 = permanent, >0 = seconds until expiry)
- **Schema Validation** - Per-collection JSON Schema validation for metadata (opt-in, disabled by default)
- **Webhooks** - HTTP callbacks on `doc.added`, `doc.updated`, `doc.deleted` events with 3x retry
- **Import from URL** - Fetch markdown from URLs with automatic frontmatter parsing and key derivation
- **Telemetry** - Prometheus-compatible `/metrics` endpoint with request counters, latency histograms, and DB stats
- **Multi-language** - Store same document in multiple languages
- **Template Variables** - Dynamic content with `{{variable}}` substitution
- **Collections** - Organize documents into logical groups

### APIs & Protocols
- **Dual Protocol Support** - HTTP/JSON and gRPC/Protobuf simultaneously
- **RESTful HTTP API** - Easy debugging with curl, Postman
- **High-Performance gRPC** - 16x faster, 70% smaller payload
- **gRPC Reflection** - Use grpcurl for debugging
- **CLI Client** - Full-featured command-line interface
- **Web Admin Panel** - Modern React-based UI for browsing and managing data
- **MCP Server** - Model Context Protocol for LLM integration (gRPC + REST fallback)
- **Custom MCP Tools** - YAML-defined website-specific AI tools with preconfigured defaults

### Operations
- **Export** - NDJSON or ZIP formats with filtering
- **Backup/Restore** - Full database backup and restore
- **Truncate** - Remove old revisions to save space
- **Statistics** - Real-time server and database metrics
- **Access Modes** - Read-only, write-only, or read-write
- **Bulk Import** - Load entire folders of markdown files

### Developer Experience
- **Single Binary** - No external dependencies
- **Docker Support** - 15MB Alpine Linux images
- **Hot Reload** - Development mode with automatic restart
- **Monorepo Structure** - Shared protobuf definitions
- **Multi-language Clients** - Generated code for Go, Python, Node.js, PHP
- **Web Admin Panel** - Visual interface for data management
- **Comprehensive Docs** - API reference, examples, guides

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────┐
│              Client Applications                    │
├──────────────┬──────────────┬──────────┬────────────┤
│  HTTP/JSON   │ gRPC/Protobuf│ HTTP/3   │ MCP Server │
│  Port 11023  │  Port 11024  │ 11443    │ Port 9000  │
├──────────────┴──────────────┴──────────┴────────────┤
│              MDDB Server (Go)                       │
│  ┌─────────────────────────────────────────────┐   │
│  │ Performance Layer (Extreme Mode)            │   │
│  │ - WAL (Write-Ahead Log)                     │   │
│  │ - MVCC (Snapshot Isolation)                 │   │
│  │ - Lock-Free Cache (16 shards)               │   │
│  │ - Bloom Filters (1% FP)                     │   │
│  │ - Adaptive Compression (Snappy/Zstd)        │   │
│  │ - Delta Encoding (5-10x smaller)            │   │
│  │ - Adaptive Indexing                         │   │
│  │ - Async I/O                                 │   │
│  │ - Zero-Copy I/O                             │   │
│  │ - Vectorized Operations (SIMD)              │   │
│  │ - Distributed Sharding (4 shards)           │   │
│  └─────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────┐   │
│  │ Core Layer                                  │   │
│  │ - Request Handling                          │   │
│  │ - Batch Processing (parallel)               │   │
│  │ - Metadata Indexing (lazy)                  │   │
│  │ - Template Processing                       │   │
│  │ - Revision Management                       │   │
│  └─────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────┤
│           BoltDB (Embedded Storage)                 │
│  - ACID Transactions                                │
│  - B+Tree Storage                                   │
│  - Single File Database                             │
│  - Optimized: NoFreelistSync, 100MB initial mmap    │
└─────────────────────────────────────────────────────┘
```

### Extreme Performance Mode

Enable with `MDDB_EXTREME=true` environment variable to activate all 29 optimizations.

## Quick Start

### 🐳 Docker (Recommended)

```bash
# Pull and run the latest version
docker run -d \
  --name mddb \
  -p 11023:11023 \
  -p 11024:11024 \
  -v mddb-data:/data \
  -e MDDB_EXTREME=true \
  tradik/mddb:latest

# Or use Docker Compose
curl -O https://raw.githubusercontent.com/tradik/mddb/main/docker-compose.yml
docker-compose up -d
```

**Docker Hub**: https://hub.docker.com/r/tradik/mddb

### Installation

#### Ubuntu/Debian
```bash
# Server
wget https://github.com/tradik/mddb/releases/latest/download/mddbd-latest-linux-amd64.deb
sudo dpkg -i mddbd-latest-linux-amd64.deb
sudo systemctl start mddbd
sudo systemctl enable mddbd

# Client
wget https://github.com/tradik/mddb/releases/latest/download/mddb-cli-latest-linux-amd64.deb
sudo dpkg -i mddb-cli-latest-linux-amd64.deb
```

#### RHEL/CentOS/Fedora
```bash
# Server
wget https://github.com/tradik/mddb/releases/latest/download/mddbd-latest-linux-amd64.rpm
sudo rpm -i mddbd-latest-linux-amd64.rpm
sudo systemctl start mddbd
sudo systemctl enable mddbd

# Client
wget https://github.com/tradik/mddb/releases/latest/download/mddb-cli-latest-linux-amd64.rpm
sudo rpm -i mddb-cli-latest-linux-amd64.rpm
```

#### macOS (Homebrew)
```bash
# Coming soon - Homebrew tap
brew tap tradik/mddb
brew install mddbd mddb-cli

# Or download directly
# Intel Mac
wget https://github.com/tradik/mddb/releases/latest/download/mddbd-latest-darwin-amd64.tar.gz
tar xzf mddbd-latest-darwin-amd64.tar.gz
sudo mv mddbd-latest-darwin-amd64/mddbd /usr/local/bin/

# Apple Silicon
wget https://github.com/tradik/mddb/releases/latest/download/mddbd-latest-darwin-arm64.tar.gz
tar xzf mddbd-latest-darwin-arm64.tar.gz
sudo mv mddbd-latest-darwin-arm64/mddbd /usr/local/bin/
```

#### FreeBSD
```bash
wget https://github.com/tradik/mddb/releases/latest/download/mddbd-latest-freebsd-amd64.tar.gz
tar xzf mddbd-latest-freebsd-amd64.tar.gz
sudo mv mddbd-latest-freebsd-amd64/mddbd /usr/local/bin/
```

### Building from Source

#### Prerequisites
- Go 1.26 or later
- Make (optional, for using Makefile commands)

```bash
# Clone the repository
git clone https://github.com/tradik/mddb.git
cd mddb

# Build the project
make build

# Or build manually
cd services/mddbd
go build -o mddbd .
```

### Running

```bash
# Run with default settings
make run

# Run in production mode
make run-prod

# Run in development mode with hot reload (requires air)
make install-dev-tools
make dev

# Generate gRPC code (if you modify proto files)
make install-grpc-tools
make generate-proto
```

### 🤖 MCP Server (AI/LLM Integration)

MDDB includes a Model Context Protocol (MCP) server for seamless integration with AI tools like Windsurf, Claude Desktop, and other LLM applications.

**Features:**
- Dual mode: HTTP server + stdio mode for IDE integration
- Full MDDB API access through MCP tools and resources
- Custom YAML tools: define website-specific tools with preconfigured defaults
- Docker ready with single image, mode selection via env var

**Quick Start with Docker:**
```bash
# Pull MCP image (uses same version as main MDDB)
docker pull tradik/mddb:mcp-2.3.3

# For Windsurf/Claude Desktop (stdio mode)
docker run -i --rm \
  -e MCP_MODE=stdio \
  -e MDDB_GRPC_ADDRESS=localhost:11024 \
  -e MDDB_REST_BASE_URL=http://localhost:11023 \
  tradik/mddb:mcp-2.3.3

# For HTTP mode
docker run -d -p 9000:9000 \
  -e MCP_MODE=http \
  -e MDDB_GRPC_ADDRESS=localhost:11024 \
  -e MDDB_REST_BASE_URL=http://localhost:11023 \
  tradik/mddb:mcp-2.3.3
```

**Documentation:**
- [MCP Server README](services/mddb-mcp/README.md)
- [Custom MCP Tools Guide](docs/CUSTOM-TOOLS.md)
- [Windsurf Setup Guide](services/mddb-mcp/WINDSURF_SETUP.md)
- [WSL Setup Guide](services/mddb-mcp/WSL_SETUP.md) (Windows)
- [MCP Configuration Examples](services/mddb-mcp/)

---

**Ports:**
- HTTP API: `localhost:11023`
- gRPC API: `localhost:11024`
- HTTP/3 (QUIC): `localhost:11443` (Extreme Mode only)

### Docker

```bash
# Production
make docker-up

# Development (with hot reload)
make docker-up-dev

# View logs
make docker-logs

# Stop
make docker-down
```

**Image size**: ~15 MB (Alpine Linux)

### CLI Client

```bash
# Build and install CLI client
make build-cli
make install-all

# Use the CLI
mddb-cli add blog hello en_US -f post.md
mddb-cli get blog hello en_US
mddb-cli search blog

# View manual
man mddb-cli
```

### Available Make Commands

Run `make help` to see all available commands:

```bash
make help          # Show all available commands
make build         # Build the Go service
make build-cli     # Build CLI client
make install-all   # Install CLI and man page
make test          # Run tests
make fmt           # Format code
make lint          # Run linter
make clean         # Clean build artifacts
make tidy          # Tidy Go modules
```

## 📚 Documentation

**🌐 [Visit the Official Website](https://tradik.github.io/mddb/)** - Complete documentation, downloads, and examples

- **[Quick Start Guide](docs/QUICKSTART.md)** - Get started in 5 minutes
- **[API Documentation](docs/API.md)** - Complete HTTP/JSON API reference
- **[OpenAPI/Swagger Spec](docs/openapi.yaml)** - Machine-readable API specification
- **[Swagger UI](docs/swagger.html)** - Interactive API documentation
- **[Health Check Guide](docs/HEALTHCHECK.md)** - Health checks for Docker and Kubernetes
- **[gRPC Documentation](docs/GRPC.md)** - High-performance gRPC API guide
- **[Embedding Providers Guide](docs/EMBEDDING_PROVIDERS.md)** - Vector search with OpenAI, Cohere, Voyage AI, and Ollama
- **[Web Panel Guide](docs/PANEL.md)** - Web admin interface documentation
- **[MCP Server Guide](services/mddb-mcp/README.md)** - Model Context Protocol server for LLM integration
- **[Custom MCP Tools](docs/CUSTOM-TOOLS.md)** - YAML-defined website-specific AI tools
- **[Bulk Import Guide](docs/BULK-IMPORT.md)** - Import markdown files from folders
- **[Docker Guide](docs/DOCKER.md)** - Docker deployment with Alpine Linux
- **[Usage Examples](docs/EXAMPLES.md)** - Code examples and patterns
- **[Architecture Guide](docs/ARCHITECTURE.md)** - System design and internals
- **[Deployment Guide](docs/DEPLOYMENT.md)** - Production deployment instructions

## 🎨 Example Workflows

### Blog Platform
```bash
# Add a blog post with tags
echo "# Getting Started with MDDB" | mddb-cli add blog intro en_US \
  -m "author=Jane,tags=tutorial|beginner,status=published"

# Search published posts
mddb-cli search blog -f "status=published" --sort updatedAt

# Export all blog posts
curl "http://localhost:11023/v1/export?collection=blog&format=zip" -o blog-backup.zip
```

### API Documentation
```bash
# Store versioned API docs
mddb-cli add api-docs auth-v2 en_US -f authentication.md \
  -m "version=2.0,endpoint=/api/auth,method=POST"

# Quick search by endpoint
mddb-cli search api-docs -f "endpoint=/api/auth"
```

### Multi-language Content
```bash
# Same key, different languages
mddb-cli add products laptop-x1 en_US -f laptop-en.md -m "category=electronics,price=999"
mddb-cli add products laptop-x1 pl_PL -f laptop-pl.md -m "category=electronics,price=999"
mddb-cli add products laptop-x1 de_DE -f laptop-de.md -m "category=electronics,price=999"

# Retrieve in user's language
mddb-cli get products laptop-x1 pl_PL
```

## 🔧 Technical Details

### Storage Engine
- **BoltDB** - Embedded key-value store (single file)
- **Prefix Indices** - Fast metadata queries using composite keys
- **ACID Transactions** - Guaranteed data consistency
- **Efficient Storage** - Optimized bucket structure for performance

### API Endpoints

**Core:**
- `GET /health` - Health check endpoint
- `GET /v1/stats` - Server and database statistics
- `POST /v1/add` - Add or update documents (supports `ttl` field)
- `POST /v1/get` - Retrieve documents with template support
- `POST /v1/search` - Search with metadata filters and sorting
- `POST /v1/delete` - Delete a document
- `POST /v1/delete-collection` - Delete entire collection

**Search:**
- `POST /v1/vector-search` - Semantic vector similarity search
- `POST /v1/vector-reindex` - Re-embed documents in a collection
- `GET /v1/vector-stats` - Embedding/vector statistics
- `POST /v1/fts` - Full-text search with TF scoring

**Import & Export:**
- `POST /v1/import-url` - Import markdown from URL with frontmatter parsing
- `POST /v1/export` - Export as NDJSON or ZIP
- `GET /v1/backup` - Create database backup
- `POST /v1/restore` - Restore from backup
- `POST /v1/truncate` - Clean up old revisions

**TTL:**
- `POST /v1/set-ttl` - Set or remove time-to-live on a document

**Webhooks:**
- `POST /v1/webhooks` - Register a webhook
- `GET /v1/webhooks` - List all webhooks
- `POST /v1/webhooks/delete` - Delete a webhook

**Interactive API Documentation:** Open [docs/swagger.html](docs/swagger.html) in your browser for full API documentation with try-it-out functionality.

### Extensions & Integrations
- **Webhooks** - HTTP callbacks on `doc.added`, `doc.updated`, `doc.deleted` with exponential backoff retry
- **MCP Server** - Model Context Protocol for LLM integration (gRPC + REST fallback)
- **Custom MCP Tools** - YAML-defined website-specific tools with defaults (semantic_search, search_documents, full_text_search)
- **PHP Client** - Single-file client, zero dependencies, PHP 8.0+
- **Python Client** - Single-file client, zero dependencies, Python 3.8+
- **Configurable** - Environment-based configuration

### Command-Line Client
- **mddb-cli** - Full-featured CLI client similar to mysql-client
- **Man Page** - Complete Unix-style manual page
- **Bash Completion** - Tab completion support (future)
- **Piping Support** - Works seamlessly with Unix pipes

## 🚀 Quick Examples

### Add a Document
```bash
curl -X POST http://localhost:11023/v1/add \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "key": "hello-world",
    "lang": "en_US",
    "meta": {"category": ["blog"], "author": ["John Doe"]},
    "contentMd": "# Hello World\n\nWelcome to MDDB!"
  }'
```

### Get Document with Template Variables
```bash
curl -X POST http://localhost:11023/v1/get \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "key": "homepage",
    "lang": "en_GB",
    "env": {"year": "2024", "siteName": "My Blog"}
  }'
```

### Search with Filters
```bash
curl -X POST http://localhost:11023/v1/search \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "filterMeta": {"category": ["blog"]},
    "sort": "addedAt",
    "asc": false,
    "limit": 10
  }'
```

### Export as NDJSON
```bash
curl -X POST http://localhost:11023/v1/export \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "filterMeta": {"category": ["blog"]},
    "format": "ndjson"
  }' > export.ndjson
```

### Backup Database
```bash
curl "http://localhost:11023/v1/backup?to=backup-$(date +%s).db"
```

### Truncate Old Revisions
```bash
curl -X POST http://localhost:11023/v1/truncate \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "keepRevs": 3,
    "dropCache": true
  }'
```

### Vector Search
```bash
curl -X POST http://localhost:11023/v1/vector-search \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "query": "getting started with databases",
    "topK": 5,
    "threshold": 0.7,
    "includeContent": true
  }'
```

### Full-Text Search
```bash
curl -X POST http://localhost:11023/v1/fts \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "query": "markdown database tutorial",
    "limit": 10
  }'
```

### Import from URL
```bash
curl -X POST http://localhost:11023/v1/import-url \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "docs",
    "url": "https://raw.githubusercontent.com/tradik/mddb/main/README.md",
    "lang": "en_US"
  }'
```

### Using CLI Client

```bash
# Add document from file
mddb-cli add blog hello en_US -f post.md -m "category=blog,author=John"

# Get document
mddb-cli get blog hello en_US

# Search with filter
mddb-cli search blog -f "category=blog" -l 10

# Full-text search
mddb-cli fts blog --query="database tutorial" --limit=10

# Import from URL
mddb-cli import-url docs https://example.com/guide.md en_US

# Set TTL (expires in 1 hour)
mddb-cli set-ttl cache temp-key en --ttl=3600

# Manage webhooks
mddb-cli webhook register --url=https://your-app.com/hook --events=doc.added,doc.updated
mddb-cli webhook list
mddb-cli webhook delete <webhook-id>

# Export to file
mddb-cli export blog -o backup.ndjson

# Create backup
mddb-cli backup daily-backup.db

# Show statistics
mddb-cli stats
```

### Using Web Admin Panel

```bash
# Install dependencies
make panel-install

# Run in development mode
make panel-dev

# Or use Docker Compose
docker-compose up -d

# Access panel at http://localhost:3000
```

**Features:**
- 📊 Server statistics dashboard
- 📁 Browse collections and documents
- 🔍 Advanced filtering by metadata
- 📄 View document content and metadata
- ✏️ Edit documents with live markdown preview
- ➕ Create new documents
- 📝 Split-view markdown editor (edit/preview/both)
- 🎨 Markdown toolbar with formatting buttons
- 💡 Syntax highlighting for code blocks
- 📋 Pre-built templates (blog, docs, README, API)
- 📋 Copy markdown content
- 🎨 Modern, responsive UI

### Bulk Import from Folder

```bash
# Load all .md files from a folder
./scripts/load-md-folder.sh ./docs blog

# Load recursively with custom language
./scripts/load-md-folder.sh ./content articles -r -l pl_PL

# Add custom metadata to all files
./scripts/load-md-folder.sh ./posts blog -m "author=John Doe" -m "status=published"

# Dry run to preview what would be imported
./scripts/load-md-folder.sh ./docs blog -d

# Verbose output with progress
./scripts/load-md-folder.sh ./docs blog -v
```

The folder loader script automatically:
- Generates unique keys from filenames
- Extracts frontmatter metadata (YAML-style)
- Supports recursive folder scanning
- Shows progress with statistics
- Handles errors gracefully

## 🐘 PHP Client

Single-file client with zero external dependencies. Requires PHP 8.0+ with `curl` extension.

```php
<?php
require_once 'mddb.php';

// Connect in read-write mode
$db = mddb::connect('localhost:11023', 'write');

// Add a document with TTL (expires in 1 hour)
$db->collection('blog')->add('hello', 'en_US', ['category' => ['tutorial']], '# Hello World');

// Search by metadata
$results = $db->collection('blog')->search('category', 'tutorial');

// Full-text search
$results = $db->collection('blog')->ftsSearch('hello world', 50);

// Vector/semantic search
$results = $db->collection('blog')->vectorSearch('getting started guide', 5, 0.7, true);

// Import from URL
$doc = $db->collection('blog')->importUrl('https://example.com/post.md', 'en_US');

// Webhooks
$db->registerWebhook('https://your-app.com/hook', ['doc.added', 'doc.updated'], 'blog');
$hooks = $db->listWebhooks();

// TTL management
$db->collection('cache')->setTtl('temp-key', 'en', 3600);
```

**Location:** [services/php-extension/mddb.php](services/php-extension/mddb.php)

## 🐍 Python Client

Single-file client with zero external dependencies. Uses only Python stdlib (`urllib`, `json`). Requires Python 3.8+.

```python
from mddb import MDDB

# Connect in read-write mode
db = MDDB.connect('localhost:11023', 'write')
db = db.collection('blog')

# Add a document
db.add('hello', 'en_US', {'category': ['tutorial']}, '# Hello World')

# Search by metadata
results = db.search('category', 'tutorial')

# Full-text search
results = db.fts_search('hello world', limit=50)

# Vector/semantic search
results = db.vector_search('getting started guide', top_k=5, threshold=0.7, include_content=True)

# Import from URL
doc = db.import_url('https://example.com/post.md', 'en_US')

# Webhooks
db.register_webhook('https://your-app.com/hook', ['doc.added', 'doc.updated'], 'blog')
hooks = db.list_webhooks()

# TTL management
db.set_ttl('temp-key', 'en', 3600)

# Server operations
stats = db.stats()
db.backup('daily.db')
```

**Location:** [services/python-extension/mddb.py](services/python-extension/mddb.py)

## 🔍 Vector Search & RAG

MDDB includes built-in vector search powered by embedding providers (OpenAI, Cohere, Voyage AI, Ollama). Documents are automatically embedded in the background when added.

### Configuration

```bash
# Environment variables for embedding
export MDDB_EMBEDDING_PROVIDER=openai      # openai, cohere, voyage, or ollama
export MDDB_EMBEDDING_API_KEY=sk-...       # API key (OpenAI/Cohere/Voyage)
export MDDB_EMBEDDING_MODEL=text-embedding-3-small
export MDDB_EMBEDDING_DIMENSIONS=1536

# For Cohere (best multilingual support):
# export MDDB_EMBEDDING_PROVIDER=cohere
# export MDDB_EMBEDDING_API_KEY=cohere_api_key...
# export MDDB_EMBEDDING_MODEL=embed-english-v3.0

# For Ollama (local, free):
# export MDDB_EMBEDDING_PROVIDER=ollama
# export MDDB_EMBEDDING_API_URL=http://localhost:11434
# export MDDB_EMBEDDING_MODEL=nomic-embed-text
```

### Semantic Search Examples

```bash
# Add documents (embeddings generated automatically in background)
curl -X POST http://localhost:11023/v1/add \
  -d '{"collection": "kb", "key": "billing-faq", "lang": "en_US",
       "meta": {"category": ["billing"]},
       "contentMd": "# Billing FAQ\n\nTo cancel your subscription, go to Settings > Billing > Cancel."}'

# Semantic search - finds docs by meaning, not exact keywords
curl -X POST http://localhost:11023/v1/vector-search \
  -d '{"collection": "kb", "query": "how do I stop paying?", "topK": 5, "threshold": 0.7, "includeContent": true}'

# Filter by metadata during vector search
curl -X POST http://localhost:11023/v1/vector-search \
  -d '{"collection": "kb", "query": "refund process", "topK": 3, "filterMeta": {"category": ["billing"]}}'

# Re-index embeddings after changing provider/model
curl -X POST http://localhost:11023/v1/vector-reindex \
  -d '{"collection": "kb", "force": true}'

# Check embedding statistics
curl http://localhost:11023/v1/vector-stats
```

### RAG Pipeline Example

```python
from mddb import MDDB
import openai

db = MDDB.connect('localhost:11023', 'read').collection('kb')

# Step 1: Retrieve relevant context via semantic search
results = db.vector_search(
    query="how to cancel subscription",
    top_k=3,
    threshold=0.7,
    include_content=True
)

# Step 2: Build context from retrieved documents
context = "\n\n".join([r["document"]["contentMd"] for r in results["results"]])

# Step 3: Send to LLM with retrieved context
response = openai.chat.completions.create(
    model="gpt-4o",
    messages=[
        {"role": "system", "content": f"Answer based on this context:\n\n{context}"},
        {"role": "user", "content": "How do I cancel my subscription?"}
    ]
)
print(response.choices[0].message.content)
```

> **Full RAG guide**: See [docs/RAG-PIPELINE.md](docs/RAG-PIPELINE.md) for a complete WordPress → MDDB → LLM pipeline with diagrams.

### CLI Vector Search

```bash
# Search using CLI (coming in future CLI update)
mddb-cli search kb -f "category=billing"

# Use full-text search as a complement to vector search
mddb-cli fts kb --query="cancel subscription" --limit=5
```

## 📊 Telemetry & Monitoring

MDDB exposes a Prometheus-compatible `GET /metrics` endpoint (enabled by default):

```bash
curl http://localhost:11023/metrics
```

Available metrics:
- `mddb_http_requests_total` - request counter (method, path, status)
- `mddb_http_request_duration_seconds` - latency histogram
- `mddb_documents_total` / `mddb_revisions_total` - per-collection counts
- `mddb_database_size_bytes` - database file size
- `mddb_vector_embeddings_total` / `mddb_embedding_queue_size` - vector status
- `go_goroutines` / `go_memstats_*` - Go runtime

Set `MDDB_METRICS=false` to disable.

> **Full guide**: See [docs/TELEMETRY.md](docs/TELEMETRY.md) for Prometheus config, Grafana dashboards, alerting rules, and Kubernetes setup.

## 🗺️ Roadmap

### Implemented Features ✅
- **Authentication** - JWT tokens and API keys with bcrypt password hashing
- **Authorization** - Collection-level RBAC with Read/Write/Admin permissions
- **User Management** - Multi-user support with admin roles
- **Group-Based Permissions** - Organize users into groups with inherited permissions
- **GraphQL API** - Modern GraphQL endpoint with full CRUD operations and authentication directives
- ~~**Schema Validation** - JSON Schema validation for metadata~~ (Implemented in v2.3.1)

### Planned Features
- **Streaming Export** - Memory-efficient ZIP export
- **Replication** - Built-in replication support
- **Plugins** - Plugin system for custom extensions

## 📁 Monorepo Structure

```
mddb/
├── proto/                    # Shared Protocol Buffer definitions
│   ├── mddb.proto           # Main service definition
│   ├── generate.sh          # Code generation for all languages
│   └── README.md
├── services/
│   ├── mddbd/               # Go server
│   │   ├── main.go
│   │   ├── grpc_server.go
│   │   └── proto/           # Generated Go code
│   ├── mddb-cli/            # CLI client
│   ├── mddb-mcp/            # MCP server (Model Context Protocol)
│   │   ├── cmd/             # HTTP + stdio binaries
│   │   ├── internal/        # MCP implementation
│   │   ├── Dockerfile       # Multi-mode Docker build
│   │   └── README.md        # MCP documentation
│   ├── mddb-panel/          # Web admin panel (React)
│   │   ├── src/             # React components
│   │   ├── public/          # Static assets
│   │   └── Dockerfile       # Docker build
│   ├── php-extension/       # PHP client (zero deps)
│   └── python-extension/    # Python client (zero deps)
├── clients/                 # Client libraries
│   ├── python/              # Python client
│   │   ├── mddb_client/     # Generated code
│   │   └── example.py
│   ├── nodejs/              # Node.js client
│   │   ├── proto/           # Proto files
│   │   └── example.js
│   └── go/                  # Go client library
├── scripts/                 # Utility scripts
│   └── load-md-folder.sh    # Bulk import script
├── examples/                # Example files
└── docs/                    # Documentation
```

### Shared Protobuf

All services and clients use the same Protocol Buffer definitions from `proto/`:
- **Single source of truth** for API contracts
- **Automatic code generation** for multiple languages
- **Version control** for API changes
- **Type safety** across all implementations

Generate code for all languages:
```bash
make generate-proto
```

## 🤝 Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.

For security vulnerabilities, please see our [Security Policy](SECURITY.md).

See [CHANGELOG.md](CHANGELOG.md) for version history.

## 📄 License

This project is licensed under the BSD 3-Clause License - see the [LICENSE](LICENSE) file for details.

## 🔗 Links

- [Documentation](docs/)
- [API Reference](docs/API.md)
- [Examples](docs/EXAMPLES.md)
- [Changelog](CHANGELOG.md)

## 📚 Standards & References

This project follows industry standards and best practices:

- **[RFC 2119](https://www.ietf.org/rfc/rfc2119.txt)** - Key words for use in RFCs to Indicate Requirement Levels
  - Defines the meaning of MUST, SHOULD, MAY, etc. used in our documentation
  - Ensures consistent interpretation of requirement levels across all specifications