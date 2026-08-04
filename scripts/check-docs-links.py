#!/usr/bin/env python3
"""Fail the docs build on defects a crawler would report against the live site.

Every finding is something an SEO or accessibility crawl of mddb.tradik.com
would flag:

  1. Links that 404 — site-relative (`/docs/tls/`, `../images/logo.svg`) and
     absolute links to the canonical domain alike, resolved against the output.
  2. Links that 308 — see `classify`, which models Cloudflare Pages routing.
  3. `<img>` elements with no `alt` attribute at all.
  4. Indexable pages with no meta description, no `<title>`, or no inbound link.

Two deliberate non-findings:

  * `alt=""` is correct for a decorative image. Flagging it would push authors
    toward inventing descriptions that make screen readers announce the same
    thing twice. Only the absent attribute leaves a real gap.
  * A title over TITLE_LIMIT is a warning, never an error — that is where
    results truncate the display, not a defect. A headline that reads well at
    62 characters beats one mangled to fit.

`noindex` pages are exempt from the class-4 checks: they never appear in
results, so a description, a title or an inbound link buys them nothing.

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
    """Collects href/src values and images that declare no alt attribute.

    Markup shown inside <code>/<pre> arrives escaped in the generated HTML, so
    documented examples are text to the parser and never counted as real
    elements.
    """

    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.links: list[str] = []
        self.images_without_alt: list[str] = []
        self.anchors: list[str] = []
        self.description: str | None = None
        self.robots: str = ""
        self.title: str = ""
        self._in_title = False

    def handle_endtag(self, tag: str) -> None:
        if tag == "title":
            self._in_title = False

    def handle_data(self, data: str) -> None:
        if self._in_title:
            self.title += data

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        if tag == "title":
            self._in_title = True
        attr = dict(attrs)
        if tag == "img" and "alt" not in attr:
            self.images_without_alt.append(attr.get("src") or "(no src)")
        if tag == "meta":
            if attr.get("name") == "description":
                self.description = attr.get("content") or ""
            elif attr.get("name") == "robots":
                self.robots = attr.get("content") or ""
        # Only <a href> counts as navigation. <link rel="canonical"> points a
        # page at itself, so counting it would make every page look linked and
        # the orphan check would never fire.
        if tag == "a" and attr.get("href"):
            self.anchors.append(attr["href"])
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


def served_file(root: str, page: str, url: str) -> str | None:
    """The file a request for this URL actually returns, or None if nothing does.

    Mirrors `classify`'s lookup order so the two never disagree about which
    file a link reaches.
    """
    target = resolve(root, page, url)
    for candidate in (target, os.path.join(target, "index.html"), target + ".html"):
        if os.path.isfile(candidate):
            return candidate
    return None


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


def scan_page(page: str) -> LinkCollector:
    """Parse one built page.

    HTMLParser handles href/src, meta tags and images; og:image and
    twitter:image live in a `content` attribute that is only a URL for those
    specific meta tags, so they are appended separately rather than by treating
    every `content` as a link.
    """
    with open(page, encoding="utf-8", errors="ignore") as handle:
        body = handle.read()
    collector = LinkCollector()
    collector.feed(body)
    collector.links += META_IMAGE.findall(body)
    return collector


class Findings:
    """Everything one run turned up, keyed by offender → the pages involved."""

    def __init__(self) -> None:
        self.missing: dict[str, set[str]] = defaultdict(set)
        self.redirects: dict[str, tuple[str, set[str]]] = {}
        self.no_alt: dict[str, set[str]] = defaultdict(set)
        self.no_description: set[str] = set()
        self.orphans: set[str] = set()
        self.no_title: set[str] = set()
        self.long_titles: list[tuple[str, int, str]] = []

    def __bool__(self) -> bool:
        """True when something should fail the build.

        `long_titles` is deliberately excluded: TITLE_LIMIT is where search
        engines truncate the *display*, not a defect. A headline that reads
        well at 62 characters is worth more than one mangled to fit, so it is
        reported and left to the author rather than blocking a deploy.
        """
        return bool(
            self.missing
            or self.redirects
            or self.no_alt
            or self.no_description
            or self.orphans
            or self.no_title
        )


# The site root is everyone's entry point, so nothing has to link to it.
ENTRY_POINTS = ("index.html",)

# Roughly where search results truncate a title.
TITLE_LIMIT = 60


def check(root: str) -> Findings:
    found = Findings()
    indexable_pages: set[str] = set()
    linked: set[str] = set()

    for page in iter_pages(root):
        source = os.path.relpath(page, root)
        parsed = scan_page(page)

        for src in parsed.images_without_alt:
            found.no_alt[src].add(source)

        # A noindex page is never shown in results, so neither a description
        # nor an inbound link buys it anything — 404.html and the taxonomy
        # pages are the cases that matter here.
        indexable = "noindex" not in parsed.robots.lower()
        if indexable:
            indexable_pages.add(source)
            if not (parsed.description or "").strip():
                found.no_description.add(source)
            title = " ".join(parsed.title.split())
            if not title:
                found.no_title.add(source)
            elif len(title) > TITLE_LIMIT:
                found.long_titles.append((source, len(title), title))

        for raw in parsed.anchors:
            url = normalise(raw)
            if url is None:
                continue
            hit = served_file(root, page, url)
            # A page linking to itself does not make it discoverable.
            if hit is not None and os.path.relpath(hit, root) != source:
                linked.add(os.path.relpath(hit, root))

        for raw in parsed.links:
            url = normalise(raw)
            if url is None:
                continue
            verdict, detail = classify(root, page, url)
            if verdict == "missing":
                found.missing[raw].add(source)
            elif verdict == "redirect":
                found.redirects.setdefault(raw, (detail, set()))[1].add(source)

    # An indexable page nothing links to is reachable only from the sitemap.
    # Crawlers deprioritise it and readers never find it at all.
    found.orphans = indexable_pages - linked - set(ENTRY_POINTS)
    return found


def report(title: str, entries: dict[str, set[str]], relation: str = "linked from") -> None:
    print(f"{title}\n")
    for url in sorted(entries):
        sources = sorted(entries[url])
        shown = ", ".join(sources[:5])
        more = f" (+{len(sources) - 5} more)" if len(sources) > 5 else ""
        print(f"  {url}\n      {relation}: {shown}{more}")
    print()


def main() -> int:
    root = sys.argv[1] if len(sys.argv) > 1 else "dist"
    if not os.path.isdir(root):
        print(f"error: output directory {root!r} does not exist — run 'make docs-build'")
        return 2

    found = check(root)
    total = sum(1 for _ in iter_pages(root))

    if found.long_titles:
        print(
            f"⚠️  {len(found.long_titles)} title(s) over {TITLE_LIMIT} characters — "
            "search results will truncate them:\n"
        )
        for page, length, title in sorted(found.long_titles):
            print(f"  {length:>3}  {title}\n       {page}")
        print()

    if not found:
        print(
            f"✅ {total} pages in {root}/: no broken or redirecting links, every image has "
            "alt, every indexable page has a description and at least one inbound link"
        )
        return 0

    if found.missing:
        report(f"❌ {len(found.missing)} internal link target(s) that 404 in {root}/:", found.missing)
    if found.redirects:
        report(
            f"❌ {len(found.redirects)} internal link(s) that 308 — link the final URL instead:",
            {f"{url}  →  {final}": pages for url, (final, pages) in found.redirects.items()},
        )
    if found.no_alt:
        report(
            f"❌ {len(found.no_alt)} image(s) with no alt attribute "
            '(use alt="" only if the image is decorative):',
            found.no_alt,
            relation="on",
        )
    if found.no_description:
        print(
            f"❌ {len(found.no_description)} indexable page(s) with no meta description "
            "(set `description:` in the front matter):\n"
        )
        for page in sorted(found.no_description):
            print(f"  {page}")
        print()
    if found.orphans:
        print(
            f"❌ {len(found.orphans)} orphan page(s) — indexable but nothing links to them, "
            "so they are reachable only from the sitemap:\n"
        )
        for page in sorted(found.orphans):
            print(f"  {page}")
        print()
    if found.no_title:
        print(f"❌ {len(found.no_title)} indexable page(s) with no <title>:\n")
        for page in sorted(found.no_title):
            print(f"  {page}")
        print()
    return 1


if __name__ == "__main__":
    sys.exit(main())
