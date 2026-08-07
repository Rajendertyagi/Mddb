# Patch Series

Ordered vendor patch series for the MDDB Windows port.

**Upstream baseline:** `dbc9def` — "Add GitHub Actions workflow for MDDB Panel build"
**Total patches:** 15
**Last updated:** 2026-08-07

---

## 0001 — Windows builds via cross-compilation

- **Commit:** `14c73ec`
- **Type:** Windows-only
- **Upstreamable:** No (platform-specific build infra)
- **Status:** Applied
- **Files:**
  - `services/mddbd/cpu_usage_unix.go` (new)
  - `services/mddbd/cpu_usage_windows.go` (new)
  - `services/mddbd/disk_usage_unix.go` (new)
  - `services/mddbd/disk_usage_windows.go` (new)
  - `services/mddbd/go.mod` (modified)
  - `services/mddbd/incident_detector.go` (modified)
  - `services/mddbd/system_handlers.go` (modified)
- **Purpose:** Replace unix-only `syscall.Getrusage` / `syscall.Statfs` usage with platform-specific helpers backed by `golang.org/x/sys/windows`. The CI workflow that cross-compiles `mddbd.exe` / `mddb-cli.exe` (`CGO_ENABLED=0`) is committed separately under `.github/workflows/build-windows.yml` and applies this vendor patch series at build time.
- **Dependencies:** None (first patch).

---

## 0002 — Shim google/renameio for Windows cross-compile

- **Commit:** `1d96373`
- **Type:** Windows-only
- **Upstreamable:** No
- **Status:** Applied
- **Files:**
  - `services/mddbd/go.mod` (modified)
- **Purpose:** `google/renameio` exports no functions on Windows by design, which breaks compilation of `github.com/coder/hnsw` (it calls `renameio.TempFile`). mddbd never calls `hnsw.SavedGraph.Save()` (vector persistence is BoltDB-backed), so a tiny stdlib-only compatibility shim providing the three symbols hnsw references is sufficient. The shim itself lives in the port layer at `Mddb-patches/third_party/renameio/` (committed directly). This patch wires it into the build via a local `replace` directive pointing at the root shim; no vendoring of hnsw and upstream source untouched.
- **Dependencies:** 0001 (cross-compile must be possible to hit this failure).

---

## 0003 — Handle (Handle, error) return from GetCurrentProcess

- **Commit:** `cf24aa3`
- **Type:** Windows-only
- **Upstreamable:** No
- **Status:** Applied
- **Files:**
  - `services/mddbd/cpu_usage_windows.go` (modified)
- **Purpose:** Fix the `GetCurrentProcess` wrapper to return `(windows.Handle, error)` correctly on Windows instead of a raw handle.
- **Dependencies:** 0001, 0002.

---

## 0004 — Runtime + tests compatibility for Windows

- **Commit:** `cdb703c`
- **Type:** Windows-only
- **Upstreamable:** No
- **Status:** Applied
- **Files:**
  - `services/mddbd/replacefile_unix.go` (new)
  - `services/mddbd/replacefile_windows.go` (new)
  - `services/mddbd/util.go` (modified)
  - `services/mddbd/replication_client.go` (modified)
  - `services/mddbd/auth_grpc_test.go` (modified)
  - `services/mddbd/auth_handlers_test.go` (modified)
  - `services/mddbd/auth_manager_test.go` (modified)
  - `services/mddbd/auth_middleware_test.go` (modified)
  - `services/mddbd/listen_addr_test.go` (modified)
  - `services/mddb-cli/main.go` (modified)
- **Purpose:**
  - `replaceFile` helper: Windows `os.Rename` cannot overwrite an existing destination, so add a remove-then-rename path (platform-split files) used by `copyFile` and `ReplicationClient.replaceDatabase`.
  - Tests: use `filepath.Join(t.TempDir(), ...)` for auth DB paths instead of `/tmp`; skip UDS listener tests on Windows (unix sockets unsupported); `shortSocketDir` uses `os.TempDir()` on Windows.
  - mddb-cli: open GraphQL playground via `cmd /c start` on Windows.
  - The `windows-latest` runtime `go test ./...` job lives in the committed `.github/workflows/build-windows.yml` and applies this series at build time.
- **Dependencies:** 0001.

---

## 0005 — De-flake TestIndexQueue_MultipleJobs

- **Commit:** `1a958ca`
- **Type:** Cross-platform correctness
- **Upstreamable:** Yes (removes timing flakiness on any slow/saturated CI)
- **Status:** Applied
- **Files:**
  - `services/mddbd/internal/indexqueue/indexqueue_test.go` (modified)
- **Purpose:** Replace a fixed 500ms sleep with `waitFor` polling; fixed sleeps are flaky on slow/saturated machines (e.g. Windows CI) where 20 workers may not finish in time.
- **Dependencies:** 0004 (Windows CI job surfaces the flake).

---

## 0006 — De-flake TestIndexQueue_StatsAfterProcessing

- **Commit:** `9c80ee4`
- **Type:** Cross-platform correctness
- **Upstreamable:** Yes
- **Status:** Applied
- **Files:**
  - `services/mddbd/internal/indexqueue/indexqueue_test.go` (modified)
- **Purpose:** Same fixed-sleep flakiness as 0005: 5 jobs / 2 workers / hard 200ms sleep raced on slower Windows CI. Poll `Stats()` via `waitFor` instead.
- **Dependencies:** 0005.

---

## 0007 — De-flake TestAuditBatchFlushLarge

- **Commit:** `685c873`
- **Type:** Cross-platform correctness
- **Upstreamable:** Yes
- **Status:** Applied
- **Files:**
  - `services/mddbd/audit_test.go` (modified)
- **Purpose:** `waitFlush()` was a fixed 700ms sleep; the audit writer flushes async on a 500ms ticker / batch-64, so on slow Windows CI only part of 200 records persisted in time (got 66). Poll `Query` until all 200 are persisted (bounded).
- **Dependencies:** 0004.

---

## 0008 — Audit: globally unique BoltDB keys during batch flush

- **Commit:** `076dc2d`
- **Type:** Cross-platform correctness (root-caused from Windows)
- **Upstreamable:** Yes (fixes a latent key-collision bug)
- **Status:** Applied
- **Files:**
  - `services/mddbd/internal/audit/audit.go` (modified)
- **Purpose:** `flushBatch()` seeded all keys in a batch from a single `b.NextSequence()` call. bbolt advances its stored sequence by only 1 per call, so consecutive batches reused overlapping sequence numbers; with identical timestamps (tight loop, coarse Windows clock) keys collapsed and `Put` silently overwrote rows (got 66 of 200). Fix: one `NextSequence()` per event → strictly monotonic, globally unique `(ts, seq)`. **Production-only** — the `TestAuditBatchFlushLarge` de-flake (poll instead of fixed sleep) lives in 0007 and is intentionally NOT touched here.
- **Dependencies:** 0007 (the polling fix revealed the deterministic count).

---

## 0009 — gRPC: close+swap+reopen DB during Restore

- **Commit:** `1dadef4`
- **Type:** Windows-only
- **Upstreamable:** No (correct on Unix as-is; Windows needs the close/reopen)
- **Status:** Applied
- **Files:**
  - `services/mddbd/grpc_server.go` (modified)
- **Purpose:** `GRPCServer.Restore` copied the backup over the live, open bbolt file. Unix allows renaming over an open file; Windows forbids removing an open file, so restore failed with "being used by another process". Wrap the whole close→copy→reopen in `Server.withRestoreLock` (GO-004): take the exclusive `restoreMu` write lock so concurrent readers drain and the handle is swapped atomically (not just on single-threaded Restore), then reassign the pointer and reset the binlog.
- **Dependencies:** 0004 (`replaceFile` helper); uses `Server.withRestoreLock` / `restoreMu` (server_restore.go).

---

## 0010 — gRPC-test: close live s.DB in newTestGRPCServer cleanup

- **Commit:** `841cf1b`
- **Type:** Windows-only
- **Upstreamable:** No
- **Status:** Applied
- **Files:**
  - `services/mddbd/grpc_server_test.go` (modified)
- **Purpose:** `GRPCServer.Restore` swaps `s.DB` with a newly reopened `*bolt.DB`. The test cleanup closed only the original handle, leaking the new one and leaving `test.db` open, which broke `t.TempDir()` cleanup on Windows. Fix: close the live `s.DB`.
- **Dependencies:** 0009.

---

## 0011 — Replace fixed sleeps with waitFor polling (indexqueue, temporal)

- **Commit:** `d978e71`
- **Type:** Cross-platform correctness
- **Upstreamable:** Yes
- **Status:** Applied
- **Files:**
  - `services/mddbd/internal/indexqueue/indexqueue_test.go` (modified)
  - `services/mddbd/internal/temporal/temporal_test.go` (modified)
- **Purpose:** Async-worker tests flaked on slow Windows CI because fixed 100–600ms sleeps were too short. Replace with `waitFor` polling against the real condition (`processed == 2` / histogram non-empty).
- **Dependencies:** 0004.

---

## 0012 — Temporal-test: correct HistogramBucket type

- **Commit:** `b6ac55b`
- **Type:** Cross-platform correctness
- **Upstreamable:** Yes (compile fix)
- **Status:** Applied
- **Files:**
  - `services/mddbd/internal/temporal/temporal_test.go` (modified)
- **Purpose:** Compile fix — the bucket type is `TemporalHistogramBucket`, not `HistogramBucket`. No logic change.
- **Dependencies:** 0011.

---

## 0013 — Indexqueue-test: poll for processed job in TestIndexQueue_EnqueueAndProcess

- **Commit:** `334d7b4`
- **Type:** Cross-platform correctness
- **Upstreamable:** Yes
- **Status:** Applied
- **Files:**
  - `services/mddbd/internal/indexqueue/indexqueue_test.go` (modified)
- **Purpose:** Same fixed-sleep flake class. Replace `time.Sleep(100ms)` + immediate check with `waitFor` polling on `processed == 1`.
- **Dependencies:** 0011.

---

## 0014 — Embed mddb-panel web UI into mddbd.exe

- **Commit:** `ac5ba6c`
- **Type:** Windows packaging (self-contained delivery)
- **Upstreamable:** No (delivery model; relevant to any embedded-UI build)
- **Status:** Applied
- **Files:**
  - `services/mddbd/webui/.gitkeep` (new)
  - `services/mddbd/ui_embed.go` (new)
  - `services/mddbd/ui_handler.go` (new)
  - `services/mddbd/ui_handler_test.go` (new)
  - `services/mddbd/main.go` (modified)
- **Purpose:** Make `mddbd.exe` fully self-contained — it serves both the JSON API and the React panel from a single binary with no Node, no separate static server, and no external CDN at runtime. A new `//go:embed webui` directive embeds the pre-built panel (CI copies `services/mddb-panel/dist` into `services/mddbd/webui` before building). `withEmbeddedUI` wraps the API mux: API/control-plane prefixes (`/v1`, `/graphql`, `/playground`, `/metrics`, `/health`, `/debug`) are delegated untouched; all other routes fall through to the embedded SPA (`index.html`) for client-side routing. The `webui` dir carries a `.gitkeep` so the package still compiles when the panel is not built (e.g. the windows-runtime `go test` job). Enabled in internal panel mode (`MDDB_PANEL_MODE`); disabled when `external`.
- **Dependencies:** 0001 (cross-compile); pairs with 0015 (panel assets).

---

## 0015 — Vendor Leaflet markers locally, make tiles optional

- **Commit:** `9f3a922`
- **Type:** Windows packaging (offline / no external deps)
- **Upstreamable:** Yes (removes unpkg CDN dependency; tiles opt-in)
- **Status:** Applied
- **Files:**
  - `services/mddb-panel/src/components/GeoPanel.jsx` (modified)
  - `services/mddb-panel/public/.gitkeep` (new)
- **Purpose:** GeoPanel previously fetched Leaflet marker icons from `unpkg.com` and basemap tiles from `openstreetmap.org` at runtime — external network dependencies that break offline / air-gapped use. Marker icons now point at locally-vendored `/marker-icon*.png` (copied from `node_modules/leaflet/dist/images` into `public/` at CI build time and embedded into `mddbd.exe` via 0014), served from the web root. The OpenStreetMap basemap tile layer is gated behind `VITE_MAP_TILES`: unset = no tiles (markers only, fully offline); set = enable basemap.
- **Dependencies:** 0014 (embedded web root serves the vendored PNGs).
