# Changelog — MDDB Sync (WordPress plugin)

All notable changes to this WordPress plugin are documented here. Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/); the plugin adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Releases are tagged `wp-vX.Y.Z` in this repository to avoid clashing with `vX.Y.Z` tags used for the core MDDB server.

## [0.1.0] - 2026-05-19

### Added

- **Initial release** — synchronises WordPress posts and pages to MDDB.
  - `wp_after_insert_post` → `POST /v1/add` (autosaves and revisions are skipped; drafts opt-in).
  - `wp_trash_post` and `before_delete_post` → `POST /v1/delete`.
  - Retry on transient `429/5xx` with exponential backoff (500 ms → 1 s → 2 s, max 3 attempts).
- **Settings → MDDB Sync** screen with: MDDB URL, API key, collection name, sync-on-save / sync-on-delete / include-drafts toggles, public-post-type checkboxes, language-detection strategy, key strategy, and a "Test connection" button that probes `/v1/search`.
- **Language detection** — Polylang (`pll_get_post_language`) → WPML (`wpml_post_language_details` filter) → site locale (`get_locale()`). All values are normalised to `lang_REGION` (e.g. `pl_PL`).
- **Key strategies** — `posttype-id` (default, e.g. `post-42`), `posttype-slug` (e.g. `post-hello-world`), or permalink path (e.g. `2026/05/hello-world`).
- **MDDB document mapping** — meta carries `postType`, `status`, `title`, `slug`, `permalink`, `authorId`, `author`, `publishedAt`, `modifiedAt`, `excerpt`, `categories`, `tags`; `contentMd` runs the body through the standard `the_content` filter then `wp_strip_all_tags()` so shortcodes/blocks/oEmbed get expanded.
- **GitHub Releases auto-update channel** — hooks into `pre_set_site_transient_update_plugins` and `plugins_api`, polls `repos/tradik/mddb/releases/latest` (cached 12h), looks for a `mddb-sync-<version>.zip` asset.
- **CI workflow** ([`.github/workflows/wordpress-plugin.yml`](../../.github/workflows/wordpress-plugin.yml)) — matrix runs PHP 8.1 / 8.2 / 8.3 / 8.4 with `composer audit`, `composer lint` (WordPress security ruleset + PHPCompatibilityWP), `composer stan` (PHPStan level 5, WordPress-aware via `szepeviktor/phpstan-wordpress`), and `composer test:coverage` (PHPUnit 10 + xdebug). Coverage gate enforces ≥90 % lines on the 8.3 leg. Tagging `wp-vX.Y.Z` triggers the `build` job which packs `mddb-sync-X.Y.Z.zip` and publishes a GitHub Release with the asset attached.
- **Plugin uses BrainMonkey** for WP-function mocking; no WordPress install needed for tests.
- **Custom action** `mddb_sync_error` ($tag, WP_Error, $postId) — fires on every sync failure so consumers can wire Sentry/Action Scheduler glue without us adding hard deps.

### Security

- All settings input is sanitised through dedicated `Settings::sanitize()` paths (`esc_url_raw`, `sanitize_key`, allow-listed enum values for language/key strategies).
- AJAX `Test connection` endpoint requires `manage_options` capability + nonce.
- Output in the settings template is escaped with `esc_attr` / `esc_html` / `esc_url`.
- Optional `Authorization: Bearer` header — empty key path covers internal dev instances without baking credentials.
