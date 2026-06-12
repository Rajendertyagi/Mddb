// Run with: node --test src/lib/highlight.test.js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { splitHighlightFragment } from './highlight.js';

test('splits matches wrapped in <mark> into ordered segments', () => {
  assert.deepEqual(splitHighlightFragment('a<mark>b</mark>c'), [
    { text: 'a', highlighted: false },
    { text: 'b', highlighted: true },
    { text: 'c', highlighted: false },
  ]);
});

test('FE-001: a malicious fragment stays inert text, never markup', () => {
  const payload = 'pre <img src=x onerror=alert(1)> <mark>hit</mark> post';
  const segs = splitHighlightFragment(payload);

  // The dangerous markup is preserved verbatim inside a NON-highlighted text
  // segment — it is data, not an element. React escapes it on render, so no
  // onerror handler is ever attached to the DOM.
  assert.equal(segs[0].text, 'pre <img src=x onerror=alert(1)> ');
  assert.equal(segs[0].highlighted, false);
  assert.equal(segs[1].text, 'hit');
  assert.equal(segs[1].highlighted, true);

  // Every segment is a plain string (never an HTML node / __html wrapper).
  for (const s of segs) {
    assert.equal(typeof s.text, 'string');
  }
});

test('a <script> payload is not highlighted and not split mid-tag', () => {
  const segs = splitHighlightFragment('<script>alert(1)</script><mark>x</mark>');
  assert.equal(segs[0].text, '<script>alert(1)</script>');
  assert.equal(segs[0].highlighted, false);
  assert.equal(segs[1].text, 'x');
  assert.equal(segs[1].highlighted, true);
});

test('handles empty and nullish input', () => {
  assert.deepEqual(splitHighlightFragment(''), [{ text: '', highlighted: false }]);
  assert.deepEqual(splitHighlightFragment(null), [{ text: '', highlighted: false }]);
  assert.deepEqual(splitHighlightFragment(undefined), [{ text: '', highlighted: false }]);
});
