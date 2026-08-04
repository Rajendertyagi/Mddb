# MDDB Scripts

This directory contains utility scripts for MDDB operations.

## Available Scripts

### load-md-folder.sh

Bulk import markdown files from a folder into MDDB database.

**Features:**
- Automatic key generation from filenames
- YAML frontmatter metadata extraction
- Recursive folder scanning
- Progress tracking with statistics
- Dry run mode for preview
- Custom metadata support
- Multi-language support

**Usage:**
```bash
./scripts/load-md-folder.sh <folder_path> <collection> [options]
```

**Examples:**
```bash
# Basic import
./scripts/load-md-folder.sh ./docs blog

# Recursive import with custom language
./scripts/load-md-folder.sh ./content articles -r -l pl_PL

# Add custom metadata
./scripts/load-md-folder.sh ./posts blog -m "author=John" -m "status=published"

# Dry run (preview only)
./scripts/load-md-folder.sh ./docs blog -d

# Verbose output
./scripts/load-md-folder.sh ./docs blog -v
```

**Options:**
- `-l, --lang LANG` - Language code (default: en_US)
- `-r, --recursive` - Process subfolders recursively
- `-m, --meta KEY=VALUE` - Add metadata (can be used multiple times)
- `-s, --server URL` - MDDB server URL
- `-v, --verbose` - Verbose output
- `-d, --dry-run` - Preview without executing
- `-b, --batch-size N` - Progress update frequency
- `-h, --help` - Show help message

**Documentation:**
See [BULK-IMPORT.md](../docs/BULK-IMPORT.md) for detailed documentation.

### check-docs-links.py

Fails the documentation build on defects a crawl of mddb.tradik.com would
report. Checks every `href`, `src` and `og:image`/`twitter:image` for links
that would return 404 **or 308**, resolving both site-relative links and
absolute links back to the canonical domain against the build output. Also
reports `<img>` elements with **no `alt` attribute**, and indexable pages with
**no meta description**, **no `<title>`**, or **no inbound link** (orphans).
Titles over 60 characters are printed as a warning without failing the build.
External hosts are never fetched, so the check is offline and deterministic —
it gates the `deploy-docs` workflow before the Cloudflare Pages upload.

Pages marked `noindex` are exempt from the description, title and orphan
checks: they never appear in results, so none of it buys them anything.
`404.html` and the taxonomy pages are the cases this matters for.

Orphan detection counts only `<a href>` from *other* pages. Counting every
`href` looks equivalent and is not: `<link rel="canonical">` points each page
at itself, so every page would appear linked and the check would never fire.

On `alt`: a *missing* attribute is the defect, `alt=""` is not. An empty `alt`
is the correct WCAG treatment for a decorative image — the site logo sits
inside a link that already carries the text "MDDB", so describing it again
would make a screen reader announce the same thing twice. Flagging `alt=""`
would push authors toward exactly that.

Resolution models **Cloudflare Pages routing**, not the output directory:
Pages strips `.html` and appends a missing trailing slash on a directory,
answering the original URL with a 308 each time. So `/docs/api/swagger` is
valid even though the file on disk is `swagger.html`, while linking
`/docs/api/swagger.html` or `/docs/config` is reported with the final URL to
use instead. Redirecting links still work for visitors, but they cost a round
trip and a hop of crawl budget, so they are treated as failures.

**Usage:**
```bash
python3 scripts/check-docs-links.py [output_dir]   # default: dist
make docs-linkcheck                                # build, then check
```

Exit codes: `0` clean, `1` broken links found (each printed with the pages
linking to it), `2` output directory missing.

## Using with Makefile

```bash
# Import folder
make import-folder FOLDER=./docs COLLECTION=blog

# Preview import
make import-folder-dry FOLDER=./docs COLLECTION=blog

# Recursive import
make import-folder-recursive FOLDER=./docs COLLECTION=blog

# With custom options
make import-folder FOLDER=./docs COLLECTION=blog LANG=pl_PL META="author=John"
```

## Requirements

- Bash shell
- `mddb-cli` command available in PATH
- Running MDDB server

## Environment Variables

- `MDDB_SERVER` - Server URL (default: http://localhost:11023)
- `MDDB_CLI` - CLI command path (default: mddb-cli)

## See Also

- [Bulk Import Documentation](../docs/BULK-IMPORT.md)
- [Examples](../examples/)
- [API Documentation](../docs/API.md)
