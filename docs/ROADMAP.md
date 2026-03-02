# MDDB Roadmap

Detailed roadmap showing implemented features and future plans.

## ✅ Implemented Features (v2.5.0)

### Core Database (v1.0 - v2.3)

**v1.0 - Foundation (Released)**
- ✅ Document Management - Full CRUD with metadata and collections
- ✅ Revision History - Complete version control with content snapshots
- ✅ Metadata Search - Indexed multi-value tag queries
- ✅ Multi-language Support - Same key, multiple languages
- ✅ HTTP/JSON REST API - RESTful API with full documentation
- ✅ Docker Support - Multi-arch images (15MB Alpine)
- ✅ BoltDB Storage - Embedded ACID database

**v1.2 - Templates**
- ✅ Template Variables - Dynamic `{{variable}}` substitution

**v1.3 - Bulk Operations**
- ✅ Bulk Import - Load markdown folders with script

**v1.4 - Developer Experience**
- ✅ Hot Reload - Development mode with auto-restart

**v1.5 - gRPC Protocol**
- ✅ gRPC/Protobuf API - High-performance binary protocol (16x faster)
- ✅ gRPC Reflection - Use grpcurl for debugging
- ✅ Protobuf Clients - Generated code for Go, Python, Node.js, PHP

**v2.0 - Document TTL**
- ✅ Document TTL - Auto-expiring documents with background cleanup
- ✅ TTL Management - Set/update/remove TTL via API

**v2.1 - Vector Search**
- ✅ Vector Search - Semantic similarity with auto-embeddings
- ✅ OpenAI Embeddings - text-embedding-3-small/large support
- ✅ Ollama Embeddings - Local embedding models
- ✅ Background Indexing - Non-blocking embedding generation
- ✅ Webhooks - HTTP callbacks on doc.added/updated/deleted with retry

**v2.2 - Full-Text Search**
- ✅ Full-Text Search - Built-in inverted index with TF scoring
- ✅ Stop Word Filtering - Remove common words
- ✅ Import from URL - Fetch markdown from URLs with frontmatter parsing

**v2.3 - Integrations**
- ✅ MCP Server - Model Context Protocol for LLM integration
- ✅ Telemetry - Prometheus-compatible /metrics endpoint
- ✅ Schema Validation - Per-collection JSON Schema validation (v2.3.1)
- ✅ Custom MCP Tools - YAML-defined website-specific AI tools (v2.3.2)

### Security & Access Control (v2.4)

**v2.4 - Authentication & Authorization**
- ✅ JWT Authentication - JSON Web Tokens with expiry
- ✅ API Keys - Long-lived keys for scripts and automation
- ✅ bcrypt Password Hashing - Secure password storage
- ✅ Collection-level RBAC - Read/Write/Admin permissions
- ✅ User Management - Multi-user support with admin roles
- ✅ User CRUD - Create, read, update, delete users (admin only)
- ✅ Permission Checks - Enforced at every API call

**v2.4.1 - Group Permissions**
- ✅ Group Management - Organize users into logical groups
- ✅ Group Permissions - Grant collection permissions to groups
- ✅ Inherited Permissions - Users get permissions from all their groups
- ✅ Multiple Groups - Users can belong to multiple groups

### Modern APIs (v2.5) 🆕

**v2.5.0 - GraphQL & Enhanced Tooling**
- ✅ GraphQL API - Flexible queries with schema introspection
- ✅ GraphQL Playground - Interactive development tool
- ✅ Authentication Directives - @auth, @hasRole, @hasPermission
- ✅ Query Complexity Limits - Prevent expensive queries
- ✅ CLI GraphQL Support - Use GraphQL via mddb-cli --graphql
- ✅ CLI Playground Command - Open GraphQL Playground in browser
- ✅ Web Panel GraphQL Toggle - Switch between REST and GraphQL
- ✅ Web Panel Settings - API mode selection and configuration
- ✅ Embedding Providers - Cohere and Voyage AI support added

---

## 🚧 Planned Features (Future Releases)

### v2.6 - Performance & Scale (Q2 2026)

**Streaming Export**
- ⏳ Memory-efficient ZIP export for large datasets
- ⏳ Streaming NDJSON export with backpressure
- ⏳ Resumable exports with checkpoints
- ⏳ Progress tracking and cancellation

**Performance Optimizations**
- ⏳ Query result caching with TTL
- ⏳ Prepared statement equivalent for gRPC
- ⏳ Connection pooling for concurrent requests
- ⏳ Adaptive batch sizing based on payload

### v2.7 - Replication (Q3 2026)

**Read Replicas**
- ⏳ Built-in replication for horizontal scaling
- ⏳ Read-only replicas for load distribution
- ⏳ Automatic replica discovery
- ⏳ Replication lag monitoring

**Leader Election**
- ⏳ Automatic failover to replica
- ⏳ Split-brain prevention
- ⏳ Health checks and heartbeats

### v2.8 - Clustering (Q4 2026)

**Multi-node Setup**
- ⏳ Distributed consensus (Raft)
- ⏳ Automatic shard distribution
- ⏳ Cluster membership management
- ⏳ Cross-node queries

**High Availability**
- ⏳ Quorum-based writes
- ⏳ Read from any node
- ⏳ Node recovery and re-sync

### v3.0 - Extensibility (2027)

**Plugin System**
- ⏳ Go plugin architecture
- ⏳ Custom storage backends
- ⏳ Custom embedding providers
- ⏳ Custom authentication providers
- ⏳ Custom search algorithms
- ⏳ Plugin marketplace

**Event Streaming**
- ⏳ Kafka integration for event publishing
- ⏳ NATS integration for lightweight messaging
- ⏳ Redis Streams support
- ⏳ Change data capture (CDC)

### v3.1 - GraphQL Advanced (2027)

**GraphQL Subscriptions**
- ⏳ Real-time updates via WebSocket
- ⏳ Document change notifications
- ⏳ Collection-level subscriptions
- ⏳ Filtered subscriptions by metadata

**GraphQL Federation**
- ⏳ Apollo Federation support
- ⏳ Subgraph schema
- ⏳ Reference resolution

### v3.2 - Advanced Search (2027)

**Hybrid Search**
- ⏳ Combined vector + full-text search
- ⏳ Configurable ranking weights
- ⏳ Re-ranking with cross-encoders
- ⏳ Query expansion and synonyms

**Advanced Vector Search**
- ⏳ HNSW index for faster vector queries
- ⏳ Product quantization for memory efficiency
- ⏳ Multi-vector documents
- ⏳ Hybrid sparse-dense vectors

### v3.3 - Multi-tenancy (2027)

**Namespace Isolation**
- ⏳ Tenant-level data isolation
- ⏳ Per-tenant quotas
- ⏳ Cross-tenant admin queries
- ⏳ Tenant provisioning API

**Resource Limits**
- ⏳ Per-tenant rate limiting
- ⏳ Storage quotas
- ⏳ Bandwidth limits
- ⏳ Webhook rate limiting

### v3.4 - Advanced Caching (2027)

**External Cache Integration**
- ⏳ Redis integration for distributed cache
- ⏳ Memcached support
- ⏳ Cache invalidation webhooks
- ⏳ Cache warming strategies

**Intelligent Caching**
- ⏳ Query result caching with automatic invalidation
- ⏳ Partial result caching
- ⏳ Cache analytics and metrics
- ⏳ Adaptive cache sizing

---

## 🎯 Feature Requests from Community

Vote on features: [GitHub Discussions](https://github.com/tradik/mddb/discussions)

**Most Requested:**
1. 🔥 GraphQL Subscriptions (real-time updates) - **In roadmap v3.1**
2. 🔥 Read Replicas (horizontal scaling) - **In roadmap v2.7**
3. 🔥 Plugin System (custom extensions) - **In roadmap v3.0**
4. 🔥 Streaming Export (memory-efficient) - **In roadmap v2.6**
5. 🔥 Hybrid Search (vector + full-text) - **In roadmap v3.2**

**Under Consideration:**
- 📋 S3/Cloud Storage backend
- 📋 Advanced analytics dashboard
- 📋 Document relationships/links
- 📋 Automatic backups to cloud
- 📋 GUI for schema validation rules
- 📋 Built-in image optimization
- 📋 Markdown linting and validation
- 📋 Multi-region deployment
- 📋 GraphQL schema stitching
- 📋 OpenAPI v3.1 spec generation

---

## 📦 Version History

### Latest Releases

**v2.5.0** (2026-03-02) - GraphQL & Enhanced Tooling
- Added GraphQL API with Playground
- Added CLI GraphQL support (--graphql, graphql command, playground command)
- Added Web Panel GraphQL toggle and settings
- Added Cohere and Voyage AI embedding providers
- Fixed linting issues in codebase

**v2.4.1** (2026-02-28) - Group Permissions
- Added group management system
- Added group-based permissions
- Added users to groups assignment
- Updated Web Panel with Groups interface

**v2.4.0** (2026-02-25) - Authentication & Authorization
- Added JWT authentication
- Added API key support
- Added collection-level RBAC
- Added user management API and UI
- Updated Web Panel with auth features

**v2.3.3** (2026-02-20) - Custom MCP Tools
- Added YAML-defined MCP tools
- Added preconfigured semantic_search, search_documents, full_text_search tools
- Updated MCP Docker image

**v2.3.1** (2026-02-15) - Schema Validation
- Added per-collection JSON Schema validation
- Added validation API endpoints
- Updated Web Panel with validation UI

**v2.3.0** (2026-02-10) - MCP & Telemetry
- Added MCP Server (stdio + HTTP modes)
- Added Prometheus metrics endpoint
- Added Windsurf integration guide
- Updated Docker images

**v2.2.0** (2026-02-01) - Full-Text Search
- Added built-in inverted index
- Added TF scoring algorithm
- Added stop word filtering
- Added import from URL feature
- Updated CLI with fts and import-url commands

**v2.1.0** (2026-01-20) - Vector Search & Webhooks
- Added vector search with auto-embeddings
- Added OpenAI and Ollama embedding providers
- Added webhooks with retry logic
- Updated Web Panel with vector search interface

**[Full changelog](../CHANGELOG.md)**

---

## 🛠️ Development Status

**Current Focus:** v2.5.0 Release (GraphQL API)
- ✅ GraphQL schema design
- ✅ Resolver implementation
- ✅ Authentication directives
- ✅ CLI GraphQL support
- ✅ Web Panel GraphQL toggle
- ✅ Documentation
- ✅ Testing

**Next Up:** v2.6.0 (Performance & Scale)
- ⏳ Streaming export implementation
- ⏳ Performance benchmarking
- ⏳ Query result caching
- ⏳ Connection pooling

---

## 💬 Feedback & Suggestions

Have ideas for MDDB? We want to hear from you!

- **Feature Requests:** [GitHub Discussions](https://github.com/tradik/mddb/discussions/categories/ideas)
- **Bug Reports:** [GitHub Issues](https://github.com/tradik/mddb/issues)
- **Questions:** [GitHub Discussions Q&A](https://github.com/tradik/mddb/discussions/categories/q-a)
- **Contribute:** See [CONTRIBUTING.md](../CONTRIBUTING.md)

---

**[← Back to README](../README.md)** | **[See changelog →](../CHANGELOG.md)**
