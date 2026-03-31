# MCP Server Configuration

MDDB has a built-in MCP (Model Context Protocol) server implementing the **2025-11-25** specification. This document covers all MCP configuration options.

**[→ LLM client setup (Claude, Cursor, ChatGPT, Ollama)](LLM_CONNECTIONS.md)** | **[→ Custom YAML tools](CUSTOM-TOOLS.md)**

## Transports

| Transport | Endpoint | Spec Version | Status |
|-----------|----------|-------------|--------|
| **Stdio** | stdin/stdout | 2025-11-25 | Default for Claude Desktop |
| **Streamable HTTP** | `POST/GET/DELETE /mcp` | 2025-11-25 | Recommended for remote |
| **SSE (legacy)** | `GET /sse` + `POST /message` | 2024-11-05 | Backward compatible |

All transports run on the MCP port (default: 9000, configurable via `MDDB_MCP_ADDR`).

### Streamable HTTP (Recommended)

```bash
# Initialize
curl -X POST http://localhost:9000/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}'

# Session ID returned in MCP-Session-Id header — use it for all subsequent requests
```

### Client Configuration

```json
{
  "mcpServers": {
    "mddb": {
      "transport": "streamable-http",
      "url": "http://localhost:9000/mcp"
    }
  }
}
```

## Environment Variables

### Core

| Env Var | Default | Description |
|---------|---------|-------------|
| `MDDB_MCP_ENABLED` | `true` | Enable MCP HTTP server |
| `MDDB_MCP_ADDR` | `:9000` | MCP HTTP listen address |
| `MDDB_MCP_PORT` | `9000` | MCP HTTP port (alternative to ADDR) |
| `MDDB_MCP_STDIO` | `false` | Run in MCP stdio mode (replaces HTTP/gRPC) |
| `MDDB_MCP_DOMAIN` | — | Server hostname for Panel config generation |
| `MDDB_MCP_MODE` | — | Access mode override: `read`, `write`, or `wr` (inherits `MDDB_MODE`) |

### Server Info

| Env Var | Default | Description |
|---------|---------|-------------|
| `MDDB_MCP_SERVER_NAME` | `mddbd` | Server name in MCP initialize response |
| `MDDB_MCP_SERVER_DESCRIPTION` | — | Server description |
| `MDDB_MCP_SERVER_VENDOR` | — | Vendor name |
| `MDDB_MCP_SERVER_HOMEPAGE` | — | Homepage URL |
| `MDDB_MCP_INSTRUCTIONS` | — | System prompt for LLM — guidance on how to use this server |

### Tools

| Env Var | Default | Description |
|---------|---------|-------------|
| `MDDB_MCP_CONFIG` | — | Path to YAML file with custom tool definitions |
| `MDDB_MCP_BUILTIN_TOOLS` | `true` | Set to `false` to hide all 54 built-in tools (only custom tools exposed) |

### API Key Authentication

| Env Var | Default | Description |
|---------|---------|-------------|
| `MDDB_MCP_API_KEY_ENABLED` | `false` | Require API key for MCP HTTP access |
| `MDDB_MCP_API_KEYS` | — | Static keys: `key1:name1,key2:name2` (defined at startup) |
| `MDDB_MCP_API_KEY_CACHE_TTL` | `60s` | Cache TTL for dynamic key lookups |

Two sources of keys:
- **Static** — `MDDB_MCP_API_KEYS` env var (defined at startup, no restart needed to validate)
- **Dynamic** — REST API (`/v1/mcp/keys`) stored in BoltDB, persists across restarts, managed without downtime

Clients authenticate via:
- `X-API-Key: <key>` header
- `Authorization: Bearer <key>` header
- `?api_key=<key>` query parameter (for SSE connections)

```bash
# Static keys only
docker run -d \
  -e MDDB_MCP_API_KEY_ENABLED=true \
  -e "MDDB_MCP_API_KEYS=sk-prod:claude-prod,sk-dev:cursor-dev" \
  -p 9000:9000 \
  tradik/mddb:latest
```

#### Dynamic Key Management API

Keys stored in internal BoltDB bucket (`_mcp_api_keys`). Requires admin auth when `MDDB_AUTH_ENABLED=true`.

```bash
# Create a key (full key shown only once in response)
curl -X POST http://localhost:11023/v1/mcp/keys \
  -H "Authorization: Bearer <admin-token>" \
  -d '{"name": "claude-prod", "expiresAt": 0}'
# Response: {"key":"mcp_a1b2c3d4e5f6...","name":"claude-prod","createdAt":1711...}

# List keys (full keys NOT shown, only prefixes)
curl http://localhost:11023/v1/mcp/keys \
  -H "Authorization: Bearer <admin-token>"

# Delete a key
curl -X DELETE http://localhost:11023/v1/mcp/keys \
  -H "Authorization: Bearer <admin-token>" \
  -d '{"key": "mcp_a1b2c3d4e5f6..."}'

# Disable a key (revoke without deleting)
curl -X POST http://localhost:11023/v1/mcp/keys/disable \
  -H "Authorization: Bearer <admin-token>" \
  -d '{"key": "mcp_a1b2c3d4e5f6..."}'
```

Key features:
- **TTL support** — `expiresAt` Unix timestamp (0 = never expires)
- **Disable without delete** — revoke instantly, keep for audit
- **Cache** — lookups cached for `MDDB_MCP_API_KEY_CACHE_TTL` (default 60s), auto-invalidated on create/delete/disable
- **Panel UI** — manage keys from the web panel (LLM Connections → API Keys tab)

### Rate Limiting

| Env Var | Default | Description |
|---------|---------|-------------|
| `MDDB_MCP_RATE_LIMIT_ENABLED` | `false` | Enable per-client rate limiting |
| `MDDB_MCP_RATE_LIMIT_REQUESTS` | `100` | Max requests per window |
| `MDDB_MCP_RATE_LIMIT_WINDOW` | `60` | Window duration in seconds |
| `MDDB_MCP_RATE_LIMIT_BURST` | `20` | Burst allowance above limit |
| `MDDB_MCP_RATE_LIMIT_BY` | `ip` | Rate limit key: `ip`, `api_key`, or `session` |

Response headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`.
Returns `429 Too Many Requests` with `Retry-After` header when exceeded.

### Request Logging

| Env Var | Default | Description |
|---------|---------|-------------|
| `MDDB_MCP_LOGGING_ENABLED` | `false` | Enable structured JSON audit logs |
| `MDDB_MCP_LOGGING_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |

Logs are written to stderr in JSON format:

```json
{"timestamp":"2026-03-26T10:30:00Z","method":"POST","path":"/mcp","status":200,"duration_ms":45,"client_ip":"1.2.3.4","key_name":"claude-prod","session_id":"abc123","user_agent":"claude-code/1.0"}
```

## Access Modes

Each protocol can have its own read/write mode:

| Env Var | Protocol | Default |
|---------|----------|---------|
| `MDDB_MODE` | **Global** | `wr` |
| `MDDB_API_MODE` | HTTP/REST + GraphQL | inherits global |
| `MDDB_GRPC_MODE` | gRPC | inherits global |
| `MDDB_MCP_MODE` | MCP (all transports) | inherits global |
| `MDDB_HTTP3_MODE` | HTTP/3 (QUIC) | inherits global |

In read-only mode:
- Built-in tools with `readOnlyHint=true` (search, stats, list, export) → **allowed**
- Built-in tools with writes/deletes → **blocked** with error
- Custom tools with read-only actions (`semantic_search`, `search_documents`, `full_text_search`) → **allowed**
- Custom tools with write actions → **blocked**

```bash
# Public MCP: read-only, custom tools only, API keys, rate limited
docker run -d \
  -e MDDB_MCP_MODE=read \
  -e MDDB_MCP_BUILTIN_TOOLS=false \
  -e MDDB_MCP_API_KEY_ENABLED=true \
  -e "MDDB_MCP_API_KEYS=sk-public:external" \
  -e MDDB_MCP_RATE_LIMIT_ENABLED=true \
  -e MDDB_MCP_RATE_LIMIT_REQUESTS=60 \
  -e MDDB_MCP_LOGGING_ENABLED=true \
  -e MDDB_MCP_CONFIG=/app/tools.yml \
  tradik/mddb:latest
```

## Protocol Features (2025-11-25)

| Feature | Description |
|---------|-------------|
| **Tool Annotations** | `readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint` on all tools |
| **Structured Output** | `outputSchema` on 9 key tools for client-side validation |
| **5 Prompts** | `analyze-collection`, `search-help`, `summarize-collection`, `import-guide`, `rag-pipeline` |
| **Completion** | Autocomplete for collection names and prompt arguments |
| **Logging** | `logging/setLevel` with RFC 5424 levels (debug → emergency) |
| **Progress Tokens** | `notifications/progress` for long-running operations |
| **Cursor Pagination** | `tools/list` and `resources/list` |
| **Notifications** | `notifications/initialized`, `notifications/cancelled` |
| **Memory RAG Tools** | 6 tools for conversational memory: start session, add message, recall, summarize, list sessions, history |

## Memory RAG Tools

Built-in MCP tools for conversational memory and RAG:

| Tool | Description | Write? |
|------|-------------|--------|
| `memory_start_session` | Start a new conversation session | Yes |
| `memory_add_message` | Add a message (auto-embedded) | Yes |
| `memory_recall` | Semantic/hybrid/keyword recall across sessions | No |
| `memory_summarize` | Generate and store session summary | Yes |
| `memory_list_sessions` | List sessions with filters | No |
| `memory_session_history` | Get chronological message history | No |

### Example: Agent with Memory

```json
// 1. Start session
{"name": "memory_start_session", "arguments": {"user_id": "user-1", "scenario": "support"}}

// 2. Store messages as conversation progresses
{"name": "memory_add_message", "arguments": {"session_id": "abc...", "role": "user", "content": "How do I use vector search?"}}
{"name": "memory_add_message", "arguments": {"session_id": "abc...", "role": "assistant", "content": "Use POST /v1/vector-search..."}}

// 3. In a future session, recall relevant context
{"name": "memory_recall", "arguments": {"query": "vector search usage", "user_id": "user-1", "top_k": 5, "include_content": true}}

// 4. Summarize when done
{"name": "memory_summarize", "arguments": {"session_id": "abc..."}}
```

## Built-in Prompts

| Prompt | Arguments | Description |
|--------|-----------|-------------|
| `analyze-collection` | `collection` (required) | Document count, metadata keys, content patterns, suggestions |
| `search-help` | `use_case` (required) | Which search method to use for your use case |
| `summarize-collection` | `collection` (required), `limit` | Topic overview, themes, distribution |
| `import-guide` | `source` (required) | Step-by-step import from wordpress/url/file/api/scraping |
| `rag-pipeline` | `collection` (required), `model` | RAG pipeline design with embedding and search strategy |

---

**[← Back to README](../README.md)** | **[→ LLM Connections](LLM_CONNECTIONS.md)** | **[→ Custom Tools](CUSTOM-TOOLS.md)**
