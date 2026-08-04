"""Shared helpers for the scripts that inspect the built documentation site.

`check-docs-links.py` reports defects in the output and `prune-sitemap.py`
edits one file in it. Both walk the same tree, read the same tags out of the
same generated HTML, and take the output directory as an argument — so the
path handling and the tag patterns live here rather than in each script.

Nothing in this module touches the network: the scripts gate a deploy and must
stay offline and deterministic.
"""

from __future__ import annotations

from pathlib import Path

# Canonical site origin. Absolute links to it are internal, so they resolve
# against the output directory exactly like a root-relative link would.
SITE_ORIGIN = "https://mddb.tradik.com"
# The plaintext variant exists only to recognise a self-link written with the
# wrong scheme, so that it is still resolved against the output instead of
# being skipped as external. It is a string to match, never an address to
# fetch — nothing in these scripts makes network calls. Derived from
# SITE_ORIGIN rather than written out, so the two cannot drift apart.
SITE_ORIGINS = (SITE_ORIGIN, SITE_ORIGIN.replace("https://", "http://", 1))

# `[^<]*` and `[^"]*` rather than a lazy `.*?`: none of these values contain
# the delimiter that ends them, and the lazy dotall form backtracks
# super-linearly on malformed input.
LOC_PATTERN = r"<loc>([^<]*)</loc>"
ROBOTS_PATTERN = r'<meta name="robots" content="([^"]*)"'
CANONICAL_PATTERN = r'<link rel="canonical" href="([^"]*)"'


def resolve_within(root: str, path: str) -> Path:
    """Resolve `path` and prove it sits inside `root`, or raise.

    `Path.relative_to` raises when the target escapes, and the *returned*
    object is the resolved, verified path — callers open that rather than the
    string they came in with, so nothing untrusted reaches the filesystem.
    Links are arbitrary text from a built page and the output directory is a
    CLI argument, so neither is assumed to stay in the tree.
    """
    target = Path(path).resolve()
    target.relative_to(Path(root).resolve())
    return target


def within_root(root: str, path: str) -> bool:
    """True when `path` stays inside the output directory."""
    try:
        resolve_within(root, path)
    except ValueError:
        return False
    return True


def read_within(root: str, path: str) -> str:
    """Read a file that is proven to live inside the output directory."""
    return resolve_within(root, path).read_text(encoding="utf-8", errors="ignore")
