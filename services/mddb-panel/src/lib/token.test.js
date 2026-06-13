// Run with: node --test src/lib/token.test.js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { TOKEN_KEY, LEGACY_TOKEN_KEYS, isValidJwtShape } from './token.js';

test('TOKEN_KEY is the canonical storage key', () => {
  assert.equal(TOKEN_KEY, 'mddb_auth_token');
});

test('legacy keys are the stale ones graphql.js used to read (FE-005)', () => {
  assert.deepEqual(LEGACY_TOKEN_KEYS, ['token', 'apiKey']);
});

test('isValidJwtShape accepts a well-formed JWT', () => {
  assert.equal(
    isValidJwtShape('eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhZG1pbiJ9.abc-DEF_123'),
    true,
  );
});

test('isValidJwtShape rejects malformed / empty / non-string tokens', () => {
  for (const bad of ['', 'not-a-jwt', 'a.b', 'a.b.c.d', null, undefined, 42, {}]) {
    assert.equal(isValidJwtShape(bad), false, `expected ${String(bad)} to be invalid`);
  }
});
