import { MddbClient, MddbHttpError, type HttpFetcher } from '../src/client';

type Call = { url: string; body: unknown; headers: Record<string, string> };

function recordFetcher(response: { status: number; data: unknown; bodyText?: string } | Error): {
  fetcher: HttpFetcher;
  calls: Call[];
} {
  const calls: Call[] = [];
  const fetcher: HttpFetcher = async (url, body, headers) => {
    calls.push({ url, body, headers });
    if (response instanceof Error) throw response;
    return response;
  };
  return { fetcher, calls };
}

const UID = 'mddb-uid';

describe('MddbClient.proxyUrl', () => {
  it('routes through "auth" when an API key is configured', () => {
    const { fetcher } = recordFetcher({ status: 200, data: {} });
    const c = new MddbClient({ uid: UID, hasApiKey: true, fetcher });
    expect(c.proxyUrl('/v1/stats')).toBe(`/api/datasources/proxy/uid/${UID}/auth/v1/stats`);
  });

  it('routes through "noauth" when no API key is configured', () => {
    const { fetcher } = recordFetcher({ status: 200, data: {} });
    const c = new MddbClient({ uid: UID, hasApiKey: false, fetcher });
    expect(c.proxyUrl('/v1/stats')).toBe(`/api/datasources/proxy/uid/${UID}/noauth/v1/stats`);
  });

  it('prefixes a leading slash on the MDDB path when missing', () => {
    const { fetcher } = recordFetcher({ status: 200, data: {} });
    const c = new MddbClient({ uid: UID, hasApiKey: true, fetcher });
    expect(c.proxyUrl('v1/stats')).toBe(`/api/datasources/proxy/uid/${UID}/auth/v1/stats`);
  });

  it('requires a UID', () => {
    const { fetcher } = recordFetcher({ status: 200, data: {} });
    expect(() => new MddbClient({ uid: '', hasApiKey: false, fetcher })).toThrow(/UID/);
  });
});

describe('MddbClient.post', () => {
  it('POSTs through the proxy URL with JSON content-type and no Authorization header', async () => {
    const { fetcher, calls } = recordFetcher({ status: 200, data: { ok: true } });
    const c = new MddbClient({ uid: UID, hasApiKey: true, fetcher });
    const data = await c.post('/v1/stats', { foo: 'bar' });
    expect(data).toEqual({ ok: true });
    expect(calls[0].url).toBe(`/api/datasources/proxy/uid/${UID}/auth/v1/stats`);
    expect(calls[0].headers).toEqual({ 'Content-Type': 'application/json' });
    // The bearer token is injected by Grafana's pluginproxy from the plugin.json
    // route — the frontend never sees the secret, hence no Authorization here.
    expect(calls[0].headers.Authorization).toBeUndefined();
    expect(calls[0].body).toEqual({ foo: 'bar' });
  });

  it('throws MddbHttpError on non-2xx and surfaces bodyText', async () => {
    const { fetcher } = recordFetcher({ status: 500, data: null, bodyText: 'boom' });
    const c = new MddbClient({ uid: UID, hasApiKey: false, fetcher });
    await expect(c.post('/v1/stats', {})).rejects.toMatchObject({
      name: 'MddbHttpError',
      status: 500,
      body: 'boom',
    });
  });

  it('MddbHttpError exposes status and body fields', () => {
    const err = new MddbHttpError('m', 418, 'tea');
    expect(err).toBeInstanceOf(Error);
    expect({ name: err.name, status: err.status, body: err.body }).toEqual({
      name: 'MddbHttpError',
      status: 418,
      body: 'tea',
    });
  });
});

describe('MddbClient.ping', () => {
  it('reports success on 2xx and on 404/405 (alive but probe path unsupported)', async () => {
    for (const status of [200, 204, 404, 405]) {
      const { fetcher } = recordFetcher({ status, data: {} });
      const c = new MddbClient({ uid: UID, hasApiKey: false, fetcher });
      await expect(c.ping()).resolves.toEqual({ ok: true, message: 'Connected to MDDB.' });
    }
  });

  it('reports auth failure on 401/403', async () => {
    for (const status of [401, 403]) {
      const { fetcher } = recordFetcher({ status, data: {} });
      const c = new MddbClient({ uid: UID, hasApiKey: true, fetcher });
      const res = await c.ping();
      expect(res.ok).toBe(false);
      expect(res.message).toMatch(/Authentication failed/);
    }
  });

  it('reports server failure on 5xx', async () => {
    const { fetcher } = recordFetcher({ status: 503, data: null });
    const c = new MddbClient({ uid: UID, hasApiKey: false, fetcher });
    const res = await c.ping();
    expect(res.ok).toBe(false);
    expect(res.message).toMatch(/server error/);
  });

  it('reports unexpected statuses', async () => {
    const { fetcher } = recordFetcher({ status: 302, data: {} });
    const c = new MddbClient({ uid: UID, hasApiKey: false, fetcher });
    await expect(c.ping()).resolves.toEqual({
      ok: false,
      message: 'Unexpected response (HTTP 302).',
    });
  });

  it('captures network errors as the message', async () => {
    const { fetcher } = recordFetcher(new Error('ECONNREFUSED'));
    const c = new MddbClient({ uid: UID, hasApiKey: false, fetcher });
    await expect(c.ping()).resolves.toEqual({ ok: false, message: 'ECONNREFUSED' });
  });
});
