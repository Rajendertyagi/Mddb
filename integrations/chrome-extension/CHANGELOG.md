# Changelog — MDDB Browser Extension

All notable changes to the `integrations/chrome-extension` package are documented in this
file. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), versions
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security

- **Validate the sender of `runtime.onMessage`** (`src/background.ts`) — the
  `mddb:refresh` handler ignored the message sender. It now accepts a message only when
  `sender.id === chrome.runtime.id` **and** `sender.tab` is unset, i.e. from the extension's
  own trusted surfaces (popup / options / internal pages). A content script injected into a
  web page can no longer coax a refresh (which would force traffic to the configured MDDB
  server with its auth header). Tests cover the foreign-id and content-script (`sender.tab`)
  rejections; `background.ts` stays at 100% coverage.

## [0.1.0] — 2026-05-22

### Added

- Initial release of `@tradik/mddb-browser-extension` (Chrome Manifest V3, minimum Chrome 120).
- Toolbar popup with live stats from `GET /v1/stats`: total documents, total revisions,
  collection count, mode, uptime, and a sorted list of the top 5 collections.
- Toolbar badge counter — refreshed on a configurable cron (30 s – 1 h, default 60 s) via
  the `alarms` API; colour codes connected/error/unconfigured states.
- One-click link from the popup to the MDDB admin panel (defaults to the server origin on
  port 3000, overridable per-install).
- Options page (opens in its own tab) for the MDDB server URL, optional `X-API-Key`,
  optional admin-panel URL override, and refresh interval — values stored locally only via
  `chrome.storage.local`.
- "Test connection" button on the options page that pings `GET /v1/health` and
  distinguishes between auth, server, and network failures.
- Per-origin runtime host permission — the extension asks for access to the configured
  server origin only when the user saves a URL, never the full web.
- Bundled privacy policy and terms of use (`privacy.html` / `terms.html`) plus footer
  links to the canonical pages at `https://tradik.com/privacy` and
  `https://tradik.com/terms`.
- esbuild bundling of three ESM entrypoints (`popup`, `options`, `background`) plus a
  zero-dependency `scripts/package.mjs` that emits a Chrome Web Store-ready zip into
  `dist/`.
- 98 Jest tests (jsdom + chrome.* stub) with ≥90 % coverage enforced on statements,
  branches, functions, and lines.
- GitHub Actions workflow `chrome-extension.yml` covering format check, ESLint, tests with
  coverage, `npm audit`, esbuild bundle, packaging, smoke install of the zip, and
  automatic publish of `chrome-ext-v*` tag pushes to a GitHub Release with the signed zip
  attached.

[0.1.0]: https://github.com/tradik/mddb/releases/tag/chrome-ext-v0.1.0
