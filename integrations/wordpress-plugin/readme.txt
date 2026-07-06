=== MDDB Sync ===
Contributors: tradik
Tags: mddb, search, sync, markdown, polylang, wpml
Requires at least: 6.2
Tested up to: 6.7
Requires PHP: 8.2
Stable tag: 0.2.0
License: BSD-3-Clause
License URI: https://opensource.org/licenses/BSD-3-Clause

Synchronise WordPress posts and pages to an MDDB (Markdown Database) instance, with optional MCP remote publishing back into WordPress.

== Description ==

On save → POST /v1/add. On trash/delete → POST /v1/delete. Detects post language via Polylang / WPML / site locale. Self-updates from GitHub Releases (look for the `mddb-sync-<version>.zip` asset on the latest `wp-v*` tag).

Features:

* Sync on publish / update — autosaves and revisions are skipped.
* Clean entries on trash and delete.
* Optional draft sync.
* Public-post-type checkboxes (post, page, custom types).
* Language detection: Polylang → WPML → site locale.
* Three key strategies: post-type + ID, post-type + slug, or permalink path.
* Retry with exponential backoff on 429 / 5xx.
* Settings → MDDB Sync screen with a "Test connection" button.
* Optional remote publishing: /wp-json/mddb-sync/v1/publish and /status let MDDB's MCP tools create, update and (un)publish posts/pages — with tags, categories, custom taxonomies, meta fields and Polylang/WPML translations. Off by default, bearer-key protected.

== Installation ==

1. Upload `mddb-sync-<version>.zip` via Plugins → Add New → Upload Plugin.
2. Activate.
3. Go to Settings → MDDB Sync — fill in the MDDB URL and (optionally) the API key.
4. Save changes, then click "Probe /v1/search" to verify connectivity.

== Frequently Asked Questions ==

= Where do I get an MDDB instance? =

See https://github.com/tradik/mddb — the README walks through Docker, snap, and binary distributions.

= Does it work with Polylang / WPML? =

Yes — the language detector first asks Polylang, then WPML, then falls back to `get_locale()`. You can also force one of the three from the settings screen.

= How does updating work? =

The plugin queries GitHub Releases (`repos/tradik/mddb/releases/latest`) once every 12 h. When a newer release ships with a `mddb-sync-<version>.zip` asset, WordPress's standard Dashboard → Updates flow offers it.

== Changelog ==

= 0.2.0 =
* Remote publishing (opt-in): /wp-json/mddb-sync/v1/publish and /status endpoints for MDDB's MCP tools — create/update posts and pages with tags, categories, custom taxonomies, meta fields, scheduling and Polylang/WPML translations; change publishing status incl. trash/untrash.
* New settings: "Remote publishing (MCP)" toggle (off by default) and bearer publish key (auto-generated on first enable).
* Built-in HTML-escaping Markdown converter for contentMarkdown payloads; contentHtml is sanitised with wp_kses_post.
* PHP support window 8.2–8.5; security hardening for logs, release notes, update ZIP host and https-only endpoint URL.

= 0.1.1 =
* Mapper now exports every `get_post_meta()` key, every taxonomy attached to the post type, ACF `get_fields()` (namespaced `acf:*`), and normalised SEO fields from Yoast / RankMath / SEOPress.
* New `mddb_sync_meta` filter for downstream customisation.
* Term filter — per-taxonomy term-ID checklist on the settings screen scopes what gets synced (AND across taxonomies, OR inside one).
* "Sync everything" button — paged AJAX bulk re-sync with progress bar.

= 0.1.0 =
* Initial release. See CHANGELOG.md in the repository for full details.
