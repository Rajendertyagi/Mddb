---
title: "Zero-Shot Classification"
slug: "docs/zero-shot-classification"
description: "Classify documents into predefined labels in MDDB without any training data, using embedding similarity between the text and each label."
status: publish
---

# Zero-Shot Classification

## What Is It?

Zero-shot classification is a technique that categorizes text into predefined labels **without any training data**. Instead of training a model on labeled examples, it uses embedding similarity to determine which label best describes a document.

MDDB implements this using the same embedding infrastructure as vector search. Given a document (or raw text) and a list of candidate labels, it:

1. Converts the document into a vector (embedding)
2. Converts each candidate label into a vector
3. Computes cosine similarity between the document vector and each label vector
4. Returns labels ranked by similarity score

This means you can classify any document into any set of categories on the fly — no model training, no labeled datasets, no ML pipeline.

## When to Use It

- **Content tagging** — Automatically tag blog posts, articles, or docs with topics
- **Routing** — Route incoming documents to the right team/queue based on content
- **Filtering** — Determine if content matches a category before further processing
- **Quality gates** — Check if a document is "tutorial" vs "reference" vs "changelog"
- **RAG pipelines** — Pre-classify retrieved documents before sending to LLM
- **Moderation** — Flag content as "appropriate" / "inappropriate" / "needs review"

## How It Works Internally

```
Document/Text                    Labels
     │                      ┌─── "programming"
     │                      ├─── "cooking"
     ▼                      ├─── "sports"
┌──────────┐                └─── "music"
│ Embedding │                      │
│ Provider  │◄─────────────────────┘
│ (OpenAI,  │         EmbedBatch()
│  Ollama)  │
└──────────┘
     │                      ┌─── [0.87] "programming"
     │ cosine               ├─── [0.21] "music"
     │ similarity           ├─── [0.18] "sports"
     ▼                      └─── [0.12] "cooking"
  Ranked Results
```

**Key optimization:** If classifying a stored document that already has an embedding in the vector store (from vector search indexing), MDDB reuses the existing embedding instead of re-computing it.

## Prerequisites

An embedding provider must be configured. Set at minimum:

```bash
MDDB_EMBEDDING_PROVIDER=openai       # or: ollama, voyage, cohere
MDDB_EMBEDDING_API_KEY=sk-...        # required for openai, voyage, cohere
```

See [Embedding Providers](EMBEDDING_PROVIDERS.md) for full configuration.

## API Reference

### `POST /v1/classify`

**Request body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `collection` | string | No* | Collection name (for document reference) |
| `key` | string | No* | Document key (for document reference) |
| `lang` | string | No | Language code (default: `"en"`) |
| `text` | string | No* | Raw text to classify |
| `labels` | string[] | **Yes** | Candidate labels to rank (max 100) |
| `topK` | int | No | Return only top K labels (0 = all) |
| `multi` | bool | No | If true, return all labels above threshold |
| `threshold` | float | No | Minimum similarity score (default: 0.0) |

\* Provide either `text` **or** `collection` + `key` (with optional `lang`).

**Response:**

```json
{
  "results": [
    { "label": "programming", "score": 0.87 },
    { "label": "music", "score": 0.21 },
    { "label": "sports", "score": 0.18 },
    { "label": "cooking", "score": 0.12 }
  ],
  "model": "text-embedding-3-small",
  "dimensions": 1536
}
```

## Usage Examples

### Classify raw text

```bash
curl -X POST http://localhost:11023/v1/classify \
  -H "Content-Type: application/json" \
  -d '{
    "text": "Go is a statically typed, compiled language designed at Google for systems programming",
    "labels": ["programming", "cooking", "sports", "music", "science"]
  }'
```

### Classify an existing document

If the document is already stored and has been embedded (via vector reindex or auto-embedding), the existing embedding is reused — no extra API call to the embedding provider.

```bash
curl -X POST http://localhost:11023/v1/classify \
  -d '{
    "collection": "blog",
    "key": "intro-to-go",
    "lang": "en",
    "labels": ["tutorial", "news", "opinion", "review", "changelog"]
  }'
```

### Get only the top label

```bash
curl -X POST http://localhost:11023/v1/classify \
  -d '{
    "text": "Recipe for chocolate cake with ganache frosting",
    "labels": ["food", "technology", "finance", "health"],
    "topK": 1
  }'
```

Response:
```json
{
  "results": [{ "label": "food", "score": 0.91 }],
  "model": "text-embedding-3-small",
  "dimensions": 1536
}
```

### Filter by threshold

Return only labels with similarity above 0.5:

```bash
curl -X POST http://localhost:11023/v1/classify \
  -d '{
    "text": "Introduction to machine learning with Python and TensorFlow",
    "labels": ["programming", "machine learning", "data science", "cooking", "music"],
    "multi": true,
    "threshold": 0.5
  }'
```

### Sentiment-style classification

Zero-shot classification works for any label scheme — including sentiment:

```bash
curl -X POST http://localhost:11023/v1/classify \
  -d '{
    "text": "This product is absolutely terrible, waste of money",
    "labels": ["positive", "negative", "neutral"]
  }'
```

### Language detection

```bash
curl -X POST http://localhost:11023/v1/classify \
  -d '{
    "text": "Bonjour, comment allez-vous aujourd'\''hui?",
    "labels": ["english", "french", "german", "spanish", "polish"]
  }'
```

## gRPC

The `Classify` RPC is available in the `MDDB` service:

```protobuf
rpc Classify(ClassifyRequest) returns (ClassifyResponse);
```

```go
resp, err := client.Classify(ctx, &proto.ClassifyRequest{
    Text:   "Go is a compiled programming language",
    Labels: []string{"programming", "cooking", "sports"},
})
for _, r := range resp.Results {
    fmt.Printf("%s: %.2f\n", r.Label, r.Score)
}
```

## MCP Tool

The `classify_document` tool is available to LLM agents via MCP:

```json
{
  "name": "classify_document",
  "arguments": {
    "text": "How to deploy Kubernetes on AWS with Terraform",
    "labels": ["devops", "frontend", "database", "security", "networking"]
  }
}
```

Or classify a stored document:

```json
{
  "name": "classify_document",
  "arguments": {
    "collection": "docs",
    "key": "k8s-deploy",
    "lang": "en",
    "labels": ["devops", "frontend", "database", "security"]
  }
}
```

## Panel Client (JavaScript)

```javascript
const result = await mddb.classify({
  text: "Introduction to React hooks and state management",
  labels: ["frontend", "backend", "devops", "database"],
});

console.log(result.results[0].label); // "frontend"
console.log(result.results[0].score); // 0.89
```

## Tips

- **Label quality matters** — Use descriptive labels. "machine learning" works better than "ML" because embeddings capture more meaning from longer phrases.
- **Number of labels** — Works well with 2-100 labels. Beyond 100 labels, consider splitting into hierarchical classification (first broad category, then subcategory).
- **Threshold tuning** — Cosine similarity scores are relative. A score of 0.3 may be the "best" match if all labels are loosely related. Use `topK: 1` to always get the best match regardless of absolute score.
- **Reuse embeddings** — If documents are already indexed for vector search, classification is nearly free since it reuses stored embeddings.
- **Provider choice** — Larger embedding models (e.g., `text-embedding-3-large`) tend to produce better classification results than smaller ones.

## Related Documentation

- [Embedding Providers](EMBEDDING_PROVIDERS.md) — Configure OpenAI, Ollama, Voyage AI, Cohere
- [Search Algorithms](SEARCH.md) — Vector search, FTS, hybrid search
- [API Documentation](API.md) — Full endpoint reference
- [LLM Connections](LLM_CONNECTIONS.md) — MCP tool integration
