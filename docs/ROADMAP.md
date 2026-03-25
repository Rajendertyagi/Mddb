# MDDB Roadmap

Detailed roadmap showing implemented features and future plans.

## Implemented Features

### v1.0–v2.3 — Core Database
- Document CRUD with metadata, collections, multi-language
- Revision history with full snapshots
- Template variables, bulk import
- gRPC/Protobuf API (16x faster than HTTP)
- Document TTL with background cleanup
- Vector search (OpenAI, Ollama embeddings)
- Webhooks with retry logic
- Full-text search (TF-IDF), import from URL
- MCP server (stdio + HTTP), Prometheus telemetry
- Schema validation, custom YAML-based MCP tools

### v2.4 — Security
- JWT authentication, API keys, bcrypt
- Collection-level RBAC (read/write/admin)
- User/group management with inherited permissions

### v2.5 — GraphQL
- GraphQL API with Playground, authentication directives
- CLI GraphQL support, Web Panel toggle
- Cohere and Voyage AI embedding providers

### v2.6–v2.7 — Search & Replication
- Advanced FTS: BM25, BM25F, PMISparse, 7 search modes, 18 languages, fuzzy, synonyms
- Hybrid search (BM25 + vector, alpha blending, RRF)
- Vector algorithms: Flat, HNSW, IVF, PQ, SQ, BQ
- Zero-shot classification
- Leader-follower replication with binlog streaming
- Aggregations (facets, histograms)

### v2.8 — Storage & Multi-Format
- Per-collection storage backends (BoltDB, memory, S3/MinIO)
- File upload (PDF, DOCX, HTML, ODT, RTF, TEX, YAML → Markdown)
- Automation system (triggers, crons, webhooks with templates)
- Cross-collection search, duplicate detection

### v2.9 — Quantization & Infrastructure (current)
- Per-collection vector quantization (int8 = 4x, int4 = 8x compression)
- Server-Sent Events (SSE) for real-time document change notifications
- pprof profiling endpoints
- HTTP connection pooling (shared transport)
- Built-in TLS/HTTPS support

---

## Planned Features

### v2.10 — Observability & Security (Q2 2026)

**Observability**
- ⏳ OpenTelemetry / distributed tracing
- ⏳ Slow query logging (threshold-based)
- ⏳ Structured JSON logging with configurable levels

**Security**
- ⏳ Encryption at rest (AES-256-GCM on BoltDB values)
- ⏳ Comprehensive audit log (who/what/when)
- ⏳ Field-level encryption for sensitive metadata
- ⏳ Key rotation mechanism

**Backup & Recovery**
- ⏳ Incremental backups (binlog-based)
- ⏳ Point-in-time recovery (PITR)
- ⏳ Scheduled auto-backup (cron + S3/GCS destination)

### v2.11 — Search & AI (Q3 2026)

**Geosearch**
- ⏳ Postcode/GPS-based distance search
- ⏳ Geo-bounding box queries
- ⏳ Geospatial index

**Advanced Vector Search**
- ⏳ Cross-encoder re-ranking (two-stage retrieval)
- ⏳ Sparse-dense hybrid vectors (SPLADE/ColBERT)
- ⏳ Multi-vector documents
- ⏳ Streaming embeddings (embed during upload)

### v3.0 — Extensibility (Q4 2026)

**Plugin System**
- ⏳ Go plugin architecture
- ⏳ Custom storage backends
- ⏳ Custom embedding providers
- ⏳ Custom authentication providers
- ⏳ Custom search algorithms

**Event Streaming**
- ⏳ Kafka integration
- ⏳ NATS integration
- ⏳ Redis Streams support
- ⏳ Change Data Capture (CDC)

### v3.1 — Real-Time & GraphQL (2027)

**GraphQL Subscriptions**
- ⏳ Real-time updates via WebSocket
- ⏳ Document change subscriptions
- ⏳ Filtered subscriptions by collection/metadata

**GraphQL Federation**
- ⏳ Apollo Federation support
- ⏳ Subgraph schema, reference resolution

### v3.2 — Clustering & HA (2027)

**Distributed Consensus**
- ⏳ Raft-based consensus
- ⏳ Automatic leader election & failover
- ⏳ Split-brain prevention
- ⏳ Quorum-based writes

**Sharding**
- ⏳ Horizontal sharding by collection/key hash
- ⏳ Cross-shard queries
- ⏳ Automatic shard rebalancing

### v3.3 — Multi-Tenancy (2027)

- ⏳ Tenant-level data isolation
- ⏳ Per-tenant quotas (storage, requests, bandwidth)
- ⏳ Per-tenant rate limiting
- ⏳ Tenant provisioning API
- ⏳ Cross-tenant admin queries

### v3.4 — External Cache (2027)

- ⏳ Redis integration for distributed cache
- ⏳ Memcached support
- ⏳ Cache invalidation webhooks
- ⏳ Query result caching with automatic invalidation
- ⏳ Cache analytics and adaptive sizing

---

## Under Consideration

- 📋 Advanced analytics dashboard
- 📋 Document relationships/links (graph queries)
- 📋 Automatic cloud backups (S3/GCS/Azure scheduled)
- 📋 GUI for schema validation rules
- 📋 Built-in image optimization
- 📋 Markdown linting and validation
- 📋 Multi-region deployment
- 📋 WebAssembly plugin support
- 📋 OpenAPI v3.1 spec auto-generation

---

## Feedback & Suggestions

- **Feature Requests:** [GitHub Discussions](https://github.com/tradik/mddb/discussions/categories/ideas)
- **Bug Reports:** [GitHub Issues](https://github.com/tradik/mddb/issues)
- **Contribute:** See [CONTRIBUTING.md](../CONTRIBUTING.md)

---

**[← Back to README](../README.md)** | **[See changelog →](../CHANGELOG.md)**
