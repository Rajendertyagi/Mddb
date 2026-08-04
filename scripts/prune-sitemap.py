#!/usr/bin/env python3
"""Keep the generated sitemap to canonical, indexable URLs only.

A sitemap entry asks a crawler to index that exact URL. Two kinds of page
contradict the request and are dropped here:

  * `noindex` — the page says "do not index me" while the sitemap says the
    opposite. Google reports the pair as an error and spends crawl budget
    fetching a page it will discard.
  * non-self-canonical — the page names a *different* URL as canonical, so the
    listed one is by definition not the version to index.

The SSG lists every published page and cannot know either fact: both signals
come from the theme, emitted after the sitemap is written. So the pruning
happens here, once the output exists, and the sitemap entry is removed rather
than the tag — the tag is the deliberate signal.

On this site the affected page is `/index/`, which trips both rules.
`docs/index.md` renders the homepage *and* a near-empty duplicate at that URL,
and the file cannot be unpublished because that removes the homepage too.

Usage: python3 scripts/prune-sitemap.py [output_dir]
"""

from __future__ import annotations

import os
import re
import sys

SITE_ORIGIN = "https://mddb.tradik.com"
URL_BLOCK = re.compile(r"[ \t]*<url>.*?</url>\n?", re.S)
LOC = re.compile(r"<loc>(.*?)</loc>", re.S)
ROBOTS = re.compile(r'<meta name="robots" content="([^"]*)"')
CANONICAL = re.compile(r'<link rel="canonical" href="([^"]*)"')


def page_for(root: str, url: str) -> str | None:
    """The built file a sitemap URL refers to, or None if it is not a page."""
    path = url.replace(SITE_ORIGIN, "").strip("/")
    candidate = os.path.join(root, path, "index.html") if path else os.path.join(root, "index.html")
    return candidate if os.path.isfile(candidate) else None


def reason_to_drop(page: str, url: str) -> str | None:
    """Why this URL does not belong in the sitemap, or None if it does."""
    with open(page, encoding="utf-8", errors="ignore") as handle:
        body = handle.read()

    robots = ROBOTS.search(body)
    if robots and "noindex" in robots.group(1).lower():
        return "noindex"

    canonical = CANONICAL.search(body)
    if canonical and canonical.group(1).rstrip("/") != url.rstrip("/"):
        return f"canonical → {canonical.group(1)}"
    return None


def main() -> int:
    root = sys.argv[1] if len(sys.argv) > 1 else "dist"
    sitemap = os.path.join(root, "sitemap.xml")
    if not os.path.isfile(sitemap):
        print(f"error: {sitemap} not found — run 'make docs-build'")
        return 2

    with open(sitemap, encoding="utf-8") as handle:
        body = handle.read()

    dropped: list[tuple[str, str]] = []

    def prune(match: re.Match[str]) -> str:
        block = match.group(0)
        loc = LOC.search(block)
        if not loc:
            return block
        url = loc.group(1)
        page = page_for(root, url)
        if page is None:
            return block
        reason = reason_to_drop(page, url)
        if reason is None:
            return block
        dropped.append((url, reason))
        return ""

    pruned = URL_BLOCK.sub(prune, body)

    if not dropped:
        print(
            f"✅ sitemap.xml lists only canonical, indexable URLs ({body.count('<url>')} entries)"
        )
        return 0

    with open(sitemap, "w", encoding="utf-8") as handle:
        handle.write(pruned)
    print(f"🧹 removed {len(dropped)} entr(y/ies) from sitemap.xml:")
    for url, reason in dropped:
        print(f"  {url}  ({reason})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
