// FE-010 — the inline chat markdown renderer md() in index.html had an
// over-escaped code-fence regex (\\w / \\n / [\\s\\S]), so triple-backtick code
// blocks never rendered. These tests extract md() from the HTML and exercise it.
// Run with: node --test index-md.test.mjs
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

import { extractFunctionSource } from './extract-fn.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const html = readFileSync(join(here, 'index.html'), 'utf8');

// Extract `function md(...) { ... }` and build a callable.
// eslint-disable-next-line no-new-func
const md = new Function(`return (${extractFunctionSource(html, 'md')})`)();

test('FE-010: a multi-line ```code``` block renders as <pre><code>', () => {
  const out = md('intro\n```js\nconst x = 1;\nconst y = 2;\n```\nouttro');
  assert.match(out, /<pre><code>[\s\S]*const x = 1;[\s\S]*<\/code><\/pre>/);
  assert.ok(!out.includes('```'), `raw fences leaked: ${out}`);
});

test('FE-010: HTML inside a code block stays escaped (no XSS regression)', () => {
  const out = md('```\n<script>alert(1)</script>\n```');
  assert.ok(out.includes('&lt;script&gt;'), `script not escaped: ${out}`);
  assert.ok(!out.includes('<script>'), 'raw <script> leaked');
});

test('FE-010: inline markdown still works', () => {
  assert.match(md('**bold**'), /<strong>bold<\/strong>/);
  assert.match(md('`code`'), /<code>code<\/code>/);
});
