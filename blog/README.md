# MDDB Blog

Release announcements and engineering notes, published through the same
pipeline as the docs: each post is a Markdown file with YAML frontmatter,
ingested into the `blog` collection and rendered by the SSG.

## Conventions

- **Filename:** `YYYY-MM-DD-short-title.md` (date first — keeps the folder
  chronologically sorted).
- **Frontmatter:** same fields as `docs/*.md`:

```yaml
---
title: "Post title"
slug: "blog/short-title"
description: "One-sentence summary shown in listings and meta tags."
status: publish
---
```

- **Slug prefix** `blog/` puts the post in the blog section of the generated
  site. Use `status: draft` to keep a post out of publication.
- Diagrams: ` ```mermaid ` fences render on GitHub and in the site's
  Markdown viewer.
- Tone: write for a human who is deciding whether to upgrade — lead with what
  changed for *them*, keep API details in the docs and link out.
- Never mention third-party database products by name — describe our features
  on their own merits.

## Publishing

Posts are loaded like any other Markdown folder:

```bash
scripts/load-md-folder.sh --collection blog blog/
```
