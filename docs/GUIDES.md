---
title: Guides & Use Cases
slug: guides
description: Practical step-by-step guides for common MDDB use cases.
status: publish
---

# Guides & Use Cases

Practical step-by-step guides for common MDDB use cases. Each guide walks through a real scenario from setup to deployment.

## AI & Chat

### [AI Chat Widget for Any Website](/docs/uses-website-chat/)

Add a RAG-powered AI chatbot to any website. Runs entirely locally with Docker and Ollama — no cloud API fees.

**Stack:** MDDB + mddb-chat (Rust) + mddb-chat-widget (TypeScript) + Ollama  
**Time:** ~30 minutes  
**Cost:** Free (local LLM)

→ [Read the Website Chat Guide](/docs/uses-website-chat/)

---

## Content Analysis

### [WordPress Content Analyzer](/docs/uses-wordpress-analyzer/)

Export your WordPress site into MDDB, then use Claude or any LLM to analyze it. Find broken links, missing meta descriptions, duplicate content, and SEO gaps — all from a single conversation.

**Stack:** MDDB + WordPress export + Claude CLI  
**Time:** ~45 minutes  
**Use when:** You have a WordPress site and want AI-assisted content auditing

→ [Read the WordPress Analyzer Guide](/docs/uses-wordpress-analyzer/)

---

### [YouTube Channel Transcriber](/docs/uses-youtube-transcribe/)

Scrape transcripts from an entire YouTube channel, load them into MDDB, then query them with an LLM. Ask questions across hundreds of videos without watching them.

**Stack:** MDDB + yt-dlp + Claude CLI  
**Time:** ~30 minutes  
**Use when:** You want to search and analyze video transcripts with AI

→ [Read the YouTube Transcriber Guide](/docs/uses-youtube-transcribe/)

---

## Common Patterns

### AI Agent Memory

Store conversation sessions, recall context with semantic search, and summarize past interactions. MDDB provides 6 dedicated MCP tools for conversational memory.

```bash
# Store a memory
curl -X POST http://localhost:11023/v1/add \
  -d '{"collection":"memory","key":"session-abc","lang":"en_US",
       "meta":{"session":["abc"],"type":["conversation"]},
       "contentMd":"User asked about billing..."}'

# Recall relevant memories
curl -X POST http://localhost:11023/v1/vector-search \
  -d '{"collection":"memory","query":"billing questions","topK":5}'
```

See [MCP Server Config](/docs/mcp/) for the full memory MCP tools reference.

---

### RAG Pipeline

Auto-embed documents on ingest, retrieve context via hybrid search, expose to LLMs through MCP.

```bash
# 1. Ingest with auto-embedding
curl -X POST http://localhost:11023/v1/ingest \
  -F "file=@docs.pdf" \
  -F 'meta={"collection":"kb","key":"docs","lang":"en_US"}'

# 2. Hybrid search for RAG context
curl -X POST http://localhost:11023/v1/hybrid-search \
  -d '{"collection":"kb","query":"how to configure auth","topK":5,"alpha":0.5}'
```

See [RAG Pipeline](/docs/rag-pipeline/) for the complete guide.

---

### Documentation Chatbot

Import documentation, embed automatically, expose via MCP — instant AI-powered support for your docs.

1. Upload your docs: `POST /v1/upload` (supports .md, .pdf, .docx, .html)
2. Enable embeddings: set `MDDB_EMBEDDING_PROVIDER=openai`
3. Connect via MCP: add MDDB to your Claude/Cursor/Windsurf config
4. Ask questions directly in your AI client

See [Chat Widget](/docs/chat/) to embed a chatbot on your website.

---

### Multi-Language Content

Store the same document in multiple languages using the same key, different `lang` field.

```bash
# Store in English
curl -X POST http://localhost:11023/v1/add \
  -d '{"collection":"blog","key":"hello-world","lang":"en_US","contentMd":"# Hello World"}'

# Store in Polish
curl -X POST http://localhost:11023/v1/add \
  -d '{"collection":"blog","key":"hello-world","lang":"pl_PL","contentMd":"# Witaj Świecie"}'

# Retrieve by language
curl -X POST http://localhost:11023/v1/get \
  -d '{"collection":"blog","key":"hello-world","lang":"pl_PL"}'
```

---

## More Examples

See the [Examples page](/docs/examples/) for code samples in curl, Python, PHP, and MCP configurations.
