---
title: "MDDB Configuration Reference"
slug: "docs/config"
description: "MDDB Configuration Reference"
status: publish
---

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
- [Temporal Tracking](#temporal-tracking)
- [Spell Correction](#spell-correction)
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
| `MDDB_PATH` | `"mddb.db"` | string | Path to the BoltDB database file (also `--db` CLI flag, `database.path` in YAML) |
| `MDDB_MODE` | `"wr"` | string | Access mode: `"read"`, `"write"`, or `"wr"` (read+write) (also `--mode` CLI flag, `database.mode` in YAML) |
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
| `MDDB_MCP_BUILTIN_TOOLS` | `true` | bool | — | Set to `false` to expose only custom YAML tools |
| `MDDB_MCP_MODE` | `"wr"` | string | — | MCP access mode: `"read"`, `"write"`, or `"wr"` |

### MCP API Key Authentication

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_MCP_API_KEY_ENABLED` | `false` | bool | Enable API key authentication for MCP endpoints |
| `MDDB_MCP_API_KEYS` | `""` | string | Static API keys: `key1:name1,key2:name2` |
| `MDDB_MCP_API_KEY_CACHE_TTL` | `"5m"` | duration | Cache TTL for dynamic API key lookups |

### MCP Rate Limiting

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_MCP_RATE_LIMIT_ENABLED` | `false` | bool | Enable per-client rate limiting for MCP |
| `MDDB_MCP_RATE_LIMIT_REQUESTS` | `100` | int | Maximum requests per window |
| `MDDB_MCP_RATE_LIMIT_WINDOW` | `"60s"` | duration | Rate limit time window |
| `MDDB_MCP_RATE_LIMIT_BURST` | `20` | int | Maximum burst size |
| `MDDB_MCP_RATE_LIMIT_BY` | `"ip"` | string | Rate limit key: `"ip"`, `"api_key"`, or `"session"` |

### MCP Logging

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_MCP_LOGGING_ENABLED` | `false` | bool | Enable structured JSON audit logs for MCP requests |
| `MDDB_MCP_LOGGING_LEVEL` | `"info"` | string | Minimum log level: `"debug"`, `"info"`, `"warn"`, `"error"` |

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

## Audit Log (ISO 27001 / SOC 2)

Structured authentication and mutation trail persisted to a dedicated `audit` BoltDB bucket. Events are buffered and flushed asynchronously so hot-path handlers never block on disk I/O. Queryable via admin-only `GET /v1/audit`.

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_AUDIT_ENABLED` | `false` | bool | Enable the audit log. When disabled, `AuditManager` is a no-op and `/v1/audit` returns 404. |
| `MDDB_AUDIT_RETENTION_DAYS` | `90` | int | Retention window. A background trimmer runs every hour and deletes events older than the cutoff. |

Query parameters on `GET /v1/audit`: `from` / `to` (RFC3339) or `fromNanos` / `toNanos`, `actor`, `action`, `result` (`ok`/`fail`), `limit` (default 100). Response shape: `{events: [...], count, dropped}` — `dropped` counts events lost when the in-memory buffer was full.

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
| `MDDB_VECTOR_DEFAULT_ALGORITHM` | `"flat"` | string | Default algorithm: `"flat"`, `"hnsw"`, `"ivf"`, `"pq"`, `"opq"`, `"sq"`, `"bq"` |
| `MDDB_VECTOR_BQ_RERANK_FACTOR` | `10` | int | Binary quantization rerank factor |
| `MDDB_VECTOR_PARALLEL_WORKERS` | `NumCPU` (max 16) | int | Number of goroutines for parallel vector scoring |
| `MDDB_VECTOR_PARALLEL_MIN_SIZE` | `2048` | int | Minimum collection size to enable parallel search |

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

## TLS / HTTPS / mTLS

See [TLS.md](TLS.md) for the full setup guide (cert generation, recipes, troubleshooting).

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_TLS_ENABLED` | `false` | bool | Enable built-in TLS (HTTPS) on the HTTP listener |
| `MDDB_TLS_CERT` | `""` | string | Path to server TLS certificate (PEM) |
| `MDDB_TLS_KEY` | `""` | string | Path to server TLS private key (PEM) |
| `MDDB_TLS_CLIENT_CA` | `""` | string | Path to PEM bundle of trusted client CAs — enables **mTLS** when set |
| `MDDB_TLS_CLIENT_AUTH` | `"require"` | string | mTLS mode when `MDDB_TLS_CLIENT_CA` is set: `require` (reject anonymous clients) or `request` (verify only if cert presented) |

`MinVersion` is pinned to TLS 1.2. mTLS is automatically skipped on UDS listeners (filesystem permissions already authenticate the local peer).

---

## Unix Domain Socket transport

`MDDB_HTTP_ADDR` and `MDDB_GRPC_ADDR` accept either a TCP `host:port` (default) or a Unix Domain Socket address of the form `unix:/absolute/path.sock`. The server creates the socket with owner-only `0600` permissions, removes any stale socket file from a previous run, and unlinks the socket on graceful shutdown.

```bash
# HTTP API on a UDS, gRPC on TCP, no public HTTP port
MDDB_HTTP_ADDR=unix:/var/run/mddb/http.sock \
MDDB_GRPC_ADDR=:11024 \
./mddbd
```

TLS is automatically disabled on UDS listeners (peer is authenticated by filesystem permissions; API keys / JWT still apply on top). Per-IP rate limits in SSE collapse to a single bucket on UDS — apply application-level rate limiting if you need to differentiate clients.

Clients with native UDS support:

| Client | Address form |
|--------|--------------|
| Python (`services/python-extension/mddb.py`) | `MDDB.connect('unix:/var/run/mddb/http.sock')` |
| PHP (`services/php-extension/mddb.php`) | `mddb::connect('unix:/var/run/mddb/http.sock')` |
| Python gRPC (`clients/python/`) | `grpc.insecure_channel('unix:/var/run/mddb/grpc.sock')` |
| Node gRPC (`clients/nodejs/`) | `new MDDBClient('unix:/var/run/mddb/grpc.sock', creds)` |
| `curl` | `curl --unix-socket /var/run/mddb/http.sock http://localhost/v1/healthz` |

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
| `MDDB_FTS_DEFAULT_LANG` | `"en"` | string | Default language for stemming and stop words |

---

## Temporal Tracking

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_TEMPORAL` | `false` | bool | Enable document lifecycle event tracking (create/update/access) |

When enabled, provides endpoints: `POST /v1/temporal/query`, `POST /v1/temporal/hot`, `POST /v1/temporal/histogram`. Per-collection opt-in via Collection Settings (`trackAccess`, `trackHot`).

---

## Spell Correction

| Env Var | Default | Type | Description |
|---------|---------|------|-------------|
| `MDDB_SPELL` | `false` | bool | Enable SymSpell-style spell checker for FTS queries |

When enabled, provides endpoints: `POST /v1/spell-suggest`, `POST /v1/spell-cleanup`, `GET/PUT/DELETE /v1/spell-dictionary`. Enable `spellCorrect: true` on a collection for auto-correction.

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

## Per-Collection Configuration (v2.9.14+ additions)

Per-collection attributes are persisted via `PUT /v1/collection-config` (REST), `SetCollectionConfig` (gRPC), the `set_collection_config` MCP tool, or the Admin Panel — **not** via env vars. The settings below extend what has been configurable since earlier versions.

| Field | Default | Type | Description |
|-------|---------|------|-------------|
| `maxRevisions` | `0` | integer | (v2.9.14+) Revision retention cap per document. `0` = unlimited. When `> 0`, each `add`/`update` trims older revisions in the same BoltDB transaction so history stays capped even under high write-churn. |
| `trackAccess` | `false` | bool | Record per-read access events (needs `MDDB_TEMPORAL=true`) |
| `trackHot` | `false` | bool | Maintain a hot-docs leaderboard |
| `spellCorrect` | `false` | bool | Auto-correct FTS queries (needs `MDDB_SPELL=true`) |
| `spellLang` | `""` | string | Override spell-correction language for this collection |
| `quantization` | `"float32"` | string | Vector quantization level: `float32`, `int8`, or `int4` |
| `storageBackend` | `"boltdb"` | string | `boltdb`, `memory`, or `s3` |

**Example — set a 20-revision cap:**
```bash
curl -X PUT http://localhost:11023/v1/collection-config \
  -H 'Content-Type: application/json' \
  -d '{"collection":"blog","type":"default","maxRevisions":20}'
```

---

## Curation Rules (v2.9.14+)

Curation is data, not configuration — rules live in a dedicated bolt bucket and are managed at runtime via `/v1/curation` (see [API.md](API.md) and [SEARCH.md](SEARCH.md)). No server-level flag controls the subsystem; it's always on and has zero overhead when no rules match an incoming query.

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
  parallelWorkers: 0              # 0 = auto (NumCPU, max 16)
  parallelMinSize: 2048           # min collection size for parallel search

# Temporal Tracking
temporal: false

# Spell Correction
spell: false

# MCP Security
mcp:
  apiKeyEnabled: false
  apiKeys: ""
  rateLimitEnabled: false
  rateLimitRequests: 100
  rateLimitWindow: "60s"
  rateLimitBurst: 20
  rateLimitBy: "ip"
  loggingEnabled: false
```

---

**Total: 65+ environment variables** across 17 categories, 10 CLI flags, full YAML config file support.
