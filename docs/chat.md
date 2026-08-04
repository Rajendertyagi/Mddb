---
title: "MDDB Chat Widget"
slug: "docs/chat"
description: "MDDB Chat Widget"
status: publish
---

# MDDB Chat Widget

Embed an AI-powered chatbot on any website with a single script tag. RAG-powered answers from your MDDB knowledge base. Supports OpenAI, Claude, Ollama, Bielik, Groq, Mistral, and more.



## Features

- **RAG-Powered** — answers from your MDDB knowledge base via hybrid BM25 + vector search
- **10+ LLM Providers** — OpenAI, Claude, Ollama, Bielik, Groq, Mistral, DeepSeek, Gemini
- **WebSocket Streaming** — real-time streaming responses with typing indicators
- **Shadow DOM** — fully isolated CSS, no conflicts with your site's styles
- **Session Persistence** — localStorage-based session continuity across page loads
- **< 30KB gzipped** — lightweight IIFE bundle, zero framework dependencies
- **Queue System** — graceful handling of multiple concurrent users

## Architecture

Three components work together:

| Component | Description | Port |
|-----------|-------------|------|
| `mddb-chat-widget` | TypeScript widget (Shadow DOM, streaming) | CDN / self-hosted |
| `mddb-chat` | Rust/Axum WebSocket server, tool-calling loop | 11030 |
| `mddbd` | MDDB knowledge base, hybrid search | 11023 (HTTP), 11024 (gRPC) |

```
Your Website                    MDDB Infrastructure
     |                                   |
<script>                         +---------------+
mddb-chat.min.js                 |     mddbd     |
     |                           |  :11024 gRPC  |
     | WebSocket                 +-------^-------+
     v                                   |
+-----------+  gRPC                     |
| mddb-chat |---------------------------+
|   :11030  |
|  (Rust)   |---- HTTP ---> LLM Provider
+-----------+           (OpenAI / Claude / Ollama / ...)
```

## Quick Start

### Docker Compose (Recommended)

```bash
# Clone the repository
git clone https://github.com/tradik/mddb.git
cd mddb

# Start MDDB + chat server + widget server
make docker-up

# Services:
# MDDB HTTP:   http://localhost:11023
# MDDB gRPC:   localhost:11024
# Chat server: ws://localhost:11030/ws
# Widget demo: http://localhost:11032
```

### Embed on Your Website

```html
<!-- Add to your HTML before </body> -->
<script>
  window.MDDB_CHAT_CONFIG = {
    serverUrl: 'ws://localhost:11030/ws',
    title: 'Documentation Assistant',
    primaryColor: '#2563eb'
  };
</script>
<script src="http://localhost:11032/mddb-chat.min.js" defer></script>
```

### Self-Hosted Widget

```bash
# Build the widget
cd services/mddb-chat-widget
npm install
npm run build
# Output: dist/mddb-chat.min.js
```

## Configuration

### Chat Server Environment Variables

```bash
# LLM Provider
MDDB_CHAT_LLM_PROVIDER=openai        # openai | anthropic | ollama | groq | mistral
MDDB_CHAT_LLM_API_KEY=sk-...         # API key (not needed for Ollama)
MDDB_CHAT_LLM_MODEL=gpt-4o-mini      # Model name
MDDB_CHAT_LLM_BASE_URL=              # Custom base URL (optional)

# MDDB connection
MDDB_GRPC_ADDRESS=localhost:11024
MDDB_COLLECTION=docs                 # Default collection to search

# Auth to mddbd (SEC-007): keep these OUT of config.toml (it is gitignored).
# When the config's auth_username/auth_password are blank, these env vars are
# used instead. In docker-compose.dev.yml they default to the mddbd admin
# credentials from your .env (copy .env.example to .env first).
MDDB_CHAT_AUTH_USERNAME=admin
MDDB_CHAT_AUTH_PASSWORD=             # set in .env — never commit a real password

# Server
MDDB_CHAT_PORT=11030
MDDB_CHAT_MAX_SESSIONS=100
MDDB_CHAT_RATE_LIMIT=10              # Messages per minute per session
```

### Widget Configuration

```javascript
window.MDDB_CHAT_CONFIG = {
  serverUrl: 'ws://localhost:11030/ws',  // Chat server WebSocket URL
  title: 'Documentation Assistant',      // Widget header title
  subtitle: 'Powered by MDDB',           // Widget header subtitle
  primaryColor: '#2563eb',               // Accent color (hex)
  position: 'bottom-right',             // bottom-right | bottom-left
  welcomeMessage: 'Hello! How can I help you today?'
};
```

## LLM Provider Examples

### OpenAI

```bash
MDDB_CHAT_LLM_PROVIDER=openai
MDDB_CHAT_LLM_API_KEY=sk-...
MDDB_CHAT_LLM_MODEL=gpt-4o-mini
```

### Anthropic Claude

```bash
MDDB_CHAT_LLM_PROVIDER=anthropic
MDDB_CHAT_LLM_API_KEY=sk-ant-...
MDDB_CHAT_LLM_MODEL=claude-haiku-4-5-20251001
```

### Ollama (Local, Free)

```bash
MDDB_CHAT_LLM_PROVIDER=ollama
MDDB_CHAT_LLM_BASE_URL=http://localhost:11434
MDDB_CHAT_LLM_MODEL=llama3.2
```

### Bielik (Polish)

```bash
MDDB_CHAT_LLM_PROVIDER=ollama
MDDB_CHAT_LLM_BASE_URL=http://localhost:11434
MDDB_CHAT_LLM_MODEL=bielik-11b-v2.3-instruct
```

## How RAG Works

1. **User sends a message** via the chat widget
2. **Chat server** receives it via WebSocket
3. **Hybrid search** — MDDB performs BM25 + vector search on your knowledge base
4. **Context injection** — top results are injected into the LLM prompt
5. **Streaming response** — the LLM streams its answer back through the chat server
6. **Widget renders** — the response appears with markdown formatting in the widget

## Step-by-Step Guide

See the [Website Chat Guide](/docs/uses-website-chat/) for a complete walkthrough with Docker and Ollama.

## Source Code

- Widget: [`services/mddb-chat-widget`](https://github.com/tradik/mddb/tree/main/services/mddb-chat-widget)
- Server: [`services/mddb-chat`](https://github.com/tradik/mddb/tree/main/services/mddb-chat)

