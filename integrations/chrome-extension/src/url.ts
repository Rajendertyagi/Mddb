export const DEFAULT_PANEL_PORT = 3000;

export function normalizeServerUrl(input: string): string {
  const trimmed = input.trim();
  if (!trimmed) {
    throw new Error('Server URL is required');
  }

  let parsed: URL;
  try {
    parsed = new URL(trimmed);
  } catch {
    throw new Error('Server URL must be a valid absolute URL (e.g. https://mddb.example.com)');
  }

  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new Error('Server URL must use http:// or https://');
  }

  // Strip trailing slash from pathname so we can concatenate paths safely.
  const path = parsed.pathname.replace(/\/+$/, '');
  return `${parsed.protocol}//${parsed.host}${path}`;
}

export function derivePanelUrl(serverUrl: string, override?: string | null): string {
  if (override && override.trim()) {
    return normalizeServerUrl(override);
  }
  const parsed = new URL(serverUrl);
  parsed.port = String(DEFAULT_PANEL_PORT);
  parsed.pathname = '/';
  return parsed.toString().replace(/\/$/, '');
}

export function originOf(url: string): string {
  return new URL(url).origin;
}
