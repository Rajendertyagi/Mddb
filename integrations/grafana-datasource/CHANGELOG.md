# Changelog — MDDB Grafana datasource plugin

All notable changes to the `integrations/grafana-datasource` package are documented in this file. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] — 2026-05-20

### Added

- Initial release of `tradik-mddb-datasource` — native Grafana 10/11/12/13 datasource plugin for MDDB.
- Frontend plugin (TypeScript + React) bundled with webpack 5 to `dist/module.js` (AMD).
- Five query types backed by MDDB endpoints:
  - **Temporal histogram** (`/v1/temporal/histogram`) — time-series of create/update/access events bucketed by day/week/month, mapped to a Grafana time-series DataFrame.
  - **Hot documents** (`/v1/temporal/hot`) — top-N most accessed documents (table).
  - **Metadata aggregate** (`/v1/aggregate`) — facet value counts or date-histogram buckets per metadata key.
  - **Full-text search** (`/v1/fts`) — BM25 / boolean / phrase / proximity results with highlights.
  - **Database stats** (`/v1/stats`) — per-collection document counts, revisions, embeddings.
- Config editor (URL, default collection, default language, encrypted API key via Grafana `secureJsonData`).
- Query editor with field visibility driven by selected query type and Grafana template-variable interpolation on `collection`, `query`, `facetKey`.
- HTTP client routes requests through the Grafana `BackendSrv` proxy (no CORS issues) and exposes a `/testDatasource` probe that distinguishes auth vs server vs network failures.
- `MddbHttpError` carries status + body for diagnosable failures.
- Unit tests for query building, response → DataFrame transformation, HTTP client, and the `DataSourceApi` lifecycle — Jest with ≥90% coverage thresholds enforced on the four pure-logic modules.
- Dockerfile bakes the bundled plugin into the official `grafana/grafana:13.0.1` image with `GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS` preset.
- `make package` produces a versioned plugin zip suitable for `grafana-cli plugins install --pluginUrl` distribution.

[0.1.0]: https://github.com/tradik/mddb/releases/tag/grafana-v0.1.0
