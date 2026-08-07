# MDDB Windows Port

A vendor patch layer that makes the upstream [MDDB](https://github.com/mddb) project run natively on Windows x64.

This is **not** a feature fork. The upstream MDDB repository is the base; this project adds the smallest possible set of changes to support Windows builds, runtime, APIs, filesystem, networking, tests, and CI.

## Goal

- Run MDDB natively on Windows x64.
- Keep divergence from upstream as small as possible.
- Every change must directly support Windows compatibility (build, runtime, API, filesystem, networking, tests, CI, or a cross-platform correctness issue discovered during the port).
- No new application features, UI changes, behavioral changes, or unrelated refactoring.

## How the Patch System Works

All Windows compatibility work lives in `Mddb-patches/` as an ordered series of vendor patches plus supporting documentation.

```
Mddb-patches/
├── README.md           ← this file
├── SERIES.md           ← per-patch documentation
├── PATCH_INDEX.md      ← searchable index
├── WINDOWS_PORT.md     ← technical architecture notes
├── CHANGELOG.md        ← changes to the port itself
├── STATUS.md           ← current project status
└── patches/
    ├── 0001-...
    ├── 0002-...
    └── ...
```

Each patch is a standard `git format-patch` artifact. The series is ordered and numbered. Applying every patch in order reproduces the full Windows-native repository.

## Recovery Requirement

At any time the repository can be reset to a clean upstream checkout. Applying every patch in `Mddb-patches/patches/` in order **must** reproduce the complete Windows-native repository exactly. If it does not, the patch set is incorrect.

## How to Apply the Patches

From a clean checkout of the upstream baseline commit:

```bash
git checkout <upstream-baseline>
git am Mddb-patches/patches/*.patch
```

Current upstream baseline: `dbc9def` (see `STATUS.md`).

To verify a clean application reproduces the current tree:

```bash
git checkout -b verify <upstream-baseline>
git am Mddb-patches/patches/*.patch
git diff main          # should be empty
```

## Supported Platforms

- **Windows**: Windows 10 / Windows 11 / Windows Server 2016+ (x64).
- **Build**: native `go build` with `CGO_ENABLED=0`, or cross-compile from Linux via the `build-windows.yml` CI workflow.
- **Go**: same version as upstream (see upstream `go.mod`).

## Repository Workflow

1. Sync with upstream if needed.
2. Implement the minimum required Windows compatibility change.
3. Generate the vendor patch (`git format-patch`).
4. Store it in `Mddb-patches/patches/`, continuing the numbered sequence.
5. Update `SERIES.md`, `PATCH_INDEX.md`, `CHANGELOG.md`, and `STATUS.md`.
6. Present the patch + docs for review.
7. Wait for approval.
8. Commit the source changes together with the updated `Mddb-patches/`.
9. Push.

A vendor patch is **not complete** until its documentation has been updated.
