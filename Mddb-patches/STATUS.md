# Project Status

Current status of the MDDB Windows port.

**Last updated:** 2026-08-07

---

## Upstream Baseline

| Item | Value |
|------|-------|
| Baseline commit | `dbc9def` — "Add GitHub Actions workflow for MDDB Panel build" |
| Baseline date | 2026-08-06 |
| Latest synchronized upstream commit | `dbc9def` (not yet synced beyond the baseline) |

## Patch Series

| Item | Value |
|------|-------|
| Total vendor patches | 13 |
| Patch range | `0001` – `0013` |
| Series complete (reproduces `main` from baseline) | Yes — verified `git diff` empty, `git diff-tree -r` exit 0 |

## Windows Build Status

| Target | Status |
|--------|--------|
| `mddbd.exe` cross-compile (Linux → Windows, `CGO_ENABLED=0`) | Passing |
| `mddb-cli.exe` cross-compile (Linux → Windows, `CGO_ENABLED=0`) | Passing |
| Native `go test ./...` on `windows-latest` | Passing (root `mddb` + all subpackages green) |

## Known Issues

None currently open. All known Windows build/runtime/test/CI gaps are covered by patches 0001–0013.

## Blockers

None.

## Next Milestones

1. **Sync with latest upstream MDDB** and re-apply the vendor patch series; resolve any conflicts with minimal new patches.
2. **Upstream the cross-platform correctness fixes** (patches 0005, 0006, 0007, 0008, 0011, 0012, 0013) to reduce long-term divergence.
3. **Expand native Windows CI coverage** if upstream adds new tests or subsystems.
