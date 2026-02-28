# MDDB API Documentation

> **Note**: The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in [RFC 2119](https://www.ietf.org/rfc/rfc2119.txt).

## Table of Contents
- [Overview](#overview)
- [Configuration](#configuration)
- [Endpoints](#endpoints)
  - [POST /v1/add](#post-v1add)
  - [POST /v1/get](#post-v1get)
  - [POST /v1/search](#post-v1search)
  - [POST /v1/vector-search](#post-v1vector-search)
  - [POST /v1/vector-reindex](#post-v1vector-reindex)
  - [GET /v1/vector-stats](#get-v1vector-stats)
  - [POST /v1/export](#post-v1export)
  - [GET /v1/backup](#get-v1backup)
  - [POST /v1/restore](#post-v1restore)
  - [POST /v1/truncate](#post-v1truncate)
  - [GET /v1/stats](#get-v1stats)
- [Data Models](#data-models)
- [Error Handling](#error-handling)

## Overview

MDDB is a lightweight markdown database server built with Go and BoltDB. It provides a RESTful API for storing, retrieving, and managing markdown documents with metadata.

**Base URL**: `http://localhost:11023`

**API Version**: `v1`

## Configuration

The server can be configured using environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `MDDB_ADDR` | `:11023` | Server address and port |
| `MDDB_MODE` | `wr` | Access mode: `read`, `write`, or `wr` (read+write) |
| `MDDB_PATH` | `mddb.db` | Path to the BoltDB database file |
| `MDDB_EMBEDDING_PROVIDER` | `none` | Embedding provider: `openai`, `ollama`, `voyage`, or `none` |
| `MDDB_EMBEDDING_API_KEY` | | API key for OpenAI or Voyage AI |
| `MDDB_EMBEDDING_API_URL` | *(per provider)* | API base URL (see [Vector Search](#vector-search-configuration)) |
| `MDDB_EMBEDDING_MODEL` | *(per provider)* | Embedding model name |
| `MDDB_EMBEDDING_DIMENSIONS` | *(per provider)* | Vector dimensions |

### Access Modes

- **`read`**: Read-only mode. Write operations will return `403 Forbidden`
- **`write`**: Write-only mode (not commonly used)
- **`wr`**: Read and write mode (recommended for most use cases)

## Endpoints

### POST /v1/add

Add or update a markdown document in a collection.

**Request Body**:
```json
{
  "collection": "blog",
  "key": "homepage",
  "lang": "en_GB",
  "meta": {
    "category": ["blog", "featured"],
    "author": ["John Doe"],
    "tags": ["golang", "database"]
  },
  "contentMd": "# Welcome\n\nThis is the homepage content."
}
```

**Response**:
```json
{
  "id": "blog|homepage|en_gb",
  "key": "homepage",
  "lang": "en_GB",
  "meta": {
    "category": ["blog", "featured"],
    "author": ["John Doe"],
    "tags": ["golang", "database"]
  },
  "contentMd": "# Welcome\n\nThis is the homepage content.",
  "addedAt": 1699296000,
  "updatedAt": 1699296000
}
```

**Features**:
- Creates a new document or updates an existing one
- Automatically generates a deterministic ID based on collection, key, and lang
- Maintains revision history
- Updates metadata indices
- Tracks `addedAt` (first creation) and `updatedAt` (last modification) timestamps

**cURL Example**:
```bash
curl -X POST http://localhost:11023/v1/add \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "key": "homepage",
    "lang": "en_GB",
    "meta": {
      "category": ["blog"]
    },
    "contentMd": "# Welcome to my blog"
  }'
```

---

### POST /v1/get

Retrieve a specific document by collection, key, and language.

**Request Body**:
```json
{
  "collection": "blog",
  "key": "homepage",
  "lang": "en_GB",
  "env": {
    "year": "2024",
    "siteName": "My Blog"
  }
}
```

**Response**:
```json
{
  "id": "blog|homepage|en_gb",
  "key": "homepage",
  "lang": "en_GB",
  "meta": {
    "category": ["blog"]
  },
  "contentMd": "# Welcome to My Blog in 2024",
  "addedAt": 1699296000,
  "updatedAt": 1699296000
}
```

**Features**:
- Retrieves the latest version of a document
- Supports templating via `env` parameter
- Template variables in content are replaced: `%%varName%%` → value from `env`

**Template Example**:

If your content contains:
```markdown
# Welcome to %%siteName%% in %%year%%
```

And you provide:
```json
{
  "env": {
    "year": "2024",
    "siteName": "My Blog"
  }
}
```

The response will contain:
```markdown
# Welcome to My Blog in 2024
```

**cURL Example**:
```bash
curl -X POST http://localhost:11023/v1/get \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "key": "homepage",
    "lang": "en_GB",
    "env": {"year": "2024"}
  }'
```

---

### POST /v1/search

Search for documents in a collection with optional metadata filtering and sorting.

**Request Body**:
```json
{
  "collection": "blog",
  "filterMeta": {
    "category": ["blog", "tutorial"],
    "author": ["John Doe"]
  },
  "sort": "updatedAt",
  "asc": false,
  "limit": 10,
  "offset": 0
}
```

**Parameters**:
- `collection` (required): Collection name
- `filterMeta` (optional): Metadata filters (AND between keys, OR between values)
- `sort` (optional): Sort field - `addedAt`, `updatedAt`, or `key`
- `asc` (optional): Sort order - `true` for ascending, `false` for descending
- `limit` (optional): Maximum number of results (default: 50)
- `offset` (optional): Number of results to skip (default: 0)

**Response**:
```json
[
  {
    "id": "blog|post1|en_gb",
    "key": "post1",
    "lang": "en_GB",
    "meta": {
      "category": ["blog"],
      "author": ["John Doe"]
    },
    "contentMd": "# Post 1",
    "addedAt": 1699296000,
    "updatedAt": 1699296100
  },
  {
    "id": "blog|post2|en_gb",
    "key": "post2",
    "lang": "en_GB",
    "meta": {
      "category": ["tutorial"],
      "author": ["John Doe"]
    },
    "contentMd": "# Post 2",
    "addedAt": 1699295000,
    "updatedAt": 1699296200
  }
]
```

**Filtering Logic**:
- Multiple values for the same key are combined with OR
- Multiple keys are combined with AND
- Example: `{"category": ["blog", "tutorial"], "author": ["John"]}` means:
  - (category = "blog" OR category = "tutorial") AND (author = "John")

**cURL Example**:
```bash
curl -X POST http://localhost:11023/v1/search \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "filterMeta": {"category": ["blog"]},
    "sort": "addedAt",
    "asc": true,
    "limit": 10
  }'
```

---

### POST /v1/vector-search

Perform semantic (vector) search using natural language queries. Documents are automatically embedded when added (if an embedding provider is configured). The search finds documents by meaning, not just exact metadata matches.

**Request Body**:
```json
{
  "collection": "docs",
  "query": "how to authenticate users",
  "topK": 5,
  "threshold": 0.3,
  "filterMeta": {
    "category": ["tutorial"]
  },
  "includeContent": true
}
```

**Parameters**:
- `collection` (required): Collection name
- `query` (required*): Natural language search query (will be embedded server-side)
- `queryVector` (optional*): Pre-computed embedding vector (use instead of `query`)
- `topK` (optional): Maximum results to return (default: 5)
- `threshold` (optional): Minimum similarity score 0.0-1.0 (default: 0.0)
- `filterMeta` (optional): Metadata pre-filter (same logic as `/v1/search`)
- `includeContent` (optional): Include `contentMd` in results (default: false)

\* Either `query` or `queryVector` is required.

**Response**:
```json
{
  "results": [
    {
      "document": {
        "id": "docs|auth-guide|en_us",
        "key": "auth-guide",
        "lang": "en_US",
        "meta": {"category": ["tutorial"]},
        "contentMd": "# Authentication Guide\n...",
        "addedAt": 1709136000,
        "updatedAt": 1709136000
      },
      "score": 0.89,
      "rank": 1
    },
    {
      "document": {
        "id": "docs|login-flow|en_us",
        "key": "login-flow",
        "lang": "en_US",
        "meta": {"category": ["tutorial"]},
        "contentMd": "# Login Flow\n...",
        "addedAt": 1709135000,
        "updatedAt": 1709135000
      },
      "score": 0.74,
      "rank": 2
    }
  ],
  "total": 2,
  "model": "text-embedding-3-small",
  "dimensions": 1536
}
```

**Response Fields**:
- `results`: Array of matched documents with similarity scores
  - `document`: Full document object
  - `score`: Cosine similarity score (0.0-1.0, higher = more similar)
  - `rank`: Position in results (1-based)
- `total`: Number of results returned
- `model`: Embedding model used
- `dimensions`: Vector dimensionality

**How It Works**:
1. When a document is added via `/v1/add`, its content is automatically embedded in the background
2. The query text is embedded using the same model
3. Cosine similarity is computed between the query vector and all document vectors
4. Results are ranked by similarity score
5. If `filterMeta` is provided, only documents matching the metadata filter are searched (hybrid search)

**cURL Example**:
```bash
curl -X POST http://localhost:11023/v1/vector-search \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "docs",
    "query": "how to authenticate users",
    "topK": 5,
    "includeContent": true
  }'
```

---

### POST /v1/vector-reindex

Re-embed all documents in a collection. Useful after changing the embedding provider/model, or for initial indexing of existing documents.

**Request Body**:
```json
{
  "collection": "docs",
  "force": false
}
```

**Parameters**:
- `collection` (required): Collection name
- `force` (optional): If `true`, re-embed all documents regardless of content changes. If `false`, skip documents whose content hasn't changed (default: false)

**Response**:
```json
{
  "embedded": 42,
  "skipped": 8,
  "failed": 0,
  "errors": []
}
```

**cURL Example**:
```bash
curl -X POST http://localhost:11023/v1/vector-reindex \
  -H 'Content-Type: application/json' \
  -d '{"collection": "docs", "force": false}'
```

---

### GET /v1/vector-stats

Get embedding/vector search statistics.

**Response**:
```json
{
  "enabled": true,
  "provider": "text-embedding-3-small",
  "model": "text-embedding-3-small",
  "dimensions": 1536,
  "index_ready": true,
  "collections": {
    "docs": {
      "total_documents": 50,
      "embedded_documents": 48
    },
    "blog": {
      "total_documents": 120,
      "embedded_documents": 120
    }
  }
}
```

**cURL Example**:
```bash
curl http://localhost:11023/v1/vector-stats
```

---

### Vector Search Configuration

#### Embedding Providers

| Provider | `MDDB_EMBEDDING_PROVIDER` | Default Model | Default Dimensions | API Key Required |
|----------|--------------------------|---------------|-------------------|-----------------|
| OpenAI | `openai` | `text-embedding-3-small` | 1536 | Yes |
| Voyage AI (Anthropic) | `voyage` | `voyage-3` | 1024 | Yes |
| Ollama (local) | `ollama` | `nomic-embed-text` | 768 | No |
| Disabled | `none` or empty | - | - | - |

#### Provider-Specific Configuration

**OpenAI**:
```bash
MDDB_EMBEDDING_PROVIDER=openai
MDDB_EMBEDDING_API_KEY=sk-...
MDDB_EMBEDDING_API_URL=https://api.openai.com/v1    # default
MDDB_EMBEDDING_MODEL=text-embedding-3-small          # default
MDDB_EMBEDDING_DIMENSIONS=1536                        # default
```

**Voyage AI (Anthropic)**:
```bash
MDDB_EMBEDDING_PROVIDER=voyage
MDDB_EMBEDDING_API_KEY=pa-...
MDDB_EMBEDDING_API_URL=https://api.voyageai.com/v1   # default
MDDB_EMBEDDING_MODEL=voyage-3                         # default
MDDB_EMBEDDING_DIMENSIONS=1024                        # default
```

**Ollama (local, no API key needed)**:
```bash
MDDB_EMBEDDING_PROVIDER=ollama
MDDB_EMBEDDING_API_URL=http://localhost:11434          # default
MDDB_EMBEDDING_MODEL=nomic-embed-text                  # default
MDDB_EMBEDDING_DIMENSIONS=768                          # default
```

#### Performance Benchmarks (Apple M2)

| Documents | Dimensions | Search Latency | Throughput |
|-----------|-----------|---------------|------------|
| 1,000 | 768 | ~0.9 ms | ~1,064 qps |
| 1,000 | 1,536 | ~1.8 ms | ~544 qps |
| 5,000 | 768 | ~4.8 ms | ~210 qps |
| 10,000 | 768 | ~9.7 ms | ~104 qps |
| 10,000 | 1,536 | ~19 ms | ~52 qps |
| 50,000 | 768 | ~50 ms | ~20 qps |
| 50,000 | 1,536 | ~96 ms | ~10 qps |

Metadata pre-filtering significantly reduces search time (e.g., filtering to 10% of 10K docs: ~1.1 ms vs ~9.7 ms).

---

### POST /v1/export

Export documents from a collection in NDJSON or ZIP format.

**Request Body**:
```json
{
  "collection": "blog",
  "filterMeta": {
    "category": ["blog"]
  },
  "format": "ndjson"
}
```

**Parameters**:
- `collection` (required): Collection name
- `filterMeta` (optional): Metadata filters (same as search)
- `format` (required): Export format - `ndjson` or `zip`

**Response (NDJSON)**:
```
{"id":"blog|post1|en_gb","key":"post1","lang":"en_GB","meta":{"category":["blog"]},"contentMd":"# Post 1","addedAt":1699296000,"updatedAt":1699296100}
{"id":"blog|post2|en_gb","key":"post2","lang":"en_GB","meta":{"category":["blog"]},"contentMd":"# Post 2","addedAt":1699295000,"updatedAt":1699296200}
```

**Response (ZIP)**:
Binary ZIP file containing markdown files named as `{key}.{lang}.md`

**cURL Examples**:

NDJSON export:
```bash
curl -X POST http://localhost:11023/v1/export \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "filterMeta": {"category": ["blog"]},
    "format": "ndjson"
  }' > export.ndjson
```

ZIP export:
```bash
curl -X POST http://localhost:11023/v1/export \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "format": "zip"
  }' > export.zip
```

---

### GET /v1/backup

Create a backup of the database file.

**Query Parameters**:
- `to` (optional): Backup file name (default: `backup-{timestamp}.db`)

**Response**:
```json
{
  "backup": "backup-1699296000.db"
}
```

**cURL Example**:
```bash
curl "http://localhost:11023/v1/backup?to=backup-$(date +%s).db"
```

**Notes**:
- Creates a copy of the entire BoltDB database file
- Backup is created in the same directory as the database
- Does not interrupt server operations

---

### POST /v1/restore

Restore the database from a backup file.

**Request Body**:
```json
{
  "from": "backup-1699296000.db"
}
```

**Response**:
```json
{
  "restored": "backup-1699296000.db"
}
```

**cURL Example**:
```bash
curl -X POST http://localhost:11023/v1/restore \
  -H 'Content-Type: application/json' \
  -d '{"from": "backup-1699296000.db"}'
```

**⚠️ Warning**:
- This operation replaces the current database
- The server briefly closes and reopens the database connection
- All current data will be replaced with the backup

---

### POST /v1/truncate

Truncate revision history and optionally clear cache.

**Request Body**:
```json
{
  "collection": "blog",
  "keepRevs": 3,
  "dropCache": true
}
```

**Parameters**:
- `collection` (required): Collection name
- `keepRevs` (required): Number of recent revisions to keep per document (0 = delete all history)
- `dropCache` (optional): Whether to drop cache (placeholder for future use)

**Response**:
```json
{
  "status": "truncated"
}
```

**cURL Example**:
```bash
curl -X POST http://localhost:11023/v1/truncate \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "keepRevs": 3,
    "dropCache": true
  }'
```

**Use Cases**:
- Reduce database size by removing old revisions
- Keep only recent history for auditing
- Clean up after bulk imports

---

### GET /v1/stats

Get server and database statistics.

**Request**: No body required (GET request)

**Response**:
```json
{
  "databasePath": "mddb.db",
  "databaseSize": 16384,
  "mode": "wr",
  "collections": [
    {
      "name": "blog",
      "documentCount": 42,
      "revisionCount": 156,
      "metaIndexCount": 84
    }
  ],
  "totalDocuments": 42,
  "totalRevisions": 156,
  "totalMetaIndices": 84,
  "uptime": ""
}
```

**Response Fields**:
- `databasePath`: Path to the database file
- `databaseSize`: Database file size in bytes
- `mode`: Access mode (read, write, wr)
- `collections`: Array of collection statistics
  - `name`: Collection name
  - `documentCount`: Number of documents in collection
  - `revisionCount`: Number of revisions in collection
  - `metaIndexCount`: Number of metadata indices in collection
- `totalDocuments`: Total documents across all collections
- `totalRevisions`: Total revisions across all collections
- `totalMetaIndices`: Total metadata indices across all collections

**cURL Example**:
```bash
curl http://localhost:11023/v1/stats
```

**CLI Example**:
```bash
mddb-cli stats
```

**Use Cases**:
- Monitor database growth
- Check collection sizes before operations
- Verify indexing status
- Performance monitoring and capacity planning

---

## Data Models

### Document

```go
{
  "id": string,              // Auto-generated: "collection|key|lang"
  "key": string,             // Document key (e.g., "homepage")
  "lang": string,            // Language code (e.g., "en_GB")
  "meta": {                  // Metadata (multi-value)
    "key1": ["value1", "value2"],
    "key2": ["value3"]
  },
  "contentMd": string,       // Markdown content
  "addedAt": int64,          // Unix timestamp (first creation)
  "updatedAt": int64         // Unix timestamp (last update)
}
```

### Metadata

- Metadata is stored as `map[string][]string` (key → array of values)
- Each metadata key can have multiple values
- Metadata is automatically indexed for fast searching
- Common metadata keys: `category`, `author`, `tags`, `status`, etc.

---

## Error Handling

### Error Response Format

```json
{
  "error": "error message description"
}
```

### HTTP Status Codes

| Code | Description |
|------|-------------|
| `200` | Success |
| `400` | Bad Request - Invalid JSON or missing required fields |
| `403` | Forbidden - Write operation in read-only mode |
| `404` | Not Found - Document doesn't exist |
| `500` | Internal Server Error |

### Common Errors

**Missing required fields**:
```json
{
  "error": "missing fields"
}
```

**Document not found**:
```json
{
  "error": "not found"
}
```

**Read-only mode**:
```json
{
  "error": "read-only mode"
}
```

---

## Best Practices

### 1. Document Keys
- Use descriptive, URL-friendly keys
- Keep keys consistent within a collection
- Example: `homepage`, `about-us`, `blog-post-1`

### 2. Language Codes
- Use standard language codes (ISO 639-1 + ISO 3166-1)
- Examples: `en_US`, `en_GB`, `pl_PL`, `de_DE`

### 3. Metadata
- Keep metadata keys consistent across documents
- Use arrays even for single values (for consistency)
- Index frequently queried fields

### 4. Collections
- Group related documents in collections
- Use collections like database tables
- Examples: `blog`, `pages`, `products`, `docs`

### 5. Revisions
- Regularly truncate old revisions to save space
- Keep enough history for your audit requirements
- Consider keeping 5-10 recent revisions

### 6. Backups
- Schedule regular backups
- Store backups in a different location
- Test restore procedures periodically

---

## Performance Tips

1. **Indexing**: Metadata is automatically indexed - use it for filtering
2. **Pagination**: Always use `limit` and `offset` for large result sets
3. **Batch Operations**: Use export/import for bulk operations
4. **Revisions**: Truncate old revisions regularly to keep database size manageable
5. **Read Mode**: Use read-only mode for read-heavy workloads with separate write instances
