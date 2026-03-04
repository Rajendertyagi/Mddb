# LLM Connections

Connect MDDB to AI agents and LLM tools for RAG (Retrieval-Augmented Generation) and knowledge base workflows.

## Overview

MDDB provides multiple integration paths:

| Method | Best For | Protocol |
|--------|----------|----------|
| **MCP Server** | Claude Desktop, Windsurf, IDE agents | MCP (stdio/HTTP) |
| **REST API** | ChatGPT, custom agents, any HTTP client | HTTP/JSON |
| **gRPC API** | High-performance integrations | gRPC/Protobuf |

## Claude Desktop / Claude Code

MCP is built directly into the MDDB server — no separate service needed.

### Docker (Recommended)

Add to your Claude Desktop config:

**macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
**Windows**: `%APPDATA%\Claude\claude_desktop_config.json`
**Linux**: `~/.config/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "mddb": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm", "--network", "host",
        "-v", "mddb-data:/app/data",
        "-e", "MDDB_MCP_STDIO=true",
        "tradik/mddb:latest"
      ],
      "env": {}
    }
  }
}
```

### Local Binary

Use the `mddbd` binary directly:

```json
{
  "mcpServers": {
    "mddb": {
      "command": "/path/to/mddbd",
      "args": [],
      "env": {
        "MDDB_MCP_STDIO": "true",
        "MDDB_PATH": "/path/to/mddb.db"
      }
    }
  }
}
```

### Available MCP Tools

Once connected, Claude has access to 23+ tools including:

- `add_document` / `search_documents` / `delete_document`
- `semantic_search` — vector/AI-powered search
- `full_text_search` — keyword search with TF-IDF/BM25
- `export_documents` / `create_backup` / `restore_backup`
- `vector_reindex` / `vector_stats`

## ChatGPT (Custom GPT)

Create a Custom GPT that connects to MDDB via its REST API.

1. Go to [platform.openai.com/gpts](https://platform.openai.com/gpts)
2. Create or edit a Custom GPT
3. In **Configure** → **Actions** → **Create new action**
4. Paste this OpenAPI schema (adjust the server URL):

```json
{
  "openapi": "3.1.0",
  "info": { "title": "MDDB API", "version": "2.6.2" },
  "servers": [{ "url": "https://your-mddb-server.com" }],
  "paths": {
    "/v1/search": {
      "post": {
        "operationId": "searchDocuments",
        "summary": "Search documents by metadata",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "collection": { "type": "string" },
                  "filterMeta": { "type": "object" },
                  "limit": { "type": "integer", "default": 10 }
                }
              }
            }
          }
        }
      }
    },
    "/v1/vector-search": {
      "post": {
        "operationId": "semanticSearch",
        "summary": "Semantic search using vector embeddings",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "collection": { "type": "string" },
                  "query": { "type": "string" },
                  "topK": { "type": "integer", "default": 5 }
                }
              }
            }
          }
        }
      }
    }
  }
}
```

**Note**: Your MDDB server must be publicly accessible for ChatGPT to reach it.

## Ollama (Python)

Use Ollama for local LLM inference with MDDB as the knowledge base.

```bash
pip install requests ollama
ollama pull llama3.2
```

```python
import requests
import ollama

MDDB_URL = "http://localhost:11023"

def search_mddb(query, collection="docs", top_k=5):
    resp = requests.post(f"{MDDB_URL}/v1/vector-search", json={
        "collection": collection, "query": query,
        "topK": top_k, "threshold": 0.6, "includeContent": True,
    })
    return resp.json().get("results", [])

def ask(question, collection="docs"):
    results = search_mddb(question, collection)
    context = "\n\n---\n\n".join(
        f"## {r['key']}\n{r.get('contentMd', '')[:2000]}" for r in results
    )
    response = ollama.chat(model="llama3.2", messages=[
        {"role": "user", "content": f"Answer using this context:\n{context}\n\nQuestion: {question}"},
    ])
    return response["message"]["content"]

print(ask("What is MDDB?"))
```

## DeepSeek

DeepSeek supports MCP via compatible clients (Cline, Continue, etc.).

Add the same MCP config as Claude Desktop to your MCP client settings:

```json
{
  "mcpServers": {
    "mddb": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm", "--network", "host",
        "-v", "mddb-data:/app/data",
        "-e", "MDDB_MCP_STDIO=true",
        "tradik/mddb:latest"
      ]
    }
  }
}
```

Set DeepSeek as the LLM provider in your MCP client configuration.

## Manus

Add MDDB tools to your Manus agent configuration:

```yaml
name: mddb_search
description: "Search the MDDB knowledge base"
type: api
endpoint: "http://localhost:11023/v1/vector-search"
method: POST
headers:
  Content-Type: "application/json"
body:
  collection: "docs"
  query: "{{input}}"
  topK: 5
  includeContent: true
```

## Bielik.ai

Bielik is a Polish LLM that can be connected to MDDB for RAG in Polish.

```bash
pip install requests
```

```python
import requests

MDDB_URL = "http://localhost:11023"
BIELIK_API_URL = "https://api.bielik.ai/v1"
BIELIK_API_KEY = "your-api-key"

def search_mddb(query, collection="docs"):
    resp = requests.post(f"{MDDB_URL}/v1/vector-search", json={
        "collection": collection, "query": query,
        "topK": 5, "threshold": 0.6, "includeContent": True,
    })
    return resp.json().get("results", [])

def ask(question, collection="docs"):
    results = search_mddb(question, collection)
    context = "\n\n".join(r.get("contentMd", "")[:2000] for r in results)
    resp = requests.post(f"{BIELIK_API_URL}/chat/completions", headers={
        "Authorization": f"Bearer {BIELIK_API_KEY}",
        "Content-Type": "application/json",
    }, json={
        "model": "Bielik-11B-v2.3-Instruct",
        "messages": [
            {"role": "system", "content": "Odpowiadaj korzystajac z kontekstu."},
            {"role": "user", "content": f"Kontekst:\n{context}\n\nPytanie: {question}"},
        ],
    })
    return resp.json()["choices"][0]["message"]["content"]

print(ask("Czym jest MDDB?"))
```

## Generic REST API Integration

Any LLM agent that supports HTTP tools can connect to MDDB. Key endpoints:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/search` | POST | Metadata search with pagination |
| `/v1/vector-search` | POST | Semantic/vector search |
| `/v1/fts` | POST | Full-text search (TF-IDF/BM25) |
| `/v1/get` | POST | Get specific document |
| `/v1/add` | POST | Add/update document |

See the [OpenAPI specification](openapi.yaml) for complete API details.

## Panel Configuration

The MDDB Panel includes an **LLM Connections** page (sidebar → LLM Connections) with:
- Ready-to-use configuration templates for each agent
- Dynamic server address substitution
- Copy & Download buttons
- Setup instructions per agent

**[← Back to README](../README.md)**
