# Windows Port — Technical Notes

Architecture decisions, compatibility shims, platform-specific behavior, APIs replaced, known limitations, and future cleanup for the MDDB Windows port.

**Upstream baseline:** `dbc9def`
**Last updated:** 2026-08-07

---

## Architecture Decisions

### Vendor patch layer (no fork)

The project is a thin vendor patch layer over upstream MDDB, not a fork. All Windows-specific changes are captured as ordered patches in `Mddb-patches/`. Upstream source is modified only when a shim or build constraint makes that unavoidable; the change is always the minimum required.

### Platform-split helpers

Where upstream uses unix-only syscalls directly, the port introduces platform-split files (`*_unix.go`, `*_windows.go`) behind a single function name, so the call sites stay cross-platform and the Windows implementation is isolated.

### Test de-flaking via polling

Several upstream tests used fixed `time.Sleep` calls to wait for async workers. These flake on any slow/saturated CI machine. The port replaces them with bounded `waitFor` polling against the real completion condition. These are cross-platform correctness fixes and are candidates for upstreaming.

---

## Compatibility Shims

### `replacefile` (patch 0004)

- **Files:** `services/mddbd/replacefile_unix.go`, `services/mddbd/replacefile_windows.go`
- **Why:** Windows `os.Rename` cannot overwrite an existing destination file; Unix can. `copyFile` and `ReplicationClient.replaceDatabase` need atomic-replace semantics.
- **Behavior:** Unix delegates to `os.Rename`. Windows removes the destination first, then renames. Used by `util.go` and `replication_client.go`.

### `renameio` shim (patch 0002)

- **Files:** `services/mddbd/Mddb-patches/third_party/renameio/go.mod`, `renameio.go`
- **Why:** `google/renameio` exports no functions on Windows by design, breaking compilation of `github.com/coder/hnsw`, which calls `renameio.TempFile`.
- **Behavior:** A stdlib-only module implementing the three symbols hnsw references (`TempFile`, `Cleanup`, `CloseAtomicallyReplace`) with a remove+rename fallback. Wired in via a local `replace` directive in `go.mod`. mddbd never calls `hnsw.SavedGraph.Save()` (vector persistence is BoltDB-backed), so the shim is never exercised at runtime.
- **Note:** This shim lives under `services/mddbd/Mddb-patches/third_party/` to keep it clearly separated from both upstream source and the patch archive.

### Process / disk metrics (patch 0001)

- **Files:** `cpu_usage_unix.go`, `cpu_usage_windows.go`, `disk_usage_unix.go`, `disk_usage_windows.go`
- **Why:** Upstream uses `syscall.Getrusage` and `syscall.Statfs`, which are unix-only.
- **Behavior:** Windows implementations back onto `golang.org/x/sys/windows` (`GetProcessTimes`, `GetDiskFreeSpaceEx`). Unix implementations preserve the original `syscall` usage.

---

## Platform-Specific Behavior

### Filesystem

- **Atomic replace:** remove-then-rename on Windows vs. direct `os.Rename` on Unix (see `replacefile`).
- **Temp paths:** Tests use `filepath.Join(t.TempDir(), ...)` instead of hardcoded `/tmp`.

### Networking

- **Unix domain sockets:** UDS listener tests skip on Windows (`runtime.GOOS == "windows"`), where unix sockets are unsupported. `shortSocketDir` uses `os.TempDir()` on Windows.

### CLI

- **Browser open:** mddb-cli opens the GraphQL playground via `cmd /c start` on Windows instead of the unix `open`/`xdg-open` path.

### gRPC / Storage

- **Restore:** `GRPCServer.Restore` closes the live bbolt DB, swaps the file, and reopens in place (Windows cannot remove/rename an open file). The HTTP `handleRestore` and `ReplicationClient.replaceDatabase` already followed this pattern; gRPC now matches it.

### Audit storage

- **Key uniqueness:** `flushBatch()` allocates one `bbolt.NextSequence()` per event instead of one per batch, guaranteeing strictly monotonic, globally unique `(ts, seq)` keys even when many events share the same timestamp (coarse Windows clock made the collision frequent).

---

## APIs Replaced

| Upstream API | Windows Replacement | Location |
|--------------|---------------------|----------|
| `syscall.Getrusage` | `windows.GetProcessTimes` | `cpu_usage_windows.go` |
| `syscall.Statfs` | `windows.GetDiskFreeSpaceEx` | `disk_usage_windows.go` |
| `os.Rename` (overwrite) | remove-then-rename via `replaceFile` | `replacefile_windows.go` |
| `google/renameio.TempFile` (and friends) | stdlib shim | `Mddb-patches/third_party/renameio/` |

---

## Known Limitations

- The `renameio` shim is compile-time only; it is never exercised because mddbd does not call `hnsw.SavedGraph.Save()`. If that changes, the shim would need to be validated for real behavior.
- UDS-dependent features remain unsupported on Windows (by OS design, not by the port).
- The Windows runtime CI job (`windows-latest`) is the primary signal; local Windows testing relies on it.

---

## Future Cleanup Opportunities

- **Upstream the cross-platform correctness fixes** (patches 0005, 0006, 0007, 0008, 0011, 0012, 0013): the `waitFor` polling and the audit key-uniqueness fix benefit upstream on any slow CI and fix a latent key-collision bug. Upstreaming reduces long-term divergence.
- **Remove the `renameio` shim** if upstream hnsw or MDDB adds a Windows-compatible path.
- **Consolidate platform-split files** if upstream adopts a cross-platform abstraction for process/disk metrics.
