# Changelog — MDDB Sync GitHub Action

All notable changes to the `integrations/github-action` package are documented in this file. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Migrated the action to **ESM** (`"type": "module"`) so it can consume the now ESM-only `@actions/core` v3 and `@actions/glob` v0.7.
- Bumped `@actions/core` 1.x → 3.0.1 and `@actions/glob` 0.5 → 0.7.
- `tsconfig` `module`/`moduleResolution` → `NodeNext`; relative imports use explicit `.js` specifiers; the CommonJS entry guard (`require.main === module`) is replaced with the ESM-native `import.meta.url` check.
- Test tooling moved to ESM: ts-jest ESM preset under `--experimental-vm-modules`, `@actions/core` mocked via `jest.unstable_mockModule`; `jest.config` / `eslint.config` renamed to `.cjs`.
- Rebuilt the committed `dist/` as an ESM bundle (no behaviour change — same inputs/outputs).

## [0.1.0] — 2026-05-20

### Added

- Initial release of `tradik/mddb/integrations/github-action`.
- Native Node 24 JavaScript action (bundled with `@vercel/ncc`) — no Docker startup cost.
- File walker built on `@actions/glob` supporting multi-pattern globs, inline `!`-negation and explicit `ignore` patterns.
- Three key-derivation strategies: `path` (slugified relative path), `hash` (sha1 of content), `filename` (basename).
- Smart content wrapping: markdown / plain text stored verbatim; JSON / YAML / TOML / HTML / common code files wrapped in fenced code blocks with the correct language hint for FTS/vector indexing.
- Per-document MDDB metadata: `source`, `path`, `extension`, `size`, and (when present) `repository` + `ref` populated from GitHub-provided env vars.
- HTTP client with exponential-backoff retries on `408 / 425 / 429 / 5xx` and network errors; ping/auth probe surfaces credential vs. server problems clearly.
- Configurable concurrency (1–64 parallel uploads), per-request timeout, and a `verify-ssl: false` escape hatch for self-signed dev MDDB instances.
- `dry-run: true` mode prints the file list without contacting the server — useful for CI sanity checks.
- `fail-on-error: false` to turn per-document upload failures into job warnings instead of red builds.
- Outputs: `documents-scanned`, `documents-added`, `documents-failed`.
- 57 unit tests covering inputs / walker / document / client / main, with 90%+ Jest coverage thresholds enforced.

[0.1.0]: https://github.com/tradik/mddb/releases/tag/gha-v0.1.0
