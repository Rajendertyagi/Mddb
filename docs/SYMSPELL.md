---
title: "Spell Correction (SymSpell-style)"
slug: "docs/symspell"
description: "Spell Correction (SymSpell-style)"
status: publish
---

# Spell Correction (SymSpell-style)

MDDB includes a lightweight spell checker that can automatically correct FTS search queries and on-demand document content. It uses Levenshtein distance with frequency-weighted candidate ranking — similar to SymSpell — implemented on top of the existing `agnivade/levenshtein` library (no extra dependencies).

## Feature Flags

| Environment variable | Default | Description |
|---|---|---|
| `MDDB_SPELL` | `false` | Enable spell checking (`true` to enable) |

Per-collection opt-in via Collection Settings (panel) or `PUT /v1/collection-config`:

| Field | Type | Description |
|---|---|---|
| `spellCorrect` | bool | Auto-correct FTS queries for this collection |
| `spellLang` | string | Override the language used for spell correction (defaults to query `lang`) |

## API Endpoints

### POST /v1/spell-suggest

Returns token-level spell correction suggestions.

**Request:**
```json
{
  "collection": "docs",
  "text": "Docuemnt retreival systtem",
  "lang": "en",
  "maxSuggestions": 5
}
```

**Response:**
```json
{
  "originalText": "Docuemnt retreival systtem",
  "suggestedText": "document retrieval system",
  "tokenSuggestions": [
    { "original": "Docuemnt",  "corrected": "document",  "confidence": 0.65 },
    { "original": "retreival", "corrected": "retrieval", "confidence": 0.65 },
    { "original": "systtem",   "corrected": "system",    "confidence": 0.65 }
  ]
}
```

> Returns HTTP 503 while the spell index is loading on startup.

---

### POST /v1/spell-cleanup

Applies the best correction to each token and returns the cleaned text.

**Request:**
```json
{ "collection": "docs", "text": "Ths is bad speling", "lang": "en" }
```

**Response:**
```json
{ "original": "Ths is bad speling", "cleaned": "This is bad spelling", "correctionsApplied": 2 }
```

---

### GET /v1/spell-dictionary?collection=blog&lang=en

Returns all custom dictionary words for a collection+language.

---

### PUT /v1/spell-dictionary

Adds words to the custom dictionary. Custom words are never spell-corrected (treated as valid domain terms).

```json
{ "collection": "blog", "lang": "en", "words": ["mddb", "boltdb", "grpc"] }
```

---

### DELETE /v1/spell-dictionary

Removes words from the custom dictionary.

```json
{ "collection": "blog", "lang": "en", "words": ["grpc"] }
```

## Auto-correction in FTS

When `spellCorrect: true` is set on a collection, `POST /v1/fts` will automatically correct the query before executing the search. The response includes a `spellCorrected` field:

```json
{
  "results": [...],
  "spellCorrected": {
    "original": "docuemnt retreival",
    "corrected": "document retrieval"
  }
}
```

The panel's FTS Search view shows a badge above results when a correction was applied.

## Storage

Custom dictionaries are persisted in a `spelldicts` BoltDB bucket:

| Key format | Value |
|---|---|
| `{lang}\|{word}` | 4-byte frequency (uint32 LE) — global lang dict |
| `col\|{collection}\|{lang}\|{word}` | 4-byte frequency (uint32 LE) — collection-specific |

On startup all words are loaded asynchronously into in-memory frequency maps. The API returns HTTP 503 until loading is complete.

## Token Filtering

Only tokens that are:
- At least 3 characters long
- Not purely numeric
- Not URL-like (contain `/`, `:`, `.`, `@`)

...are considered for spell correction. Proper nouns, UUIDs, and code identifiers are generally left unchanged.

## Panel

The **Spell Checker** panel (sidebar → "Spell Checker") provides:

- **Test Spell Checker**: interactive text input with per-token suggestion display
- **Custom Dictionary**: add/remove domain-specific words that should not be corrected

Enable `spellCorrect` in Collection Settings to activate auto-correction on FTS queries.
