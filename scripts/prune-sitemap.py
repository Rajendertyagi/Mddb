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
# `[^<]*` rather than `.*?`: neither a URL nor these tag bodies contain `<`,
# and the lazy dotall form backtracks super-linearly on malformed input.
LOC = re.compile(r"<loc>([^<]*)</loc>")
ROBOTS = re.compile(r'<meta name="robots" content="([^"]*)"')
CANONICAL = re.compile(r'<link rel="canonical" href="([^"]*)"')


def url_blocks(body: str):
    """Yield (start, end) of each <url>…</url> block, in document order.

    Scanned with str.find rather than a regex: matching across the block needs
    a dotall lazy quantifier, which backtracks super-linearly when a closing
    tag is missing.
    """
    pos = 0
    while True:
        start = body.find("<url>", pos)
        if start == -1:
            return
        end = body.find("</url>", start)
        if end == -1:
            return
        end += len("</url>")
        # Swallow the trailing newline and the block's own indentation so
        # removing an entry does not leave a blank line behind.
        line_start = body.rfind("\n", 0, start) + 1
        if body[line_start:start].strip() == "":
            start = line_start
        if body[end : end + 1] == "\n":
            end += 1
        yield start, end
        pos = end


def within_root(root: str, path: str) -> bool:
    """True when `path` stays inside the output directory."""
    root_real = os.path.realpath(root)
    target = os.path.realpath(path)
    return target == root_real or target.startswith(root_real + os.sep)


def read_within(root: str, path: str) -> str:
    """Read a file, refusing anything that resolves outside the output tree.

    Both the output directory and the sitemap `<loc>` values are untrusted
    input, so containment is checked immediately before the read rather than
    being assumed from how the path was built.
    """
    if not within_root(root, path):
        raise ValueError(f"refusing to read outside {root!r}: {path!r}")
    with open(path, encoding="utf-8", errors="ignore") as handle:
        return handle.read()


def page_for(root: str, url: str) -> str | None:
    """The built file a sitemap URL refers to, or None if it is not a page.

    A sitemap `<loc>` is text, and the output directory is an argument, so the
    joined path is confined to the tree before it is opened.
    """
    path = url.replace(SITE_ORIGIN, "").strip("/")
    candidate = os.path.join(root, path, "index.html") if path else os.path.join(root, "index.html")
    if not within_root(root, candidate):
        return None
    return candidate if os.path.isfile(candidate) else None


def reason_to_drop(root: str, page: str, url: str) -> str | None:
    """Why this URL does not belong in the sitemap, or None if it does."""
    body = read_within(root, page)

    robots = ROBOTS.search(body)
    if robots and "noindex" in robots.group(1).lower():
        return "noindex"

    canonical = CANONICAL.search(body)
    if canonical and canonical.group(1).rstrip("/") != url.rstrip("/"):
        return f"canonical → {canonical.group(1)}"
    return None


def main() -> int:
    # Resolve once so every derived path is compared against a real, absolute
    # root rather than whatever shape the argument arrived in.
    root = os.path.realpath(sys.argv[1] if len(sys.argv) > 1 else "dist")
    sitemap = os.path.join(root, "sitemap.xml")
    if not os.path.isfile(sitemap):
        print(f"error: {sitemap} not found — run 'make docs-build'")
        return 2

    body = read_within(root, sitemap)

    dropped: list[tuple[str, str]] = []
    keep: list[str] = []
    cursor = 0

    for start, end in url_blocks(body):
        loc = LOC.search(body[start:end])
        reason = None
        if loc:
            page = page_for(root, loc.group(1).strip())
            if page is not None:
                reason = reason_to_drop(root, page, loc.group(1).strip())
        if reason is None:
            continue
        keep.append(body[cursor:start])
        cursor = end
        dropped.append((loc.group(1).strip(), reason))

    keep.append(body[cursor:])
    pruned = "".join(keep)

    if not dropped:
        print(
            f"✅ sitemap.xml lists only canonical, indexable URLs ({body.count('<url>')} entries)"
        )
        return 0

    if not within_root(root, sitemap):
        raise ValueError(f"refusing to write outside {root!r}: {sitemap!r}")
    with open(sitemap, "w", encoding="utf-8") as handle:
        handle.write(pruned)
    print(f"🧹 removed {len(dropped)} entr(y/ies) from sitemap.xml:")
    for url, reason in dropped:
        print(f"  {url}  ({reason})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
