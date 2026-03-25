# MDDB — AI-Native Document Database

[![Go Version](https://img.shields.io/badge/Go-1.26-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-green.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/tradik/mddb)](https://github.com/tradik/mddb/releases)
[![Docker](https://img.shields.io/docker/v/tradik/mddb?label=docker)](https://hub.docker.com/r/tradik/mddb)
[![Docker Pulls](https://img.shields.io/docker/pulls/tradik/mddb)](https://hub.docker.com/r/tradik/mddb)
[![Tests](https://github.com/tradik/mddb/workflows/Tests/badge.svg)](https://github.com/tradik/mddb/actions)

**AI-native document database with built-in MCP server, file upload (PDF/DOCX/HTML/ODT/RTF/TEX/YAML→Markdown), vector search, RAG pipelines, and 54 MCP tools. Plugs directly into Claude, ChatGPT, Cursor, Windsurf, and any MCP-compatible agent.**

MDDB is a document database purpose-built for AI agents and LLM workflows. Upload files (PDF, DOCX, HTML, ODT, RTF, TEX, YAML, TXT) — they're auto-converted to Markdown and embedded for semantic search. Expose everything to AI agents via 54 built-in MCP tools. Integrates with [Docling](docs/INTEGRATIONS.md#1-docling--mddb-document-ingestion), [Langflow](docs/INTEGRATIONS.md#2-langflow--mddb-visual-rag-orchestration), [OpenSearch](docs/INTEGRATIONS.md#3-opensearch--mddb-scalable-search), [SSG](docs/INTEGRATIONS.md#4-ssg--static-site-generator-from-mddb), and [wpexporter](docs/INTEGRATIONS.md#5-wpexporter--wordpress-to-mddb-migration) for production pipelines. Single ~29MB binary, zero configuration, BoltDB embedded storage, triple-protocol APIs (HTTP + gRPC + GraphQL).

## 🎯 What is MDDB?

MDDB gives your AI agents a persistent, searchable knowledge base:

- **File Upload** - Upload PDF, DOCX, HTML, ODT, RTF, TEX, YAML, TXT files — auto-converted to Markdown and indexed
- **Built-in MCP Server** - 54 tools for Claude Desktop, Cursor, Windsurf, or any MCP client
- **Vector Search** - Auto-embed documents, semantic similarity with 6 index algorithms (Flat, HNSW, IVF, PQ, SQ, BQ) + per-collection quantization (int8/int4)
- **RAG-Ready** - Hybrid search (BM25 + vector) for retrieval-augmented generation
- **Integrations** - [Docling](docs/INTEGRATIONS.md), [Langflow](docs/INTEGRATIONS.md), [OpenSearch](docs/INTEGRATIONS.md), [SSG](docs/INTEGRATIONS.md), [wpexporter](docs/INTEGRATIONS.md) for production pipelines
- **Zero-Shot Classification** — Classify documents against candidate labels using embeddings, no training data
- **Custom AI Tools** - Define YAML-based MCP tools for domain-specific workflows
- **Full-Text Search** - Built-in inverted index with TF-IDF, BM25, BM25F, PMISparse, 7 search modes (simple, boolean, phrase, wildcard, proximity, range, fuzzy), typo tolerance, multi-language stemming (18 languages), synonyms
- **Full Revision History** - Every update creates a new revision with complete snapshots
- **Triple Protocol APIs** - HTTP/JSON (easy), gRPC (fast), or GraphQL (flexible)
- **Automation** - Triggers, crons, webhooks with template variables and sentiment analysis
- **Real-Time Events** - Server-Sent Events (SSE) for live document change notifications + MCP-over-SSE transport
- **Built-in TLS** - Native HTTPS support, connection pooling, pprof profiling
- **Zero Configuration** - Single ~29MB binary, embedded database, no dependencies

**Perfect for:** AI agent memory, RAG pipelines, knowledge bases for LLMs, documentation chatbots, semantic search APIs, document processing (PDF/DOCX→Markdown), static site generation, WordPress migration

## 🚀 Quick Start

### Docker Compose (Recommended) - Full Stack

Start all services with one command:

```bash
git clone https://github.com/tradik/mddb.git
cd mddb

# Production mode (all services)
docker compose up -d

# Development mode (with hot reload)
make dev-start

# Development + Ollama for embeddings
make dev-start-with-ollama
```

**Services started:**

| Service | Port | Image | Description |
|---------|------|-------|-------------|
| **mddbd** | 11023 (HTTP), 11024 (gRPC), 9000 (MCP), 11443 (HTTP/3) | `tradik/mddb:latest` | Database server with MCP built-in |
| **mddb-panel** | 3000 | `tradik/mddb:panel` | React web admin UI |

### Connect to Claude / Cursor / Windsurf (MCP)

MDDB has a built-in MCP server — no extra service needed. Add to your MCP config:

```json
{
  "mcpServers": {
    "mddb": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm", "--network", "host",
        "-v", "mddb-data:/app/data",
        "-e", "MDDB_MCP_STDIO=true",
        "tradik/mddb:latest"
      ]
    }
  }
}
```

That's it — your AI agent now has full access to your knowledge base with 54 built-in tools (add, search, vector search, classify, and more).

**[→ Full MCP setup guide](docs/LLM_CONNECTIONS.md)** | **[→ Custom MCP tools](docs/CUSTOM-TOOLS.md)**

### Docker - Individual Services

```bash
# MDDB Server only
docker run -d --name mddb \
  -p 11023:11023 -p 11024:11024 -p 9000:9000 \
  -v mddb-data:/data \
  tradik/mddb:latest

# Web Panel (connect to existing server)
docker run -d --name mddb-panel \
  -p 3000:3000 \
  -e VITE_MDDB_SERVER=host.docker.internal:11023 \
  tradik/mddb:panel

# MCP stdio mode (for Claude Desktop, Windsurf, etc.)
docker run -i --rm --network host \
  -v mddb-data:/app/data \
  -e MDDB_MCP_STDIO=true \
  tradik/mddb:latest

# Test it
curl http://localhost:11023/health
```

**Docker Hub:** https://hub.docker.com/r/tradik/mddb

### Install Binary

**Linux (Debian/Ubuntu):**
```bash
wget https://github.com/tradik/mddb/releases/latest/download/mddbd-latest-linux-amd64.deb
sudo dpkg -i mddbd-latest-linux-amd64.deb
sudo systemctl start mddbd
```

**macOS (Apple Silicon):**
```bash
wget https://github.com/tradik/mddb/releases/latest/download/mddbd-latest-darwin-arm64.tar.gz
tar xzf mddbd-latest-darwin-arm64.tar.gz
sudo mv mddbd-latest-darwin-arm64/mddbd /usr/local/bin/
mddbd
```

**CLI Client:**
```bash
# Linux
wget https://github.com/tradik/mddb/releases/latest/download/mddb-cli-latest-linux-amd64.deb
sudo dpkg -i mddb-cli-latest-linux-amd64.deb

# Usage
mddb-cli stats
mddb-cli add blog hello en_US -f post.md
mddb-cli search blog -f "tags=tutorial"
mddb-cli fts blog --query="getting started" --algorithm=bm25
```

**Other platforms:** See [Installation Guide](docs/INSTALLATION.md)

### Build from Source

```bash
git clone https://github.com/tradik/mddb.git
cd mddb
make build
./services/mddbd/mddbd
```

## 📦 Packages & Client Libraries

MDDB ships as a monorepo with multiple packages:

### Server & Tools

| Package | Language | Location | Description |
|---------|----------|----------|-------------|
| **mddbd** | Go | `services/mddbd/` | Database server (HTTP + gRPC + GraphQL + MCP) |
| **mddb-panel** | React/JS | `services/mddb-panel/` | Web admin panel |
| **mddb-cli** | Go | `services/mddb-cli/` | Command-line client with GraphQL support |
| **mddb-chat** | Rust | `services/mddb-chat/` | WebSocket chat server with LLM integration |
| **mddb-chat-widget** | JS/TS | `services/mddb-chat-widget/` | Embeddable JS chat widget |

### Client Libraries (REST)

Zero-dependency HTTP clients - copy a single file into your project:

| Library | Language | Location | Install |
|---------|----------|----------|---------|
| **PHP Extension** | PHP 8.0+ | `services/php-extension/mddb.php` | Copy `mddb.php` into your project |
| **Python Extension** | Python 3.8+ | `services/python-extension/mddb.py` | Copy `mddb.py` into your project |

**PHP:**
```php
require_once 'mddb.php';
$db = mddb::connect('localhost:11023', 'write');
$db->collection('blog')->add('hello', 'en_US', ['author' => ['John']], '# Hello');
$results = $db->collection('blog')->vectorSearch('cancel subscription', 5, 0.7);
```

**Python:**
```python
from mddb import MDDB
db = MDDB.connect('localhost:11023', 'write').collection('blog')
db.add('hello', 'en_US', {'author': ['John']}, '# Hello')
results = db.vector_search('cancel subscription', top_k=5)
```

### Client Libraries (gRPC)

High-performance clients generated from Protocol Buffers:

| Library | Language | Location | Description |
|---------|----------|----------|-------------|
| **Go Client** | Go | `services/mddbd/proto/` | Native Go gRPC stubs |
| **Python gRPC** | Python | `clients/python/` | Generated Python gRPC client |
| **Node.js gRPC** | Node.js | `clients/nodejs/` | Uses `@grpc/grpc-js` |

Proto definitions at `proto/mddb.proto` - generate clients for any language supported by protobuf.

### Docker Images ([Docker Hub](https://hub.docker.com/r/tradik/mddb))

| Image | Size | Description |
|-------|------|-------------|
| `tradik/mddb:latest` | ~29MB | Database server with MCP built-in (Alpine) |
| `tradik/mddb:panel` | ~88MB | Web admin panel (Node Alpine) |
| `tradik/mddb:cli` | ~8MB | CLI client (Alpine) |

### System Packages

| Format | Platform | Contents |
|--------|----------|----------|
| `.deb` | Debian/Ubuntu | mddbd + systemd unit + man page |
| `.rpm` | RHEL/CentOS/Fedora | mddbd + systemd unit + man page |
| `.tar.gz` | Any (Linux, macOS, FreeBSD) | Standalone binary |

## 💡 Key Features

### AI & Search
- ✅ **MCP Server** - 54 built-in tools via Model Context Protocol (stdio + HTTP) for Claude, Cursor, Windsurf, and any MCP client
- ✅ **File Upload** - Upload PDF, DOCX, HTML, ODT, RTF, TEX, YAML, TXT — auto-converted to Markdown (single and batch, configurable size limit)
- ✅ **Vector Search** - Semantic similarity with auto-embeddings (OpenAI, Ollama, Cohere, Voyage)
- ✅ **Full-Text Search** - Built-in inverted index with TF-IDF, BM25, BM25F, PMISparse scoring, 7 search modes (simple, boolean, phrase, wildcard, proximity, range, fuzzy), typo tolerance, metadata pre-filtering, multi-language stemming and stop words (18 languages)
- ✅ **Hybrid Search** - Sparse (BM25) + dense (vector) fusion with alpha blending or RRF
- ✅ **Aggregations** - Metadata facets (value counts) and date histograms with optional pre-filtering
- ✅ **Zero-Shot Classification** - Classify documents against candidate labels using embedding similarity
- ✅ **Custom MCP Tools** - Define YAML-based AI tools for domain-specific workflows
- ✅ **RAG Pipeline** - Built-in support for retrieval-augmented generation workflows
- ✅ **Integrations** - Docling, Langflow, OpenSearch, SSG, wpexporter ([guide](docs/INTEGRATIONS.md))

### Core Functionality
- ✅ **Document Management** - Full CRUD with metadata and collections
- ✅ **Revision History** - Complete version control with snapshots
- ✅ **Metadata Search** - Fast indexed queries with multi-value tags
- ✅ **Collection Checksum** - Lightweight CRC32 checksum per collection for cache invalidation
- ✅ **Partial Document Update** - Update metadata and/or content independently
- ✅ **Document TTL** - Time-to-live with automatic cleanup
- ✅ **Automation** - Triggers, crons, webhooks with template variables, sentiment analysis, execution logs
- ✅ **Multi-language** - Same key, multiple languages
- ✅ **Schema Validation** - JSON Schema validation per collection
- ✅ **Per-Collection Storage Backends** - Choose BoltDB (default), in-memory (ephemeral), or S3/MinIO per collection

### APIs & Protocols
- ✅ **HTTP/JSON REST** - Easy debugging, extensive docs
- ✅ **gRPC/Protobuf** - 16x faster, 70% smaller payload
- ✅ **GraphQL** - Flexible queries, schema introspection, Playground
- ✅ **CLI Client** - Full-featured command-line with GraphQL support
- ✅ **Web Panel** - React UI with REST/GraphQL toggle

### Security & Access
- ✅ **Authentication** - JWT tokens and API keys
- ✅ **Authorization** - Collection-level RBAC (Read/Write/Admin)
- ✅ **User Management** - Multi-user with admin roles
- ✅ **Group Permissions** - Organize users into groups

### Replication & High Availability
- ✅ **Leader-Follower Replication** - Binlog streaming for read scaling
- ✅ **Automatic Catch-up** - Followers pull missing transactions
- ✅ **Zero-Downtime Snapshots** - Full sync for new followers
- ✅ **Cluster Monitoring** - Web panel with health and lag metrics

**[→ See all features](docs/FEATURES.md)** | **[→ Compare with alternatives](docs/COMPARISON.md)** | **[→ Performance benchmarks](docs/PERFORMANCE.md)**

## 🔄 Replication Architecture

MDDB supports leader-follower replication allowing you to scale read operations horizontally.

```mermaid
graph LR
    C[Clients] -->|Writes/Reads| L[Leader]
    C -->|Reads| F1[Follower 1]
    C -->|Reads| F2[Follower 2]
    L -->|gRPC StreamBinlog| F1
    L -->|gRPC StreamBinlog| F2
```

- **Leader**: Handles writes, maintains changes in a binary log, and streams them via gRPC.
- **Followers**: Read-only, pulls transactions, reconnects automatically.

**[→ Read Full Replication Guide](docs/REPLICATION.md)**

## 🎨 Web Admin Panel

Modern React-based UI for managing documents, users, and search with REST/GraphQL API toggle.

![MDDB Web Panel](docs/panel.png)

**Features:** Browse collections, view/edit documents, vector search, user management, API mode switching (REST ↔ GraphQL), live markdown preview.

**[→ Panel documentation](docs/PANEL.md)**

## 📖 Quick Examples

### Upload Files (PDF, DOCX, HTML, ODT, RTF, TEX, YAML, TXT)

```bash
# Upload a PDF — auto-converted to Markdown
curl -X POST http://localhost:11023/v1/upload \
  -F "file=@report.pdf" \
  -F "collection=docs" \
  -F "lang=en_US"

# Upload with custom key and metadata
curl -X POST http://localhost:11023/v1/upload \
  -F "file=@manual.docx" \
  -F "collection=docs" \
  -F "key=user-manual" \
  -F "lang=en_US" \
  -F 'meta={"category":["documentation"]}'

# Batch upload multiple files
curl -X POST http://localhost:11023/v1/upload \
  -F "files[]=@doc1.pdf" \
  -F "files[]=@doc2.html" \
  -F "files[]=@doc3.txt" \
  -F "collection=docs" \
  -F "lang=en_US"
```

### Add and Retrieve Documents

```bash
# Add a document
curl -X POST http://localhost:11023/v1/add \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "key": "hello-world",
    "lang": "en_US",
    "meta": {"author": ["John"], "tags": ["tutorial"]},
    "contentMd": "# Hello World\n\nWelcome to MDDB!"
  }'

# Get document
curl -X POST http://localhost:11023/v1/get \
  -H 'Content-Type: application/json' \
  -d '{"collection": "blog", "key": "hello-world", "lang": "en_US"}'

# Search by metadata
curl -X POST http://localhost:11023/v1/search \
  -H 'Content-Type: application/json' \
  -d '{"collection": "blog", "filterMeta": {"tags": ["tutorial"]}, "limit": 10}'
```

### Vector Search (Semantic)

```bash
# Documents auto-embedded in background
# Search by meaning, not keywords
curl -X POST http://localhost:11023/v1/vector-search \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "kb",
    "query": "how do I cancel my subscription?",
    "topK": 5,
    "threshold": 0.7,
    "includeContent": true
  }'
```

### Hybrid Search (Sparse + Dense)

Combine keyword (BM25/BM25F) and semantic (vector) search in a single query. Two merge strategies:
- **Alpha Blending**: `combined = (1-a) * BM25_score + a * vector_score` -- configurable weight
- **RRF (Reciprocal Rank Fusion)**: rank-based fusion that is robust to different score distributions

```bash
curl -X POST http://localhost:11023/v1/hybrid-search \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "docs",
    "query": "machine learning",
    "topK": 10,
    "strategy": "alpha",
    "alpha": 0.5
  }'
```

### Full-Text Search (7 Modes)

FTS supports simple, boolean, phrase, wildcard, proximity, range, and fuzzy modes with auto-detection:

```bash
# Simple search with metadata pre-filtering
curl -X POST http://localhost:11023/v1/fts \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "blog",
    "query": "getting started",
    "limit": 10,
    "algorithm": "bm25",
    "filterMeta": {"category": ["tutorial"]}
  }'

# Boolean search (AND, OR, NOT, +required, -excluded)
curl -X POST http://localhost:11023/v1/fts \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "blog",
    "query": "rust AND performance NOT garbage",
    "mode": "boolean"
  }'

# Phrase search (exact sequence)
curl -X POST http://localhost:11023/v1/fts \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "blog",
    "query": "\"machine learning\"",
    "mode": "phrase"
  }'

# Proximity search (terms within N words)
curl -X POST http://localhost:11023/v1/fts \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "blog",
    "query": "\"database performance\"~5",
    "mode": "proximity",
    "distance": 5
  }'
```

### GraphQL

```bash
# Enable GraphQL
docker run -e MDDB_GRAPHQL_ENABLED=true -p 11023:11023 tradik/mddb

# Query
curl -X POST http://localhost:11023/graphql \
  -H 'Content-Type: application/json' \
  -d '{
    "query": "{ document(collection: \"blog\", key: \"hello-world\", lang: \"en\") { contentMd meta } }"
  }'

# Interactive Playground
open http://localhost:11023/playground
```

### CLI Client

```bash
# Install CLI
wget https://github.com/tradik/mddb/releases/latest/download/mddb-cli-latest-linux-amd64.deb
sudo dpkg -i mddb-cli-latest-linux-amd64.deb

# Use CLI
mddb-cli add blog hello en_US -f post.md -m "author=John,tags=tutorial"
mddb-cli get blog hello en_US
mddb-cli search blog -f "tags=tutorial"
mddb-cli fts blog --query="getting started"
mddb-cli stats
```

**[→ More examples](docs/API_QUICK_REFERENCE.md)** | **[→ Use case examples](docs/USE_CASES.md)** | **[→ Client libraries](docs/CLIENTS.md)**

## 📚 Documentation

**🌐 [Official Website](https://tradik.github.io/mddb/)** - Complete documentation, downloads, examples

### Getting Started
- **[Quick Start Guide](docs/QUICKSTART.md)** - 5-minute setup
- **[Installation Guide](docs/INSTALLATION.md)** - All platforms (Linux, macOS, FreeBSD, Windows)
- **[Use Cases](docs/USE_CASES.md)** - Real-world examples

### API Documentation
- **[HTTP/JSON API](docs/API.md)** - Complete REST API reference
- **[gRPC API](docs/GRPC.md)** - High-performance protocol guide
- **[GraphQL API](docs/GRAPHQL.md)** - Flexible query language
- **[OpenAPI/Swagger](docs/openapi.yaml)** - Machine-readable spec
- **[Swagger UI](docs/swagger.html)** - Interactive API docs

### Features & Guides
- **[Vector Search](docs/EMBEDDING_PROVIDERS.md)** - Semantic search setup (OpenAI, Cohere, Voyage, Ollama)
- **[RAG Pipeline](docs/RAG-PIPELINE.md)** - Complete RAG implementation guide
- **[Search Algorithms](docs/SEARCH.md)** - TF-IDF, BM25, BM25F, PMISparse, Flat, HNSW, IVF, PQ, SQ, BQ
- **[Vector Quantization](docs/QUANTIZATION.md)** - Per-collection int8/int4 scalar quantization (4-8x compression)
- **[Server-Sent Events](docs/SSE.md)** - Real-time document change notifications with auth and rate limiting
- **[Full-Text Search](docs/FTS.md)** - Built-in inverted index with multi-language support
- **[Zero-Shot Classification](docs/ZERO-SHOT-CLASSIFICATION.md)** - Classify documents against labels using embeddings
- **[PMISparse](docs/PMISPARSE.md)** - Two-phase BM25 + PPMI query expansion (invented by Tradik Limited)
- **[Webhooks](docs/WEBHOOKS.md)** - Event-driven integration
- **[Automations](docs/AUTOMATIONS.md)** - Triggers, crons, webhooks, sentiment, template variables
- **[Authentication](docs/AUTH.md)** - JWT & API keys, RBAC
- **[Web Panel](docs/PANEL.md)** - Admin UI guide
- **[LLM Connections](docs/LLM_CONNECTIONS.md)** - MCP for Claude, ChatGPT, Ollama, DeepSeek
- **[Integrations](docs/INTEGRATIONS.md)** - Docling, Langflow, OpenSearch, SSG, wpexporter
- **[Bulk Import](docs/BULK-IMPORT.md)** - Load markdown folders

### Operations
- **[Docker Guide](docs/DOCKER.md)** - Container deployment
- **[Deployment](docs/DEPLOYMENT.md)** - Production setup
- **[Telemetry](docs/TELEMETRY.md)** - Prometheus metrics, Grafana
- **[Health Checks](docs/HEALTHCHECK.md)** - Docker & Kubernetes
- **[Performance](docs/PERFORMANCE.md)** - Benchmarks & tuning
- **[Architecture](docs/ARCHITECTURE.md)** - System design

### Development
- **[Client Libraries](docs/CLIENTS.md)** - PHP, Python, Go, Node.js
- **[Custom MCP Tools](docs/CUSTOM-TOOLS.md)** - YAML-defined AI tools
- **[Examples](docs/EXAMPLES.md)** - Code samples
- **[Contributing](CONTRIBUTING.md)** - Development guide
- **[Changelog](CHANGELOG.md)** - Version history

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────┐
│     AI Agents (Claude, ChatGPT, Cursor, Windsurf)   │
│     ↕ MCP (stdio / HTTP :9000)                      │
├─────────────────────────────────────────────────────┤
│         Other Clients                               │
├──────────┬──────────┬──────────┬────────────────────┤
│HTTP/JSON │gRPC/Proto│ GraphQL  │ HTTP/3             │
│  :11023  │  :11024  │ /graphql │ :11443             │
├──────────┴──────────┴──────────┴────────────────────┤
│           MDDB Server (Go)                          │
│  • File Upload (PDF/DOCX/HTML/TXT → Markdown)       │
│  • Auto-Embeddings (OpenAI, Ollama, Cohere, Voyage) │
│  • Vector + Full-Text + Hybrid Search               │
│  • Zero-Shot Classification                         │
│  • Automation (triggers, crons, webhooks)            │
│  • JWT Auth + RBAC                                  │
├─────────────────────────────────────────────────────┤
│      BoltDB (Embedded ACID Storage)                 │
│  • B+Tree index • Single-file • MVCC transactions   │
└─────────────────────────────────────────────────────┘
```

**[→ Detailed architecture](docs/ARCHITECTURE.md)**

## 🗺️ Roadmap

**[→ Full roadmap](docs/ROADMAP.md)**

## 🤝 Contributing

Contributions welcome! See **[CONTRIBUTING.md](CONTRIBUTING.md)** for guidelines.

**Security issues:** See **[SECURITY.md](SECURITY.md)**

## 📄 License

BSD 3-Clause License - see **[LICENSE](LICENSE)**

## 🔗 Quick Links

- **[GitHub](https://github.com/tradik/mddb)** - Source code
- **[Docker Hub](https://hub.docker.com/r/tradik/mddb)** - Container images
- **[Releases](https://github.com/tradik/mddb/releases)** - Download binaries
- **[Documentation](https://tradik.github.io/mddb/)** - Full docs
- **[LLM Connections](docs/LLM_CONNECTIONS.md)** - Claude, ChatGPT, Ollama, DeepSeek, Manus, Bielik.ai
- **[Issues](https://github.com/tradik/mddb/issues)** - Bug reports
