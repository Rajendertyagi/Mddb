# Changelog

Changes to the Windows port itself (not a duplicate of the upstream changelog).

## 2026-08-07 — Initial Windows port

First complete Windows x64 port of upstream MDDB, captured as an ordered vendor patch series.

### Added

- Cross-compilation support for `mddbd.exe` and `mddb-cli.exe` (`CGO_ENABLED=0`).
- `build-windows.yml` CI workflow: cross-compile job + native `windows-latest` `go test ./...` runtime job. Both jobs apply vendor patches before building/testing.
- Platform-split CPU/disk metric helpers (`cpu_usage_*`, `disk_usage_*`) backed by `golang.org/x/sys/windows`.
- `replacefile` helper with a Windows remove-then-rename path.
- `renameio` compile-time shim for Windows (port layer at `Mddb-patches/third_party/renameio/`).
- `waitFor` polling helper in tests to replace fixed `time.Sleep` calls.

### Changed

- gRPC `Restore` now closes/swaps/reopens the DB to satisfy Windows file-locking rules.
- Audit `flushBatch()` allocates one `bbolt.NextSequence()` per event for globally unique keys.
- Tests: auth DB paths use `t.TempDir()`; UDS listener tests skip on Windows; `shortSocketDir` uses `os.TempDir()` on Windows.
- mddb-cli opens the GraphQL playground via `cmd /c start` on Windows.

### Fixed

- Deterministic audit data loss on Windows (66 of 200 records) due to BoltDB key collisions.
- Multiple fixed-sleep test flakes on slow CI (indexqueue, temporal, audit).
- Compilation broken on Windows by `google/renameio` and unix-only syscalls.

### Documentation

- Created `Mddb-patches/` documentation suite: `README.md`, `SERIES.md`, `PATCH_INDEX.md`, `WINDOWS_PORT.md`, `CHANGELOG.md`, `STATUS.md`.
- Verified the patch series reproduces the current `main` tree from upstream baseline `dbc9def`.
