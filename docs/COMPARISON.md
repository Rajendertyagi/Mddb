# MDDB vs Alternatives

How MDDB compares to other database and content management solutions.

## vs Traditional Databases

### PostgreSQL / MySQL

**Advantages of MDDB:**
- ✅ **Specialized for Markdown** - Native support, not plain text blobs
- ✅ **Zero Configuration** - No database server to install and manage
- ✅ **Built-in Versioning** - Automatic revision history without triggers
- ✅ **Simpler Deployment** - Single ~27MB binary vs database server
- ✅ **Lower Resource Usage** - Minimal memory footprint (~50MB)
- ✅ **Embedded** - No network latency, direct file access
- ✅ **Vector Search Built-in** - No pgvector extension needed

**When to use PostgreSQL/MySQL instead:**
- Need SQL joins and complex relational queries
- Require multi-table transactions
- Building traditional CRUD applications
- Need mature ecosystem (ORMs, GUIs, tools)

---

## vs Document Databases

### MongoDB

**Advantages of MDDB:**
- ✅ **Markdown-First** - Purpose-built, not generic JSON storage
- ✅ **Triple Protocol** - HTTP, gRPC, GraphQL (MongoDB: wire protocol only)
- ✅ **Smaller Footprint** - 15MB Docker image vs ~400MB
- ✅ **Type-Safe APIs** - gRPC/Protobuf + GraphQL schema
- ✅ **Simpler Operations** - No sharding/replication complexity
- ✅ **Built-in Full-Text Search** - No Atlas Search needed

**When to use MongoDB instead:**
- Need horizontal sharding at scale (>100GB)
- Require geo-spatial queries
- Building complex aggregation pipelines
- Need change streams for real-time sync

### CouchDB

**Advantages of MDDB:**
- ✅ **Higher Performance** - 95x faster writes (29K vs 312 docs/s)
- ✅ **Lower Latency** - 34µs vs 3.2ms average
- ✅ **Modern APIs** - gRPC + GraphQL vs HTTP-only
- ✅ **Vector Search** - Built-in semantic search
- ✅ **Smaller Size** - 15MB vs ~200MB Docker image

**When to use CouchDB instead:**
- Need master-master replication
- Require offline-first mobile sync
- Building distributed systems

---

## vs File-Based Systems

### Git + Filesystem

**Advantages of MDDB:**
- ✅ **Instant Queries** - Indexed metadata vs grep/find
- ✅ **API Access** - REST/gRPC/GraphQL vs file system calls
- ✅ **Concurrent Access** - ACID transactions vs file locks
- ✅ **Rich Metadata** - Structured tags vs filename conventions
- ✅ **Better Performance** - Optimized for document ops
- ✅ **Vector Search** - Semantic similarity impossible with files
- ✅ **Full-Text Search** - Built-in inverted index

**When to use Git/Filesystem instead:**
- Content authored by non-technical users (GUI editors)
- Need distributed collaboration (GitHub, GitLab)
- Require merge conflict resolution
- Want human-readable diffs

---

## vs CMS Platforms

### WordPress

**Advantages of MDDB:**
- ✅ **Lightweight** - 27MB binary vs ~200MB PHP + MySQL
- ✅ **API-First** - Built for programmatic access
- ✅ **No PHP/Database** - Single Go binary
- ✅ **Version Control** - Built-in, not plugin-based
- ✅ **Developer-Friendly** - Simple API vs WordPress hooks
- ✅ **Performance** - Direct DB access vs WordPress query overhead

**When to use WordPress instead:**
- Need visual page builder
- Require thousands of plugins
- Non-technical content editors
- Want mature themes ecosystem

### Strapi

**Advantages of MDDB:**
- ✅ **Simpler** - Embedded DB vs separate database server
- ✅ **Smaller** - 15MB Docker image vs ~600MB
- ✅ **Faster Startup** - Instant vs 30-60s
- ✅ **Purpose-Built** - Markdown-specific vs generic headless CMS
- ✅ **Triple Protocol** - HTTP + gRPC + GraphQL from start

**When to use Strapi instead:**
- Need admin UI for content management
- Require rich media management (images, videos)
- Building complex content types with relationships
- Want plugin marketplace

---

## vs Vector Databases

### Pinecone / Weaviate / Qdrant

**Advantages of MDDB:**
- ✅ **All-in-One** - Vectors + metadata + full-text + storage
- ✅ **Embedded** - No separate vector DB service
- ✅ **Markdown-Native** - Built for documents, not just vectors
- ✅ **Simpler Architecture** - Single database for everything
- ✅ **Lower Cost** - No vector DB subscription needed

**When to use dedicated vector DB instead:**
- Need >1M vectors at scale
- Require advanced vector indexes (HNSW, IVF)
- Building pure semantic search (no metadata/full-text)
- Need sub-5ms vector query latency

---

## vs Search Engines

### Elasticsearch

**Advantages of MDDB:**
- ✅ **Zero Configuration** - No cluster setup, no index mappings
- ✅ **Embedded** - No separate search service
- ✅ **Simpler** - Single binary vs Java + heap tuning
- ✅ **Lower Resources** - 50MB RAM vs 2GB+ for Elasticsearch
- ✅ **Built-in Storage** - Documents + search in one place

**When to use Elasticsearch instead:**
- Need distributed search across nodes
- Require advanced text analysis (stemming, synonyms)
- Building log aggregation (ELK stack)
- Need Kibana dashboards

---

## vs Key-Value Stores

### Redis

**Advantages of MDDB:**
- ✅ **Persistent by Default** - No AOF/RDB configuration
- ✅ **Rich Metadata** - Structured tags vs string keys
- ✅ **Full Documents** - Not just key-value pairs
- ✅ **Revision History** - Built-in versioning
- ✅ **Vector Search** - Semantic similarity (Redis requires RediSearch)

**When to use Redis instead:**
- Need sub-millisecond latency for caching
- Require pub/sub messaging
- Building real-time leaderboards
- Need data structures (lists, sets, sorted sets)

**MDDB + Redis:** Use MDDB for storage, Redis for caching

---

## vs Static Site Generators

### Hugo / Jekyll / Gatsby

**Advantages of MDDB:**
- ✅ **Dynamic Content** - No rebuild needed for updates
- ✅ **API Access** - Query content programmatically
- ✅ **Real-time Updates** - Instant vs regenerate + deploy
- ✅ **Search Built-in** - Metadata + full-text + vector search
- ✅ **Multi-language** - Same key, multiple languages

**When to use SSG instead:**
- Need maximum performance (pre-rendered HTML)
- Building purely static sites
- Want CDN distribution
- Content changes infrequently

**MDDB + SSG:** Use MDDB as CMS, SSG pulls content via API for builds

---

## Performance Comparison

Benchmark: 3000 documents, batch inserts

| Database | Throughput | Avg Latency | Memory Usage |
|----------|------------|-------------|--------------|
| **MDDB (Batch API)** | **29,810 docs/s** | **34µs** | **~50MB** |
| MongoDB | 5,176 docs/s | 192µs | ~200MB |
| PostgreSQL | 4,324 docs/s | 231µs | ~150MB |
| MySQL | 1,214 docs/s | 822µs | ~300MB |
| CouchDB | 312 docs/s | 3,185µs | ~180MB |

**See also:** [Performance Benchmarks](PERFORMANCE.md)

---

## Feature Matrix

| Feature | MDDB | MongoDB | PostgreSQL | WordPress | Elasticsearch |
|---------|------|---------|------------|-----------|---------------|
| **Markdown Native** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Embedded** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **HTTP/JSON API** | ✅ | ❌ | ❌ | ✅ | ✅ |
| **gRPC API** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **GraphQL API** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Vector Search** | ✅ | ✅ (Atlas) | ✅ (pgvector) | ❌ | ✅ (v8.0+) |
| **Full-Text Search** | ✅ | ✅ (Atlas) | ✅ | ❌ | ✅ |
| **Revision History** | ✅ | ❌ | ❌ | ✅ (plugin) | ❌ |
| **Document TTL** | ✅ | ✅ | ❌ | ❌ | ✅ |
| **Webhooks** | ✅ | ✅ (triggers) | ✅ (NOTIFY) | ✅ | ✅ (watcher) |
| **Multi-language** | ✅ | ❌ | ❌ | ✅ (plugin) | ❌ |
| **Auth + RBAC** | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Single Binary** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Docker Size** | 15MB | 400MB | 200MB | 200MB | 600MB |

---

## When to Choose MDDB

Choose MDDB when you need:

1. **Markdown as primary data format**
2. **Embedded database** (no separate server)
3. **Version control** out of the box
4. **Triple protocol** (HTTP + gRPC + GraphQL)
5. **Vector + full-text search** built-in
6. **Simple deployment** (single binary or Docker)
7. **Low resource usage** (<100MB RAM)
8. **Developer-first** API design
9. **RAG pipelines** with semantic search
10. **Multi-language content** management

---

## When to Choose Alternatives

**Choose PostgreSQL/MySQL if:**
- Building traditional relational apps
- Need SQL joins and complex queries
- Require ACID across multiple tables

**Choose MongoDB if:**
- Need horizontal sharding at massive scale
- Building geo-spatial applications
- Require change streams

**Choose WordPress if:**
- Need visual content editor for non-technical users
- Want thousands of plugins
- Building traditional websites

**Choose Elasticsearch if:**
- Need distributed search cluster
- Building log aggregation system
- Require advanced text analysis

**Choose dedicated vector DB if:**
- Pure vector search (>1M vectors)
- Need sub-5ms latency
- Advanced vector indexing (HNSW, IVF)

---

**[← Back to README](../README.md)** | **[See all features →](FEATURES.md)**
