# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Production hardening switch** ([services/mddbd/production_guard.go](services/mddbd/production_guard.go)) — ISO 27001 A.5.15/A.8.9 / SOC 2 CC6.1. Setting `MDDB_PRODUCTION=true` requires every compliance guardrail to be satisfied before the server accepts connections: `MDDB_AUTH_ENABLED=true`, JWT secret ≥32 bytes, `MDDB_TLS_ENABLED=true` (or explicit `MDDB_TLS_INSECURE_OK=true` opt-out for dev), `MDDB_CORS_ORIGIN` not `*`, `MDDB_AUDIT_ENABLED=true`, `MDDB_RATE_LIMIT_ENABLED=true`. Missing items abort startup with a per-variable checklist pointing at the failing control. When `MDDB_PRODUCTION` is unset the guard emits a single WARN and continues with the existing defaults — **no breaking change** for current deployments.
- **Audit log** ([services/mddbd/audit.go](services/mddbd/audit.go)) — ISO 27001 A.8.15 / SOC 2 CC7.2 compliance. `AuditManager` buffers structured JSON events `{ts, actor, action, resource, collection, key, result, ip, userAgent, detail}` and flushes them asynchronously to a dedicated `audit` BoltDB bucket; no hot-path handler blocks on disk I/O. Retention is configurable via `MDDB_AUDIT_RETENTION_DAYS` (default 90) with an hourly background trimmer. Authentication attempts (JWT, API key, login, missing/invalid/disabled user) are recorded from [auth_middleware.go](services/mddbd/auth_middleware.go) and [auth_handlers.go](services/mddbd/auth_handlers.go); every write endpoint is audited automatically via the `guardWrite` wrapper in [main.go](services/mddbd/main.go). Admin-only `GET /v1/audit` query endpoint supports `from`/`to` (RFC3339 or raw nanos), `actor`, `action`, `result`, `limit`. Feature is opt-in via `MDDB_AUDIT_ENABLED=true`; when disabled the manager is a no-op and the endpoint returns 404.

### Changed
- **Timing-safe auth error** ([services/mddbd/auth_middleware.go](services/mddbd/auth_middleware.go)) — "user disabled or not found" now returns the same `invalid token` response as a bad JWT to prevent user-existence enumeration.
- **Consolidated MCP arg helpers** — moved `mcpGetBool` from [services/mddbd/mcp_tools_bulk.go](services/mddbd/mcp_tools_bulk.go) into the shared helper group in [services/mddbd/mcp_tools.go](services/mddbd/mcp_tools.go) alongside `mcpGetString`, `mcpGetInt`, `mcpGetFloat`. No behavior change.

## [2.9.14] - 2026-04-19

### Added
- **Inline facets on search** ([services/mddbd/facets.go](services/mddbd/facets.go), [services/mddbd/fts.go](services/mddbd/fts.go), [services/mddbd/hybrid_search.go](services/mddbd/hybrid_search.go)). `POST /v1/fts` and `POST /v1/hybrid-search` accept a new `facetBy` array of metadata keys; the response grows a `facets` map with per-value counts aggregated over the matched documents. Optional `facetMaxValues` caps per-key bucket count. Counts are computed post-filter / post-boost / post-curation so UIs stay in sync with what the user actually sees; missing keys produce an empty bucket list so UIs can render a stable group layout. gRPC `FTSRequest.facet_by`/`facet_max_values` (fields 9/10) and `HybridSearchRequest.facet_by`/`facet_max_values` (fields 17/18) added backwards-compatibly; responses carry a new `map<string, FacetBucketList> facets`. MCP `full_text_search` and `hybrid_search` tools expose the same parameters.
- **Curation rules — pinned & hidden results** ([services/mddbd/curation.go](services/mddbd/curation.go), [services/mddbd/curation_apply.go](services/mddbd/curation_apply.go), [services/mddbd/curation_handlers.go](services/mddbd/curation_handlers.go), [services/mddbd/grpc_curation.go](services/mddbd/grpc_curation.go), [services/mddbd/mcp_tools_curation.go](services/mddbd/mcp_tools_curation.go)). New REST `/v1/curation` (GET/POST/PUT/DELETE), gRPC RPCs `ListCurationRules` / `CreateCurationRule` / `UpdateCurationRule` / `DeleteCurationRule`, and MCP tools with matching names. Each rule targets a collection + trigger query with `matchMode: "exact"` (default) or `"contains"`. Pinned documents are spliced into fixed 1-based positions (pins with `position<=0` append after organic results); hidden documents are dropped by key. Rules are applied inside the FTS + Hybrid pipelines after scoring, boost, and filtering, but before pagination and facet counting — so facets reflect post-curation visible results. Result items carry `"pinned": true` when injected. Persistence lives in a dedicated `curation` BoltDB bucket with a per-collection in-memory cache, rehydrated on startup; binlog-integrated for follower replication.
- **Per-collection revision retention** ([services/mddbd/revision_retention.go](services/mddbd/revision_retention.go), [services/mddbd/collection_config.go](services/mddbd/collection_config.go), [services/mddbd/main.go](services/mddbd/main.go)). `CollectionConfig.maxRevisions` (REST `PUT /v1/collection-config`, gRPC `SetCollectionConfig.max_revisions`, MCP `set_collection_config.max_revisions`, Admin Panel field) enforces a synchronous cap on revision history: every `addDocument` writes a new revision, then `trimRevisions` deletes the oldest entries inside the same BoltDB transaction so total stays at most `N`. `0` (default) preserves the existing unlimited behavior. Trimmed keys are mirrored into the binlog so followers converge on the same pruned state. Applies to `restoreRevision` too.
- **Admin panel** ([services/mddb-panel/src/components/CurationPanel.jsx](services/mddb-panel/src/components/CurationPanel.jsx), [services/mddb-panel/src/components/CollectionConfigModal.jsx](services/mddb-panel/src/components/CollectionConfigModal.jsx), [services/mddb-panel/src/components/FTSSearchPanel.jsx](services/mddb-panel/src/components/FTSSearchPanel.jsx)). New sidebar entry **Curation Rules** lists/creates/edits/deletes rules with pin-and-hide editors. Collection Settings modal gains a **Revision History Retention** field. FTS Search panel gains a comma-separated `Facets:` input and renders per-key value-count chips above the result list; pinned results are badged `PINNED` for visual distinction.

## [2.9.13] - 2026-04-19

### Added
- **Geo distance sort on hybrid search** ([services/mddbd/hybrid_search.go](services/mddbd/hybrid_search.go)). `POST /v1/hybrid-search` accepts a new `sort` field. With `sort: "distance"` and a `geo` filter attached, the post-merge result set is re-ordered by `distanceMeters` ascending so the nearest matching documents surface first; `sort: "combined"` (the default) keeps the existing score-based ordering. Validation rejects `sort: "distance"` without a `geo` filter and unknown sort values. gRPC `HybridSearch` carries the new field but only accepts `combined` — distance sort is HTTP-only because the gRPC request has no geo payload.
- **GeoJSON polygon and multi-polygon containment** ([services/mddbd/geo_polygon.go](services/mddbd/geo_polygon.go), [services/mddbd/geo_handlers.go](services/mddbd/geo_handlers.go)). New endpoint `POST /v1/geo-polygon` accepts a GeoJSON `Polygon` (outer ring + optional holes) or a `MultiPolygon` (union of polygons) and returns every indexed point inside the shape. Implementation does a bounding-box R-tree prefilter then ray-casts each candidate; response time tracks the polygon's bbox rather than the whole collection. Coordinate order is `[lng, lat]` per RFC 7946; rings may be open or closed. Exposed as read-only MCP tool `geo_polygon`.
- **Query string DSL with nested grouping** ([services/mddbd/fts_query_expr.go](services/mddbd/fts_query_expr.go)). New FTS `mode: "expression"` runs through a proper recursive-descent parser that handles parenthesized grouping, operator precedence (NOT > AND > OR), implicit AND between adjacent atoms, and mixed atom types in one query — terms, fuzzy (`term~2`), phrases, proximity (`"phrase"~5`), wildcards, and NOT. Evaluator reuses the existing per-atom scorers (`SearchBM25`, `SearchPhrase`, `SearchProximity`, `SearchWildcard`, `SearchBM25Fuzzy`), so scores stay consistent with single-mode results. Legacy flat `"boolean"` mode is unchanged.
- **Search-result highlighting with context fragments** ([services/mddbd/fts_highlight.go](services/mddbd/fts_highlight.go)). `POST /v1/fts` with `highlight: true` returns a `highlights[]` array per result — snippets taken from the raw `ContentMD` around matched terms, with each match wrapped in a caller-configurable tag (default `<mark>…</mark>`). Adjacent hits cluster into one fragment, clusters rank by hit count, the top `maxHighlights` (default 3) are kept, then re-sorted by document offset so UIs render in reading flow. Boundaries snap to word edges; ellipsis markers flag truncation. Works uniformly across every FTS mode including the new `expression` mode.

### Changed
- **Proto `HybridSearchRequest.sort` field (16)** added — backwards compatible, pre-2.9.13 clients simply omit it. Regenerated for Go / Python / Node.js / PHP via `buf`.
- **SEARCH.md** gains a dedicated "Expression Search" subsection and a "Highlighting with Fragments" subsection; API.md and openapi.yaml surface the new parameters.
- **Version bump** — `VERSION = "2.9.13"` across `services/mddbd/main.go`, Makefile, docker-compose labels, mddb-panel package.json, CLI manpage, snapcraft, SSG landing page.

## [2.9.12] - 2026-04-18

### Added
- **Per-query boost / demote for FTS and hybrid search** ([services/mddbd/fts_boost.go](services/mddbd/fts_boost.go)). Clients can now supply a `boost` map keyed by `"metaKey:metaValue"` on both `/v1/fts` and `/v1/hybrid-search` (and their gRPC equivalents) to multiply the score of documents that carry the matching metadata pair — positive values boost (`5.0` → 5×), negative values demote (`-2.0` → ½×). Boosts combine multiplicatively when multiple entries match the same document, and the combined factor is floored at `0.001` so a stack of demotions cannot collapse the score. No reindex is required. The panel's FTS and Hybrid search views grow a collapsible "Boost / Demote" section that mirrors the existing field-weights UI, and the MCP `full_text_search` / `hybrid_search` tools accept the new parameter verbatim.
- **Async bulk ingest with job tracking** ([services/mddbd/bulk_ingest_job.go](services/mddbd/bulk_ingest_job.go), [services/mddbd/bulk_ingest_handlers.go](services/mddbd/bulk_ingest_handlers.go)). New endpoints for long-running ingest workloads where the HTTP response should not block:
  - `POST /v1/bulk-ingest-job` — queue a job; returns HTTP 202 with a job ID
  - `GET /v1/bulk-ingest-job/{id}` — poll status (counters, errors, timestamps)
  - `DELETE /v1/bulk-ingest-job/{id}` — cancel a pending job
  - `GET /v1/bulk-ingest-jobs?collection=X` — list jobs newest-first

  Jobs are drained FIFO by a single worker in 500-document chunks; payloads live in an in-memory queue while status records are persisted to the new `bulk_jobs` BoltDB bucket. A startup recovery pass flips any orphan `pending`/`processing` job from a crashed run to `failed` so observers never see stale non-terminal status. Optional `callbackUrl` receives a `POST` with the final job record on completion. MCP tools `bulk_ingest_submit` / `_status` / `_list` / `_cancel` expose the same surface.
- **Prefix autocomplete** ([services/mddbd/fts_autocomplete.go](services/mddbd/fts_autocomplete.go)). New `GET /v1/autocomplete?collection=X&q=mar[&field=title&topN=10]` returns top-N terms starting with the given prefix, ranked by document frequency. The implementation scans the existing FTS inverted index (`fts` bucket for global, `ftsf` for field-scoped) so no additional indexing is required; scan is bounded at 10000 entries to keep pathological prefixes fast. The panel's FTS search input gains a debounced (150ms) dropdown of suggestions with doc-count badges, and the MCP `autocomplete` tool mirrors the HTTP API.

### Changed
- **Proto `FTSRequest` / `HybridSearchRequest` each gain a `map<string, double> boost`** field — field 8 on FTSRequest, field 15 on HybridSearchRequest. All language clients (Go, Python, Node.js, PHP) regenerated via `buf generate`.
- **OpenAPI** ([docs/openapi.yaml](docs/openapi.yaml)) — added `boost` to `FTSSearchRequest` and `HybridSearchRequest`; added new `/v1/bulk-ingest-job` / `/v1/bulk-ingest-job/{id}` / `/v1/bulk-ingest-jobs` / `/v1/autocomplete` paths plus `BulkIngestSubmitRequest` and `BulkIngestJob` schemas.
- **Version bump** — [services/mddbd/main.go](services/mddbd/main.go) `VERSION = "2.9.12"`, Makefile, docker-compose.yml labels, mddb-panel package.json.

### Fixed
- **26 broken documentation links** producing 404s on mddb.tradik.com. Root causes:
  - `docs/GUIDES.md` — absolute links missing `/docs/` prefix (e.g. `/uses-website-chat/` → `/docs/uses-website-chat/`)
  - `docs/EMBEDDING_PROVIDERS.md` — links to non-existent files (`VECTOR_SEARCH.md`, `API_ENDPOINTS.md`, `ADMIN_PANEL.md`, `MCP_CONFIG.md`, `SEARCH.md`) replaced with correct slugs
  - `docs/ARCHITECTURE.md` — `WEBHOOKS.md` link (file doesn't exist) replaced with reference to `AUTOMATIONS.md`; `../CHANGELOG.md` → GitHub URL
  - `docs/COMPARISON.md` — `PERFORMANCE.md` → `BENCHMARK.md`
  - `docs/BULK-IMPORT.md` — `CLI.md` (doesn't exist) replaced with `INSTALLATION.md`
  - `docs/FEATURES.md` — `TEMPORAL_TRACK.md` → `TEMPORAL-TRACK.md` (underscore/dash mismatch)
  - `docs/INSTALLATION.md` — `../services/mddb-mcp/WSL_SETUP.md` → `/docs/mcp/`
  - `docs/PANEL.md` — `../docs/` and `../services/mddbd/README.md` → valid site URLs
  - `docs/README.md` — `../BENCHMARK.md` → `BENCHMARK.md`; `openapi.yaml` / `swagger.html` → `/docs/api/swagger.html`
  - `docs/GRPC.md` — `../proto/mddb.proto` → GitHub blob URL
  - `docs/ROADMAP.md` — `../CONTRIBUTING.md`, `../CHANGELOG.md` → GitHub blob URLs
  - `docs/LLM_CONNECTIONS.md` — `openapi.yaml` → `/docs/api/swagger.html`
  - `docs/examples/sample-with-frontmatter.md` — `../docs/` double-nesting fixed
  - All `../LICENSE` links across docs → GitHub blob URL
- **Footer branding** — added "Made by tradik" link and JSON-LD Organization schema to [services/ssg-template/base.html](services/ssg-template/base.html)

## [2.9.11] - 2026-04-11

### Added
- **Unix Domain Socket (UDS) transport** for HTTP and gRPC servers. `MDDB_HTTP_ADDR` and `MDDB_GRPC_ADDR` (and the equivalent CLI flags / config fields) now accept `unix:/absolute/path.sock` in addition to classic `host:port`. The server creates the socket with `0600` permissions (owner-only), removes any stale socket file left by a prior run, and cleans up the socket on graceful shutdown. TLS is automatically skipped on UDS listeners — filesystem permissions already authenticate the peer.
  - New helper [services/mddbd/listen_addr.go](services/mddbd/listen_addr.go) with `parseListenAddr`, `openListener`, `closeListener`, `isUnixAddr` — used by both the HTTP listener in [services/mddbd/main.go](services/mddbd/main.go) and the gRPC listener in [services/mddbd/grpc_server.go](services/mddbd/grpc_server.go).
  - Unit tests in [services/mddbd/listen_addr_test.go](services/mddbd/listen_addr_test.go) cover TCP / UDS / stale-socket / cleanup paths.
  - **Python client** ([services/python-extension/mddb.py](services/python-extension/mddb.py)) gains `unix:` scheme support via a zero-dependency `_UnixHTTPConnection` / `_UnixHTTPHandler` backed by `socket.AF_UNIX` — stdlib-only, no new deps.
  - **PHP client** ([services/php-extension/mddb.php](services/php-extension/mddb.php)) gains `unix:` scheme support via libcurl's `CURLOPT_UNIX_SOCKET_PATH`.
  - **Rationale**: replaces a previously considered FFI / `libmddb.so` path. UDS delivers the same "zero-network, embedded-ish" UX for PHP/Python/Node sidecars at ~5 % of the cost — no C ABI to maintain, no cgo in the host process, no Go runtime leaking into PHP-FPM workers.

- **Mutual TLS (mTLS) / client certificate authentication** for the HTTP(S) listener. New config fields `tls.clientCAFile` and `tls.clientAuth` (env: `MDDB_TLS_CLIENT_CA`, `MDDB_TLS_CLIENT_AUTH`). When `clientCAFile` points to a PEM bundle of trusted CAs, MDDB will verify client certificates chaining to those CAs. `clientAuth` may be `require` (default, reject unauthenticated clients) or `request` (verify only if client presents a cert).
  - New helper [services/mddbd/tls_config.go](services/mddbd/tls_config.go) (`buildServerTLSConfig`) builds the full `crypto/tls.Config` once at startup, so the HTTP server now uses `ServeTLS(lis, "", "")` with a pre-built config. `MinVersion` pinned to TLS 1.2.
  - mTLS is ignored on UDS listeners (a UDS socket already authenticates the peer by filesystem permissions).
  - [services/mddbd/server_config.go](services/mddbd/server_config.go) adds `ClientCAFile` / `ClientAuth` to `TLSConfig` with YAML and env bindings.

### Fixed
- **Landing page — Mermaid diagram rendering** ([services/ssg-template/index.html](services/ssg-template/index.html)). The Replication section's Mermaid `graph LR` was rendered into a `<pre>` with `mermaid@10` and `startOnLoad: true`, which produced `Syntax error in text` in mermaid 10.9.5. Switched to `<div class="mermaid">` with inline content, upgraded to `mermaid@11` (matching [page.html](services/ssg-template/page.html)), and explicitly call `mermaid.run()` after init.
- **Landing page — stale MCP tool count**. Hero badge, feature card, sr-only feature list, JSON-LD `featureList`, meta description and the "Quick Start" terminal comment all claimed "72 MCP Tools". Audited the real count against the dispatch switch in [services/mddbd/mcp_tools.go](services/mddbd/mcp_tools.go) (67 top-level `case` branches in `mcpCallTool`) vs the declarations in [services/mddbd/mcp_custom_tools.go](services/mddbd/mcp_custom_tools.go)'s `mcpBuiltinTools()` (66). Found one orphan — `aggregate` — that was dispatchable but not declared, so it was invisible to MCP clients (`tools/list` never returned it, so clients could not call it). Added the missing declaration, bringing the authoritative count to **67**. Updated landing page and README accordingly.
- **Landing page — missing geosearch mention**. The landing page made zero reference to geospatial search despite it shipping in 2.9.10. Added a dedicated feature-strip chip, an sr-only bullet, and a JSON-LD `featureList` entry.
- **`/docs/geosearch/` 404** ([docs/GEOSEARCH.md](docs/GEOSEARCH.md)). The file was missing SSG frontmatter (`title`, `slug: "docs/geosearch"`, `description`, `status`), so the static site generator never emitted the corresponding output directory — the sidebar link in [services/ssg-template/base.html](services/ssg-template/base.html#L78) and any inbound link from a prior PR both 404'd. Added frontmatter.
- **Landing page — stale JSON-LD metadata**. `softwareVersion` bumped to `2.9.11`, `datePublished` bumped to `2026-04-11`, `featureList` expanded with geosearch, UDS and mTLS.

### GraphQL — full resurrection (was a stub since v2.7.0)
- **All 11 queries and 21 mutations declared in [services/mddbd/graphql/schema.graphql](services/mddbd/graphql/schema.graphql) are now implemented.** Prior to 2.9.11 the resolvers in `services/mddbd/graphql/schema.resolvers.go` were `panic("not implemented")` stubs for everything except `login` and a partial `deleteDocument`, and `SimpleGraphQLAdapter` returned `"not yet implemented - use REST API"` for every data operation. Both files are fully replaced.
- **New adapter** [services/mddbd/graphql_adapter.go](services/mddbd/graphql_adapter.go) (`GraphQLAdapter`) delegates every operation to the in-process MCP `DirectClient` ([services/mddbd/mcp_direct_client.go](services/mddbd/mcp_direct_client.go)) for documents / search / FTS / vector / schema / webhooks, and to `AuthManager` for users / groups / permissions. Same code path as REST and gRPC — no behavioural drift between protocols.
- **Expanded `gql.ServerInterface`** ([services/mddbd/graphql/resolver.go](services/mddbd/graphql/resolver.go)) from 10 methods (mostly returning `interface{}`) to 38 methods that return concrete `gql.*` types directly. Resolvers in [schema.resolvers.go](services/mddbd/graphql/schema.resolvers.go) are now thin one-liners that just delegate.
- **`@auth` and `@hasRole` directives are intentional no-op pass-throughs** ([services/mddbd/graphql/directives.go](services/mddbd/graphql/directives.go)) — all auth and per-collection permission checks happen inside the adapter so the contract lives in one place and the directive context-key gotcha is sidestepped. Per-method enforcement: `requireAuthenticated` for every operation, `requireAdmin` for user / group / permission / webhook / schema management, `CheckPermission` for read/write on a specific collection. When `AuthManager` is `nil` (auth disabled), the adapter short-circuits to allow-all to mirror a fresh-out-of-the-box deployment.
- **Default-on**: `MDDB_GRAPHQL_ENABLED` flips from `"false"` to `"true"` in [services/mddbd/main.go](services/mddbd/main.go) and [services/mddbd/endpoints_handlers.go](services/mddbd/endpoints_handlers.go). Set `MDDB_GRAPHQL_ENABLED=false` to opt out. The `/graphql` endpoint and the Playground at `/playground` are now part of the standard surface.
- **End-to-end tests** in [services/mddbd/graphql_e2e_test.go](services/mddbd/graphql_e2e_test.go) instantiate a real `Server` against a temp BoltDB and exercise `AddDocument` → `GetDocument`, `SearchDocuments` pagination, `DeleteDocument`, `GetStats` and a panic-guard smoke test that calls every read-only resolver to make sure none of them regress to the old "not implemented" behaviour.
- **Obsolete tests removed**: `services/mddbd/graphql_adapter_test.go` (asserted the old `"not yet implemented - use REST API"` stub returns) is gone. `services/mddbd/graphql/resolver_test.go` is rewritten with a `stubServer` satisfying the new interface and the original Login / DeleteDocument / MapMetaInputToInternal cases retained.
- **Panel transparency**: [services/mddb-panel/src/components/SettingsPanel.jsx](services/mddb-panel/src/components/SettingsPanel.jsx) — the REST/GraphQL toggle's tooltip is rewritten to be honest about the current state: the *server* GraphQL endpoint is fully functional, but the *panel* UI itself still issues every request through the REST client. Wiring `mddb-client.js` to dispatch through the existing [services/mddb-panel/src/lib/graphql.js](services/mddb-panel/src/lib/graphql.js) client is a panel-side refactor scheduled for a follow-up release. Use the GraphQL endpoint directly from your own client (Apollo, urql, curl) until then.
- **`docs/GRAPHQL.md`** updated with the new status, smoke-test recipes against the live endpoint, and accurate error message reference.

### Added (gRPC TLS)
- **TLS / mTLS on the gRPC listener** ([services/mddbd/main.go](services/mddbd/main.go)). The gRPC server now reuses the same `buildServerTLSConfig` (from [services/mddbd/tls_config.go](services/mddbd/tls_config.go)) as the HTTP listener and attaches the resulting `tls.Config` via `grpc.Creds(credentials.NewTLS(...))`. A single `tls.*` config block enables HTTPS *and* TLS-secured gRPC simultaneously, and `MDDB_TLS_CLIENT_CA` enables mTLS on both. Skipped on UDS listeners. New startup log line: `mddb gRPC listening on :11024 (mode=wr, db=mddb.db, tls=on, mtls=on (clientAuth=require))`. Closes the only out-of-scope item from the original 2.9.11 PR description.
- **`docs/TLS.md` extended** with a "TLS on the gRPC listener" section: Go (`google.golang.org/grpc`), Python (`grpcio`), Node (`@grpc/grpc-js`) and `grpcurl` client snippets for both HTTPS-only and full mTLS modes.

### Tests — coverage push for the 2.9.11 surface
- **`services/mddbd/tls_config_test.go`** (new) — 10 cases generating fresh ECDSA self-signed certs in `t.TempDir()` and exercising `buildServerTLSConfig` across every config permutation: disabled, missing fields, bad cert path, plain HTTPS, mTLS default-require, mTLS explicit "require", mTLS "request", bad clientAuth value, missing client CA file, empty client CA bundle. **Coverage: 0% → 100%.**
- **`services/mddbd/graphql_e2e_test.go` extended** with 8 new E2E cases against a real BoltDB plus a real `AuthManager` bootstrap helper (`gqlAdapterWithAuth` synthesizes admin claims into the request context like the HTTP middleware does). New tests cover `UpdateDocument`, `AddBatch`, `SetTTL` + `ImportURL`, `FullTextSearch`, full Schema CRUD (`SetSchema` → `GetSchema` → `ListSchemas` → `ValidateDocument` → `DeleteSchema`), full Webhook CRUD (`RegisterWebhook` → `ListWebhooks` → `DeleteWebhook`), `VectorReindex`, `DeleteCollection`, full Auth flow (`Authenticate` → `GenerateJWT` → `Me` → `Register` → `ListUsers` → `CreateAPIKey` → `SetPermission` → `UserPermissionsList`), and full Group flow (`CreateGroup` → `ListGroups` → `SetGroupPermission` → `GroupPermissionsList` → `UpdateGroup` → `DeleteGroup`). The previous panic-guard smoke test that called every read-only resolver is preserved.
- **Coverage of new 2.9.11 files**: `tls_config.go` 100%, `listen_addr.go` 91% average (parseListenAddr 100%, isUnixAddr 100%, openListener 61% — error paths only, closeListener 71%), `graphql_adapter.go` mostly 60-100% per method (a handful of paths remain at 0%: `IngestDocuments` complex permutations, `VectorSearch` requires an embedding provider, `GetClaimsFromContext` only fires inside the HTTP middleware path, and `derefFloat64` is currently unused). Whole-package coverage moved from 63.9% to 65.4%.

### Documentation — TLS / mTLS / UDS
- **New [docs/TLS.md](docs/TLS.md)** — dedicated TLS + mTLS guide covering quick-start (HTTPS-only and mTLS-required modes), the full env-var reference, `openssl` recipes for generating a demo CA + server cert + client cert, deployment patterns (proxy-fronted, direct HTTPS, service-to-service mTLS, staged rollout), and troubleshooting for the most common handshake failures.
- **[docs/config.md](docs/config.md)** — TLS table extended with `MDDB_TLS_CLIENT_CA` and `MDDB_TLS_CLIENT_AUTH`, plus a brand-new `Unix Domain Socket transport` section explaining the `unix:/path.sock` form for `MDDB_HTTP_ADDR`/`MDDB_GRPC_ADDR` with curl, Python, PHP, and gRPC client examples.
- **Sidebar** in [services/ssg-template/base.html](services/ssg-template/base.html) gains a "TLS & mTLS" link under Security & Ops.
- **README** Security section now links to `docs/TLS.md` from the TLS bullet.

## [2.9.10] - 2026-04-11

### Added
- **Geosearch** ([docs/GEOSEARCH.md](docs/GEOSEARCH.md)) — Point-in-radius and bounding-box queries pulled forward from the v2.11 roadmap. Documents attach coordinates via reserved metadata keys (`geo_lat`/`geo_lng`, `geo_hash`, or `geo_postcode`+`geo_country` with an opt-in CSV lookup), which MDDB indexes into both an in-memory R-tree (default, best overall) and a geohash prefix index (alternative, selectable per-query). Shared `geo` bucket in BoltDB, Binlog-replicated, async startup rebuild identical to the vector index lifecycle.
  - New HTTP endpoints: `POST /v1/geo-search`, `POST /v1/geo-within`, `POST /v1/geo-reindex`, `GET /v1/geo-stats`, `POST /v1/geo-encode`, `POST /v1/geo-decode`.
  - gRPC parity: `GeoSearch`, `GeoWithin`, `GeoReindex`, `GeoStats` RPCs with new proto messages; existing `Document` message untouched.
  - MCP tool surface: `geo_search`, `geo_within`, `geo_stats`, `geo_encode`, `geo_decode` (all read-only).
  - **`/v1/hybrid-search` extended** with an optional `geo: {lat, lng, radiusMeters}` field that spatially pre-filters the FTS+vector candidate pool and attaches `distanceMeters` to each result item.
  - **Panel UI**: new "Geo Search" tab with a Leaflet + OpenStreetMap map, click-to-set query center, radius slider, algorithm switch (R-tree / geohash), metadata filter composition, and result pins that open documents in the shared viewer. Adds `leaflet` as a panel dep.
  - **New files**: [services/mddbd/geo_index.go](services/mddbd/geo_index.go), [geo_store.go](services/mddbd/geo_store.go), [geo_postcodes.go](services/mddbd/geo_postcodes.go), [geo_hash.go](services/mddbd/geo_hash.go), [geohash_index.go](services/mddbd/geohash_index.go), [geo_handlers.go](services/mddbd/geo_handlers.go), [geo_grpc.go](services/mddbd/geo_grpc.go), [mcp_direct_client_geo.go](services/mddbd/mcp_direct_client_geo.go), [mcp_tools_geo.go](services/mddbd/mcp_tools_geo.go), + tests for each.
  - **Dependency**: `github.com/tidwall/rtree v1.10.0` (pure Go, no cgo).
  - Reserved metadata keys: `geo_lat`, `geo_lng`, `geo_hash`, `geo_postcode`, `geo_country`.
  - Out of scope (deferred to a follow-up): anti-meridian crossing bboxes, 3D/altitude, automatic postcode dataset downloads, GeoJSON ingest.
  - **GraphQL not wired up**: geo queries are *not* exposed via `/graphql` in this release. The GraphQL subsystem in this project is a pre-existing stub — every query resolver panics `not implemented` and `SimpleGraphQLAdapter` returns `"not yet implemented - use REST API"` for every method. This is independent of geosearch and will be addressed in a dedicated follow-up PR `graphql: wire up query resolvers` that implements the adapter for all queries, including geo. Until then, use REST (`/v1/geo-*`), gRPC, or MCP.

## [2.9.9] - 2026-04-11

### Security
- **Upgraded `google.golang.org/grpc` to v1.80.0** in `services/mddbd` — fixes GO-2026-4762 (authorization bypass via missing leading `/` in `:path` pseudo-header). Reached by `startGRPCServer` at [grpc_server.go:99](services/mddbd/grpc_server.go#L99).
- **Pinned Go toolchain to 1.26.2** across all modules (`services/mddbd`, `services/mddb-cli`, `tools/bench`, `test`) and `go.work`, plus matching bumps in Dockerfiles (`golang:1.26.2-alpine`) and GitHub Actions workflows (`test.yml`, `release.yml`, `govulncheck.yml`). Fixes 5 stdlib vulnerabilities flagged by `govulncheck`:
  - GO-2026-4947 — `crypto/x509` unexpected work during chain building
  - GO-2026-4946 — `crypto/x509` inefficient policy validation
  - GO-2026-4870 — `crypto/tls` unauthenticated TLS 1.3 KeyUpdate DoS
  - GO-2026-4866 — `crypto/x509` case-sensitive `excludedSubtrees` auth bypass
  - GO-2026-4865 — `html/template` JsBraceDepth XSS
- **Added `govulncheck` GitHub Actions workflow** (`.github/workflows/govulncheck.yml`) — scans all three Go modules on push/PR and nightly (06:00 UTC) with `GOWORK=off` to mirror the isolation of the Tests workflow.

### Fixed
- **Wiki table row separator rendering** ([services/mddbd/wikitext.go](services/mddbd/wikitext.go)) — `|-` row separators in wiki tables were previously unreachable: the more-generic `|` data-row branch ran first and swallowed them as empty `| |` rows. Reordered so `|-` is checked first. Surfaced by `staticcheck SA4017` during the Go 1.26.2 upgrade.
- **`TestCharsetReader_UTF8`** ([services/mddbd/wiki_import_test.go](services/mddbd/wiki_import_test.go)) — replaced an empty `if r != nil { /* comment */ }` branch with a real `t.Errorf` so UTF-8 pass-through regressions actually fail the test (`staticcheck SA9003`).
- **`swapParallelConfig`/`MinSize`/`Workers` test helpers** ([services/mddbd/vector_parallel_test.go](services/mddbd/vector_parallel_test.go)) — take `int32` directly (matching the underlying `atomic.Int32`) instead of `int`+conversion, eliminating `gosec G115` overflow warnings.

### Chore
- **License consistency sweep — BSD-3-Clause everywhere** — the canonical `LICENSE` file at the repo root declares BSD-3-Clause, but documentation, packaging metadata, and even distributed artifacts had drifted to claim MIT in several places. Audited and fixed all of them in one pass:
  - **Distributed artifacts (critical — these ship to end users)**:
    - [.github/workflows/release.yml](.github/workflows/release.yml) — RPM spec for both `mddbd` and `mddb-cli` changed from `License: MIT` to `License: BSD-3-Clause`. Affects every `.rpm` package built from a release tag.
    - [scripts/mddb_model.py](scripts/mddb_model.py) — Open WebUI module frontmatter changed from `license: MIT` to `license: BSD-3-Clause`. This file is published to the Open WebUI Community registry and imported as a RAG model by end users.
    - [services/mddb-cli/mddb-cli.1](services/mddb-cli/mddb-cli.1) — manpage copyright line changed from `Copyright (c) 2024 MDDB Project. License MIT.` to `Copyright (c) 2025-2026 Tradik Limited. License BSD-3-Clause.`. Installed by `.deb`, `.rpm`, and Homebrew packages into `/usr/share/man/`.
  - **Documentation (medium — user-visible docs)**:
    - [docs/DOCKER_HUB.md](docs/DOCKER_HUB.md) — badge URL, License section, and "MIT licensed, community driven" tagline — this file is pushed as the Docker Hub repository README.
    - [docs/DOCKER.md](docs/DOCKER.md), [docs/GRPC.md](docs/GRPC.md), [docs/PANEL.md](docs/PANEL.md), [proto/README.md](proto/README.md) — License footers.
    - [services/mddb-panel/README.md](services/mddb-panel/README.md), [services/mddb-cli/README.md](services/mddb-cli/README.md) — License sections.
  - **Package metadata (low — missing fields added)**:
    - [services/mddb-panel/package.json](services/mddb-panel/package.json), [services/mddb-chat-widget/package.json](services/mddb-chat-widget/package.json) — added `"license": "BSD-3-Clause"` (were missing the field entirely).
    - [services/mddb-chat/Cargo.toml](services/mddb-chat/Cargo.toml) — added `license = "BSD-3-Clause"` and `repository` URL (both were missing, `cargo publish` would have failed).
  - **Template cleanup**:
    - [services/mddb-panel/src/lib/markdown-templates.js](services/mddb-panel/src/lib/markdown-templates.js) — the "blog" and "readme" markdown templates offered to panel users now default to BSD-3-Clause instead of MIT, for consistency (these are placeholders users edit, but nudging the default matters).
- **Untracked committed build binaries** — removed `services/mddbd/mddb` (34 MB), `services/mddb-cli/mddb-cli`, and `tools/bench/mddb-bench` from the repo and expanded `.gitignore` so `go build` artifacts cannot slip into history again.
- **`buf breaking` CI guard** ([.github/workflows/test.yml](.github/workflows/test.yml)) — the breaking-change check now skips (with a GitHub Actions warning) when the base branch has no `buf.yaml`. Only applies to the one-shot buf-migration PR, where main's pre-migration layout with its legacy `services/mddbd/proto/mddb.proto` duplicate would otherwise break image building on the target side.

### Added
- **Wikipedia XML Dump Import** (`/v1/import-wiki`) — Stream and import MediaWiki XML dumps (including `.xml.bz2` compressed) directly into MDDB.
  - Streaming XML parser — processes multi-GB dumps without loading into memory
  - Automatic wikitext-to-Markdown conversion (headings, bold/italic, links, lists, tables, templates, references, categories)
  - Namespace filtering (default: ns=0, articles only), redirect skipping, max page limit
  - Batch processing with configurable batch size (default 500) and progress logging every 10K pages
  - Supports multipart file upload or raw octet-stream with query params
  - Metadata extraction: `wiki_id`, `wiki_title`, `wiki_ns`, `wiki_rev_id`, `wiki_timestamp`, `wiki_contributor`
  - `skipFts` option for faster bulk imports (run `/v1/fts-reindex` after)
  - New files: `wiki_import.go`, `wikitext.go`, `wikitext_test.go`, `wiki_import_test.go`
- **Database Path Configuration** — Database file location now configurable via CLI flag, YAML config, or environment variable.
  - CLI: `--db /path/to/mddb.db`, `--mode wr`
  - YAML config: `database.path`, `database.mode`
  - Env var: `MDDB_PATH`, `MDDB_MODE` (unchanged, still supported)
  - Precedence: CLI flags > env vars > config file > defaults

### Removed
- **`services/mddbd/proto/mddb.proto`** — stale duplicate of the source-of-truth `proto/mddb.proto` at the repo root. Nothing actually referenced it: Go imports the generated `mddb/proto` package (the `.pb.go` files, which remain), Docker builds already read the root `proto/mddb.proto` via `COPY proto /proto`, and `buf generate` reads from the root too. The duplicate had drifted 2KB behind source.
- **`services/mddbd/generate.sh`** — dead duplicate of `proto/generate.sh` using hardcoded relative paths to the stale copy above. Three competing code generators was two too many.

### Changed
- **Protobuf code generation now uses `buf`** — Replaces the `protoc`-based `proto/generate.sh` script as the primary code generator.
  - New files: [buf.yaml](buf.yaml) (lint rules + module config), [buf.gen.yaml](buf.gen.yaml) (pinned plugin versions)
  - Pinned plugins for reproducibility: `protocolbuffers/go:v1.36.11`, `grpc/go:v1.6.1`, `protocolbuffers/python:v31.1`, `grpc/python:v1.71.0`, `protocolbuffers/js:v3.21.4`, `grpc/node:v1.13.0`, `protocolbuffers/php:v31.1`, `grpc/php:v1.72.0`
  - Legacy `protoc`-based script preserved as `proto/generate-legacy.sh`; the main `proto/generate.sh` now wraps `buf generate` and falls back to the legacy script if `buf` is not installed.
  - `proto/generate.sh` also syncs `proto/mddb.proto` → `clients/nodejs/proto/mddb.proto` after generation — required because `@grpc/proto-loader` loads the file at runtime.
  - CI now runs `buf lint`, `buf breaking` (on PRs, against base branch), and `git diff --exit-code` after `buf generate` + nodejs sync to catch drift between `.proto` source and committed generated code.
  - Fixes the long-standing quirk documented in the repo memory where `generate.sh` placed files in the wrong directory due to `-I proto` stripping the path prefix.
  - Fixes [docs/GRPC.md](docs/GRPC.md) broken link that pointed at the now-deleted `services/mddbd/proto/mddb.proto`.
- **`services/mddbd` Docker images no longer regenerate proto** — [services/mddbd/Dockerfile](services/mddbd/Dockerfile) and [Dockerfile.dev](services/mddbd/Dockerfile.dev) used to install `protoc-gen-go@latest` + `protoc-gen-go-grpc@latest` at image build time and regenerate `.pb.go` files from scratch, **silently overriding the pinned plugin versions** from buf.gen.yaml and producing Docker images with different proto bindings than local builds.
  - Fix: Dockerfile now just copies the already-committed `services/mddbd/proto/*.pb.go` files (CI enforces these match `buf generate` output via `git diff --exit-code`).
  - Removes `protobuf` + `protobuf-dev` from Alpine apk packages and `go install protoc-gen-go*` steps — smaller image, faster build, reproducible across environments.
  - `services/mddb-chat` Dockerfile is unchanged — Rust's `tonic-build` legitimately needs `protoc` at cargo build time (no committed Rust bindings equivalent to `.pb.go`), and the `tonic-build` crate version is pinned in `Cargo.lock`.
- **`go mod tidy`** across all Go modules (`services/mddbd`, `services/mddb-cli`, `tools/bench`, `test`) — dependency lists now match actual imports after recent feature additions.
- **Go workspace for the monorepo** — [`go.work`](go.work) committed at the repo root, listing `services/mddbd`, `services/mddb-cli`, and `tools/bench`.
  - Enables cross-module refactoring, unified `go build ./...` from the repo root, and gopls "goto definition" across module boundaries.
  - `test/` module intentionally excluded (pre-existing `package main` conflicts in benchmark scripts); its `replace mddb => ../services/mddbd` in `test/go.mod` remains as a standalone fallback.
  - **CI runs with `GOWORK=off`** in both [test.yml](.github/workflows/test.yml) and [release.yml](.github/workflows/release.yml) — each module builds and tests in strict isolation so missing `require` entries (that workspace mode would hide) fail fast.
  - `.gitignore` updated: `go.work` is now committed; `go.work.sum` is ignored (not needed when every module maintains its own `go.sum`).
  - Docker builds are unaffected — `COPY services/mddbd/` never picks up the repo-root `go.work`, so containers naturally run in isolated mode.
  - See README.md "Development with Go Workspace" section for details.

### Fixed
- **Vector search RLock regression from 2.9.8** — `VectorIndex.Search` and `SearchWithFilter` held the read lock for the entire multi-millisecond parallel scoring phase, serializing every writer (`Add`/`Remove`) against searches and partially defeating the 2.5x parallel speedup under write-heavy workloads.
  - Fix: release `RLock` immediately after `snapshotMap()` copies slice headers; parallel scoring now runs lock-free on the owned snapshot.
  - Impact: concurrent `/v1/add` / auto-embedding worker throughput restored during search traffic.
- **`parallelSearchConfig` data race** — Global worker/minSize config was plain `int` fields, mutated directly by tests and read concurrently by searches — fragile if any test ever called `t.Parallel()`.
  - Fix: both fields now `atomic.Int32` with `Workers()` / `MinSize()` accessors; tests use `swapParallelConfig`/`swapParallelWorkers`/`swapParallelMinSize` helpers that atomically swap and return a restore closure.
  - Verified with `go test -race -count=10 ./services/mddbd/` — all parallel tests clean.
- **`batchCosineSim` panic on empty input** — On ARM64 the CGo wrapper indexed `&query[0]` / `&matrix[0]` / `&out[0]` without guarding against empty slices, crashing the server instead of no-oping. Added length guards.
- **`vector_parallel.go` memory model clarity** — Added comment above `partials[workerID]` write explaining why disjoint index writes are race-free per the Go memory model and `wg.Wait()` happens-before — prevents future "fix" attempts that would add unnecessary mutex.

## [2.9.8] - 2026-04-06

### Added
- **Goroutine Parallel Vector Search** — Multi-threaded scoring for Flat and IVF search paths.
  - Flat search: map snapshot + fan-out scoring across goroutines on disjoint index ranges
  - IVF search: parallel cluster probing with per-cluster goroutines
  - Auto worker count: `runtime.NumCPU()` (capped at 16), configurable via `MDDB_VECTOR_PARALLEL_WORKERS`
  - Minimum collection size threshold (default 2048) to avoid goroutine overhead on small collections, configurable via `MDDB_VECTOR_PARALLEL_MIN_SIZE`
  - Zero contention during scoring — each worker writes to its own result slice
  - Deterministic ordering with docID tiebreaker for stable results
  - **~2.5x speedup** on 50K×768 (24ms → 9.7ms), **~2.8x** on 50K×1536 (38ms → 13.5ms)
  - New file: `vector_parallel.go`, tests: `vector_parallel_test.go`
- **OPQ (Optimized Product Quantization)** — New vector index algorithm extending PQ with learned orthogonal rotation matrix.
  - Decorrelates dimensions before subspace splitting for better quantization quality (~1-3% recall improvement over standard PQ)
  - Alternating optimization: jointly learns rotation matrix + PQ codebooks (5 iterations default)
  - Rotation via Procrustes alignment with Gram-Schmidt re-orthogonalization
  - ADC search on rotated query, re-ranking with exact similarity on original vectors
  - API: `"algorithm": "opq"` in vector search requests
  - New file: `vector_opq.go`
- **Configuration documentation update** — Added 15+ missing environment variables to `docs/config.md`:
  - `MDDB_VECTOR_PARALLEL_WORKERS`, `MDDB_VECTOR_PARALLEL_MIN_SIZE` (parallel search)
  - `MDDB_TEMPORAL`, `MDDB_SPELL` (feature toggles)
  - MCP API key authentication, rate limiting, and logging settings

## [2.9.7] - 2026-04-06

### Added
- **ARM NEON/SME Vector Math Acceleration** — Hardware-accelerated similarity functions for vector search using ARM SIMD instructions.
  - **3-tier dispatch**: SME (Apple M4+, Cortex-X925+) → NEON (all ARM64) → scalar Go (x86/other)
  - Accelerated functions: `cosineSimilarity`, `dotProductSimilarity`, `euclideanSimilarity`, `euclideanDistSq`
  - **Batch cosine similarity**: Single CGo call for entire collection search (zero per-vector overhead)
  - NEON: 4-way float32 FMA via `float32x4_t` + `vfmaq_f32` intrinsics
  - SME: Scalable Vector Extension in streaming mode (`__arm_locally_streaming`) for wider SIMD on M4+
  - Runtime hardware detection: macOS (`sysctlbyname`), Linux (`getauxval`)
  - Zero allocations, zero external dependencies (~200 lines of C vendored in-tree)
  - Build tag `nosme` to force pure Go scalar on ARM64
  - Cross-platform: on amd64 compiles to identical pure Go code (no CGo required)
  - New benchmarks: `BenchmarkCosineSim{768,1024,1536,3072}`, `BenchmarkBatchCosineSim_{1K,10K,50K}_{768,1536}`
  - New files: `vector_math_scalar.go`, `vector_math_arm64.go`, `vector_math_arm64_neon.c`, `vector_math_arm64_sme.c`, `vector_math_test.go`, `vector_math_bench_test.go`

## [2.9.6] - 2026-04-05

### Added
- **Temporal Tracking** — Document lifecycle event tracking with analytics API.
  - Records `create`, `update`, and `access` events per document
  - **3 new HTTP endpoints**: `POST /v1/temporal/query` (event history for a doc), `POST /v1/temporal/hot` (top-N most accessed docs), `POST /v1/temporal/histogram` (activity histogram by day/week/month)
  - Per-collection opt-in: `trackAccess` (record GET events), `trackHot` (hot-docs leaderboard) via Collection Settings
  - **Panel**: new "Temporal Analytics" panel with activity histogram and hot-docs leaderboard
  - Async writes via buffered channel + `db.Batch()` — zero overhead on read/write path
  - Configurable via `MDDB_TEMPORAL=false` to disable globally
- **Spell Correction** — SymSpell-style spell checker for FTS queries and document content.
  - Uses Levenshtein distance with frequency-weighted ranking (no new dependencies)
  - **3 new HTTP endpoints**: `POST /v1/spell-suggest` (token suggestions with confidence), `POST /v1/spell-cleanup` (apply corrections), `GET/PUT/DELETE /v1/spell-dictionary` (custom per-collection dictionaries)
  - FTS integration: enable `spellCorrect: true` on a collection to auto-correct queries; `FTSSearchResponse` now includes `spellCorrected` field
  - **Panel**: new "Spell Checker" panel with interactive test UI and custom dictionary management
  - `SpellSuggestionBadge` shown in FTS results when query was auto-corrected
  - Configurable via `MDDB_SPELL=false` to disable globally; async dictionary loading with HTTP 503 guard
- **Memory RAG** — Conversational memory system for RAG applications. Store, retrieve, and semantically search conversation history across sessions.
  - **6 new HTTP endpoints**: `POST /v1/memory/session` (create session), `POST /v1/memory/message` (add message), `POST /v1/memory/recall` (semantic/hybrid/keyword recall), `POST /v1/memory/summarize` (session summary), `POST /v1/memory/sessions` (list sessions), `POST /v1/memory/history` (message history)
  - **6 new MCP tools**: `memory_start_session`, `memory_add_message`, `memory_recall`, `memory_summarize`, `memory_list_sessions`, `memory_session_history`
  - **3 dedicated collections**: `memory_sessions`, `memory_messages`, `memory_summaries`
  - **Hybrid recall**: Combines vector search (semantic) + FTS (keyword) with Reciprocal Rank Fusion (RRF) for optimal context retrieval
  - **Auto-embedding**: Messages are automatically embedded for semantic search when an embedding provider is configured
  - **Session TTL**: Default 30-day auto-expiry, configurable per session
  - **User/session/role filtering**: Filter recall by userId, sessionId, or message role
  - **Session summarization**: Generate and store conversation summaries with embeddings
- **20 new tests** for Memory RAG handlers and helpers

## [2.9.4] - 2026-03-26

### Added
- **MCP API Key Authentication** ([#29](https://github.com/tradik/mddb/issues/29)) — Protect MCP endpoints with API keys. Enable with `MDDB_MCP_API_KEY_ENABLED=true`, define keys in `MDDB_MCP_API_KEYS=key1:name1,key2:name2`. Supports `X-API-Key` header, `Authorization: Bearer`, and `?api_key=` query param (for SSE).
- **MCP Request Logging / Audit Trail** ([#30](https://github.com/tradik/mddb/issues/30)) — Structured JSON audit logs for all MCP requests. Enable with `MDDB_MCP_LOGGING_ENABLED=true`. Logs method, path, status, duration, client IP, API key name, session ID, and user agent.
- **MCP Rate Limiting** ([#31](https://github.com/tradik/mddb/issues/31)) — Per-client rate limiting for MCP endpoints. Enable with `MDDB_MCP_RATE_LIMIT_ENABLED=true`. Configurable via `MDDB_MCP_RATE_LIMIT_REQUESTS` (default: 100), `MDDB_MCP_RATE_LIMIT_WINDOW` (default: 60s), `MDDB_MCP_RATE_LIMIT_BURST` (default: 20), `MDDB_MCP_RATE_LIMIT_BY` (ip/api_key/session). Returns `X-RateLimit-*` headers and `Retry-After` on 429.
- **Dynamic MCP API Key Management** ([#33](https://github.com/tradik/mddb/issues/33)) — REST API for creating, listing, disabling, and deleting MCP API keys stored in BoltDB. Keys persist across restarts. Supports TTL expiry and disable-without-delete. Endpoints: `POST/GET/DELETE /v1/mcp/keys`, `POST /v1/mcp/keys/disable`. Requires admin auth. Cache with configurable TTL (`MDDB_MCP_API_KEY_CACHE_TTL`).
- **Panel: MCP API Keys tab** — New "API Keys" tab in LLM Connections for creating, viewing, and managing MCP API keys from the web panel.
- **Panel: Metadata filter scroll fix** ([#14](https://github.com/tradik/mddb/issues/14)) — Filter panel now uses `max-h-[70vh]` instead of `max-h-60` for proper scrolling with many metadata fields.

## [2.9.3] - 2026-03-25

### Added
- **MCP Protocol 2025-11-25** — Upgraded from 2024-11-05 to the latest MCP specification
- **Streamable HTTP transport** (`/mcp`) — New standard transport supporting POST (JSON-RPC), GET (SSE stream), DELETE (session termination), `MCP-Session-Id` header. Legacy SSE transport (`/sse` + `/message`) preserved for backward compatibility
- **Tool annotations** — All 52+ tools annotated with `readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint` hints per MCP spec. Enables AI clients (Claude, Cursor) to auto-approve safe tools
- **Structured output schemas** — `outputSchema` on 9 key tools: `get_stats`, `search_documents`, `semantic_search`, `full_text_search`, `hybrid_search`, `vector_stats`, `get_checksum`, `classify_document`, `aggregate`
- **5 MCP Prompts** — Built-in prompt templates: `analyze-collection`, `search-help`, `summarize-collection`, `import-guide`, `rag-pipeline`
- **Completion/autocomplete** — `completion/complete` for collection names and prompt arguments (source, model, algorithm)
- **MCP Logging** — `logging/setLevel` method + `notifications/message` support with RFC 5424 severity levels (debug through emergency)
- **Notification handling** — Server accepts `notifications/initialized`, `notifications/cancelled`, `notifications/roots/list_changed` without error
- **Progress token infrastructure** — `notifications/progress` support for long-running tools (vector_reindex, ingest_documents, fts_reindex, create_backup, etc.)
- **Cursor-based pagination** — `tools/list` and `resources/list` support cursor parameter per spec
- **Per-protocol access modes** — Independent read/write control per protocol via `MDDB_API_MODE`, `MDDB_GRPC_MODE`, `MDDB_MCP_MODE`, `MDDB_HTTP3_MODE`. Each overrides the global `MDDB_MODE` for its protocol. Example: `MDDB_MCP_MODE=read` makes MCP read-only while API remains read-write.
- **`MDDB_MCP_BUILTIN_TOOLS=false`** — Disable all 54 built-in MCP tools, exposing only custom YAML tools. Useful for restricting AI agents to domain-specific tools only.
- **39 new tests** — MCP protocol (29), per-protocol mode enforcement (10) including follower/global mode scenarios

### Security
- **MCP now respects global read-only mode** — `MDDB_MODE=read` and follower replication role correctly block MCP write tools. Previously MCP DirectClient bypassed the mode check entirely.
- **globalMode propagation** — Server's access mode (including follower-forced read-only) flows through the full MCP chain: Server → MCPHandler → MCPToolServer → write guard

### Fixed
- **Custom MCP tools work in read-only/follower mode** ([#27](https://github.com/tradik/mddb/issues/27)) — Custom tools with read-only actions (`full_text_search`, `search_documents`, `semantic_search`, `fts_languages`) are now allowed in read-only mode. Previously all custom tools were incorrectly blocked because they had no entries in the annotation map.

### Changed
- Tool call errors now return `isError: true` in result content (per spec) instead of JSON-RPC error codes
- `ping` response is now an empty object `{}` per MCP spec (was `{"result":"pong"}`)
- Resource read errors use code `-32002` (resource not found) instead of generic `-32000`
- Capabilities now advertise `prompts`, `logging`, `completions`, and `listChanged: true` for tools and resources

## [2.9.2] - 2026-03-25

### Added
- **MCP-over-SSE transport** — Spec-compliant MCP transport over Server-Sent Events at `/sse` + `/message` on MCP port (9000). Enables remote MCP connections without stdio for web-based agents and remote servers.
- **Panel SSE integration** — Real-time toast notifications for document changes, auto-refresh document list, Live/Offline connection indicator with auto-reconnect and exponential backoff
- **7 new MCP-over-SSE tests** — endpoint, full message flow, session validation, error handling

### Changed
- Version bump to 2.9.2 across all files

## [2.9.1] - 2026-03-25

### Fixed
- **SSE auth enforcement** — when auth is enabled, SSE now requires JWT/API key (returns 401 without token). Previously SSE was open to unauthenticated users.
- **SSE RBAC filtering** — events are only sent to clients with `PermRead` on the collection. `readOnly` field in each event indicates if client has `PermWrite`.

### Added
- **SSE per-IP rate limiting** — max concurrent SSE connections per IP address (default: 5). Prevents resource exhaustion. Configurable via `MDDB_SSE_MAX_PER_IP`.
- **SSE on MCP port** — `/events` endpoint available on MCP HTTP server (port 9000)
- **SSE connected event** includes `mode` ("read" or "readwrite") and `user` fields

## [2.9.0] - 2026-03-24

### Added
- **Per-Collection Vector Quantization** — Each collection can now configure its own vector quantization level for both storage and in-memory search. Supported formats:
  - `float32` (default) — Full precision, no compression
  - `int8` — 4x compression with ~1% recall drop, recommended for most use cases
  - `int4` — 8x compression with ~2-3% recall drop, ideal for large collections
- **Quantized Vector Index** (`vector_index_quantized.go`) — In-memory flat index that stores and searches vectors directly in int8/int4 format without dequantization
- **Quantized storage format (v2)** — New binary serialization with version byte prefix, backward-compatible with existing float32 records
- **Auto-select quantized searcher** — Vector search automatically uses the quantized index when the collection has quantization configured
- **`quantization` field in Collection Config API** — `PUT /v1/collection-config` now accepts `quantization` field (`"float32"`, `"int8"`, `"int4"`)
- **Quantization info in vector stats** — `GET /v1/vector-stats` now shows `quantization` per collection
- **Panel: Vector Quantization selector** — Collection Settings modal now includes Vector Quantization dropdown
- **New documentation** — `docs/QUANTIZATION.md` with full guide, examples, storage savings table, and technical details
- **17 new tests** — Round-trip quantization, similarity accuracy, storage integration, compression ratio verification

- **Server-Sent Events (SSE)** — Real-time document change notifications via `GET /v1/events`. Broadcasts `doc.added`, `doc.updated`, `doc.deleted` events. Per-collection filtering via `?collection=X`. Default enabled, configurable via `MDDB_SSE_ENABLED=false`. Keep-alive heartbeat every 30s.
- **pprof profiling endpoints** — Runtime CPU/memory profiling at `/debug/pprof/` (heap, goroutine, CPU profile, trace, allocs, block, mutex). Disabled by default, enable via `MDDB_PPROF_ENABLED=true`.
- **HTTP connection pooling** — Shared `http.Transport` with keep-alive for all outbound requests (webhooks, triggers, crons, import-url). Configurable via `MDDB_HTTP_POOL_MAX_IDLE`, `MDDB_HTTP_POOL_MAX_PER_HOST`, `MDDB_HTTP_POOL_IDLE_TIMEOUT`.
- **Built-in TLS/HTTPS** — Native TLS support without reverse proxy. Configure via `MDDB_TLS_ENABLED=true`, `MDDB_TLS_CERT`, `MDDB_TLS_KEY` or YAML config. Works for HTTP API server.

### Fixed
- **quantization.go**: Add bounds validation in `dequantizeInt8`/`dequantizeInt4` — prevents panic on corrupted or truncated quantized vector data
- **vector_store.go**: Return error instead of nil vector when dequantization fails due to data length mismatch
- **lockfree_cache.go**: Fix incorrect `size` tracking when updating an existing cache key — eviction was triggered unnecessarily and size counter drifted
- **lockfree_cache.go**: Fix goroutine leak — `cleanup()` goroutine now stops via `Close()` method; called on server shutdown
- **lockfree_cache.go**: Fix misleading "FIFO eviction" comment — Go map iteration is random, not ordered
- **mvcc.go**: Fix race condition in `Delete` — replaced `Load`+`Store` with `LoadOrStore` to prevent concurrent writes from being silently lost
- **mvcc.go**: Fix memory leak in GC — uncommitted versions from abandoned transactions were never cleaned up; GC now skips only versions belonging to active transactions
- **mvcc.go**: Optimize `Commit`/`Rollback` from O(all keys) to O(affected keys) via `txnKeys` tracking map
- **vector_store.go**: Harden `CountByCollection` chunk suffix stripping with `n >= 0` guard to reduce false deduplication risk

### Changed
- **Use Cases section linked to guides** — `docs/index.html` Use Cases section now links to step-by-step guides (`uses/website-chat.md`, `uses/wordpress-analyzer.md`, `uses/youtube-transcribe.md`); added "Use Cases" nav link
- Updated docs: README, CHANGELOG, openapi.yaml, docs/index.html, man page
- `vector_store.go` — `PutQuantized`, `PutChunksQuantized`, `LoadCollectionQuantized` methods, auto-detect v1/v2 format on read
- `vector_handlers.go` — Quantization-aware reindex, auto-algorithm selection, stats enrichment

## [2.8.0] - 2026-03-15

### Added
- **Per-Collection Storage Backends** — Each collection can now use its own storage backend instead of the server-wide default. Supported backends:
  - `boltdb` (default) — Embedded BoltDB, same as before
  - `memory` — In-memory ephemeral storage, data lost on restart. Ideal for scratch/cache collections
  - `s3` — S3-compatible object storage (AWS S3, MinIO, Cloudflare R2, etc.) with configurable endpoint, bucket, region, credentials, and prefix
- **Storage backend configuration via API** — `PUT /v1/collection-config` now accepts `storageBackend` and `storageConfig` fields
- **Storage backend configuration via Panel** — Collection Settings modal now includes Storage Backend selector with S3 configuration form
- **StorageBackend interface** (`storage_backend.go`) — Pluggable storage abstraction with `BackendRegistry` for per-collection routing
- **MemoryBackend** (`storage_memory.go`) — Thread-safe in-memory document store
- **S3Backend** (`storage_s3.go`) — S3-compatible storage using `minio-go/v7`, with auto bucket creation
- **Aggregations** — New `POST /v1/aggregate` endpoint for metadata facets and date histograms
  - Facets: count distinct values per metadata key with `count` or `value` ordering
  - Histograms: group documents by `addedAt`/`updatedAt` with `day`/`week`/`month`/`year` intervals
  - Optional `filterMeta` pre-filtering (same as `/v1/search`)
  - MCP tool: `aggregate`
- **Advanced Full-Text Search Modes** — The `/v1/fts` endpoint now supports 8 search types:
  - Boolean search: `AND`, `OR`, `NOT` operators and `+`/`-` prefix notation
  - Phrase search: exact consecutive phrase matching via positional index
  - Wildcard search: `*` (any chars) and `?` (single char) pattern matching
  - Proximity search: terms within N words of each other (`"rust performance"~5`)
  - Range search: numeric/date range filtering on metadata and timestamps (`rangeMeta`)
  - Auto-detect mode: automatically selects search type from query syntax
  - New `mode` parameter: `"auto"`, `"simple"`, `"boolean"`, `"phrase"`, `"wildcard"`, `"proximity"`
  - Positional index for phrase/proximity search (new `ftsp` bucket)
  - Panel: search mode selector, proximity distance slider, syntax hints
- **Multi-Language Full-Text Search** — Language-aware stemming and stop word filtering for 18 languages (English, Polish, German, French, Spanish, Italian, Portuguese, Dutch, Russian, Swedish, Norwegian, Danish, Finnish, Hungarian, Romanian, Turkish, Arabic, Tamil). Each document's `lang` field determines the FTS pipeline. New endpoints: `GET /v1/fts-languages`, `POST /v1/fts-reindex`. New config: `MDDB_FTS_DEFAULT_LANG`. New `lang` parameter in FTS search requests.
- **Chat Service Improvements** — Anthropic Claude LLM provider, tool-use support (search, get document), improved widget UI with typing indicators and error handling
- **File Upload Enhancements** — Extended `POST /v1/upload` with support for PDF, DOCX, HTML, ODT, RTF, TeX, YAML, and plain text file formats alongside Markdown

### Changed
- Updated docs: README, openapi.yaml, docs/index.html, API.md, SEARCH.md, man page
- Updated OpenAPI spec with FTS `mode`, `distance`, and `rangeMeta` schema fields

### Fixed
- **golint** — Fixed all 190 warnings across the codebase (added doc comments to all exported types, methods, constants)
- **gosec** — Fixed all 108 security issues in non-generated code:
  - G115: Added `safeInt32()` / `safeUint16()` helpers for overflow-safe integer conversions
  - G706/G704/G703/G705/G117/G404: Added targeted `#nosec` annotations with justification comments

## [2.7.1] - 2026-03-10

### Added
- **`POST /v1/delete-batch` endpoint** — Batch delete documents via REST API (previously only available in MCP/gRPC)
- **MCP tools**: `list_revisions` and `restore_revision` added to builtin tool schemas and validation map
- **MCP endpoint list**: Added missing `ingest_documents` and `upload_file` to `/v1/endpoints` MCP tools list
- **OpenAPI spec**: Added `/v1/delete-batch` endpoint definition with full request/response schema

### Fixed
- **Panel proxy 404 fix** — `server.js` mounted proxy at `/v1` which caused Express to strip the prefix; switched to root mount with `pathFilter: '/v1/**'` ([#17](https://github.com/tradik/mddb/issues/17))
- Panel endpoint counts now correct: HTTP(78), gRPC(54), MCP(53) — previously showed HTTP(76), MCP(51)
- MCP tool count updated from 52 to 53 across README, LLM_CONNECTIONS.md, and all documentation
- API.md expanded with ~35 missing endpoint docs (delete-batch, delete-collection, hybrid-search, cross-search, find-duplicates, collection-config, webhooks, revisions, automation, auth endpoints, system endpoints)

## [2.7.0] - 2026-03-06

### Added
- **Search Stats** — All search endpoints (`/v1/fts`, `/v1/vector-search`, `/v1/hybrid-search`) now return `searchStats` object with `durationMs`, `queryTerms`, `totalTokens`, and `indexSize`. Controlled by `MDDB_SEARCH_STATS` env var (default: enabled)
- **Distance Metrics**: Configurable distance metric for vector and hybrid search (cosine, dot_product, euclidean) via `distanceMetric` parameter
- **Document Revision History** — New `POST /v1/revisions` endpoint lists all revisions of a document. New `POST /v1/revisions/restore` restores a document to a previous revision
- **Collection attributes**: Per-collection type, description, icon, color, and custom metadata (`/v1/collection-config`, `/v1/collection-configs`)
- **Cross-collection search**: Search across multiple collections using a source document's embedding or text query (`/v1/cross-search`)
- **Duplicate detection**: Find exact and similar documents within a collection using content hashes and embedding similarity (`/v1/find-duplicates`, `find_duplicates` MCP tool)
- **4 new MCP tools**: `get_collection_config`, `set_collection_config`, `list_collection_configs`, `cross_search`
- **MCP tools**: `list_revisions`, `restore_revision`
- Panel: Full-page document editor (replaces constrained modal)
- Panel: Edit button moved to document header for easier access
- Panel: Document revision history viewer with restore capability
- Panel: Search stats display (duration, tokens, query terms) in all search panels

### Fixed
- Panel: Document content now refreshes after save, so re-opening editor shows updated content

### Changed
- Version bumped to 2.7.0 across all services and documentation

## [2.6.9] - 2026-03-05

### Added
- **Partial Document Update** — `PATCH /v1/update` for updating metadata and/or content independently
  - Meta only: `{"collection":"blog","key":"p1","lang":"en","meta":{"tag":["go"]}}`
  - Content only: `{"collection":"blog","key":"p1","lang":"en","contentMd":"new content"}`
  - Both: include both fields. Clear meta: `{"meta":{}}`
  - gRPC: `UpdateDocument` RPC. MCP: `update_document` tool
- **Document Metadata Read** — `GET /v1/doc-meta` returns metadata without content (lightweight)
  - gRPC: `GetDocumentMeta` RPC. MCP: `get_document_meta` tool
- **Zero-Shot Classification** — `POST /v1/classify` classifies documents against candidate labels using embedding similarity
  - By reference: provide `collection`, `key`, `lang` (reuses existing embedding if available)
  - By raw text: provide `text` field (embeds on the fly)
  - Labels embedded in a single batch call for efficiency
  - Parameters: `topK`, `multi` (return all above threshold), `threshold`
  - gRPC: `Classify` RPC. MCP: `classify_document` tool
- Panel: `updateDocument`, `getDocumentMeta`, and `classify` client methods

## [2.6.8] - 2026-03-05

### Added
- **Metadata Tag Filtering in Search** — Select metadata tags to filter FTS, vector, and hybrid search results in the panel. Dynamically loads available tags from collection. Multi-select with AND across keys, OR within values. New `MetaFilterBar` component.
- **`GET /v1/meta-keys` Endpoint** — List unique metadata keys and values for a collection. Powers the tag filter UI in the panel.
- **`GET /v1/checksum` Endpoint** — Lightweight CRC32-based collection checksum that changes on any document add/update/delete. Enables cache invalidation without downloading all documents. Also included in `/v1/stats` response per collection.
- **FTS `filterMeta` Support** — Full-text search now accepts `filterMeta` parameter for metadata pre-filtering (already supported in vector and hybrid search).

### Changed
- Version bumped to 2.6.8 across all services and documentation

## [2.6.7] - 2026-03-05

### Added
- **PMISparse Search Algorithm** - Two-phase sparse retrieval with PMI query expansion (invented by Tradik Limited)
  - BM25 scoring for direct term matches + automatic PPMI-based query expansion from corpus co-occurrence statistics
  - Lazy per-collection training with sliding-window co-occurrence matrix, automatic invalidation on document changes
  - Fuzzy variant combining edit-distance tolerance with PMI expansion for maximum recall
  - Works standalone (`algorithm: "pmisparse"`) and as the FTS component in hybrid search
  - Configurable parameters: k1, b, alpha, expansionK, windowSize, minCount, topK
  - Expansion matches marked with `~` prefix in `matchedTerms` for transparency
  - Dedicated documentation: `docs/PMISPARSE.md`
- **Sentiment Analysis for Triggers** - Keyword-based sentiment scoring for automation triggers
  - `AnalyzeSentiment()` returns score from -1.0 (negative) to +1.0 (positive) using built-in lexicon (~100 positive, ~100 negative words)
  - Optional `sentimentEnabled` condition on triggers with configurable min/max range
  - AND/OR logic (`conditionLogic`) when combining sentiment with search conditions
  - Markdown-aware text stripping before analysis
- **Automation Execution Logs** - Track webhook execution history
  - `GET /v1/automation-logs` endpoint with cursor-based pagination
  - Filter by `ruleId` and `status` (success, error, skipped)
  - TTL-based automatic cleanup with configurable retention (`MDDB_AUTOMATION_LOGS_TTL`, default: 7d)
  - Panel Logs tab with auto-refresh toggle and status filter
- **`MDDB_AUTOMATIONS` env var** - Single toggle to enable/disable entire automation system
  - `MDDB_AUTOMATION_LOGS` - Enable/disable automation execution logging
  - `MDDB_AUTOMATION_LOGS_TTL` - Log retention period (default: 7d)
- **Webhook Template Variables** - Dynamic `{{variable}}` substitution in webhook URLs and custom headers
  - Trigger variables: `{{doc.id}}`, `{{doc.key}}`, `{{doc.lang}}`, `{{doc.meta.FIELD}}`, `{{collection}}`, `{{score}}`, `{{sentiment}}`, `{{trigger.id}}`, `{{trigger.name}}`, `{{timestamp}}`, `{{webhook.id}}`, `{{event}}`
  - Cron variables: `{{cron.id}}`, `{{cron.name}}`, `{{timestamp}}`, `{{webhook.id}}`, `{{event}}`
  - Panel: collapsible help section listing available variables in webhook form

### Changed
- Version bumped to 2.6.7 across all services and documentation

## [2.6.6] - 2026-03-04

### Added
- **Automation System** - Triggers, Crons, and Webhook Targets for automated workflows
  - **Triggers**: Fire webhooks when new documents match search criteria (FTS/vector/hybrid) above threshold
  - **Crons**: Schedule periodic trigger execution using cron expressions (`robfig/cron/v3`)
  - **Webhook Targets**: Named HTTP endpoints with custom headers and configurable methods
  - Unified storage in single `automation` BoltDB bucket with `type` field
  - HTTP API: `GET/POST /v1/automation`, `GET/PUT/DELETE /v1/automation/{id}`, `POST /v1/automation/{id}/test`
  - gRPC RPCs: `ListAutomation`, `CreateAutomation`, `UpdateAutomation`, `DeleteAutomation`, `TestAutomation`
  - MCP tools: `list_automation`, `create_automation`, `update_automation`, `delete_automation`, `test_automation`
  - Env vars: `MDDB_TRIGGERS`, `MDDB_CRONS`, `MDDB_WEBHOOKS` (all default: false)
  - Webhook payload with retry backoff (0s, 1s, 5s, 15s) and custom X-MDDB headers
- **Panel: Automation Tab** - Full automation management UI
  - Type filter tabs (All/Webhooks/Triggers/Crons) with icons (Webhook/Zap/Clock)
  - Dynamic forms per type with collection/webhook/trigger dropdowns
  - Enable/disable toggle, test button (dry run with matching docs and scores)
- **Automation Tests** - 15 unit tests covering CRUD, validation, and edge cases

### Changed
- Version bumped to 2.6.6 across all services and documentation

## [2.6.5] - 2026-03-04

### Added
- **Hybrid Search** (`/v1/hybrid-search`) - New endpoint combining BM25/BM25F keyword search with vector semantic search
  - Alpha Blending strategy: weighted linear interpolation `combined = (1-α) * BM25 + α * vector`
  - RRF (Reciprocal Rank Fusion) strategy: rank-based fusion robust to different score distributions
  - Configurable: `strategy`, `alpha`, `rrfK`, `algorithm`, `vectorAlgorithm`
  - gRPC `HybridSearch` RPC and `hybrid_search` MCP tool
- **In-Graph FTS Filtering** - `filterMeta` parameter on full-text search endpoint
  - Pre-filters candidate documents by metadata before BM25 scoring
  - Supports OR within key, AND across keys
  - Added `filter_meta` field to FTS proto message
- **Panel: Hybrid Search Mode** - New search mode with strategy/alpha/algorithm controls
- **Panel: Command Modal** - Copy-ready API examples in curl, PHP, Python, and JavaScript for all search operations
- **Panel: System Info Default** - Default to System Information view after login

### Fixed
- Stale gRPC/MCP entries in endpoint documentation (removed non-existent `Delete`/`DeleteCollection` gRPC methods)

## [2.6.4] - 2026-03-04

### Added
- **BM25F Field-Weighted Search** - New full-text search algorithm
  - Weights matches in different document fields (title, tags, body) independently
  - Default weights: meta.title=3.0, meta.tags=2.0, meta.category=2.0, meta.description=1.5, content=1.0
  - Custom per-query field weights via `fieldWeights` parameter
  - Supports fuzzy matching with field weights
  - Field-level inverted index stored in dedicated BoltDB buckets
  - Algorithm option: `"bm25f"` in FTS API
- **Panel BM25F UI** - Field weights configuration panel with collapsible field weight editor, custom field support
- **Optimized Docker Pipeline** - Pre-built Go binaries instead of compiling in Docker
- **Scalar Quantization (SQ)** - New vector search algorithm
  - Quantizes float32 vectors to uint8 (0-255) using per-dimension min/max scaling
  - ADC-style search with precomputed distance tables + exact cosine re-ranking
  - ~75% memory reduction vs flat index
  - Algorithm option: `"sq"` in vector search API
- **Binary Quantization (BQ)** - New vector search algorithm
  - Reduces each float32 to 1 bit (sign bit), packed into uint64 words
  - Hamming distance for ultra-fast coarse ranking via `math/bits.OnesCount64`
  - Re-ranks top candidates with exact cosine similarity
  - ~97% memory reduction vs flat index
  - Algorithm option: `"bq"` in vector search API
- **Porter Stemming** for Full-Text Search
  - Pure Go Porter Stemmer implementation (no external deps)
  - Stems indexed terms and query terms for better recall
  - Configurable: `MDDB_FTS_STEMMING` (default: true)
  - Per-query disable via `disableStem` request field
- **Synonym Support** for Full-Text Search
  - Per-collection synonym dictionaries stored in BoltDB
  - HTTP endpoints: `POST/GET/DELETE /v1/synonyms`
  - Built-in default synonym groups (10 English groups)
  - Bidirectional query-time expansion
  - Configurable: `MDDB_FTS_SYNONYMS` (default: true)
  - Per-query disable via `disableSynonyms` request field
- **Compression Configuration**
  - `MDDB_COMPRESSION_ENABLED` - enable/disable adaptive compression
  - `MDDB_COMPRESSION_SMALL_THRESHOLD` - Snappy threshold (default: 1024)
  - `MDDB_COMPRESSION_MEDIUM_THRESHOLD` - Zstd threshold (default: 10240)
- **Extended Configuration**
  - New config sections: `fts`, `compression`, `vector` in YAML config file
  - All new features configurable via env vars, YAML, or CLI flags
  - FTS response includes `stemmingActive` and `synonymsActive` status

### Changed
- Panel VectorSearchPanel: added SQ and BQ to algorithm dropdown
- Panel FTSSearchPanel: added stemming/synonyms toggles
- Panel mddb-client: added synonym CRUD methods
- Version bumped to 2.6.4

## [2.6.2] - 2026-03-04

### Added
- **Embedding Chunking** - Auto-split long documents into chunks before embedding
  - Paragraph-based splitting with sentence and hard-split fallbacks
  - Multi-key chunk storage: `vec|collection|docID#0`, `vec|collection|docID#1`, etc.
  - Chunk deduplication in vector search: best-chunk-score per document
  - Oversampling (topK * 3) for accurate top-K after deduplication
  - Configurable via `MDDB_EMBEDDING_CHUNK_SIZE` (default 1500) and `MDDB_EMBEDDING_CHUNK_ENABLED` (default true)
  - Backward-compatible with existing non-chunked embeddings
  - Chunk stats in `/v1/vector-stats` and `/v1/vector-reindex` responses
- **Panel Mode** - `MDDB_PANEL_MODE` environment variable
  - `internal` (default): CORS enabled, browser accesses API directly
  - `external`: CORS disabled, panel reverse-proxies all `/v1/*` requests
  - Express production server (`server.js`) with `http-proxy-middleware`
  - Panel always uses relative `/v1` URLs (works in both modes)

### Changed
- Panel Dockerfile uses Express server instead of Vite preview for production
- Panel `mddb-client.js` simplified to always use relative `/v1` API base
- Vector search handlers (HTTP, gRPC, MCP) use oversampling + chunk deduplication
- All 4 vector searchers (flat, HNSW, IVF, PQ) handle chunk keys in filter matching

## [2.3.3] - 2026-02-28

### Added
- **Custom MCP Tools** - YAML-defined website-specific AI tools for mddb-mcp
  - Define custom tools in `config.yaml` under `custom_tools:` key
  - 3 supported actions: `semantic_search`, `search_documents`, `full_text_search`
  - Preconfigured defaults merged with user arguments at runtime
  - Custom tools appear alongside 23 built-in tools in `tools/list`
  - Startup validation: name conflicts, valid actions, valid param types
  - Works on both transports: stdio (Claude Desktop) and HTTP
  - Deduplicated tool definitions (~420 lines removed, shared `builtinTools()`)
  - Dedicated documentation: `docs/CUSTOM-TOOLS.md`

## [2.3.2] - 2026-02-28

### Added
- **Telemetry** - Prometheus-compatible `/metrics` endpoint
  - HTTP request counters with method, path, and status labels
  - Request duration histograms (12 buckets from 1ms to 10s)
  - Database metrics: file size, documents, revisions, meta indices per collection
  - Vector search metrics: embeddings count, index readiness, queue size
  - Webhook and schema counts
  - Go runtime: goroutines, memory stats, GC metrics
  - Zero external dependencies (pure Go text exposition format)
  - DB stats cached for 15s to minimize scan overhead
  - Configurable via `MDDB_METRICS` env var (enabled by default)
  - Dedicated documentation: `docs/TELEMETRY.md` with Grafana queries and alerting rules

## [2.3.1] - 2026-02-28

### Added
- **Schema Validation** - JSON Schema validation for document metadata
  - Per-collection schemas (opt-in, disabled by default)
  - HTTP endpoints: `/v1/schema/set`, `/v1/schema/get`, `/v1/schema/delete`, `/v1/schema/list`, `/v1/validate`
  - gRPC RPCs: `SetSchema`, `GetSchema`, `DeleteSchema`, `ListSchemas`, `ValidateDocument`
  - Supported rules: `required`, `properties` (types), `enum`, `pattern`, `minItems`/`maxItems`
  - Automatic validation on document add/update when schema is set
  - CLI commands: `schema set/get/delete/list`, `validate`
  - MCP tools: `set_schema`, `get_schema`, `delete_schema`, `list_schemas`, `validate_document`
  - PHP and Python extension support
  - Dedicated documentation: `docs/SCHEMA-VALIDATION.md`
- **SECURITY.md** - Security policy and vulnerability reporting process
- **CONTRIBUTING.md** - Contribution guidelines with setup instructions

## [2.1.0] - 2025-01-09

### Added
- **Health check endpoints** - `/health` and `/v1/health` for monitoring
  - Simple JSON response with status and mode
  - Database connectivity verification
  - HTTP 200 for healthy, 503 for unhealthy
- **OpenAPI/Swagger documentation** - Complete API specification
  - OpenAPI 3.0.3 specification in `docs/openapi.yaml`
  - Interactive Swagger UI in `docs/swagger.html`
  - Machine-readable API documentation
  - Try-it-out functionality for all endpoints
- **Health check documentation** - Comprehensive guide in `docs/HEALTHCHECK.md`
  - Docker and Docker Compose examples
  - Kubernetes liveness and readiness probes
  - Load balancer configuration (Nginx, HAProxy, Traefik)
  - Manual health check methods (curl, wget, httpie)
  - Monitoring integration examples
  - Troubleshooting guide

### Changed
- Updated Docker health checks to use `/health` endpoint
- Updated docker-compose.yml with proper health check configuration
- Updated Dockerfile with health check using wget
- Simplified performance claims in documentation (more pragmatic, less boastful)
- Removed Polish documentation files (English only)
- Fixed license badge and references (BSD-3-Clause)

## [2.0.3] - 2025-11-07

### Added
- **Document deletion functionality** - Delete documents with confirmation dialog
  - Delete button in document list items
  - Delete button in document viewer header
  - Confirmation modal with document details
  - Automatic list refresh after deletion
- **Error handling improvements** - Better error boundaries and fallbacks
  - ReactMarkdown error boundary with raw content fallback
  - Progressive document loading (immediate display + background fetch)
  - User-friendly error messages with recovery options
- **Docker image for mddb-panel** - Complete containerization
  - Multi-stage Docker build for production
  - Development Docker configuration
  - Docker Compose integration
  - Makefile targets for panel Docker operations

### Fixed
- **Blank document viewer issue** - Documents now display immediately with content
- **Document content loading** - Fixed API integration for full document fetching
- **ReactMarkdown compatibility** - Removed deprecated className prop
- **Content overflow issues** - Fixed margin and layout problems in document viewer
- **UI responsiveness** - Better loading states and user feedback
- **golangci-lint errors** - Fixed unchecked error returns in JSON encoding

### Improved
- **Document viewer layout** - Better container constraints and overflow handling
- **User experience** - Immediate feedback when clicking documents
- **Error recovery** - Users can always access document content in some form
- **Development workflow** - Added panel to development Docker setup

## [Unreleased]

### Added
- **Web Admin Panel (mddb-panel)** - Modern React-based admin interface
  - Server statistics dashboard
  - Collection browser with document count
  - Document list with metadata preview
  - Document viewer with full content and metadata
  - **Document editor** - Edit markdown content and metadata
  - **New document creation** - Create documents directly from UI
  - **Markdown editor with live preview** - Split view with real-time rendering
  - **Markdown toolbar** - Quick formatting buttons (bold, italic, headings, lists, etc.)
  - **Syntax highlighting** - Code blocks with language-specific highlighting
  - **Markdown templates** - Pre-built templates (blog, docs, README, API, changelog)
  - Advanced filtering by metadata fields
  - Sort by date, key, or custom fields
  - Copy document content to clipboard
  - Modern UI with TailwindCSS and Lucide icons
  - Built with React 19, Vite 6, and Zustand 5
  - Docker support with multi-stage builds
  - Proxy configuration for API requests
- Bulk import script for loading markdown files from folders
- `load-md-folder.sh` script with features:
  - Automatic key generation from filenames
  - YAML frontmatter metadata extraction
  - Recursive folder scanning
  - Progress tracking with statistics
  - Dry run mode for preview
  - Custom metadata support
  - Multi-language support
- Makefile targets for folder import:
  - `import-folder` - Import markdown files
  - `import-folder-dry` - Preview import without executing
  - `import-folder-recursive` - Import recursively
- Makefile targets for panel:
  - `panel-install` - Install panel dependencies
  - `panel-dev` - Run panel in development mode
  - `panel-build` - Build panel for production
  - `panel-preview` - Preview production build
- Comprehensive bulk import documentation (BULK-IMPORT.md)
- README section for bulk import usage
- README section for web admin panel
- Docker Compose configuration for panel service

## [2.0.2] - 2025-11-07

### Changed
- Updated quic-go to v0.55.0 (HTTP/3 improvements)
- Updated Alpine base image to 3.22 (security updates)
- Updated Go dependencies (crypto, net, sys, mod, text, tools)
- Disabled automatic workflow triggers (manual only)
- Removed Docker buildcache (not needed for users)
- Removed dev Docker images (production only)

### Fixed
- Docker build context issues
- Docker Hub description update (now manual)

## [2.0.1] - 2025-11-07

### Fixed
- Fixed all golangci-lint issues (18 total)
  - errcheck: Added error checking for binary.Write, file.Close, res.Body.Close
  - staticcheck: Removed redundant nil check, optimized fmt usage, fixed pointer usage
  - unused: Removed unused struct fields (mu, workerPool, current)
- Fixed proto definitions for UpdateDocument, DeleteDocument, and batch responses
- Added missing SaveRevision and NotFound fields to proto messages

### Added
- Docker Hub integration with automated builds
- Multi-platform Docker images (AMD64 + ARM64)
- Comprehensive Docker Hub documentation
- GitHub Actions workflow for Docker builds and pushes
- Docker Compose configuration for production deployment

### Changed
- Updated golangci-lint to v2.6.1 with Go 1.26 support
- Improved error handling across codebase
- Optimized buffer pool usage to avoid allocations

## [2.0.0] - 2025-11-07

### Major Performance Release

**Significant performance improvements through multiple optimization strategies**

#### Performance Enhancements
- Protobuf serialization for smaller payloads
- BoltDB tuning (NoFreelistSync, FreelistMapType, optimized mmap)
- Conditional metadata reindexing
- Batch processing with single transactions
- Parallel processing with worker pools
- Connection pooling for gRPC
- Bucket caching
- Optional revision history
- Single transaction search
- Lazy indexing with async queue
- Read-through document cache
- Batch delete and update operations

#### Advanced Features (Extreme Mode)
Enable with `MDDB_EXTREME=true` environment variable:
- Write-Ahead Log (WAL) with periodic sync
- Lock-free cache with 16 shards
- MVCC snapshot isolation
- Bloom filters for fast lookups
- Delta encoding for smaller revisions
- Adaptive compression (Snappy/Zstd)
- HTTP/3 + QUIC support
- Adaptive indexing
- Async I/O operations
- Zero-copy I/O
- Vectorized operations (SIMD)
- Distributed sharding

### Benchmark Results

Tested with 3000 documents:
- MDDB (Batch API): 29,810 docs/s, 34µs avg latency
- MongoDB: 5,176 docs/s, 192µs avg latency
- PostgreSQL: 4,324 docs/s, 231µs avg latency
- MySQL: 1,214 docs/s, 822µs avg latency
- CouchDB: 312 docs/s, 3,185µs avg latency

### Added
- HTTP/3 server on port 11443 (Extreme Mode)
- Comprehensive performance benchmarking suite
- Comparison tests with MongoDB, PostgreSQL, MySQL, CouchDB

## [1.0.0] - Initial Release

### Added
- Initial release of MDDB
- RESTful API for markdown document management
- **gRPC API** - High-performance binary protocol (70% smaller payload than JSON)
- **Dual protocol support** - HTTP (port 11023) and gRPC (port 11024) run simultaneously
- **Docker images** - Optimized Alpine Linux images (~15 MB)
- **Docker Compose** - Production and development configurations
- **Shared Protobuf** - Monorepo structure with centralized proto definitions
- **Multi-language clients** - Generated code for Go, Python, Node.js, PHP
- BoltDB-based storage engine
- Document versioning and revision history
- Metadata indexing and search
- Multi-language support
- Template variable substitution
- Export functionality (NDJSON and ZIP formats)
- Backup and restore capabilities
- Revision truncation for database maintenance
- Access mode control (read, write, read-write)
- **Statistics endpoint** - `/v1/stats` for server and database monitoring
- **Command-line client (mddb-cli)** - Full-featured CLI similar to mysql-client
- **Unix man page** - Complete manual page for CLI
- Comprehensive documentation
- Makefile with development and build targets
- Systemd service configuration

### Features

#### Core Functionality
- Add/update markdown documents with metadata
- Retrieve documents by key and language
- Search with metadata filtering
- Sort by addedAt, updatedAt, or key
- Pagination support
- Template variable substitution (%%var%% syntax)

#### Storage
- BoltDB embedded database
- Automatic metadata indexing
- Revision history tracking
- Efficient prefix-based indices
- ACID transactions

#### API Endpoints
- `POST /v1/add` - Add or update documents
- `POST /v1/get` - Retrieve documents
- `POST /v1/search` - Search with filters
- `POST /v1/export` - Export as NDJSON or ZIP
- `GET /v1/backup` - Create backup
- `POST /v1/restore` - Restore from backup
- `POST /v1/truncate` - Truncate revision history
- `GET /v1/stats` - Server and database statistics

#### Developer Experience
- Comprehensive Makefile with colored output
- Hot reload support with Air
- Cross-platform builds (Linux, Windows, macOS)
- Test coverage reporting
- Code formatting and linting targets
- Development and production modes

#### Command-Line Client
- `mddb-cli` - Full-featured CLI client
- Unix-style commands (add, get, search, export, backup, restore, truncate, stats)
- Man page documentation (`man mddb-cli`)
- JSON and human-readable output formats
- Pipe-friendly content-only mode
- Metadata filtering and search
- Template variable support
- Batch operation support
- Server statistics display

#### Documentation
- Quick start guide
- Complete API documentation
- Usage examples with multiple languages
- Architecture overview with diagrams
- Production deployment guide
- Docker and systemd configurations

### Technical Details
- Go 1.26+ required
- BoltDB for storage
- HTTP/JSON API
- Single binary deployment
- No external dependencies

## [0.1.0] - 2024-11-06

### Added
- Initial project structure
- Basic MDDB server implementation
- Core API endpoints
- Documentation suite
- Build system with Makefile
- Docker support

---

## Release Notes

### Version 0.1.0 (Initial Release)

This is the first release of MDDB - a lightweight markdown database with a RESTful API.

**Key Features:**
- Store and manage markdown documents with metadata
- Full revision history
- Fast metadata-based search
- Multi-language support
- Export capabilities
- Easy backup and restore

**Getting Started:**
```bash
make build
make run
```

See the [Quick Start Guide](docs/QUICKSTART.md) for detailed instructions.

**Requirements:**
- Go 1.26 or later
- 512 MB RAM minimum
- Linux, macOS, or Windows

**Known Limitations:**
- Single-writer (BoltDB limitation)
- No built-in authentication
- No full-text search (planned for future release)

**Future Plans:**
- Full-text search integration
- Built-in authentication
- GraphQL API
- Replication support
- Plugin system

---

## Contributing

When contributing, please:
1. Update this CHANGELOG with your changes
2. Follow [Keep a Changelog](https://keepachangelog.com/) format
3. Add entries under `[Unreleased]` section
4. Use these categories: Added, Changed, Deprecated, Removed, Fixed, Security

## Links

- [Repository](https://github.com/tradik/mddb)
- [Documentation](docs/)
- [Issues](https://github.com/tradik/mddb/issues)
