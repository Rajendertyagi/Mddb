# Changelog — MDDB Grafana datasource plugin

All notable changes to the `integrations/grafana-datasource` package are documented in this file. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Dedicated CI workflow** (`.github/workflows/grafana-datasource.yml`) — brings the plugin in line with the other four `integrations/` packages, which all had their own pipeline. On every PR / push touching `integrations/grafana-datasource/**`: a `test` matrix (Node 22 & 24) runs `npm ci` → `format:check` → `lint` → `jest --coverage` (≥90% threshold) → webpack `build`, with coverage + plugin-zip artefacts uploaded on Node 24; a `security` job runs `npm audit --omit=dev --audit-level=high`; a `smoke` job unzips the packaged plugin and validates `plugin.json` (`type: datasource`, `id`, `info.version`). On a `grafana-ds-v*` tag, a `release` job verifies the tag matches **both** `package.json` and `src/plugin.json` `info.version`, moves floating `grafana-ds-v<major>` / `<major>.<minor>` tags, and publishes a GitHub Release with the zip. `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24: true`, top-level `permissions: contents: read`, same action pins as the rest of the repo. CI badge added to the README.

### Fixed

- **`npm run package` was broken** — it pointed at a non-existent `scripts/package.js`. Repointed to build + zip `dist/` into `tradik-mddb-datasource-<version>.zip` (mirroring `make package`), so the CI release job and local packaging both work.

## [0.1.0] — 2026-05-22

### Added

- Initial release of `tradik-mddb-datasource` — native Grafana 11/12/13 datasource plugin for MDDB (`grafanaDependency: ">=11.0.0"`).
- Frontend plugin (TypeScript + React) bundled with webpack 5 to `dist/module.js` (AMD).
- Five query types backed by MDDB endpoints:
  - **Temporal histogram** (`/v1/temporal/histogram`) — time-series of create/update/access events bucketed by day/week/month, mapped to a Grafana time-series DataFrame.
  - **Hot documents** (`/v1/temporal/hot`) — top-N most accessed documents (table).
  - **Metadata aggregate** (`/v1/aggregate`) — facet value counts or date-histogram buckets per metadata key.
  - **Full-text search** (`/v1/fts`) — BM25 / boolean / phrase / proximity results with highlights.
  - **Database stats** (`/v1/stats`) — per-collection document counts, revisions, embeddings.
- **Auth model: Grafana datasource proxy routes** — `plugin.json` declares an `auth` route (with `Authorization: Bearer {{ .SecureJsonData.apiKey }}` header) and a `noauth` route (no header). The frontend client picks the route based on `secureJsonFields.apiKey` and never handles the secret itself; Grafana's `pluginproxy` injects the bearer token server-side. This is the only correct pattern for unsigned, frontend-only plugins — the alternative (reading `secureJsonData` on the frontend) cannot work because Grafana strips secrets before sending instance settings to the browser.
- Config editor (URL, default collection, default language, encrypted API key stored in `secureJsonData`, never displayed back after save).
- Query editor with field visibility driven by selected query type and Grafana template-variable interpolation on `collection`, `query`, `facetKey`.
- `MddbHttpError` carries status + body for diagnosable failures.
- `testDatasource()` probes `/v1/stats` through the same proxy route, distinguishing 401/403 (auth), 5xx (server), and network failures.
- React types: components return `ReactElement` instead of the deprecated global `JSX.Element`.
- DataFrame construction uses `createDataFrame()` (the `MutableDataFrame` constructor was deprecated in `@grafana/data` 10+).
- Unit tests for query building, response → DataFrame transformation, HTTP client, and the `DataSourceApi` lifecycle — Jest with ≥90% coverage thresholds enforced on the four pure-logic modules.
- Dockerfile bakes the bundled plugin into the official `grafana/grafana:13.0.1` image with `GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS` preset.
- `make package` produces a versioned plugin zip suitable for `grafana-cli plugins install --pluginUrl` distribution.

[Unreleased]: https://github.com/tradik/mddb/compare/grafana-ds-v0.1.0...HEAD
[0.1.0]: https://github.com/tradik/mddb/releases/tag/grafana-ds-v0.1.0
