# MDDB Use Cases

Real-world examples and use cases for MDDB.

## 1. Documentation Platforms

Store and version API documentation, technical guides, and knowledge bases.

### Features Used
- Revision history for tracking documentation changes
- Metadata search for finding docs by category, version, status
- Multi-language support for internationalized documentation
- Template variables for environment-specific content

### Example

```bash
# Store API documentation with versioning
mddb-cli add api-docs authentication en_US -f auth.md \
  -m "version=2.0,status=published,category=security"

# Search published API docs
mddb-cli search api-docs -f "status=published" --sort updatedAt

# Get specific version with environment variables
curl -X POST http://localhost:11023/v1/get \
  -d '{"collection": "api-docs", "key": "authentication", "lang": "en_US",
       "env": {"apiUrl": "https://api.example.com"}}'
```

**Perfect for:** Technical documentation, API references, developer guides, internal wikis

---

## 2. Content Management Systems (CMS)

Manage blog posts, articles, and pages with multi-language support.

### Features Used
- Multi-language documents (same key, different languages)
- Rich metadata (author, tags, category, published status)
- Full-text search for content discovery
- Webhooks for cache invalidation on publish

### Example

```bash
# Multi-language blog posts
mddb-cli add blog "getting-started" en_US -f post-en.md \
  -m "author=John,tags=tutorial|beginner,status=published"
mddb-cli add blog "getting-started" pl_PL -f post-pl.md \
  -m "author=John,tags=tutorial|beginner,status=published"

# Search published posts
mddb-cli search blog -f "status=published" -l 20

# Full-text search
mddb-cli fts blog --query="getting started markdown" --limit=10

# Register webhook for CDN invalidation
curl -X POST http://localhost:11023/v1/webhooks \
  -d '{"url": "https://cdn.example.com/invalidate", "events": ["doc.updated", "doc.added"], "collection": "blog"}'
```

**Perfect for:** Blogs, news sites, magazines, multi-language content platforms

---

## 3. Configuration Management

Store configuration templates with dynamic variable substitution.

### Features Used
- Template variables ({{variable}}) for environment-specific configs
- Revision history for config change tracking
- Metadata for environment (prod, staging, dev)
- Document TTL for temporary configurations

### Example

```bash
# Store nginx config template
cat > nginx.conf.md << 'EOF'
server {
    listen 80;
    server_name {{domain}};
    root /var/www/{{app_name}};

    location / {
        proxy_pass http://{{backend_host}}:{{backend_port}};
    }
}
EOF

mddb-cli add configs nginx-prod en_US -f nginx.conf.md \
  -m "env=production,service=web,version=1.0"

# Retrieve with variables substituted
curl -X POST http://localhost:11023/v1/get \
  -d '{"collection": "configs", "key": "nginx-prod", "lang": "en_US",
       "env": {"domain": "example.com", "app_name": "myapp",
               "backend_host": "localhost", "backend_port": "3000"}}'
```

**Perfect for:** Infrastructure configs, Kubernetes manifests, deployment templates, environment-specific settings

---

## 4. Knowledge Bases

Build searchable knowledge bases with rich categorization.

### Features Used
- Vector search for semantic "how do I..." queries
- Full-text search for keyword-based discovery
- Metadata filtering (category, difficulty, tags)
- Import from URL for external documentation

### Example

```bash
# Add troubleshooting guides
mddb-cli add kb vpn-setup en_US -f vpn-guide.md \
  -m "category=support,difficulty=advanced,tags=network|vpn|troubleshooting"

# Import from URL
curl -X POST http://localhost:11023/v1/import-url \
  -d '{"collection": "kb", "url": "https://docs.example.com/faq.md", "lang": "en_US"}'

# Semantic search - find by meaning, not keywords
curl -X POST http://localhost:11023/v1/vector-search \
  -d '{"collection": "kb", "query": "I cant connect to the network from home",
       "topK": 5, "threshold": 0.7, "includeContent": true}'

# Filter by difficulty
mddb-cli search kb -f "category=support,difficulty=beginner"
```

**Perfect for:** Internal wikis, support documentation, FAQs, troubleshooting guides

---

## 5. Microservices Communication

Share templates, content, and configuration between services with high-performance gRPC.

### Features Used
- gRPC protocol for low latency (16x faster than HTTP)
- Template variables for service-specific customization
- Multi-language for localized templates
- MCP server for LLM integration

### Example (Go)

```go
import (
    "context"
    "google.golang.org/grpc"
    mddb "mddb/proto"
)

// Connect to MDDB
conn, _ := grpc.Dial("localhost:11024", grpc.WithInsecure())
client := mddb.NewMDDBClient(conn)

// Get email template
doc, err := client.Get(context.Background(), &mddb.GetRequest{
    Collection: "templates",
    Key:        "email-welcome",
    Lang:       "en_US",
    Env: map[string]string{
        "userName":  "John Doe",
        "loginUrl":  "https://app.example.com/login",
    },
})

// Use template
emailBody := doc.GetContentMd()
sendEmail(userEmail, "Welcome!", emailBody)
```

**Perfect for:** Template storage, shared content, configuration distribution, service-to-service communication

---

## 6. RAG / AI Knowledge Base

Build retrieval-augmented generation (RAG) pipelines with semantic vector search.

### Features Used
- Automatic background embedding generation
- Vector search for semantic retrieval
- Full-text search as fallback
- Import from URL for easy knowledge ingestion
- MCP server for direct LLM integration

### Example

```bash
# Configure embedding provider
export MDDB_EMBEDDING_PROVIDER=openai
export MDDB_EMBEDDING_API_KEY=sk-...
export MDDB_EMBEDDING_MODEL=text-embedding-3-small
export MDDB_EMBEDDING_DIMENSIONS=1536

# Add documents (auto-embedded in background)
mddb-cli add kb faq-billing en_US -f billing.md -m "category=billing"

# Import from URL
curl -X POST http://localhost:11023/v1/import-url \
  -d '{"collection": "kb", "url": "https://docs.example.com/pricing.md", "lang": "en_US"}'

# Semantic search
curl -X POST http://localhost:11023/v1/vector-search \
  -d '{"collection": "kb", "query": "how do I cancel my subscription?",
       "topK": 5, "threshold": 0.7, "includeContent": true}'
```

**RAG Pipeline (Python):**

```python
from mddb import MDDB
import openai

db = MDDB.connect('localhost:11023', 'read').collection('kb')

# Step 1: Retrieve context
results = db.vector_search(
    query="how to cancel subscription",
    top_k=3,
    threshold=0.7,
    include_content=True
)

# Step 2: Build context
context = "\n\n".join([r["document"]["contentMd"] for r in results["results"]])

# Step 3: Send to LLM
response = openai.chat.completions.create(
    model="gpt-4o",
    messages=[
        {"role": "system", "content": f"Answer based on:\n\n{context}"},
        {"role": "user", "content": "How do I cancel my subscription?"}
    ]
)
print(response.choices[0].message.content)
```

**Perfect for:** RAG pipelines, AI assistants, semantic search, chatbot knowledge bases, Q&A systems

**See also:** [RAG Pipeline Guide](RAG-PIPELINE.md)

---

## 7. Event-Driven Architecture

Trigger downstream systems when documents change.

### Features Used
- Webhooks with retry logic
- Event types: `doc.added`, `doc.updated`, `doc.deleted`
- Collection filtering
- Webhook payload includes full document

### Example

```bash
# Register webhook for search index sync
curl -X POST http://localhost:11023/v1/webhooks \
  -d '{"url": "https://search.example.com/index", "events": ["doc.added", "doc.updated"], "collection": "blog"}'

# Register webhook for CDN invalidation
curl -X POST http://localhost:11023/v1/webhooks \
  -d '{"url": "https://cdn.example.com/purge", "events": ["doc.updated"], "collection": "pages"}'

# Register webhook for notifications
curl -X POST http://localhost:11023/v1/webhooks \
  -d '{"url": "https://notify.example.com/slack", "events": ["doc.deleted"], "collection": "legal-docs"}'
```

**Webhook Payload:**

```json
{
  "event": "doc.added",
  "collection": "blog",
  "key": "new-post",
  "lang": "en_US",
  "timestamp": 1709395200,
  "document": {
    "collection": "blog",
    "key": "new-post",
    "lang": "en_US",
    "contentMd": "# New Post...",
    "meta": {"author": ["John"], "tags": ["news"]},
    "addedAt": 1709395200
  }
}
```

**Perfect for:** Real-time notifications, search index sync, CDN invalidation, audit logging, Slack/Discord notifications

---

## 8. Temporary/Cached Content with TTL

Store time-limited content that auto-expires.

### Features Used
- Document TTL (time-to-live) in seconds
- Background cleanup worker
- Set/update/remove TTL on existing documents

### Example

```bash
# Add document with 1 hour expiry
curl -X POST http://localhost:11023/v1/add \
  -d '{"collection": "cache", "key": "session-abc123", "lang": "en",
       "ttl": 3600, "contentMd": "# Session Data\n\nUser: john@example.com"}'

# Update TTL to 2 hours
curl -X POST http://localhost:11023/v1/set-ttl \
  -d '{"collection": "cache", "key": "session-abc123", "lang": "en", "ttl": 7200}'

# Remove TTL (make permanent)
curl -X POST http://localhost:11023/v1/set-ttl \
  -d '{"collection": "cache", "key": "session-abc123", "lang": "en", "ttl": 0}'

# CLI
mddb-cli set-ttl cache session-abc123 en --ttl=3600
```

**Perfect for:** Cache layers, session data, temporary content, rate-limiting state, auto-cleanup

---

## 9. Version-Controlled Content

Track all changes with complete audit trail.

### Features Used
- Automatic revision history
- Full content snapshots per revision
- Metadata versioning
- Truncate old revisions to save space

### Example

```bash
# Initial version
mddb-cli add docs readme en_US -f README-v1.md -m "version=1.0,author=Alice"

# Update creates new revision
mddb-cli add docs readme en_US -f README-v2.md -m "version=2.0,author=Bob"

# Another update
mddb-cli add docs readme en_US -f README-v3.md -m "version=3.0,author=Alice"

# Keep only last 3 revisions
curl -X POST http://localhost:11023/v1/truncate \
  -d '{"collection": "docs", "keepRevs": 3, "dropCache": true}'
```

**Perfect for:** Legal documents, compliance, audit trails, contract versioning, policy management

---

## 10. GraphQL-First Applications

Modern web apps with flexible data requirements.

### Features Used
- GraphQL API with schema introspection
- GraphQL Playground for development
- Authentication directives (@auth, @hasRole)
- Flexible field selection
- Nested queries

### Example

**Enable GraphQL:**
```bash
docker run -e MDDB_GRAPHQL_ENABLED=true -p 11023:11023 tradik/mddb
```

**Query:**
```graphql
query GetBlogPosts {
  search(input: {
    collection: "blog"
    filterMeta: [{key: "status", values: ["published"]}]
    sort: "addedAt"
    asc: false
    limit: 10
  }) {
    results {
      document {
        key
        contentMd
        meta
        addedAt
      }
    }
    total
  }
}
```

**Mutation:**
```graphql
mutation CreatePost {
  addDocument(input: {
    collection: "blog"
    key: "new-post"
    lang: "en"
    meta: [
      {key: "author", values: ["John"]}
      {key: "tags", values: ["tutorial", "graphql"]}
    ]
    contentMd: "# Hello GraphQL"
  }) {
    id
    addedAt
  }
}
```

**Vector Search:**
```graphql
query SemanticSearch {
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

**Perfect for:** Modern web apps, mobile apps, React/Vue/Angular frontends, Apollo Client integration

---

## 11. Multi-language E-commerce

Product catalogs with translations.

### Example

```bash
# English product
mddb-cli add products laptop-x1 en_US -f laptop-en.md \
  -m "category=electronics,price=999,sku=LAP-001,stock=50"

# Polish translation
mddb-cli add products laptop-x1 pl_PL -f laptop-pl.md \
  -m "category=electronics,price=999,sku=LAP-001,stock=50"

# German translation
mddb-cli add products laptop-x1 de_DE -f laptop-de.md \
  -m "category=electronics,price=999,sku=LAP-001,stock=50"

# Get user's language
mddb-cli get products laptop-x1 pl_PL
```

**Perfect for:** E-commerce, product catalogs, multi-region sites

---

## 12. Monitoring & Observability

Track MDDB metrics with Prometheus and Grafana.

### Example

```bash
# Enable metrics (enabled by default)
export MDDB_METRICS=true

# Scrape metrics
curl http://localhost:11023/metrics

# prometheus.yml
scrape_configs:
  - job_name: 'mddb'
    static_configs:
      - targets: ['localhost:11023']
```

**Available Metrics:**
- `mddb_http_requests_total` - request counters
- `mddb_http_request_duration_seconds` - latency histograms
- `mddb_documents_total` - document counts per collection
- `mddb_database_size_bytes` - DB file size
- `mddb_vector_embeddings_total` - embedding count
- `go_goroutines`, `go_memstats_*` - Go runtime

**Perfect for:** DevOps, SRE, production monitoring

**See also:** [Telemetry Guide](TELEMETRY.md)

---

## 13. Bulk Content Migration

Import large amounts of content from existing systems.

### Example

```bash
# Bulk import from folder
./scripts/load-md-folder.sh ./content blog -r -v

# With custom metadata
./scripts/load-md-folder.sh ./docs kb -m "status=published" -m "author=Migration Script"

# Dry run first
./scripts/load-md-folder.sh ./content blog -d

# Import specific files
for file in ./posts/*.md; do
  mddb-cli add blog "$(basename "$file" .md)" en_US -f "$file" -m "imported=true"
done
```

**Perfect for:** Content migrations, initial data load, batch processing

**See also:** [Bulk Import Guide](BULK-IMPORT.md)

---

**[← Back to README](../README.md)** | **[See all features →](FEATURES.md)**
