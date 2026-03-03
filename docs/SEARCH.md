# MDDB Search Algorithms

MDDB provides three search methods: **Metadata Search**, **Full-Text Search**, and **Vector Search**. Each method supports multiple algorithms selectable at query time via the `algorithm` parameter.

## Overview

| Method | Algorithms | Best For |
|--------|-----------|----------|
| Metadata Search | Indexed filters | Exact tag/category matching |
| Full-Text Search | TF-IDF, BM25 | Keyword-based document retrieval |
| Vector Search | Flat, HNSW, IVF, PQ | Semantic similarity by meaning |

## Full-Text Search

Full-text search uses an inverted index built from document content. Queries are tokenized, stop words are removed, and documents are scored by relevance.

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

### Typo Tolerance (Fuzzy Search)

Both TF-IDF and BM25 support typo tolerance via the `fuzzy` parameter. When enabled, the search finds indexed terms within Levenshtein edit distance of each query term.

| `fuzzy` | Tolerance | Example |
|---------|-----------|---------|
| `0` (default) | Off — exact matching only | "javascrip" → no match |
| `1` | 1 edit (insert, delete, or substitute) | "javascrip" → "javascript" |
| `2` | 2 edits | "javasript" → "javascript" |

**Scoring:** Fuzzy matches receive a 0.8x score penalty compared to exact matches, so exact results always rank higher.

**Matched terms format:** Fuzzy matches appear as `queryTerm~indexedTerm` (e.g., `javascrip~javascript`) in the `matchedTerms` array, making it easy to distinguish exact vs fuzzy matches.

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

### Comparison Table

| Algorithm | Accuracy | Speed | Memory | Best For |
|-----------|----------|-------|--------|----------|
| Flat | Exact | Slow | 1x | < 10K docs |
| HNSW | ~97% | Fast | ~2x | 10K-1M docs |
| IVF | ~94% | Medium | ~1.1x | 100K+ docs |
| PQ | ~90% | Fast | ~0.03x | 500K+ docs, low memory |

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

1. **Vector + Metadata**: Use `filterMeta` in vector search to narrow semantic results by category
2. **FTS for keywords, Vector for meaning**: Use FTS when users search for specific terms, vector when queries are natural language questions
3. **BM25 for varied-length docs**: Switch from TF-IDF to BM25 when your collection has documents of very different lengths

**[← Back to README](../README.md)**
