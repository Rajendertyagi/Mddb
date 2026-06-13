# Changelog — MDDB Sync (WordPress plugin)

All notable changes to this WordPress plugin are documented here. Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/); the plugin adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Releases are tagged `wp-vX.Y.Z` in this repository to avoid clashing with `vX.Y.Z` tags used for the core MDDB server.

## [Unreleased]

### Changed

- **PHP support window moved to 8.2–8.5** (`.github/workflows/wordpress-plugin.yml`, `composer.json`, `phpcs.xml`, `mddb-sync.php`, `README.md`) — the CI matrix now tests PHP **8.2, 8.3, 8.4, 8.5** (added 8.5, dropped the near-EOL 8.1). The declared minimum (`composer.json` `php: >=8.2`, plugin header `Requires PHP: 8.2`) and the PHPCompatibility `testVersion 8.2-` are aligned so "tested == declared". Verified locally on PHP 8.5.7: phpcs (11/11), PHPUnit (77/77), PHPStan all green.

### Security

- **data hygiene for logs and release notes** ([`includes/class-client.php`](includes/class-client.php), [`includes/class-updater.php`](includes/class-updater.php)) — (1) `Client::addDocument()/deleteDocument()` put the **entire** server response body into the `WP_Error` message that ends up in `error_log()`, allowing log spam, CR/LF log forging, and disclosure of large/sensitive payloads. A new `responseSnippet()` truncates the body to 200 chars on a single line. (2) `Updater::providePluginInformation()` returned the GitHub release body as `description`/`changelog` unsanitised, rendering arbitrary HTML in the wp-admin "View details" modal — now passed through `wp_kses_post()`. New PHPUnit tests cover both.
- **validate the release-ZIP download host** ([`includes/class-updater.php`](includes/class-updater.php)) — `latestRelease()` passed the GitHub asset URL straight to WordPress's auto-updater, which downloads and installs the ZIP (arbitrary PHP). A manipulated API response / poisoned transient could substitute a hostile package (RCE). New `isTrustedZipUrl()` requires https + a GitHub-releases host allowlist; anything else yields an empty `zipUrl` (no update offered). Test covers a malicious host.
- **enforce `https://` for the MDDB endpoint** ([`includes/class-settings.php`](includes/class-settings.php)) — `Settings::sanitize()` accepted any URL that passed `wp_http_validate_url()`, including plain `http://`. Since the client attaches `Authorization: Bearer <apiKey>` to every request, an `http://` endpoint leaked the API key and full document bodies in cleartext (eavesdropping / MITM). A new `Settings::isAllowedUrl()` now requires `https://`, permitting `http://` **only** for local development hosts (`localhost`, `127.0.0.1`, `::1`). Rejected URLs raise an `add_settings_error()` admin notice instead of being silently dropped. Existing `https://` configurations are unaffected. New tests cover https-accepted, remote-http-rejected, and http-localhost/127.0.0.1/`[::1]`-accepted.

## [0.1.1] - 2026-05-19

### Added

- **Full meta capture in [`Mapper`](includes/class-mapper.php)** — every entry from `get_post_meta($id, '', false)` now ships, with scalar values cast to string and serialized arrays/objects JSON-encoded so they stay indexable. All taxonomies attached to the post type are walked via `get_object_taxonomies()`, not just `category`/`post_tag` (legacy aliases preserved). When ACF is active, `get_fields()` is layered on top with keys prefixed `acf:` so parsed values (URLs, post IDs, arrays) are stored alongside the raw `wp_postmeta` row.
- **Normalised SEO fields** ([`includes/class-seo.php`](includes/class-seo.php)) — extracts `seoTitle`, `seoDescription`, `seoFocusKeyword`, `seoCanonical`, `seoRobots*`, `ogTitle`/`ogDescription`/`ogImage`, `twitterTitle`/`twitterDescription`/`twitterImage`, and `seoSource` from **Yoast SEO**, **RankMath**, or **SEOPress**. Source with most filled fields wins (Yoast tie-breaker). Raw `_yoast_wpseo_*` / `rank_math_*` / `_seopress_*` keys also ship as-is from the post-meta dump.
- **`mddb_sync_meta` filter** — `apply_filters('mddb_sync_meta', $meta, $post)` is the last step in `Mapper::metaFor`. Use it to redact secrets, drop noisy keys (`_edit_lock`, `_edit_last`), or inject your own fields without forking.
- **Term filter ([`Settings::termFilter`](includes/class-settings.php), [`Sync::matchesTermFilter`](includes/class-sync.php))** — per-taxonomy term-ID allow-list rendered as a checklist on **Settings → MDDB Sync**. AND across taxonomies (every constrained taxonomy must match), OR inside one (any allowed term in the list is enough). Empty list for a taxonomy = no constraint. Applied to both the live `wp_after_insert_post` path and the bulk re-sync.
- **"Sync everything" button + paged AJAX bulk re-sync** ([`includes/class-bulk.php`](includes/class-bulk.php), [`includes/class-admin.php`](includes/class-admin.php)) — admin button walks `WP_Query` in 25-post pages, pushes each through `Sync::syncPost`, and reports progress (`processed / total — ok / skipped / failed`) with a `<progress>` bar. Capability-gated (`manage_options`) + nonce-protected. Stop button aborts the loop client-side. Up to ten last error messages surfaced inline.

### Tests

- 69 PHPUnit tests, **91.69 % line coverage** (Brain Monkey for WP-function mocking). New suites: `SeoTest`, `BulkTest`, `TermFilterTest` plus extended `MapperTest`, `SettingsTest`, `SyncTest`.

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
