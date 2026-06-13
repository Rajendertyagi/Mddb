// FE-001 — safe rendering of FTS highlight fragments.
//
// The MDDB FTS server builds each highlight `fragment` by wrapping matched
// terms in `<mark>` INSIDE the raw, unescaped document content. The fragment is
// therefore attacker-controlled HTML: a document containing
// `<img src=x onerror=alert(1)>` would execute in the admin panel if the
// fragment were injected via `dangerouslySetInnerHTML` (stored XSS).
//
// splitHighlightFragment turns the fragment into an ordered list of plain-text
// segments split on the `<mark>` / `</mark>` markers. Callers render each
// segment as React children (text), which React escapes — so the only element
// ever emitted is our own `<mark>` wrapper, and every other byte of document
// content is shown literally instead of being parsed as markup.
//
// Segments at odd indices were wrapped in `<mark>` by the server (matches);
// even indices are the surrounding text.
export function splitHighlightFragment(fragment) {
  const parts = String(fragment ?? '').split(/<\/?mark>/g);
  return parts.map((text, i) => ({ text, highlighted: i % 2 === 1 }));
}
