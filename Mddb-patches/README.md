# MDDB Windows Port

A vendor patch layer that makes the upstream [MDDB](https://github.com/mddb) project run natively on Windows x64.

This is **not** a feature fork. The upstream MDDB repository is the base; this project adds the smallest possible set of changes to support Windows builds, runtime, APIs, filesystem, networking, tests, and CI.

## Goal

- Run MDDB natively on Windows x64.
- Keep divergence from upstream as small as possible.
- Every change must directly support Windows compatibility (build, runtime, API, filesystem, networking, tests, CI, or a cross-platform correctness issue discovered during the port).
- No new application features, UI changes, behavioral changes, or unrelated refactoring.

## Architecture

The Git repository stores **only**:

1. The pristine upstream MDDB source (read-only — no upstream file is ever committed modified).
2. The Windows Port Layer inside `Mddb-patches/`.
3. GitHub Actions workflows.

The upstream source tree is **never** modified in Git. All Windows compatibility changes are vendor patches applied ephemerally by CI.

```
Mddb-patches/
├── README.md           ← this file
├── SERIES.md           ← per-patch documentation
├── PATCH_INDEX.md      ← searchable index
├── WINDOWS_PORT.md     ← technical architecture notes
├── CHANGELOG.md        ← changes to the port itself
├── STATUS.md           ← current project status
├── third_party/        ← Go shims / compatibility modules (e.g. renameio)
└── patches/
    ├── 0001-...
    ├── 0002-...
    └── ...
```

Each patch is a standard `git format-patch` artifact that modifies **upstream files only** (services/**). The series is ordered and numbered.

## Build Architecture

The GitHub Actions runner is the **only place** where upstream files are allowed to change:

1. Checkout pristine repository.
2. Apply vendor patches from `Mddb-patches/patches/`.
3. Build.
4. Test.
5. Produce artifacts.
6. Destroy the runner.

The patched source exists **only inside the ephemeral GitHub Actions workspace**. It is never committed back to Git.

## Recovery Requirement

At any time the repository can be reset to a clean upstream checkout. Applying every patch in `Mddb-patches/patches/` in order **must** reproduce the complete Windows-native repository exactly. If it does not, the patch set is incorrect.

## How to Apply the Patches

From a clean checkout of the upstream baseline commit:

```bash
git checkout <upstream-baseline>
git apply Mddb-patches/patches/*.patch
```

Current upstream baseline: `dbc9def` (see `STATUS.md`).

To verify a clean application reproduces the patched state:

```bash
git checkout -b verify <upstream-baseline>
git apply Mddb-patches/patches/*.patch
git diff <patched-reference>   # should be empty
```

## Supported Platforms

- **Windows**: Windows 10 / Windows 11 / Windows Server 2016+ (x64).
- **Build**: native `go build` with `CGO_ENABLED=0`, or cross-compile from Linux via the `build-windows.yml` CI workflow.
- **Go**: same version as upstream (see upstream `go.mod`).

## Repository Workflow

1. Sync with upstream if needed.
2. Implement the minimum required Windows compatibility change as a vendor patch (modifies upstream files only) or as port-layer code in `Mddb-patches/`.
3. Generate the vendor patch (`git format-patch` against the pristine upstream baseline).
4. Store it in `Mddb-patches/patches/`, continuing the numbered sequence.
5. Update `SERIES.md`, `PATCH_INDEX.md`, `CHANGELOG.md`, and `STATUS.md`.
6. Verify the patch series reproduces the patched state from pristine upstream.
7. Present the patch + docs for review.
8. Wait for approval.
9. Commit the updated `Mddb-patches/` (upstream source stays pristine).
10. Push.

A vendor patch is **not complete** until its documentation has been updated and verified.
