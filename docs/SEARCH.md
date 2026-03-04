# MDDB Search Algorithms

MDDB provides four search methods: **Metadata Search**, **Full-Text Search**, **Vector Search**, and **Hybrid Search**. Each method supports multiple algorithms selectable at query time via the `algorithm` parameter.

## Overview

| Method | Algorithms | Best For |
|--------|-----------|----------|
| Metadata Search | Indexed filters | Exact tag/category matching |
| Full-Text Search | TF-IDF, BM25, BM25F | Keyword-based document retrieval |
| Vector Search | Flat, HNSW, IVF, PQ, SQ | Semantic similarity by meaning |
| Hybrid Search | Alpha Blending, RRF | Combined keyword + semantic relevance |

## Full-Text Search

Full-text search uses an inverted index built from document content. Queries are tokenized, stop words are removed, and documents are scored by relevance.

### Text Processing Pipeline

1. **Lowercasing** - All text converted to lowercase
2. **Tokenization** - Split on non-alphanumeric characters, minimum 2 characters
3. **Stop Word Removal** - ~90 common English words filtered out
4. **Stemming** (v2.6.4+) - Porter Stemmer reduces words to their root form (e.g., "running" -> "run", "organization" -> "organ"). Enabled by default, configurable via `MDDB_FTS_STEMMING`.
5. **Synonym Expansion** (v2.6.4+, query-time only) - Query terms are expanded with configured synonyms. Bidirectional: if "big" has synonym "large", searching "large" also finds "big". Configurable via `MDDB_FTS_SYNONYMS`.

#### Per-Query Control

Both stemming and synonyms can be disabled per-query using request fields:
```json
{
  "collection": "docs",
  "query": "running fast",
  "algorithm": "bm25",
  "disableStem": true,
  "disableSynonyms": true
}
```

#### Synonym Management API

```bash
# Add synonyms
curl -X POST http://localhost:11023/v1/synonyms \
  -d '{"collection":"docs","term":"big","synonyms":["large","huge","enormous"]}'

# List synonyms
curl http://localhost:11023/v1/synonyms?collection=docs

# Delete synonyms
curl -X DELETE http://localhost:11023/v1/synonyms \
  -d '{"collection":"docs","term":"big"}'
```

### TF-IDF (default)

Classic Term Frequency-Inverse Document Frequency scoring.

**Formula:**
```
score = sum(TF(term, doc) * IDF(term))
where:
  TF(term, doc) = count(term in doc) / total_terms(doc)
  IDF(term)     = log(N / df(term))
```

**When to use:** General-purpose keyword search. Good for short queries and when document lengths are similar.

### BM25

Okapi BM25 is an improved ranking function that adds document length normalization. Longer documents are penalized so they don't dominate results simply because they contain more terms.

**Formula:**
```
score = sum(IDF(term) * (TF * (k1 + 1)) / (TF + k1 * (1 - b + b * dl/avgdl)))
where:
  k1    = 1.2  (term frequency saturation)
  b     = 0.75 (document length normalization)
  dl    = document length (in terms)
  avgdl = average document length across collection
  IDF   = ln((N - df + 0.5) / (df + 0.5) + 1)
```

**When to use:** When documents vary significantly in length (e.g., mix of short FAQs and long guides). BM25 prevents long documents from dominating results.

### BM25F (Field-Weighted)

BM25F extends BM25 by scoring term matches in different document fields with different weights. A match in the title can be worth more than a match in the body text.

Documents are automatically indexed per-field: `content` (body text) and each metadata key as `meta.<key>` (e.g., `meta.title`, `meta.tags`, `meta.description`).

**Formula:**
```
score = sum(IDF(term) * tf_tilde / (k1 + tf_tilde))
where:
  tf_tilde = sum_field(w_f * tf(term, doc, field) / (1 - b + b * dl_f / avgdl_f))
  w_f     = field weight (e.g., title=3.0, content=1.0)
  dl_f    = length of field f in document
  avgdl_f = average length of field f across collection
```

**Default Field Weights:**

| Field | Default Weight |
|-------|---------------|
| `content` | 1.0 |
| `meta.title` | 3.0 |
| `meta.tags` | 2.0 |
| `meta.category` | 2.0 |
| `meta.description` | 1.5 |

Custom weights can be passed per-query via `fieldWeights`. Fields not in the weights map are ignored.

**When to use:** When documents have structured metadata (title, tags, etc.) and you want title matches to rank higher than body-only matches. Best for content management, documentation, and knowledge bases.

### Typo Tolerance (Fuzzy Search)

All algorithms (TF-IDF, BM25, BM25F) support typo tolerance via the `fuzzy` parameter. When enabled, the search finds indexed terms within Levenshtein edit distance of each query term.

| `fuzzy` | Tolerance | Example |
|---------|-----------|---------|
| `0` (default) | Off — exact matching only | "javascrip" → no match |
| `1` | 1 edit (insert, delete, or substitute) | "javascrip" → "javascript" |
| `2` | 2 edits | "javasript" → "javascript" |

**Scoring:** Fuzzy matches receive a 0.8x score penalty compared to exact matches, so exact results always rank higher.

**Matched terms format:** Fuzzy matches appear as `queryTerm~indexedTerm` (e.g., `javascrip~javascript`) in the `matchedTerms` array, making it easy to distinguish exact vs fuzzy matches.

### In-Graph Metadata Filtering (v2.6.5+)

FTS supports `filterMeta` to narrow results by metadata before scoring, just like vector search. This is useful for scoped keyword searches (e.g., search only within a specific category).

```bash
# BM25 search filtered to "tutorial" category
curl -X POST http://localhost:11023/v1/fts \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "blog",
    "query": "markdown database",
    "algorithm": "bm25",
    "filterMeta": {"category": ["tutorial"], "status": ["published"]}
  }'
```

**Filter logic:** AND between different metadata keys, OR between values of the same key (same as metadata search).

### API Examples

```bash
# TF-IDF (default)
curl -X POST http://localhost:11023/v1/fts \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "blog",
    "query": "markdown database tutorial",
    "limit": 10
  }'

# BM25
curl -X POST http://localhost:11023/v1/fts \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "blog",
    "query": "markdown database tutorial",
    "limit": 10,
    "algorithm": "bm25"
  }'

# BM25F (field-weighted, with custom weights)
curl -X POST http://localhost:11023/v1/fts \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "blog",
    "query": "markdown database tutorial",
    "limit": 10,
    "algorithm": "bm25f",
    "fieldWeights": {
      "content": 1.0,
      "meta.title": 5.0,
      "meta.tags": 2.0
    }
  }'

# With typo tolerance (works with any algorithm)
curl -X POST http://localhost:11023/v1/fts \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "blog",
    "query": "markdwn datbase tutrial",
    "limit": 10,
    "algorithm": "bm25",
    "fuzzy": 1
  }'
```

### MCP Tool

```json
{
  "tool": "full_text_search",
  "arguments": {
    "collection": "blog",
    "query": "markdown database",
    "algorithm": "bm25",
    "fuzzy": 1,
    "limit": 10
  }
}
```

## Vector Search

Vector search embeds the query text into a high-dimensional vector and finds documents with the most similar embeddings using cosine similarity.

### Flat (default)

Exact brute-force search. Compares the query vector against every document vector in the collection.

| Property | Value |
|----------|-------|
| Accuracy | 100% (exact) |
| Speed | O(n) - linear with collection size |
| Memory | Original vectors only |
| Build time | None |

**When to use:** Small collections (< 10K documents) or when perfect recall is required.

### HNSW (Hierarchical Navigable Small World)

Approximate nearest neighbor search using a multi-layer graph structure. Each layer connects vectors to their nearest neighbors, enabling logarithmic search time.

| Property | Value |
|----------|-------|
| Accuracy | ~95-99% recall |
| Speed | O(log n) |
| Memory | Vectors + graph edges (~2x flat) |
| Build time | O(n log n) |
| Parameters | M=16, efSearch=100 |

**When to use:** Best general-purpose algorithm for medium to large collections (10K-1M documents). Excellent speed/accuracy trade-off.

### IVF (Inverted File Index)

Clusters vectors using k-means, then searches only the nearest clusters. Requires training after loading vectors.

| Property | Value |
|----------|-------|
| Accuracy | ~90-98% recall (depends on nProbe) |
| Speed | O(n/k) where k = number of clusters |
| Memory | Vectors + cluster assignments |
| Build time | O(n * iterations) for k-means training |
| Parameters | nClusters=sqrt(N), nProbe=10 |

**When to use:** Large collections (> 100K documents) where you need faster search than flat but HNSW memory overhead is too high.

### PQ (Product Quantization)

Compresses vectors by splitting them into subspaces and quantizing each subspace independently. Dramatically reduces memory usage at the cost of some accuracy.

| Property | Value |
|----------|-------|
| Accuracy | ~85-95% recall |
| Speed | Fast (compressed distance computation) |
| Memory | ~32x compression (8 bytes per vector vs 256 for flat) |
| Build time | O(n * iterations) for codebook training |
| Parameters | 8 subspaces, 256 codebook entries |

**When to use:** Very large collections (> 500K documents) where memory is the primary constraint. Re-ranks top candidates with exact cosine for better accuracy.

### SQ (Scalar Quantization)

Compresses vectors by quantizing each float32 dimension to uint8 (8-bit). Simpler than PQ with better accuracy but less compression.

| Property | Value |
|----------|-------|
| Accuracy | ~92-98% recall |
| Speed | Fast (integer distance computation) |
| Memory | ~4x compression (1 byte per dimension vs 4 for flat) |
| Build time | O(n) - just min/max calibration |
| Parameters | Automatic calibration |

**When to use:** Medium to large collections where you need memory savings with better accuracy than PQ. Good middle ground between flat and PQ.

### Comparison Table

| Algorithm | Accuracy | Speed | Memory | Best For |
|-----------|----------|-------|--------|----------|
| Flat | Exact | Slow | 1x | < 10K docs |
| HNSW | ~97% | Fast | ~2x | 10K-1M docs |
| IVF | ~94% | Medium | ~1.1x | 100K+ docs |
| PQ | ~90% | Fast | ~0.03x | 500K+ docs, low memory |
| SQ | ~95% | Fast | ~0.25x | 50K+ docs, balanced |

### Algorithm Selection Guide

```
Collection size < 10,000?
  → Use Flat (exact results, fast enough)

Collection size 10,000 - 1,000,000?
  → Use HNSW (best speed/accuracy trade-off)

Collection size > 100,000 and memory constrained?
  → Use IVF (good accuracy, moderate memory)

Collection size > 500,000 and very memory constrained?
  → Use PQ (aggressive compression, acceptable accuracy)

Need guaranteed exact results?
  → Always use Flat regardless of size
```

### API Examples

```bash
# Flat (exact, default)
curl -X POST http://localhost:11023/v1/vector-search \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "kb",
    "query": "how to cancel my subscription?",
    "topK": 5,
    "algorithm": "flat"
  }'

# HNSW (fast approximate)
curl -X POST http://localhost:11023/v1/vector-search \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "kb",
    "query": "how to cancel my subscription?",
    "topK": 5,
    "algorithm": "hnsw"
  }'

# IVF (clustered)
curl -X POST http://localhost:11023/v1/vector-search \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "kb",
    "query": "how to cancel my subscription?",
    "topK": 5,
    "algorithm": "ivf"
  }'

# PQ (compressed)
curl -X POST http://localhost:11023/v1/vector-search \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "kb",
    "query": "how to cancel my subscription?",
    "topK": 5,
    "algorithm": "pq"
  }'
```

### MCP Tool

```json
{
  "tool": "semantic_search",
  "arguments": {
    "collection": "kb",
    "query": "how to cancel subscription",
    "algorithm": "hnsw",
    "top_k": 5
  }
}
```

### Fallback Behavior

If the selected algorithm's index is not yet ready (e.g., HNSW graph still building, IVF/PQ still training), the server automatically falls back to the **flat** algorithm and includes the actual algorithm used in the response.

## Hybrid Search (v2.6.5+)

Hybrid search combines FTS (keyword) and vector (semantic) search into a single query, producing results ranked by a fused score. This gives you the best of both worlds: exact keyword matching plus semantic understanding.

### How It Works

1. **FTS search** — runs BM25 or BM25F against the inverted index
2. **Vector search** — embeds the query and searches the vector index
3. **Fusion** — merges results using the selected strategy
4. **Return** — deduplicated results with combined scores

### Alpha Blending (default)

Weighted combination of normalized FTS and vector scores.

**Formula:**
```
combined = (1 - alpha) * normalizedFTS + alpha * vectorScore
```

- `alpha = 0.0` → pure keyword (FTS only)
- `alpha = 0.5` → equal weight (default)
- `alpha = 1.0` → pure semantic (vector only)

FTS scores are min-max normalized to 0-1 range. Vector scores are already 0-1 (cosine similarity).

### RRF (Reciprocal Rank Fusion)

Rank-based fusion that doesn't depend on score magnitudes. Works well when FTS and vector scores are not directly comparable.

**Formula:**
```
score = 1/(k + rank_fts) + 1/(k + rank_vector)
```

- `k` (default 60) controls how much top ranks dominate. Higher k = more equal weighting across ranks.
- Documents appearing in both result sets get both rank contributions.
- Documents appearing in only one set get a single rank contribution.

### Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `strategy` | `"alpha"` | `"alpha"` or `"rrf"` |
| `alpha` | `0.5` | Weight for alpha blending (0-1) |
| `rrfK` | `60` | RRF k parameter |
| `algorithm` | `"bm25"` | FTS algorithm: `"bm25"`, `"bm25f"` |
| `vectorAlgorithm` | `"flat"` | Vector algorithm: `"flat"`, `"hnsw"`, `"ivf"`, `"pq"`, `"sq"` |
| `topK` | `10` | Number of results to return |
| `fuzzy` | `0` | Typo tolerance for FTS (0, 1, 2) |
| `threshold` | `0.0` | Minimum vector similarity |
| `filterMeta` | — | Metadata filters (applied to both FTS and vector) |
| `includeContent` | `false` | Include document content in results |
| `fieldWeights` | — | BM25F field weights |
| `disableStem` | `false` | Disable stemming for FTS |
| `disableSynonyms` | `false` | Disable synonym expansion for FTS |

### API Examples

```bash
# Alpha blending (default, equal weight)
curl -X POST http://localhost:11023/v1/hybrid-search \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "kb",
    "query": "how to deploy with Docker",
    "topK": 10,
    "strategy": "alpha",
    "alpha": 0.5
  }'

# Keyword-heavy search (alpha=0.2)
curl -X POST http://localhost:11023/v1/hybrid-search \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "kb",
    "query": "nginx configuration reverse proxy",
    "topK": 10,
    "strategy": "alpha",
    "alpha": 0.2,
    "algorithm": "bm25f",
    "fieldWeights": {"meta.title": 5.0, "content": 1.0}
  }'

# RRF fusion
curl -X POST http://localhost:11023/v1/hybrid-search \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "kb",
    "query": "cancel subscription refund policy",
    "topK": 10,
    "strategy": "rrf",
    "rrfK": 60,
    "fuzzy": 1
  }'

# With metadata filtering
curl -X POST http://localhost:11023/v1/hybrid-search \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "kb",
    "query": "kubernetes pod scaling",
    "topK": 5,
    "filterMeta": {"category": ["devops"], "status": ["published"]}
  }'
```

### MCP Tool

```json
{
  "tool": "hybrid_search",
  "arguments": {
    "collection": "kb",
    "query": "how to deploy with Docker",
    "top_k": 10,
    "strategy": "alpha",
    "alpha": 0.5,
    "algorithm": "bm25"
  }
}
```

### Response Format

```json
{
  "results": [
    {
      "document": { "id": "...", "key": "deploy-docker", "lang": "en_US", "meta": {...} },
      "combinedScore": 0.78,
      "ftsScore": 0.65,
      "vectorScore": 0.91,
      "matchedTerms": ["deploy", "docker"],
      "rank": 1
    }
  ],
  "total": 5,
  "strategy": "alpha",
  "alpha": 0.5,
  "ftsAlgorithm": "bm25",
  "vectorAlgorithm": "flat"
}
```

### Strategy Selection Guide

```
Need precise keyword matching + semantic understanding?
  → Use Alpha Blending with alpha=0.5 (balanced)

Queries are specific terms (error codes, product names)?
  → Use Alpha Blending with alpha=0.2 (keyword-heavy)

Queries are natural language questions?
  → Use Alpha Blending with alpha=0.8 (semantic-heavy)

FTS and vector score ranges differ significantly?
  → Use RRF (rank-based, ignores score magnitudes)

Not sure?
  → Start with Alpha Blending at 0.5, adjust based on results
```

## Metadata Search

Metadata search uses BoltDB prefix indices for exact matching on document metadata tags. No algorithm selection is needed - it always uses the built-in index.

### Pagination

Use `offset` and `limit` for pagination. The response includes an `X-Total-Count` header with the total number of matching documents (before pagination).

```bash
# First page
curl -v -X POST http://localhost:11023/v1/search \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "blog",
    "filterMeta": {"category": ["tutorial"], "status": ["published"]},
    "sort": "updatedAt",
    "limit": 10,
    "offset": 0
  }'
# Response header: X-Total-Count: 234

# Second page
curl -X POST http://localhost:11023/v1/search \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "blog",
    "filterMeta": {"category": ["tutorial"]},
    "sort": "updatedAt",
    "limit": 10,
    "offset": 10
  }'
```

**Filter logic:** AND between different metadata keys, OR between values of the same key.

## Combining Search Methods

For best results, combine search methods:

1. **Hybrid Search** (v2.6.5+): Use `/v1/hybrid-search` for a single query that combines FTS + vector search with automatic score fusion. Best for general-purpose search where you want both keyword precision and semantic recall.
2. **Vector + Metadata**: Use `filterMeta` in vector search to narrow semantic results by category
3. **FTS + Metadata** (v2.6.5+): Use `filterMeta` in FTS to scope keyword search to specific metadata values
4. **FTS for keywords, Vector for meaning**: Use FTS when users search for specific terms, vector when queries are natural language questions
5. **BM25F for structured docs**: Use BM25F when documents have meaningful titles and tags — matches in titles will rank higher than body-only matches

**[← Back to README](../README.md)**
