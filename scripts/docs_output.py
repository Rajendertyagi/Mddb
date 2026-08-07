"""Path and URL helpers for scripts that inspect the built documentation site.

The output directory arrives as a CLI argument and the URLs come out of
generated HTML, so neither is trusted to keep a resolved path inside the tree.
Containment is proven here, once, rather than assumed at each call site.

Nothing in this module touches the network: the scripts gate a deploy and must
stay offline and deterministic.
"""

from __future__ import annotations

from pathlib import Path
from urllib.parse import urlsplit

# Canonical site origin. Absolute links to it are internal, so they resolve
# against the output directory exactly like a root-relative link would.
SITE_HOST = "mddb.tradik.com"
SITE_ORIGIN = f"https://{SITE_HOST}"


def site_path(url: str) -> str | None:
    """The site-relative path of an absolute URL on this host, else None.

    Compares the host rather than matching origin prefixes, so a self-link is
    recognised whichever scheme or capitalisation it was written with, and its
    query and fragment are dropped by the parser instead of by hand.
    """
    parts = urlsplit(url)
    if parts.netloc.lower() != SITE_HOST or parts.scheme.lower() not in ("http", "https"):
        return None
    return parts.path or "/"



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
