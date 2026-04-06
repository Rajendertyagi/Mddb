# MDDB Configuration Reference

Complete reference for all MDDB configuration parameters.

**Precedence order:** CLI flags > environment variables > YAML config file > defaults

## Table of Contents

- [General / Core](#general--core)
- [HTTP Server](#http-server)
- [gRPC Server](#grpc-server)
- [MCP (Model Context Protocol)](#mcp-model-context-protocol)
- [HTTP/3 (QUIC)](#http3-quic)
- [Authentication](#authentication)
- [Embedding / Vector Search](#embedding--vector-search)
- [Vector Index](#vector-index)
- [Full-Text Search (FTS)](#full-text-search-fts)
- [Compression](#compression)
- [Replication](#replication)
- [Automation & Triggers](#automation--triggers)
- [GraphQL](#graphql)
- [CLI Flags](#cli-flags)
- [YAML Config File](#yaml-config-file)

---

## General / Core

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_CONFIG` | `""` | string | Path to YAML config file (also `-config` / `-c` CLI flag) |
| `MDDB_PATH` | `"mddb.db"` | string | Path to the BoltDB database file |
| `MDDB_MODE` | `"wr"` | string | Access mode: `"read"`, `"write"`, or `"wr"` (read+write) |
| `MDDB_PANEL_MODE` | `"internal"` | string | Panel mode: `"internal"` (CORS enabled) or `"external"` (reverse proxy) |
| `MDDB_CORS_ORIGIN` | `"*"` | string | CORS `Access-Control-Allow-Origin` header value |
| `MDDB_METRICS` | `"true"` | bool | Enable Prometheus-compatible `/metrics` endpoint |
| `MDDB_SEARCH_STATS` | `"true"` | bool | Include `searchStats` in search responses |

---

## HTTP Server

| Env Var | Default | Type | CLI Flag | Description |
|---------|---------|------|----------|-------------|
| `MDDB_HTTP_ENABLED` | `true` | bool | `--http-enabled` | Enable/disable the HTTP API server |
| `MDDB_HTTP_ADDR` | `":11023"` | string | `--http-addr` | HTTP API listen address |
| `MDDB_HTTP_PORT` | — | string | — | Plain port number (converted to `":PORT"`) |
| `MDDB_ADDR` | `":11023"` | string | — | Legacy alias for `MDDB_HTTP_ADDR` |

---

## gRPC Server

| Env Var | Default | Type | CLI Flag | Description |
|---------|---------|------|----------|-------------|
| `MDDB_GRPC_ENABLED` | `true` | bool | `--grpc-enabled` | Enable/disable the gRPC server |
| `MDDB_GRPC_ADDR` | `":11024"` | string | `--grpc-addr` | gRPC listen address |
| `MDDB_GRPC_PORT` | — | string | — | Plain port number (converted to `":PORT"`) |

---

## MCP (Model Context Protocol)

| Env Var | Default | Type | CLI Flag | Description |
|---------|---------|------|----------|-------------|
| `MDDB_MCP_ENABLED` | `true` | bool | `--mcp-enabled` | Enable/disable the MCP server |
| `MDDB_MCP_ADDR` | `":9000"` | string | `--mcp-addr` | MCP HTTP listen address |
| `MDDB_MCP_PORT` | — | string | — | Plain port number (converted to `":PORT"`) |
| `MDDB_MCP_STDIO` | `false` | bool | `--mcp-stdio` | Run MCP in stdio mode (for Claude Desktop) |
| `MDDB_MCP_DOMAIN` | `""` | string | — | MCP server domain |
| `MDDB_MCP_CONFIG` | `""` | string | — | Path to YAML with custom MCP tool definitions |

---

## HTTP/3 (QUIC)

| Env Var | Default | Type | CLI Flag | Description |
|---------|---------|------|----------|-------------|
| `MDDB_HTTP3_ENABLED` | `false` | bool | `--http3-enabled` | Enable HTTP/3 (QUIC) server |
| `MDDB_HTTP3_ADDR` | `":11443"` | string | `--http3-addr` | HTTP/3 listen address |
| `MDDB_HTTP3_PORT` | — | string | — | Plain port number (converted to `":PORT"`) |
| `MDDB_EXTREME` | — | bool | — | Legacy alias for `MDDB_HTTP3_ENABLED` |

---

## Authentication

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_AUTH_ENABLED` | `false` | bool | Enable JWT-based authentication |
| `MDDB_AUTH_JWT_SECRET` | `""` | string | JWT signing secret (**required** when auth is enabled) |
| `MDDB_AUTH_JWT_EXPIRY` | `"24h"` | duration | JWT token expiry duration |
| `MDDB_AUTH_ADMIN_USERNAME` | `"admin"` | string | Default admin username |
| `MDDB_AUTH_ADMIN_PASSWORD` | `""` | string | Default admin password |

---

## Embedding / Vector Search

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_EMBEDDING_PROVIDER` | `""` (disabled) | string | Provider: `"openai"`, `"ollama"`, `"voyage"`, `"cohere"`, or `""` |
| `MDDB_EMBEDDING_API_KEY` | `""` | string | API key (for openai/voyage/cohere) |
| `MDDB_EMBEDDING_API_URL` | *(see below)* | string | API base URL |
| `MDDB_EMBEDDING_MODEL` | *(see below)* | string | Embedding model name |
| `MDDB_EMBEDDING_DIMENSIONS` | *(see below)* | int | Vector dimensionality |
| `MDDB_EMBEDDING_CHUNK_ENABLED` | `true` | bool | Enable text chunking before embedding |
| `MDDB_EMBEDDING_CHUNK_SIZE` | `1500` | int | Maximum chunk size in characters |

### Provider Defaults

| Provider | `API_URL` | `MODEL` | `DIMENSIONS` |
|----------|-----------|---------|---------------|
| `openai` | `https://api.openai.com/v1` | `text-embedding-3-small` | `1536` |
| `ollama` | `http://localhost:11434` | `nomic-embed-text` | `768` |
| `voyage` | `https://api.voyageai.com/v1` | `voyage-3` | `1024` |
| `cohere` | `https://api.cohere.ai/v1` | `embed-english-v3.0` | `1024` |

---

## Vector Index

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_VECTOR_DEFAULT_ALGORITHM` | `"flat"` | string | Default algorithm: `"flat"`, `"hnsw"`, `"ivf"`, `"pq"`, `"sq"`, `"bq"` |
| `MDDB_VECTOR_BQ_RERANK_FACTOR` | `10` | int | Binary quantization rerank factor |

---

## MCP Server Info

Customize the MCP server profile returned in the `initialize` response. Useful for identifying your server to LLM clients.

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_MCP_SERVER_NAME` | `"mddbd"` | string | Server name shown to MCP clients |
| `MDDB_MCP_SERVER_DESCRIPTION` | `""` | string | Human-readable server description |
| `MDDB_MCP_SERVER_VENDOR` | `""` | string | Organization / vendor name |
| `MDDB_MCP_SERVER_HOMEPAGE` | `""` | string | URL to server documentation or homepage |
| `MDDB_MCP_INSTRUCTIONS` | `""` | string | System prompt for LLM — tells the AI how to use this server |

Or via YAML config:

```yaml
mcp:
  serverInfo:
    name: "my-knowledge-base"
    description: "Company internal documentation"
    vendor: "Acme Corp"
    homepage: "https://docs.acme.com"
  instructions: |
    This is the company knowledge base. Use search_documents to find
    relevant articles before answering questions. Always cite document
    keys in your responses. Prefer the 'docs' collection for technical
    questions and 'blog' for product updates.
```

---

## Server-Sent Events (SSE)

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_SSE_ENABLED` | `true` | bool | Enable SSE event stream at `/v1/events` |
| `MDDB_SSE_MAX_CLIENTS` | `1000` | int | Maximum total concurrent SSE connections |
| `MDDB_SSE_MAX_PER_IP` | `5` | int | Maximum concurrent SSE connections per IP address |

---

## TLS / HTTPS

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_TLS_ENABLED` | `false` | bool | Enable built-in TLS (HTTPS) |
| `MDDB_TLS_CERT` | `""` | string | Path to TLS certificate file (PEM) |
| `MDDB_TLS_KEY` | `""` | string | Path to TLS private key file (PEM) |

---

## Profiling

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_PPROF_ENABLED` | `false` | bool | Enable pprof profiling endpoints at `/debug/pprof/` |

---

## HTTP Connection Pool

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_HTTP_POOL_MAX_IDLE` | `100` | int | Max idle connections in shared HTTP pool |
| `MDDB_HTTP_POOL_MAX_PER_HOST` | `10` | int | Max idle connections per target host |
| `MDDB_HTTP_POOL_IDLE_TIMEOUT` | `90` | int | Idle connection timeout in seconds |

---

## Full-Text Search (FTS)

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_FTS_STEMMING` | `true` | bool | Enable Porter stemming for FTS indexing and queries |
| `MDDB_FTS_SYNONYMS` | `true` | bool | Enable synonym expansion in FTS queries |

---

## Compression

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_COMPRESSION_ENABLED` | `true` | bool | Enable adaptive document compression |
| `MDDB_COMPRESSION_SMALL_THRESHOLD` | `1024` | int (bytes) | Below this: Snappy compression |
| `MDDB_COMPRESSION_MEDIUM_THRESHOLD` | `10240` | int (bytes) | Above this: Zstd compression |

---

## Replication

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_REPLICATION_ROLE` | `""` | string | Role: `"leader"`, `"follower"`, or `""` (standalone) |
| `MDDB_NODE_ID` | `""` | string | Unique node ID (required for replication) |
| `MDDB_REPLICATION_LEADER_ADDR` | `""` | string | Leader address for follower nodes |
| `MDDB_BINLOG_ENABLED` | `false` | bool | Enable binary log (auto-enabled for leaders) |
| `MDDB_BINLOG_PATH` | `""` | string | Custom binlog file path |

---

## Automation & Triggers

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_AUTOMATIONS` | `"enable"` | string | Set to `"disable"` to disable automation manager |
| `MDDB_AUTOMATION_LOGS` | `"enable"` | string | Set to `"disable"` to disable log storage |
| `MDDB_AUTOMATION_LOGS_TTL` | `"7d"` | duration | TTL for automation log entries |
| `MDDB_TRIGGERS` | `false` | bool | Enable automation triggers on document changes |
| `MDDB_CRONS` | `false` | bool | Enable cron scheduler for automations |

---

## GraphQL

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_GRAPHQL_ENABLED` | `false` | bool | Enable GraphQL endpoint at `/graphql` |
| `MDDB_GRAPHQL_PLAYGROUND` | `true` | bool | Enable GraphQL Playground at `/playground` |

---

## CLI Flags

| Flag | Short | Type | Description |
|------|-------|------|-------------|
| `--config` | `-c` | string | Path to YAML config file |
| `--http-enabled` | | string | Enable HTTP API (true/false) |
| `--http-addr` | | string | HTTP listen address |
| `--grpc-enabled` | | string | Enable gRPC server (true/false) |
| `--grpc-addr` | | string | gRPC listen address |
| `--mcp-enabled` | | string | Enable MCP server (true/false) |
| `--mcp-addr` | | string | MCP listen address |
| `--mcp-stdio` | | string | MCP stdio mode (true/false) |
| `--http3-enabled` | | string | Enable HTTP/3 server (true/false) |
| `--http3-addr` | | string | HTTP/3 listen address |

---

## YAML Config File

Pass via `--config config.yaml` or `MDDB_CONFIG=config.yaml`.

```yaml
# Database
path: "mddb.db"
mode: "wr"                    # read, write, wr

# HTTP Server
http:
  enabled: true
  addr: ":11023"

# gRPC Server
grpc:
  enabled: true
  addr: ":11024"

# MCP Server
mcp:
  enabled: true
  addr: ":9000"
  stdio: false
  domain: ""

# HTTP/3 (QUIC)
http3:
  enabled: false
  addr: ":11443"

# Authentication
auth:
  enabled: false
  jwtSecret: ""
  jwtExpiry: "24h"
  adminUsername: "admin"
  adminPassword: ""

# Full-Text Search
fts:
  stemmingEnabled: true
  synonymsEnabled: true

# Compression
compression:
  enabled: true
  smallThreshold: 1024
  mediumThreshold: 10240

# Vector Search
vector:
  defaultAlgorithm: "flat"
  bqRerankFactor: 10
```

---

**Total: 46 environment variables** across 13 categories, 10 CLI flags, full YAML config file support.
