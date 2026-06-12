// Regression guard for FE-008. A full supertest deep-link test runs in CI where
// node_modules is installed; this dependency-free check ensures the SPA-fallback
// route keeps the Express-5-correct catch-all pattern.
//
// Run with: node --test server.test.js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, 'server.js'), 'utf8');

test('FE-008: SPA fallback uses the Express 5 catch-all "/{*path}"', () => {
  assert.match(src, /app\.get\(\s*['"]\/\{\*path\}['"]/, 'must register /{*path}');
});

test('FE-008: the broken leading-slash-less "{*path}" pattern is gone', () => {
  assert.doesNotMatch(
    src,
    /app\.get\(\s*['"]\{\*path\}['"]/,
    "'{*path}' (no leading slash) breaks deep links on Express 5",
  );
});

test('FE-008: fallback is registered after static + proxy', () => {
  const proxyAt = src.indexOf('createProxyMiddleware');
  const staticAt = src.indexOf('express.static');
  const fallbackAt = src.indexOf("app.get('/{*path}'");
  assert.ok(proxyAt >= 0 && staticAt >= 0 && fallbackAt >= 0);
  assert.ok(proxyAt < fallbackAt && staticAt < fallbackAt, 'fallback must come last');
});
