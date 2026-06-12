// FE-002 — verify the SSG markdown viewer is hardened against XSS:
//   1. rendered markdown is sanitized through DOMPurify (pinned + SRI),
//   2. mermaid code blocks are HTML-escaped before injection,
//   3. the ?doc parameter only accepts same-origin relative *.md paths.
// Run with: node --test xss.test.mjs
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const html = readFileSync(join(here, 'md-viewer.html'), 'utf8');

// Slice out a `function name(...) { ... }` definition by brace-matching, so the
// pure validation helpers can be exercised in isolation (no DOM needed).
function extractFn(src, name) {
  const start = src.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `function ${name} not found in md-viewer.html`);
  const open = src.indexOf('{', start);
  let depth = 0;
  for (let i = open; i < src.length; i++) {
    if (src[i] === '{') depth++;
    else if (src[i] === '}') {
      depth--;
      if (depth === 0) return src.slice(start, i + 1);
    }
  }
  throw new Error(`unbalanced braces for ${name}`);
}

// Build callable versions of the inline helpers with an injected window mock.
function loadHelpers(locationHref, locationOrigin) {
  const src = `
    const window = { location: { href: ${JSON.stringify(locationHref)}, origin: ${JSON.stringify(locationOrigin)} } };
    ${extractFn(html, 'escapeHtml')}
    ${extractFn(html, 'resolveDocParam')}
    return { escapeHtml, resolveDocParam };
  `;
  // eslint-disable-next-line no-new-func
  return new Function(src)();
}

const ORIGIN = 'https://docs.example';
const HREF = `${ORIGIN}/mddb/md-viewer.html`;

test('FE-002: escapeHtml neutralises HTML metacharacters', () => {
  const { escapeHtml } = loadHelpers(HREF, ORIGIN);
  assert.equal(
    escapeHtml('<img src=x onerror=alert(1)>'),
    '&lt;img src=x onerror=alert(1)&gt;'
  );
  assert.equal(escapeHtml(`</pre><script>"&'`), '&lt;/pre&gt;&lt;script&gt;&quot;&amp;&#39;');
});

test('FE-002: resolveDocParam accepts same-origin relative .md paths', () => {
  const { resolveDocParam } = loadHelpers(HREF, ORIGIN);
  assert.equal(resolveDocParam('GUIDE.md'), 'mddb/GUIDE.md');
  assert.equal(resolveDocParam('QUICKSTART.md'), 'mddb/QUICKSTART.md');
});

test('FE-002: resolveDocParam rejects hostile values and falls back', () => {
  const { resolveDocParam } = loadHelpers(HREF, ORIGIN);
  const fallback = 'QUICKSTART.md';
  for (const bad of [
    null,
    '',
    'https://evil.example/payload.md', // absolute + scheme
    '//evil.example/payload.md',       // protocol-relative
    '../../etc/passwd',                // traversal + not .md
    '../secret.md',                    // traversal
    '/absolute/path.md',               // absolute path
    'data:text/html,<script>1</script>', // scheme
    'notmarkdown.txt',                 // wrong suffix
  ]) {
    assert.equal(resolveDocParam(bad), fallback, `expected fallback for: ${bad}`);
  }
});

test('FE-002: DOMPurify is loaded, pinned and SRI-protected', () => {
  const tag = html.match(/<script\b[^>]*dompurify@[^>]*>/i)?.[0];
  assert.ok(tag, 'DOMPurify script tag missing');
  assert.match(tag, /dompurify@3\.\d+\.\d+\//, 'DOMPurify must be pinned to an exact version');
  assert.match(tag, /integrity="sha384-[A-Za-z0-9+/=]+"/, 'DOMPurify must carry an SRI hash');
  assert.match(tag, /crossorigin=/, 'DOMPurify must set crossorigin');
});

test('FE-002: rendered markdown is sanitized (no raw marked.parse innerHTML)', () => {
  assert.match(html, /DOMPurify\.sanitize\(\s*marked\.parse\(/, 'marked output must pass through DOMPurify');
  assert.doesNotMatch(
    html,
    /innerHTML\s*=\s*marked\.parse\(/,
    'raw marked.parse() must never be assigned to innerHTML'
  );
});

test('FE-002: ?doc is routed through resolveDocParam', () => {
  assert.match(html, /const\s+mdFile\s*=\s*resolveDocParam\(\s*urlParams\.get\(['"]doc['"]\)\s*\)/);
});

test('FE-002: a defense-in-depth CSP restricts connect-src to self', () => {
  const csp = html.match(/<meta[^>]*Content-Security-Policy[^>]*>/i)?.[0];
  assert.ok(csp, 'CSP meta tag missing');
  assert.match(csp, /connect-src 'self'/, 'CSP must restrict connect-src to self');
  assert.match(csp, /object-src 'none'/, 'CSP must forbid plugins');
});
