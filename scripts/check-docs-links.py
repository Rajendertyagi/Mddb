#!/usr/bin/env python3
"""Fail the docs build when the generated site links to something that 404s.

Every finding here is a URL a crawler would hit on mddb.tradik.com and get a
404 back. Two classes are checked:

  1. Site-relative links (`/docs/tls/`, `../images/logo.svg`) — resolved against
     the output directory. Directory URLs are satisfied by an `index.html`.
  2. Absolute links to the canonical domain — these are internal links written
     the long way, so they are resolved the same as class 1.

External hosts are never fetched: the check must stay offline and
deterministic so it can gate the deploy workflow.

Usage: python3 scripts/check-docs-links.py [output_dir]
"""

from __future__ import annotations

import os
import re
import sys
from collections import defaultdict
from html.parser import HTMLParser

# Canonical site origin. Absolute links to it are internal and must resolve
# locally, exactly like a root-relative link would.
SITE_ORIGINS = ("https://mddb.tradik.com", "http://mddb.tradik.com")

# Schemes and prefixes that never point at a file in the output directory.
SKIP_PREFIXES = (
    "http://",
    "https://",
    "mailto:",
    "tel:",
    "data:",
    "javascript:",
    "#",
    "//",
)

# Cloudflare injects /cdn-cgi/ endpoints (email obfuscation, RUM beacon) at the
# edge. They are not build artefacts and are served by Cloudflare, not Pages.
SKIP_PATHS = ("/cdn-cgi/",)

# Attributes that carry a URL a crawler will follow or fetch.
URL_ATTRS = ("href", "src")


class LinkCollector(HTMLParser):
    """Collects href/src values, ignoring markup inside <code>/<pre>."""

    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.links: list[str] = []

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        for name, value in attrs:
            if name in URL_ATTRS and value:
                self.links.append(value)


def normalise(url: str) -> str | None:
    """Reduce a raw attribute value to a checkable path, or None to skip it."""
    url = url.strip()
    for origin in SITE_ORIGINS:
        if url.startswith(origin):
            url = url[len(origin) :] or "/"
            break
    else:
        if url.startswith(SKIP_PREFIXES):
            return None
    url = url.split("#", 1)[0].split("?", 1)[0]
    if not url or url.startswith(SKIP_PATHS):
        return None
    return url


def resolve(root: str, page: str, url: str) -> str:
    """Map a link to the filesystem path it would be served from."""
    if url.startswith("/"):
        return os.path.join(root, url.lstrip("/"))
    return os.path.normpath(os.path.join(os.path.dirname(page), url))


def classify(root: str, page: str, url: str) -> tuple[str, str]:
    """Return (verdict, detail) for one link, mirroring Cloudflare Pages routing.

    Pages does not serve the output directory literally. It strips `.html` and
    appends the missing trailing slash on a directory, answering the original
    URL with a 308 in both cases. A link that lands on a redirect still works,
    but it burns a round trip for the visitor and a hop of crawl budget, so it
    is reported separately from an outright 404.
    """
    target = resolve(root, page, url)

    if url.endswith(".html") and os.path.isfile(target):
        stripped = url[: -len(".html")]
        # Pages serves the directory itself for an index page.
        if os.path.basename(url) == "index.html":
            stripped = url[: -len("index.html")] or "/"
        return "redirect", stripped
    if os.path.isfile(target):
        return "ok", ""
    if os.path.isfile(os.path.join(target, "index.html")):
        return ("ok", "") if url.endswith("/") else ("redirect", url + "/")
    # Extensionless URL for a page that exists on disk as <name>.html.
    if os.path.isfile(target + ".html"):
        return "ok", ""
    return "missing", ""


def iter_pages(root: str):
    for dirpath, _dirnames, filenames in os.walk(root):
        for name in filenames:
            if name.endswith(".html"):
                yield os.path.join(dirpath, name)


META_IMAGE = re.compile(
    r'<meta[^>]+(?:property|name)="(?:og:image|twitter:image)"[^>]+content="([^"]+)"'
)


def page_links(page: str) -> list[str]:
    """Every URL a crawler would follow from this page.

    HTMLParser handles href/src; og:image and twitter:image live in a `content`
    attribute that is only a URL for these specific meta tags, so they are
    matched separately rather than by treating every `content` as a link.
    """
    with open(page, encoding="utf-8", errors="ignore") as handle:
        body = handle.read()
    collector = LinkCollector()
    collector.feed(body)
    return collector.links + META_IMAGE.findall(body)


def check(root: str) -> tuple[dict[str, set[str]], dict[str, tuple[str, set[str]]]]:
    """Return ({missing url: pages}, {redirecting url: (final url, pages)})."""
    missing: dict[str, set[str]] = defaultdict(set)
    redirects: dict[str, tuple[str, set[str]]] = {}
    for page in iter_pages(root):
        source = os.path.relpath(page, root)
        for raw in page_links(page):
            url = normalise(raw)
            if url is None:
                continue
            verdict, detail = classify(root, page, url)
            if verdict == "missing":
                missing[raw].add(source)
            elif verdict == "redirect":
                redirects.setdefault(raw, (detail, set()))[1].add(source)
    return missing, redirects


def report(title: str, entries: dict[str, set[str]]) -> None:
    print(f"{title}\n")
    for url in sorted(entries):
        sources = sorted(entries[url])
        shown = ", ".join(sources[:5])
        more = f" (+{len(sources) - 5} more)" if len(sources) > 5 else ""
        print(f"  {url}\n      linked from: {shown}{more}")
    print()


def main() -> int:
    root = sys.argv[1] if len(sys.argv) > 1 else "dist"
    if not os.path.isdir(root):
        print(f"error: output directory {root!r} does not exist — run 'make docs-build'")
        return 2

    missing, redirects = check(root)
    total = sum(1 for _ in iter_pages(root))

    if not missing and not redirects:
        print(f"✅ no broken or redirecting internal links across {total} pages in {root}/")
        return 0

    if missing:
        report(f"❌ {len(missing)} internal link target(s) that 404 in {root}/:", missing)
    if redirects:
        report(
            f"❌ {len(redirects)} internal link(s) that 308 — link the final URL instead:",
            {f"{url}  →  {final}": pages for url, (final, pages) in redirects.items()},
        )
    return 1


if __name__ == "__main__":
    sys.exit(main())
