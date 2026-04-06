---
title: "Temporal Tracking"
slug: "docs/temporal-track"
description: "Temporal Tracking"
status: publish
---

# Temporal Tracking

MDDB can record document lifecycle events — **create**, **update**, and **access** — and expose them via a dedicated API. This enables analytics like activity histograms and hot-document leaderboards.

## Feature Flags

| Environment variable | Default | Description |
|---|---|---|
| `MDDB_TEMPORAL` | `false` | Enable temporal tracking (`true` to enable) |

Per-collection opt-in via Collection Settings (panel) or `PUT /v1/collection-config`:

| Field | Type | Description |
|---|---|---|
| `trackAccess` | bool | Record per-read access events for this collection |
| `trackHot` | bool | Maintain a hot-docs leaderboard for this collection |

> **Note:** Create and update events are always recorded when temporal tracking is globally enabled. Access tracking requires `trackAccess: true` on the collection.

## API Endpoints

### POST /v1/temporal/query

Returns lifecycle events for a specific document.

**Request:**
```json
{
  "collection": "blog",
  "key": "my-post",
  "lang": "en",
  "eventType": "access",
  "from": 1700000000,
  "to": 1800000000,
  "limit": 100
}
```

- `eventType`: `"create"`, `"update"`, `"access"`, or `""` for all (default)
- `from` / `to`: Unix timestamps (default: last 30 days)

**Response:**
```json
{
  "collection": "blog",
  "docId": "...",
  "events": [
    { "docId": "...", "eventType": "access", "timestamp": 1700100000, "actor": "alice" }
  ],
  "total": 47
}
```

---

### POST /v1/temporal/hot

Returns the top-N most accessed documents in a collection.

**Request:**
```json
{
  "collection": "blog",
  "topN": 20,
  "since": 1700000000
}
```

**Response:**
```json
{
  "collection": "blog",
  "entries": [
    {
      "document": { "id": "...", "key": "popular-post", "lang": "en", ... },
      "docId": "...",
      "accessCount": 142,
      "lastAccessAt": 1744900000
    }
  ]
}
```

---

### POST /v1/temporal/histogram

Returns an event-count histogram over a time range.

**Request:**
```json
{
  "collection": "blog",
  "eventType": "access",
  "interval": "day",
  "from": 1700000000,
  "to": 1800000000
}
```

- `interval`: `"day"` (default), `"week"`, `"month"`

**Response:**
```json
{
  "collection": "blog",
  "eventType": "access",
  "interval": "day",
  "buckets": [
    { "label": "2026-04-01", "from": 1743465600, "to": 1743552000, "count": 23 }
  ]
}
```

## Storage

Two BoltDB buckets are used:

| Bucket | Key format | Value |
|---|---|---|
| `temporal` | `evt\|{collection}\|{docID}\|{eventType}\|{ts20digits}` | JSON event object |
| `temporal_hot` | `hot\|{collection}\|{docID}` | 16 bytes (8-byte count + 8-byte last-access ts) |

Events are written asynchronously via a buffered channel (capacity 2000) and flushed using `db.Batch()` every 500 ms or after 500 events — so there is no write overhead on the critical path.

## Panel

The **Temporal Analytics** panel (sidebar → "Temporal Analytics") provides:

- **Activity Histogram**: bar chart of event frequency by day/week/month
- **Hot Documents**: ranked list of most-accessed documents with timestamps

Enable `trackAccess` in Collection Settings to populate access data.
