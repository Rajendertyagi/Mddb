/**
 * Polyfills for the jsdom test environment. `@grafana/runtime` pulls in
 * `react-dom/server.browser`, which reads `TextEncoder`/`TextDecoder` off
 * globalThis. jsdom 24+ on Node 22+ no longer assigns them automatically,
 * so we attach the native Node implementations before each test file loads.
 */
import { TextDecoder, TextEncoder } from 'node:util';

if (typeof globalThis.TextEncoder === 'undefined') {
  // @ts-expect-error — jsdom global typing vs node:util mismatch is harmless here.
  globalThis.TextEncoder = TextEncoder;
}
if (typeof globalThis.TextDecoder === 'undefined') {
  // @ts-expect-error — see above.
  globalThis.TextDecoder = TextDecoder;
}
