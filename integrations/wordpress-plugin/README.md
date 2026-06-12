# MDDB Sync — WordPress plugin

[![PHP](https://img.shields.io/badge/PHP-%3E%3D8.1-777BB4?logo=php&logoColor=white)](https://www.php.net)
[![WordPress](https://img.shields.io/badge/WordPress-%3E%3D6.2-21759B?logo=wordpress&logoColor=white)](https://wordpress.org)
[![Coverage](https://img.shields.io/badge/coverage-%E2%89%A590%25-brightgreen)](#testing)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](#license)

Synchronises WordPress **posts** and **pages** (and any other public post type) to an [MDDB](https://github.com/tradik/mddb) instance.

- **On save / publish** → `POST /v1/add` — upsert by stable key.
- **On trash / delete** → `POST /v1/delete` — remove the matching MDDB document.
- **Language detection** → reads from [Polylang](https://wordpress.org/plugins/polylang/), [WPML](https://wpml.org/), or falls back to the site locale.
- **Self-updates** from this repository's GitHub Releases (no wp.org slug required).

The plugin is a thin, security-first PHP shim: settings UI, retry-aware HTTP client, hook wiring. No background queue, no JavaScript build step, no dashboard widgets.

---

## Install

### From a release zip (recommended)

1. Download `mddb-sync-<version>.zip` from the latest [GitHub Release](https://github.com/tradik/mddb/releases) (tag starts with `wp-v`).
2. In wp-admin: **Plugins → Add New → Upload Plugin** → choose the zip → **Install Now** → **Activate**.
3. Configure under **Settings → MDDB Sync**.

### From source (development)

```bash
cd integrations/wordpress-plugin
composer install
# Then symlink or copy the folder into wp-content/plugins/mddb-sync.
ln -s "$(pwd)" /path/to/wordpress/wp-content/plugins/mddb-sync
```

---

## Configuration

Settings live under **Settings → MDDB Sync** (or `wp-admin/options-general.php?page=mddb-sync`).

| Setting | Default | Notes |
|---|---|---|
| **MDDB URL** | _empty_ | Base URL, e.g. `https://mddb.example.com`. **Must be `https://`** so the API key is never sent in cleartext; `http://` is accepted only for local hosts (`localhost`, `127.0.0.1`, `::1`). Trailing slash is stripped. Plugin idles until this is set. |
| **API key** | _empty_ | Sent as `Authorization: Bearer <key>`. Leave empty for unauthenticated dev instances. |
| **Collection** | derived from site host (`example_com`) | One MDDB collection holds every synced post for this site. |
| **Sync on save / publish** | ✓ | Fires on `wp_after_insert_post`. Autosaves and revisions are ignored. |
| **Clean entries on trash / delete** | ✓ | Fires on `wp_trash_post` and `before_delete_post`. |
| **Include drafts** | ✗ | When off, only `publish` status syncs. |
| **Post types** | `post`, `page` | Any public post type can be toggled. |
| **Language detection** | Auto | Auto = Polylang → WPML → site locale. Or force one of the three. |
| **Key strategy** | `posttype-id` | Other options: `posttype-slug` or `permalink path`. |

The **Test connection** button under the form calls `POST /v1/search` against your MDDB and reports the HTTP status.

### MDDB document shape

```json
{
  "collection": "example_com",
  "key": "post-42",
  "lang": "en_US",
  "meta": {
    "postType":   ["post"],
    "status":     ["publish"],
    "title":      ["Hello world"],
    "slug":       ["hello-world"],
    "permalink":  ["https://example.com/hello-world/"],
    "authorId":   ["3"],
    "author":     ["Jane Author"],
    "publishedAt": ["2026-05-19T10:14:00+00:00"],
    "modifiedAt":  ["2026-05-19T10:14:00+00:00"],
    "categories": ["News"],
    "tags":       ["intro", "demo"]
  },
  "contentMd": "# Hello world\n\nHello world.\n"
}
```

`contentMd` runs the post body through the standard `the_content` filter, then strips tags — Shortcodes, blocks, oEmbed etc. all get expanded first so the indexed text matches the rendered page.

---

## Auto-updates

The plugin hooks into WordPress's own update mechanism by filtering `pre_set_site_transient_update_plugins` and `plugins_api`. It polls `https://api.github.com/repos/tradik/mddb/releases/latest` once every 12h (cached in a site transient) and looks for an asset named `mddb-sync-<version>.zip`. When the release tag is newer than the installed version, **Dashboard → Updates** shows the upgrade just like any wp.org plugin.

Releases are produced by the [`WordPress Plugin (mddb-sync)`](../../.github/workflows/wordpress-plugin.yml) workflow when a `wp-v*` tag is pushed (e.g. `wp-v0.2.0`).

---

## Development

```bash
make install     # composer install
make lint        # phpcs (WordPress security ruleset)
make stan        # phpstan level 5, WordPress-aware
make test        # phpunit
make coverage    # phpunit + xdebug coverage (text + clover.xml)
make audit       # composer audit (security advisories)
make ci          # audit → lint → stan → coverage
make build       # produce build/mddb-sync-<version>.zip (mirrors the GH Action)
```

### Layout

```
integrations/wordpress-plugin/
├── mddb-sync.php            # plugin header + autoloader bootstrap
├── includes/
│   ├── class-plugin.php     # orchestrator
│   ├── class-settings.php   # typed access to the option array
│   ├── class-admin.php      # Settings → MDDB Sync screen + AJAX probe
│   ├── class-client.php     # POST /v1/add, /v1/delete, /v1/search (probe)
│   ├── class-language.php   # Polylang / WPML / locale detection
│   ├── class-mapper.php     # WP_Post → MDDB document
│   ├── class-sync.php       # wp_after_insert_post / before_delete_post / wp_trash_post
│   └── class-updater.php    # GitHub Releases update channel
├── tests/                   # PHPUnit + Brain Monkey
├── composer.json
├── phpcs.xml                # WordPress security + DB + i18n + AlternativeFunctions
├── phpstan.neon.dist        # level 5 + szepeviktor/phpstan-wordpress
├── phpunit.xml.dist
└── Makefile
```

### Testing

48 tests, 92 %+ line coverage. The suite mocks every WordPress function via [Brain Monkey](https://github.com/Brain-WP/BrainMonkey) — no WordPress install needed.

CI matrix runs against PHP 8.1, 8.2, 8.3, 8.4. Coverage is enforced ≥90 % on the 8.3 leg.

---

## Hooks the plugin uses

| Type | Hook | What |
|---|---|---|
| Action | `wp_after_insert_post` | Build a document and call `/v1/add`. |
| Action | `before_delete_post` | Call `/v1/delete`. |
| Action | `wp_trash_post` | Same as delete — trash is treated as removal. |
| Filter | `pre_set_site_transient_update_plugins` | Inject "update available" payload from GitHub. |
| Filter | `plugins_api` | Provide plugin information for the "View details" popup. |
| Action | `mddb_sync_error` (emitted) | Triggered with `($tag, WP_Error, $postId)` on every sync failure — wire your own Sentry/Action Scheduler glue here. |

---

## License

BSD-3-Clause — same as the rest of the MDDB monorepo.
