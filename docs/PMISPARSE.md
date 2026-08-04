---
title: "PMISparse — Sparse Retrieval with PMI Query Expansion"
slug: "docs/pmisparse"
description: "PMISparse pairs BM25 scoring with automatic PPMI query expansion, closing the vocabulary gap between short queries and documents without synonym lists."
status: publish
---

# PMISparse — Sparse Retrieval with PMI Query Expansion

**PMISparse was invented by Tradik Limited.**

PMISparse is a two-phase sparse retrieval algorithm that combines BM25 scoring with automatic query expansion via Positive Pointwise Mutual Information (PPMI). It bridges the vocabulary gap between short user queries and document content without requiring manually curated synonym lists or external language models.

## Academic Foundations

PMISparse builds upon established techniques from information retrieval and computational linguistics:

| Reference | Contribution |
|-----------|-------------|
| Church & Hanks (1990) — *Word Association Norms, Mutual Information, and Lexicography* | Foundation of Pointwise Mutual Information (PMI) as a measure of word association strength |
| Turney & Pantel (2010) — *From Frequency to Meaning: Vector Space Models of Semantics* | Distributional semantics framework: words appearing in similar contexts have similar meanings |
| Robertson & Zaragoza (2009) — *The Probabilistic Relevance Framework: BM25 and Beyond* | BM25 term weighting with saturation and document length normalization |
| Xu & Croft (1996) — *Query Expansion Using Local and Global Document Analysis* | Automatic query expansion using corpus-derived term relationships |
| Bullinaria & Levy (2007) — *Extracting Semantic Representations from Word Co-occurrence Statistics: A Computational Study* | Positive PMI (PPMI) as a superior variant that discards negative associations |

## How It Works

PMISparse operates in two phases at query time, preceded by a one-time training step that builds the co-occurrence matrix.

### Phase 1: Direct BM25 Scoring

Each query term is scored against the inverted index using standard BM25:

```
score(d, t) = IDF(t) * (tf * (k1 + 1)) / (tf + k1 * (1 - b + b * dl / avgdl))
```

Where:
- `IDF(t) = log((N - df + 0.5) / (df + 0.5) + 1)` with `N` = total documents, `df` = document frequency
- `tf` = term frequency in the document
- `dl` = document length (in tokens)
- `avgdl` = average document length across the collection

Direct matches receive a weight of `1.0` and appear as plain terms in `matchedTerms` (e.g., `"docker"`).

### Phase 2: PMI Expansion Scoring

For each original query term, the algorithm looks up the top-K expansion terms from the pre-computed PPMI matrix. For each expansion term:

1. Skip if the expansion term is already a direct query term
2. Normalize the PPMI weight: `normW = min(ppmiWeight / 5.0, 1.0)`
3. Apply the expansion weight: `weight = alpha * normW`
4. Score documents using the same BM25 formula, but with the reduced weight

Expansion matches appear with a `~` prefix in `matchedTerms` (e.g., `"~containers"`).

The final score for each document is the sum of scores from both phases.

## PMI Matrix Construction

The PMI matrix is trained lazily per collection on the first PMISparse query and cached in memory. It is automatically invalidated when documents are added, updated, or deleted.

### Training Steps

1. **Read term lists** — Extract all per-document term lists from the reverse index (`ftsrev` bucket) for the target collection.

2. **Build co-occurrence counts** — For every term in every document, count how often each other term appears within a sliding window of `windowSize` positions on either side. This produces a symmetric co-occurrence matrix.

3. **Filter low-frequency terms** — Remove any term with total frequency below `minCount` to reduce noise from rare or misspelled words.

4. **Compute PPMI** — For each term pair `(t, n)`:

   ```
   PMI(t, n) = log( P(t, n) / (P(t) * P(n)) )
   PPMI(t, n) = max(0, PMI(t, n))
   ```

   Where:
   - `P(t) = freq(t) / totalTokens` — unigram probability
   - `P(t, n) = coOccur(t, n) / totalCoOccurrences` — joint probability from the window-based counts
   - Negative PMI values are clamped to zero (the "Positive" in PPMI), discarding pairs that co-occur less than expected by chance

5. **Cache top expansions** — For each term, sort expansions by descending PPMI score and retain only the top `topK` entries. This bounded cache keeps memory usage constant per term regardless of vocabulary size.

## Parameters

| Parameter | Value | Description |
|-----------|-------|-------------|
| `k1` | 1.5 | BM25 term frequency saturation. Higher values give more weight to repeated terms. |
| `b` | 0.75 | BM25 document length normalization. `0` = no normalization, `1` = full normalization. |
| `alpha` | 0.35 | Expansion weight multiplier. Controls how much influence expanded terms have relative to direct matches. |
| `expansionK` | 5 | Number of PPMI expansion terms used per query term at search time. |
| `windowSize` | 5 | Co-occurrence sliding window radius (5 tokens on each side). |
| `minCount` | 2 | Minimum term frequency to be included in the PMI matrix. Terms below this threshold are ignored. |
| `topK` | 10 | Maximum number of PPMI expansions cached per term after training. |

## API Usage

### Basic PMISparse Search

```bash
curl -X POST http://localhost:11023/v1/fts \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "docs",
    "query": "deploy docker",
    "algorithm": "pmisparse",
    "limit": 10
  }'
```

Response:

```json
{
  "results": [
    {
      "docID": "deploy-guide",
      "score": 3.82,
      "matchedTerms": ["deploy", "docker", "~containers", "~kubernetes"]
    }
  ]
}
```

Terms prefixed with `~` (e.g., `~containers`) are expansion matches discovered via the PPMI matrix rather than direct query terms.

### With Fuzzy Tolerance

Combine PMISparse expansion with edit-distance fuzzy matching for maximum recall:

```bash
curl -X POST http://localhost:11023/v1/fts \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "docs",
    "query": "deploment docker",
    "algorithm": "pmisparse",
    "fuzzy": 1,
    "limit": 10
  }'
```

Fuzzy mode expands misspelled query terms (e.g., "deploment" matches "deployment" via edit distance) and then applies PMI expansion on the original query terms. Fuzzy matches receive a `0.8x` score penalty, and their `matchedTerms` entries use the format `original~matched` (e.g., `"deploment~deployment"`).

### In Hybrid Search

Use PMISparse as the full-text component alongside vector search:

```bash
curl -X POST http://localhost:11023/v1/hybrid-search \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "docs",
    "query": "deploy docker",
    "algorithm": "pmisparse",
    "topK": 10,
    "alpha": 0.6
  }'
```

The `alpha` parameter here controls the blending ratio between FTS and vector scores (`0.6` = 60% text, 40% vector), not the PMI expansion weight.

## Matched Terms

The `matchedTerms` array in each result tells you exactly which terms contributed to the document's score:

| Format | Meaning | Example |
|--------|---------|---------|
| `term` | Direct query match | `"docker"` |
| `~term` | PMI expansion match | `"~containers"` |
| `original~matched` | Fuzzy match (when `fuzzy` > 0) | `"deploment~deployment"` |

This transparency lets callers highlight matched passages and understand why a result was returned, even when the original query terms do not appear in the document.

## When to Use PMISparse

| Algorithm | Best For | Query Expansion | Field Weights | Speed |
|-----------|----------|-----------------|---------------|-------|
| TF-IDF | Simple keyword matching, small collections | None | No | Fastest |
| BM25 | General-purpose full-text search | None | No | Fast |
| BM25F | Multi-field documents where title/tags matter more | None | Yes | Fast |
| **PMISparse** | **Short queries, vocabulary gap, automatic expansion** | **PPMI-based** | **No** | **Fast (after first query trains the matrix)** |

Choose PMISparse when:

- **Queries are short** (1-3 words) — Short queries suffer most from vocabulary mismatch. PMISparse automatically adds related terms to improve recall.
- **Vocabulary gap between queries and documents** — Users say "deploy" but documents say "deployment", "container orchestration", or "CI/CD pipeline". PMISparse discovers these associations from the corpus itself.
- **Automatic expansion is desired without manual synonyms** — Unlike the synonym API, PMISparse requires zero configuration. It learns term relationships directly from your data.
- **Domain-specific terminology matters** — Because the PPMI matrix is trained per collection, PMISparse adapts to the vocabulary of each collection independently. Medical, legal, or technical corpora all get their own expansion dictionaries.

Choose BM25 or BM25F instead when:

- The collection is very small (fewer than ~50 documents), where co-occurrence statistics are unreliable
- Queries are already long and specific (full sentences or paragraphs)
- You need deterministic results with no implicit expansion (e.g., exact legal search)
- Field-level weighting is required (use BM25F)
