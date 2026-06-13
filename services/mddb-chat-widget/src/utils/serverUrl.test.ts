// Run with: node --test src/utils/serverUrl.test.ts (Node >= 23.6 strips types).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { validateServerUrl } from './serverUrl.ts';

test('FE-006: wss:// is accepted for any host', () => {
  assert.equal(
    validateServerUrl('wss://chat.example.com/ws'),
    'wss://chat.example.com/ws',
  );
  assert.equal(validateServerUrl('wss://prod.example'), 'wss://prod.example/');
});

test('FE-006: ws:// is accepted only for loopback hosts', () => {
  assert.equal(
    validateServerUrl('ws://localhost:11030/ws'),
    'ws://localhost:11030/ws',
  );
  assert.equal(
    validateServerUrl('ws://127.0.0.1:11030/ws'),
    'ws://127.0.0.1:11030/ws',
  );
  assert.equal(validateServerUrl('ws://[::1]:11030/ws'), 'ws://[::1]:11030/ws');
});

test('FE-006: ws:// to a non-localhost host is rejected', () => {
  assert.equal(validateServerUrl('ws://prod.example/ws'), null);
  assert.equal(validateServerUrl('ws://chat.example.com'), null);
});

test('FE-006: non-WebSocket schemes and junk are rejected', () => {
  for (const raw of [
    '',
    'javascript:alert(1)',
    'http://prod.example',
    'https://prod.example',
    'data:text/html,<script>1</script>',
    '/relative/path',
    'chat.example.com/ws', // no scheme
    'not a url',
  ]) {
    assert.equal(validateServerUrl(raw), null, `should reject: ${JSON.stringify(raw)}`);
  }
});
