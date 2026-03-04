# Custom YAML Tools for MCP

Define website-specific MCP tools in YAML that simplify AI interactions with your MDDB data.

## Overview

By default, `mddb-mcp` exposes 23 built-in tools (add_document, search_documents, semantic_search, etc.). Custom tools let you create **domain-specific wrappers** with preconfigured defaults — so the AI sees `search_faq(query)` instead of `semantic_search(collection, query, topK, threshold, ...)`.

Custom tools are defined in `config.yaml` under the `custom_tools:` key. They are registered alongside built-in tools and work on both transports (stdio and HTTP).

## Quick Example

```yaml
# config.yaml
mcp:
  listenAddress: "0.0.0.0:9000"
mddb:
  restBaseUrl: "http://localhost:11023"

custom_tools:
  - name: search_faq
    description: "Search frequently asked questions"
    action: semantic_search
    defaults:
      collection: faq
      topK: 3
      threshold: 0.6
    parameters:
      - name: query
        type: string
        required: true
        description: "User question"

  - name: latest_news
    description: "Get latest news articles"
    action: search_documents
    defaults:
      collection: news
      sort: updatedAt
      asc: false
      limit: 5

  - name: search_products
    description: "Search product catalog by keyword"
    action: full_text_search
    defaults:
      collection: products
      limit: 10
    parameters:
      - name: query
        type: string
        required: true
```

## Configuration Reference

### CustomTool fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Tool name exposed to LLM (must not conflict with built-in names) |
| `description` | string | yes | Human-readable description shown in tools/list |
| `action` | string | yes | Underlying MDDB operation (see Supported Actions) |
| `defaults` | object | no | Default values merged with user arguments |
| `parameters` | array | no | Parameters exposed to the LLM (omit for zero-arg tools) |

### Supported Actions

| Action | Maps to | Use case |
|--------|---------|----------|
| `semantic_search` | `toolSemanticSearch` → `/v1/vector-search` | Find documents by meaning |
| `search_documents` | `toolSearchDocuments` → `/v1/search` | Filter/sort by metadata |
| `full_text_search` | `toolFTSSearch` → `/v1/fts` | Keyword search with TF scoring |

### Defaults

All defaults are optional. User-provided arguments override defaults.

| Default | Type | Actions | Description |
|---------|------|---------|-------------|
| `collection` | string | all | Target collection |
| `topK` | int | semantic_search | Max results |
| `threshold` | float | semantic_search | Minimum similarity score (0-1) |
| `includeContent` | bool | semantic_search | Include document content in results |
| `sort` | string | search_documents | Sort field (addedAt, updatedAt) |
| `asc` | bool | search_documents | Sort ascending (default false) |
| `limit` | int | search_documents, full_text_search | Max results |
| `offset` | int | search_documents | Pagination offset |
| `filterMeta` | object | semantic_search, search_documents | Metadata filter |
| `query` | string | semantic_search, full_text_search | Default query (usually overridden) |

### Parameters

Each parameter defines an argument that the LLM can provide.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Argument name |
| `type` | string | yes | JSON Schema type: `string`, `integer`, `number`, `boolean`, `object` |
| `required` | bool | no | Whether the LLM must provide this argument |
| `description` | string | no | Description shown to the LLM |

## How It Works

```
AI calls: search_faq(query: "reset password")
  → mddb-mcp finds "search_faq" in custom tools
  → Merges defaults: {collection:"faq", topK:3, threshold:0.6}
  → Merges user args: + {query:"reset password"}
  → Calls: semantic_search({collection:"faq", query:"reset password", topK:3, threshold:0.6})
  → Returns: vector search results
```

User arguments always override defaults. For example, if a custom tool has `defaults.limit: 5` but the user passes `limit: 20`, the value `20` is used.

## Validation

At startup, `mddb-mcp` validates all custom tools:

- **Name conflicts**: Custom tool names cannot match any of the 23 built-in names
- **Duplicate names**: No two custom tools can share the same name
- **Valid action**: Must be one of `semantic_search`, `search_documents`, `full_text_search`
- **Valid parameter types**: Must be valid JSON Schema types
- **Required fields**: `name`, `description`, and `action` are required

Invalid configuration causes a startup error with a descriptive message.

## Docker Deployment

```bash
# Mount your config with custom tools
docker run -i --rm --network host \
  -v ./config.yaml:/app/config.yaml \
  -e MDDB_MCP_STDIO=true \
  -e MDDB_MCP_CONFIG=/app/config.yaml \
  tradik/mddb:latest
```

Docker Compose:
```yaml
services:
  mddb-mcp:
    image: tradik/mddb:latest
    volumes:
      - ./config.yaml:/app/config.yaml
    environment:
      - MDDB_MCP_STDIO=true
      - MDDB_MCP_CONFIG=/app/config.yaml
      - MDDB_REST_BASE_URL=http://mddb:11023
```

## Windsurf / Claude Desktop

```json
{
  "mcpServers": {
    "mddb": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm", "--network", "host",
        "-v", "./config.yaml:/app/config.yaml",
        "-e", "MDDB_MCP_STDIO=true",
        "-e", "MDDB_MCP_CONFIG=/app/config.yaml",
        "tradik/mddb:latest"
      ]
    }
  }
}
```

## Use Cases

### E-commerce Site
```yaml
custom_tools:
  - name: search_products
    description: "Find products by description"
    action: semantic_search
    defaults: { collection: products, topK: 5, threshold: 0.5 }
    parameters:
      - { name: query, type: string, required: true, description: "Product search query" }

  - name: bestsellers
    description: "Show bestselling products"
    action: search_documents
    defaults: { collection: products, sort: updatedAt, asc: false, limit: 10, filterMeta: { tag: ["bestseller"] } }
```

### Knowledge Base
```yaml
custom_tools:
  - name: ask_kb
    description: "Search the knowledge base"
    action: semantic_search
    defaults: { collection: kb, topK: 5, threshold: 0.6 }
    parameters:
      - { name: query, type: string, required: true, description: "Your question" }

  - name: search_articles
    description: "Full-text search in articles"
    action: full_text_search
    defaults: { collection: articles, limit: 10 }
    parameters:
      - { name: query, type: string, required: true }
```

### Multi-language Blog
```yaml
custom_tools:
  - name: latest_posts
    description: "Get the latest blog posts"
    action: search_documents
    defaults: { collection: blog, sort: updatedAt, asc: false, limit: 5 }

  - name: search_blog
    description: "Search blog posts"
    action: full_text_search
    defaults: { collection: blog, limit: 10 }
    parameters:
      - { name: query, type: string, required: true, description: "Search keywords" }
```

## Backward Compatibility

- If no `custom_tools` key is present in config.yaml, `mddb-mcp` works exactly as before
- Built-in tools are always available regardless of custom tools configuration
- Custom tools are additive — they never replace built-in tools
