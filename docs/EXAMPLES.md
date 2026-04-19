---
title: "MDDB Usage Examples"
slug: "docs/examples"
description: "MDDB Usage Examples"
status: publish
---

# MDDB Usage Examples

## Basic Operations

### Adding Documents

```bash
# Simple document
curl -X POST http://localhost:11023/v1/add \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "key": "hello",
    "lang": "en_US",
    "meta": {"category": ["blog"]},
    "contentMd": "# Hello World"
  }'

# Document with multiple metadata values
curl -X POST http://localhost:11023/v1/add \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "key": "tutorial",
    "lang": "en_US",
    "meta": {
      "category": ["tutorial", "beginner"],
      "tags": ["golang", "database", "markdown"],
      "author": ["John Doe"]
    },
    "contentMd": "# Tutorial Content"
  }'
```

### Retrieving Documents

```bash
# Basic retrieval
curl -X POST http://localhost:11023/v1/get \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "key": "hello",
    "lang": "en_US"
  }'

# With template variables
curl -X POST http://localhost:11023/v1/get \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "key": "hello",
    "lang": "en_US",
    "env": {
      "year": "2024",
      "siteName": "My Blog"
    }
  }'
```

## Search Examples

### Basic Search

```bash
# All documents in collection
curl -X POST http://localhost:11023/v1/search \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "limit": 50
  }'

# Search by category
curl -X POST http://localhost:11023/v1/search \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "filterMeta": {"category": ["tutorial"]},
    "sort": "addedAt",
    "asc": false
  }'
```

### Advanced Filtering

```bash
# Multiple categories (OR logic)
curl -X POST http://localhost:11023/v1/search \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "filterMeta": {
      "category": ["tutorial", "guide", "howto"]
    }
  }'

# Multiple criteria (AND logic)
curl -X POST http://localhost:11023/v1/search \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "filterMeta": {
      "category": ["tutorial"],
      "author": ["John Doe"],
      "status": ["published"]
    }
  }'
```

### Pagination

```bash
# Page 1 (first 10 items)
curl -X POST http://localhost:11023/v1/search \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "limit": 10,
    "offset": 0
  }'

# Page 2 (next 10 items)
curl -X POST http://localhost:11023/v1/search \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "limit": 10,
    "offset": 10
  }'
```

### Full-Text Search (v2.9.12+ and v2.9.13+ features)

```bash
# Per-query boost / demote (v2.9.12+) — rerank without reindexing
curl -X POST http://localhost:11023/v1/fts \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "query": "tutorial",
    "algorithm": "bm25",
    "boost": {"tag:featured": 5.0, "status:archived": -2.0}
  }'

# Prefix autocomplete for type-ahead UI (v2.9.12+)
curl "http://localhost:11023/v1/autocomplete?collection=blog&q=mar&topN=5"

# Expression mode — parens, precedence, mixed atoms (v2.9.13+)
curl -X POST http://localhost:11023/v1/fts \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "query": "(rust OR golang) AND \"async runtime\"~3 NOT legacy",
    "mode": "expression"
  }'

# Highlighting with fragments (v2.9.13+) — returns snippets per result
curl -X POST http://localhost:11023/v1/fts \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "query": "markdown database",
    "highlight": true,
    "highlightTag": "<mark>",
    "maxHighlights": 2,
    "fragmentSize": 120
  }'
```

### Hybrid Search with Geo Distance Sort (v2.9.13+)

```bash
# Nearest matching venue — sort by proximity instead of fused score
curl -X POST http://localhost:11023/v1/hybrid-search \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "venues",
    "query": "coffee wifi",
    "topK": 10,
    "geo": {"lat": 52.52, "lng": 13.405, "radiusMeters": 3000},
    "sort": "distance"
  }'
```

### GeoJSON Polygon / Multi-Polygon Containment (v2.9.13+)

```bash
# Find points inside a custom polygon (Berlin Mitte bounding box)
curl -X POST http://localhost:11023/v1/geo-polygon \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "venues",
    "polygon": {
      "type": "Polygon",
      "coordinates": [
        [[13.36,52.51],[13.42,52.51],[13.42,52.53],[13.36,52.53],[13.36,52.51]]
      ]
    }
  }'

# Polygon with a hole — square minus central square
curl -X POST http://localhost:11023/v1/geo-polygon \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "venues",
    "polygon": {
      "coordinates": [
        [[0,0],[10,0],[10,10],[0,10],[0,0]],
        [[3,3],[7,3],[7,7],[3,7],[3,3]]
      ]
    }
  }'

# MultiPolygon union across two regions (Berlin + Paris)
curl -X POST http://localhost:11023/v1/geo-polygon \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "venues",
    "multiPolygon": {
      "coordinates": [
        [[[13.36,52.51],[13.42,52.51],[13.42,52.53],[13.36,52.53],[13.36,52.51]]],
        [[[2.34,48.85],[2.36,48.85],[2.36,48.87],[2.34,48.87],[2.34,48.85]]]
      ]
    }
  }'
```

### Async Bulk Ingest with Job Tracking (v2.9.12+)

```bash
# Submit a long-running import — returns 202 with a job id
JOB=$(curl -s -X POST http://localhost:11023/v1/bulk-ingest-job \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "archive",
    "documents": [ ... tens of thousands of docs ... ],
    "callbackUrl": "https://example.com/webhook"
  }' | jq -r .id)

# Poll status until completed / failed
curl "http://localhost:11023/v1/bulk-ingest-job/$JOB"

# List all jobs for a collection, newest first
curl "http://localhost:11023/v1/bulk-ingest-jobs?collection=archive"

# Cancel a pending job (already-processing jobs run to completion)
curl -X DELETE "http://localhost:11023/v1/bulk-ingest-job/$JOB"
```

## Vector Search (Semantic Search)

### Setup

First, configure an embedding provider:
```bash
# Option 1: OpenAI (cloud, best quality)
export MDDB_EMBEDDING_PROVIDER=openai
export MDDB_EMBEDDING_API_KEY=sk-your-key-here

# Option 2: Voyage AI / Anthropic (cloud)
export MDDB_EMBEDDING_PROVIDER=voyage
export MDDB_EMBEDDING_API_KEY=pa-your-key-here

# Option 3: Ollama (local, free, no API key)
export MDDB_EMBEDDING_PROVIDER=ollama
# Make sure ollama is running: ollama serve
# Pull model: ollama pull nomic-embed-text
```

### Adding Documents (auto-embedding)

When an embedding provider is configured, documents are automatically embedded when added:

```bash
# Add a document about authentication
curl -X POST http://localhost:11023/v1/add \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "docs",
    "key": "auth-guide",
    "lang": "en_US",
    "meta": {"category": ["security"], "type": ["guide"]},
    "contentMd": "# Authentication Guide\n\nThis guide covers user authentication using JWT tokens. Learn how to implement login, registration, and session management."
  }'

# Add more documents
curl -X POST http://localhost:11023/v1/add \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "docs",
    "key": "api-reference",
    "lang": "en_US",
    "meta": {"category": ["api"], "type": ["reference"]},
    "contentMd": "# API Reference\n\nComplete REST API reference with endpoints, parameters, and response formats."
  }'

curl -X POST http://localhost:11023/v1/add \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "docs",
    "key": "deployment",
    "lang": "en_US",
    "meta": {"category": ["devops"], "type": ["guide"]},
    "contentMd": "# Deployment Guide\n\nHow to deploy the application to production using Docker, Kubernetes, and CI/CD pipelines."
  }'
```

### Semantic Search (finding documents by meaning)

```bash
# Search by meaning - finds auth-guide even though query doesn't match exact keywords
curl -X POST http://localhost:11023/v1/vector-search \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "docs",
    "query": "how to login users",
    "topK": 3,
    "includeContent": true
  }'
# Returns: auth-guide (score: ~0.85), api-reference (score: ~0.42), ...
```

Response shows which documents are most relevant and their similarity scores:
```json
{
  "results": [
    {
      "document": {
        "id": "docs|auth-guide|en_us",
        "key": "auth-guide",
        "meta": {"category": ["security"], "type": ["guide"]},
        "contentMd": "# Authentication Guide\n..."
      },
      "score": 0.85,
      "rank": 1
    },
    {
      "document": {
        "id": "docs|api-reference|en_us",
        "key": "api-reference",
        "meta": {"category": ["api"], "type": ["reference"]}
      },
      "score": 0.42,
      "rank": 2
    }
  ],
  "total": 2,
  "model": "text-embedding-3-small",
  "dimensions": 1536
}
```

### Hybrid Search (vector + metadata filter)

Combine semantic search with metadata pre-filtering:
```bash
# Find security-related docs about passwords
curl -X POST http://localhost:11023/v1/vector-search \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "docs",
    "query": "password reset flow",
    "topK": 5,
    "filterMeta": {"category": ["security"]},
    "includeContent": false
  }'
```

### Search with Similarity Threshold

```bash
# Only return highly relevant results (score > 0.7)
curl -X POST http://localhost:11023/v1/vector-search \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "docs",
    "query": "kubernetes deployment",
    "topK": 10,
    "threshold": 0.7
  }'
```

### Reindex Existing Documents

If you added documents before configuring embeddings, or changed the model:
```bash
# Reindex only changed documents (smart - skips already-embedded)
curl -X POST http://localhost:11023/v1/vector-reindex \
  -H 'Content-Type: application/json' \
  -d '{"collection": "docs", "force": false}'

# Force reindex everything (after model change)
curl -X POST http://localhost:11023/v1/vector-reindex \
  -H 'Content-Type: application/json' \
  -d '{"collection": "docs", "force": true}'
```

### Check Embedding Status

```bash
# See which collections have embeddings
curl http://localhost:11023/v1/vector-stats | jq
```

Response:
```json
{
  "enabled": true,
  "provider": "text-embedding-3-small",
  "model": "text-embedding-3-small",
  "dimensions": 1536,
  "index_ready": true,
  "collections": {
    "docs": {
      "total_documents": 3,
      "embedded_documents": 3
    }
  }
}
```

## Export Examples

### NDJSON Export

```bash
# Export all documents
curl -X POST http://localhost:11023/v1/export \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "format": "ndjson"
  }' > export.ndjson

# Export filtered documents
curl -X POST http://localhost:11023/v1/export \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "filterMeta": {"status": ["published"]},
    "format": "ndjson"
  }' > published.ndjson
```

### ZIP Export

```bash
# Export as ZIP
curl -X POST http://localhost:11023/v1/export \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "format": "zip"
  }' > export.zip
```

## Backup & Restore

### Creating Backups

```bash
# Automatic timestamp
curl "http://localhost:11023/v1/backup?to=backup-$(date +%s).db"

# Custom name
curl "http://localhost:11023/v1/backup?to=my-backup.db"

# Daily backup script
#!/bin/bash
DATE=$(date +%Y-%m-%d)
curl "http://localhost:11023/v1/backup?to=backup-${DATE}.db"
```

### Restoring from Backup

```bash
curl -X POST http://localhost:11023/v1/restore \
  -H 'Content-Type: application/json' \
  -d '{"from": "backup-1699296000.db"}'
```

## Maintenance

### Truncating Revisions

```bash
# Keep last 5 revisions per document
curl -X POST http://localhost:11023/v1/truncate \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "keepRevs": 5,
    "dropCache": true
  }'

# Remove all revision history
curl -X POST http://localhost:11023/v1/truncate \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "keepRevs": 0,
    "dropCache": true
  }'
```

## Shell Scripts

### Bulk Import

```bash
#!/bin/bash
# import-posts.sh

for file in posts/*.md; do
  key=$(basename "$file" .md)
  content=$(cat "$file")
  
  curl -X POST http://localhost:11023/v1/add \
    -H 'Content-Type: application/json' \
    -d "{
      \"collection\": \"blog\",
      \"key\": \"$key\",
      \"lang\": \"en_US\",
      \"meta\": {\"category\": [\"blog\"]},
      \"contentMd\": $(echo "$content" | jq -Rs .)
    }"
done
```

### Export and Process

```bash
#!/bin/bash
# export-and-process.sh

# Export all posts
curl -X POST http://localhost:11023/v1/export \
  -H 'Content-Type: application/json' \
  -d '{"collection": "blog", "format": "ndjson"}' | \
  jq -r '.key + ": " + .meta.category[0]'
```

### Automated Backup

```bash
#!/bin/bash
# backup.sh - Add to crontab for daily backups

BACKUP_DIR="/backups/mddb"
DATE=$(date +%Y-%m-%d-%H%M%S)
KEEP_DAYS=7

# Create backup
curl "http://localhost:11023/v1/backup?to=${BACKUP_DIR}/backup-${DATE}.db"

# Remove old backups
find ${BACKUP_DIR} -name "backup-*.db" -mtime +${KEEP_DAYS} -delete
```

## Integration Examples

### Node.js Client

```javascript
// mddb-client.js
const axios = require('axios');

class MDDBClient {
  constructor(baseURL = 'http://localhost:11023') {
    this.client = axios.create({ baseURL });
  }

  async add(collection, key, lang, meta, contentMd) {
    const response = await this.client.post('/v1/add', {
      collection, key, lang, meta, contentMd
    });
    return response.data;
  }

  async get(collection, key, lang, env = {}) {
    const response = await this.client.post('/v1/get', {
      collection, key, lang, env
    });
    return response.data;
  }

  async search(collection, filterMeta = {}, options = {}) {
    const response = await this.client.post('/v1/search', {
      collection,
      filterMeta,
      ...options
    });
    return response.data;
  }
}

// Usage
const mddb = new MDDBClient();

await mddb.add('blog', 'hello', 'en_US', 
  { category: ['blog'] }, 
  '# Hello World'
);

const doc = await mddb.get('blog', 'hello', 'en_US');
console.log(doc);
```

### Python Client

```python
# mddb_client.py
import requests

class MDDBClient:
    def __init__(self, base_url='http://localhost:11023'):
        self.base_url = base_url
    
    def add(self, collection, key, lang, meta, content_md):
        response = requests.post(f'{self.base_url}/v1/add', json={
            'collection': collection,
            'key': key,
            'lang': lang,
            'meta': meta,
            'contentMd': content_md
        })
        return response.json()
    
    def get(self, collection, key, lang, env=None):
        response = requests.post(f'{self.base_url}/v1/get', json={
            'collection': collection,
            'key': key,
            'lang': lang,
            'env': env or {}
        })
        return response.json()
    
    def search(self, collection, filter_meta=None, **options):
        response = requests.post(f'{self.base_url}/v1/search', json={
            'collection': collection,
            'filterMeta': filter_meta or {},
            **options
        })
        return response.json()

# Usage
mddb = MDDBClient()

mddb.add('blog', 'hello', 'en_US', 
         {'category': ['blog']}, 
         '# Hello World')

doc = mddb.get('blog', 'hello', 'en_US')
print(doc)
```

### Go Client

```go
// mddb_client.go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
)

type MDDBClient struct {
    BaseURL string
}

func (c *MDDBClient) Add(collection, key, lang string, meta map[string][]string, contentMd string) (map[string]interface{}, error) {
    data := map[string]interface{}{
        "collection": collection,
        "key": key,
        "lang": lang,
        "meta": meta,
        "contentMd": contentMd,
    }
    
    body, _ := json.Marshal(data)
    resp, err := http.Post(c.BaseURL+"/v1/add", "application/json", bytes.NewBuffer(body))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    return result, nil
}

// Usage
func main() {
    client := &MDDBClient{BaseURL: "http://localhost:11023"}

    doc, _ := client.Add("blog", "hello", "en_US",
        map[string][]string{"category": {"blog"}},
        "# Hello World")

    fmt.Println(doc)
}
```

## Schema Validation

Schema validation enforces structure on document metadata per collection. It is opt-in -- collections without a schema accept any metadata.

### Setting Up a Schema

```bash
# Define a schema for the "products" collection
curl -X POST http://localhost:11023/v1/schema/set \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "products",
    "schema": {
      "required": ["sku", "price", "category"],
      "properties": {
        "sku":      { "type": "string", "pattern": "^SKU-[0-9]+$" },
        "price":    { "type": "number" },
        "category": { "type": "string", "enum": ["electronics", "clothing", "books"] },
        "tags":     { "type": "string", "minItems": 1, "maxItems": 10 }
      }
    }
  }'
```

### Adding Documents with Validation

```bash
# This succeeds -- all required fields present with valid values
curl -X POST http://localhost:11023/v1/add \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "products",
    "key": "laptop-1",
    "lang": "en_US",
    "meta": {
      "sku": ["SKU-10042"],
      "price": ["999.99"],
      "category": ["electronics"],
      "tags": ["laptop", "computer", "portable"]
    },
    "contentMd": "# Laptop Pro 15\n\nHigh-performance laptop."
  }'

# This fails -- missing "price", invalid sku format, bad category
curl -X POST http://localhost:11023/v1/add \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "products",
    "key": "bad-product",
    "lang": "en_US",
    "meta": {
      "sku": ["INVALID"],
      "category": ["furniture"]
    },
    "contentMd": "# Bad Product"
  }'
# Error: schema validation failed: missing required metadata key "price";
#   value "INVALID" for key "sku" does not match pattern "^SKU-[0-9]+$";
#   value "furniture" for key "category" is not in allowed enum values [electronics, clothing, books]
```

### Dry-Run Validation

```bash
# Check metadata before adding a document
curl -X POST http://localhost:11023/v1/validate \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "products",
    "meta": {
      "sku": ["SKU-20001"],
      "price": ["49.99"],
      "category": ["books"]
    }
  }'
# Response: {"valid": true, "errors": []}
```

### Listing and Managing Schemas

```bash
# List all schemas
curl -X POST http://localhost:11023/v1/schema/list \
  -H 'Content-Type: application/json' \
  -d '{}'

# Get schema for a specific collection
curl -X POST http://localhost:11023/v1/schema/get \
  -H 'Content-Type: application/json' \
  -d '{"collection": "products"}'

# Delete schema to disable validation
curl -X POST http://localhost:11023/v1/schema/delete \
  -H 'Content-Type: application/json' \
  -d '{"collection": "products"}'
```

### Blog Schema Example

```bash
# Enforce consistent blog metadata
curl -X POST http://localhost:11023/v1/schema/set \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "schema": {
      "required": ["category", "author", "status"],
      "properties": {
        "category": { "type": "string", "enum": ["blog", "tutorial", "news", "changelog"] },
        "author":   { "type": "string" },
        "status":   { "type": "string", "enum": ["draft", "published", "archived"] },
        "tags":     { "type": "string", "maxItems": 5 },
        "featured": { "type": "boolean", "maxItems": 1 }
      }
    }
  }'

# Add a valid blog post
curl -X POST http://localhost:11023/v1/add \
  -H 'Content-Type: application/json' \
  -d '{
    "collection": "blog",
    "key": "schema-validation-guide",
    "lang": "en_US",
    "meta": {
      "category": ["tutorial"],
      "author": ["Jane Doe"],
      "status": ["published"],
      "tags": ["mddb", "validation", "schema"],
      "featured": ["true"]
    },
    "contentMd": "# Schema Validation Guide\n\nLearn how to validate document metadata."
  }'
```
