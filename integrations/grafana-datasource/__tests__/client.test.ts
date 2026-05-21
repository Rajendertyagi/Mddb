import { MddbClient, MddbHttpError, type HttpFetcher } from '../src/client';

function recordFetcher(
  response: { status: number; data: unknown; bodyText?: string } | Error,
): { fetcher: HttpFetcher; calls: Array<{ url: string; body: unknown; headers: Record<string, string> }> } {
  const calls: Array<{ url: string; body: unknown; headers: Record<string, string> }> = [];
  const fetcher: HttpFetcher = async (url, body, headers) => {
    calls.push({ url, body, headers });
    if (response instanceof Error) throw response;
    return response;
  };
  return { fetcher, calls };
}

describe('MddbClient.post', () => {
  it('strips trailing slashes from the base URL and sends a bearer token when set', async () => {
    const { fetcher, calls } = recordFetcher({ status: 200, data: { ok: true } });
    const c = new MddbClient({ baseUrl: 'https://mddb.tradik.com///', apiKey: 'vk_token', fetcher });
    const data = await c.post('/v1/stats', { foo: 'bar' });
    expect(data).toEqual({ ok: true });
    expect(calls[0].url).toBe('https://mddb.tradik.com/v1/stats');
    expect(calls[0].headers).toEqual({
      'Content-Type': 'application/json',
      Authorization: 'Bearer vk_token',
    });
    expect(calls[0].body).toEqual({ foo: 'bar' });
  });

  it('omits the Authorization header when no apiKey is configured', async () => {
    const { fetcher, calls } = recordFetcher({ status: 200, data: {} });
    const c = new MddbClient({ baseUrl: 'http://localhost:11023', apiKey: '   ', fetcher });
    await c.post('/v1/stats', {});
    expect(calls[0].headers.Authorization).toBeUndefined();
  });

  it('throws MddbHttpError on non-2xx and surfaces bodyText', async () => {
    const { fetcher } = recordFetcher({ status: 500, data: null, bodyText: 'boom' });
    const c = new MddbClient({ baseUrl: 'http://x', fetcher });
    await expect(c.post('/v1/stats', {})).rejects.toMatchObject({
      name: 'MddbHttpError',
      status: 500,
      body: 'boom',
    });
  });

  it('throws a helpful error when baseUrl is missing', async () => {
    const { fetcher } = recordFetcher({ status: 200, data: {} });
    const c = new MddbClient({ baseUrl: '', fetcher });
    await expect(c.post('/v1/stats', {})).rejects.toThrow(/Missing|missing "url"/i);
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
  const url = 'http://localhost:11023';

  it('reports failure when URL is missing', async () => {
    const { fetcher } = recordFetcher({ status: 200, data: {} });
    const c = new MddbClient({ baseUrl: '', fetcher });
    await expect(c.ping()).resolves.toEqual({ ok: false, message: 'Missing URL.' });
  });

  it('reports success on 2xx and on 404/405 (alive but probe path unsupported)', async () => {
    for (const status of [200, 204, 404, 405]) {
      const { fetcher } = recordFetcher({ status, data: {} });
      const c = new MddbClient({ baseUrl: url, fetcher });
      await expect(c.ping()).resolves.toEqual({ ok: true, message: 'Connected to MDDB.' });
    }
  });

  it('reports auth failure on 401/403', async () => {
    for (const status of [401, 403]) {
      const { fetcher } = recordFetcher({ status, data: {} });
      const c = new MddbClient({ baseUrl: url, apiKey: 'vk_x', fetcher });
      const res = await c.ping();
      expect(res.ok).toBe(false);
      expect(res.message).toMatch(/Authentication failed/);
    }
  });

  it('reports server failure on 5xx', async () => {
    const { fetcher } = recordFetcher({ status: 503, data: null });
    const c = new MddbClient({ baseUrl: url, fetcher });
    const res = await c.ping();
    expect(res.ok).toBe(false);
    expect(res.message).toMatch(/server error/);
  });

  it('reports unexpected statuses', async () => {
    const { fetcher } = recordFetcher({ status: 302, data: {} });
    const c = new MddbClient({ baseUrl: url, fetcher });
    await expect(c.ping()).resolves.toEqual({
      ok: false,
      message: 'Unexpected response (HTTP 302).',
    });
  });

  it('captures network errors as the message', async () => {
    const { fetcher } = recordFetcher(new Error('ECONNREFUSED'));
    const c = new MddbClient({ baseUrl: url, fetcher });
    await expect(c.ping()).resolves.toEqual({ ok: false, message: 'ECONNREFUSED' });
  });
});
