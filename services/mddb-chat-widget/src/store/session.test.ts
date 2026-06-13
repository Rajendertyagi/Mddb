// Run with: node --test src/store/session.test.ts (Node >= 23.6 strips types).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { capMessages, MAX_STORED_MESSAGES } from './session.ts';
import type { Message } from './state';

function makeMessages(n: number): Message[] {
  return Array.from({ length: n }, (_, i) => ({ id: String(i) })) as unknown as Message[];
}

test('capMessages keeps only the most recent MAX_STORED_MESSAGES (FE-004)', () => {
  const capped = capMessages(makeMessages(120));
  assert.equal(capped.length, MAX_STORED_MESSAGES);
  // Oldest kept message is at index 120 - MAX_STORED_MESSAGES.
  assert.equal((capped[0] as { id: string }).id, String(120 - MAX_STORED_MESSAGES));
  assert.equal((capped[capped.length - 1] as { id: string }).id, '119');
});

test('capMessages passes short lists through unchanged', () => {
  const short = makeMessages(3);
  assert.equal(capMessages(short).length, 3);
});

test('capMessages tolerates non-array input', () => {
  assert.deepEqual(capMessages(undefined as unknown as Message[]), []);
  assert.deepEqual(capMessages(null as unknown as Message[]), []);
});
