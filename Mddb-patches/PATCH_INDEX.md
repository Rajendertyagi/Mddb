# Patch Index

Searchable index of the vendor patch series.

**Upstream baseline:** `dbc9def`
**Total patches:** 16
**Last updated:** 2026-08-08

## By Patch Number

| # | Commit | Description | Subsystem | Upstreamable |
|---|--------|-------------|-----------|--------------|
| 0001 | `14c73ec` | Windows builds via cross-compilation | build / CI | No |
| 0002 | `1d96373` | Shim google/renameio for Windows cross-compile | build | No |
| 0003 | `cf24aa3` | Handle return from GetCurrentProcess | platform API | No |
| 0004 | `cdb703c` | Runtime + tests compatibility for Windows | runtime / tests / CI | No |
| 0005 | `1a958ca` | De-flake TestIndexQueue_MultipleJobs | tests | Yes |
| 0006 | `9c80ee4` | De-flake TestIndexQueue_StatsAfterProcessing | tests | Yes |
| 0007 | `685c873` | De-flake TestAuditBatchFlushLarge | tests | Yes |
| 0008 | `076dc2d` | Audit: globally unique BoltDB keys | storage / audit | Yes |
| 0009 | `1dadef4` | gRPC: close+swap+reopen DB during Restore | gRPC / storage | No |
| 0010 | `841cf1b` | gRPC-test: close live s.DB in cleanup | tests | No |
| 0011 | `d978e71` | Replace fixed sleeps with waitFor polling | tests | Yes |
| 0012 | `b6ac55b` | Temporal-test: correct HistogramBucket type | tests | Yes |
| 0013 | `334d7b4` | Indexqueue-test: poll for processed job | tests | Yes |
| 0014 | `7744da5` | Embed mddb-panel web UI into mddbd.exe | packaging / web | No |
| 0015 | `4207661` | Vendor Leaflet markers, optional tiles | panel / web | Yes |
| 0016 | `e55b6c2` | Temporal-test: poll for hot docs | tests | Yes |

## By Subsystem

### Build / CI
- 0001 `14c73ec` — Windows builds via cross-compilation
- 0002 `1d96373` — Shim google/renameio for Windows cross-compile
- 0004 `cdb703c` — Runtime + tests compatibility (test-side fixes; CI job is in the committed workflow)

### Platform API
- 0003 `cf24aa3` — Handle return from GetCurrentProcess
- 0004 `cdb703c` — replaceFile helper, UDS skip, temp paths

### Storage / Audit
- 0008 `076dc2d` — Audit: globally unique BoltDB keys
- 0009 `1dadef4` — gRPC: close+swap+reopen DB during Restore

### gRPC
- 0009 `1dadef4` — gRPC: close+swap+reopen DB during Restore
- 0010 `841cf1b` — gRPC-test: close live s.DB in cleanup

### Tests
- 0004 `cdb703c` — Runtime + tests compatibility (test-side fixes)
- 0005 `1a958ca` — De-flake TestIndexQueue_MultipleJobs
- 0006 `9c80ee4` — De-flake TestIndexQueue_StatsAfterProcessing
- 0007 `685c873` — De-flake TestAuditBatchFlushLarge
- 0010 `841cf1b` — gRPC-test: close live s.DB in cleanup
- 0011 `d978e71` — Replace fixed sleeps with waitFor polling
- 0012 `b6ac55b` — Temporal-test: correct HistogramBucket type
- 0013 `334d7b4` — Indexqueue-test: poll for processed job
- 0016 `e55b6c2` — Temporal-test: poll for hot docs instead of fixed sleep

### Packaging / Web
- 0014 `7744da5` — Embed mddb-panel web UI into mddbd.exe
- 0015 `4207661` — Vendor Leaflet markers locally, optional tiles

## By Type

### Windows-only (not upstreamable)
- 0001, 0002, 0003, 0004, 0009, 0010, 0014

### Cross-platform correctness (upstreamable)
- 0005, 0006, 0007, 0008, 0011, 0012, 0013, 0015, 0016
