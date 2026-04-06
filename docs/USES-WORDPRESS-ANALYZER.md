---
title: "WordPress Website Analyzer"
slug: "docs/uses-wordpress-analyzer"
description: "WordPress Website Analyzer - MDDB use case guide"
status: publish
---

# WordPress Website Analyzer

Export content from a WordPress website into MDDB, then analyze it with Claude CLI — find broken links, missing meta tags, duplicate content, and other issues.

```
WordPress → wpexportjson → MDDB → Claude CLI (MCP) → analysis
```

This guide uses **www.malacukierenka.pl** as an example.

## Requirements

- [wpexportjson](https://github.com/tradik/wpexporter) — export WordPress content
- [MDDB](https://github.com/tradik/mddb) — document database
- [Claude CLI](https://docs.anthropic.com/en/docs/claude-code) — content analysis

---

## Step 1: Install wpexportjson

```bash
go install github.com/tradik/wpexporter/cmd/wpexportjson@latest
```

Verify the installation:

```bash
wpexportjson --help
```

If you don't have Go, download a pre-built binary from [releases](https://github.com/tradik/wpexporter/releases).

---

## Step 2: Start MDDB

```bash
docker run -d \
  --name mddb \
  -p 11023:11023 \
  -v mddb-data:/data \
  tradik/mddb:latest
```

Verify:

```bash
curl http://localhost:11023/v1/health
```

---

## Step 3: Export content from WordPress

### 3a. Export public content (no login required)

`wpexportjson` fetches content through the public WordPress REST API:

```bash
wpexportjson \
  --url https://www.malacukierenka.pl \
  --output ./export \
  --format markdown
```

This exports:
- Posts (blog articles)
- Static pages
- Categories and tags
- Authors

The `.md` files are saved to the `./export/` folder.

### 3b. Check what was downloaded

```bash
ls -la export/
ls export/posts/ | head -10
ls export/pages/ | head -10
```

Preview a file:

```bash
cat export/posts/$(ls export/posts/ | head -1) | head -30
```

Each file includes YAML frontmatter with metadata (title, date, categories, tags, author).

---

## Step 4: Load content into MDDB

### 4a. Load posts and pages

```bash
# Download the loader script (if you don't have the repository)
curl -sO https://raw.githubusercontent.com/tradik/mddb/main/scripts/load-md-folder.sh
chmod +x load-md-folder.sh

# Load posts
./load-md-folder.sh export/posts/ malacukierenka-posts --lang pl_PL --verbose

# Load pages
./load-md-folder.sh export/pages/ malacukierenka-pages --lang pl_PL --verbose
```

### 4b. Alternative — use the CLI

```bash
for md_file in export/posts/*.md; do
  key=$(basename "$md_file" .md)
  mddb-cli add malacukierenka-posts "$key" pl -f "$md_file" \
    -m "source=wordpress,site=malacukierenka.pl,type=post"
done

for md_file in export/pages/*.md; do
  key=$(basename "$md_file" .md)
  mddb-cli add malacukierenka-pages "$key" pl -f "$md_file" \
    -m "source=wordpress,site=malacukierenka.pl,type=page"
done
```

### 4c. Verify the result

```bash
curl -s http://localhost:11023/v1/stats | python3 -m json.tool
```

You should see `malacukierenka-posts` and `malacukierenka-pages` collections with documents.

---

## Step 5: Configure Claude CLI (MCP)

Create the MCP config file:

```bash
# macOS
cat > ~/Library/Application\ Support/Claude/claude_desktop_config.json << 'EOF'
{
  "mcpServers": {
    "mddb": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm", "--network", "host",
        "-e", "MDDB_MCP_STDIO=true",
        "-e", "MDDB_SERVER=http://localhost:11023",
        "tradik/mddb:latest"
      ]
    }
  }
}
EOF
```

---

## Step 6: Analyze content

Launch Claude CLI:

```bash
claude
```

### Link analysis

```
> Search all posts in the malacukierenka-posts collection.
  Find all external links (http/https).
  List those that might be outdated or broken.
```

### SEO analysis

```
> Analyze posts from malacukierenka-posts for SEO:
  - Do posts have meta descriptions?
  - Are titles the right length (50-60 characters)?
  - Do images have alt text?
  List issues sorted by severity.
```

### Duplicate content

```
> Check the malacukierenka-posts collection.
  Find posts with very similar content or the same topic.
  List potential duplicates with similarity scores.
```

### Category analysis

```
> Analyze all posts from malacukierenka-posts.
  What categories and tags are used?
  Are there posts without categories?
  Are there categories with only 1 post (possibly redundant)?
```

### Outdated content

```
> Find posts in malacukierenka-posts that:
  - Are older than 2 years
  - Reference dates, events, or prices
  - May need updating
  Provide specific recommendations on what to update.
```

### Content quality

```
> Analyze the 10 latest posts from malacukierenka-posts.
  Rate each on:
  - Length (short/medium/long)
  - Readability
  - Formatting (headings, lists, images)
  Provide recommendations for improvement.
```

### Pages vs posts comparison

```
> Compare the content of pages (malacukierenka-pages) with posts (malacukierenka-posts).
  Are static pages up to date?
  Is information on pages consistent with post content?
```

---

## Step 7 (optional): Semantic search

For better analysis, enable embeddings:

```bash
# Install Ollama and pull the model
ollama pull nomic-embed-text

# Restart MDDB with embeddings
docker stop mddb && docker rm mddb
docker run -d \
  --name mddb \
  -p 11023:11023 \
  -v mddb-data:/data \
  -e MDDB_EMBEDDING_PROVIDER=ollama \
  -e MDDB_EMBEDDING_API_URL=http://host.docker.internal:11434 \
  -e MDDB_EMBEDDING_MODEL=nomic-embed-text \
  -e MDDB_EMBEDDING_DIMENSIONS=768 \
  --add-host=host.docker.internal:host-gateway \
  tradik/mddb:latest

# Reindex
curl -X POST "http://localhost:11023/v1/vector-reindex?collection=malacukierenka-posts"
curl -X POST "http://localhost:11023/v1/vector-reindex?collection=malacukierenka-pages"
```

Now Claude CLI can find semantically related content — e.g. "posts about baking cakes" without requiring exact keyword matches.

---

## Summary

| Step | Command | What it does |
|------|---------|--------------|
| 1 | `go install .../wpexportjson` | Install the exporter |
| 2 | `docker run tradik/mddb` | Start the database |
| 3 | `wpexportjson --url ... --format markdown` | Export WP to Markdown |
| 4 | `load-md-folder.sh export/ collection` | Load into MDDB |
| 5 | `claude_desktop_config.json` | Configure MCP |
| 6 | `claude` | Analyze content |
| 7 | `ollama pull nomic-embed-text` | Add semantic search |

## Other WordPress sites

The same process works with any WordPress site that has a public REST API. Change the URL in step 3:

```bash
wpexportjson --url https://your-site.com --output ./export --format markdown
```

Example sites you can analyze:
- Company blog — communication consistency analysis
- WooCommerce store — product description analysis
- News portal — article quality analysis
- Portfolio — project presentation analysis
