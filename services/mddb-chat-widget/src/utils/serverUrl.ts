/// FE-006: validate the `data-server` WebSocket endpoint before handing it to
/// `new WebSocket(...)`. Without this, a tampered DOM attribute (or an
/// integrator typo) could redirect the whole chat session — messages and
/// sessionId — to an arbitrary host, including over unencrypted ws://.

const LOCAL_HOSTNAMES = new Set(['localhost', '127.0.0.1', '::1', '[::1]']);

function isLocalHostname(hostname: string): boolean {
  return LOCAL_HOSTNAMES.has(hostname);
}

/// validateServerUrl returns the normalized URL string when `raw` is an
/// acceptable WebSocket endpoint, or null otherwise. Encrypted `wss://` is
/// allowed for any host; plaintext `ws://` is allowed only for loopback hosts
/// (local development). Anything else — a relative path, a non-WebSocket scheme
/// (`http:`, `javascript:`, …) or an unparseable string — is rejected.
export function validateServerUrl(raw: string): string | null {
  if (!raw) return null;
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return null;
  }
  if (url.protocol === 'wss:') return url.toString();
  if (url.protocol === 'ws:' && isLocalHostname(url.hostname)) return url.toString();
  return null;
}
