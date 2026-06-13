// FE-007 — verify the SSG markdown viewer loads CDN assets with Subresource
// Integrity and pinned versions. Run with: node --test sri.test.mjs
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const html = readFileSync(join(here, 'md-viewer.html'), 'utf8');

// Match every <script>/<link> pointing at a public CDN.
const cdnTagRe =
  /<(?:script|link)\b[^>]*\b(?:src|href)=["']https:\/\/(?:cdn\.jsdelivr\.net|cdnjs\.cloudflare\.com)\/[^"']+["'][^>]*>/gi;

test('FE-007: every CDN resource uses SRI + crossorigin', () => {
  const tags = html.match(cdnTagRe) ?? [];
  assert.ok(tags.length >= 3, `expected >=3 CDN tags, found ${tags.length}`);
  for (const tag of tags) {
    assert.match(tag, /integrity="sha384-[A-Za-z0-9+/=]+"/, `missing SRI hash: ${tag}`);
    assert.match(tag, /crossorigin=/, `missing crossorigin: ${tag}`);
  }
});

test('FE-007: mermaid is pinned to an exact version (no floating @10/@11)', () => {
  assert.doesNotMatch(html, /mermaid@1[01]\//, 'mermaid must be pinned to an exact patch version');
  assert.match(html, /mermaid@11\.15\.0\//);
});
