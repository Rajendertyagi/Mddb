import { MddbApiError, getHealth, getStats } from '../src/client';

function mockFetch(impl: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) {
  return impl as unknown as typeof fetch;
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

describe('client', () => {
  it('GET /v1/health sends Accept header and no API key by default', async () => {
    const fetchImpl = mockFetch(async (input, init) => {
      const url = typeof input === 'string' ? input : input.toString();
      expect(url).toBe('https://srv.test/v1/health');
      const headers = init?.headers as Record<string, string>;
      expect(headers.Accept).toBe('application/json');
      expect(headers['X-API-Key']).toBeUndefined();
      return jsonResponse({ status: 'healthy', mode: 'wr' });
    });
    const out = await getHealth({ baseUrl: 'https://srv.test', fetchImpl });
    expect(out.status).toBe('healthy');
  });

  it('GET /v1/stats forwards API key when provided', async () => {
    const fetchImpl = mockFetch(async (_input, init) => {
      const headers = init?.headers as Record<string, string>;
      expect(headers['X-API-Key']).toBe('mk_secret');
      return jsonResponse({
        totalDocuments: 12,
        totalRevisions: 14,
        collections: [{ name: 'blog', documentCount: 12 }],
      });
    });
    const out = await getStats({
      baseUrl: 'https://srv.test',
      apiKey: 'mk_secret',
      fetchImpl,
    });
    expect(out.totalDocuments).toBe(12);
    expect(out.collections[0].name).toBe('blog');
  });

  it('handles trailing slashes in baseUrl', async () => {
    const fetchImpl = mockFetch(async (input) => {
      const url = typeof input === 'string' ? input : input.toString();
      expect(url).toBe('https://srv.test/v1/health');
      return jsonResponse({ status: 'healthy' });
    });
    await getHealth({ baseUrl: 'https://srv.test/', fetchImpl });
  });

  it('throws MddbApiError on non-2xx', async () => {
    const fetchImpl = mockFetch(async () => jsonResponse({ error: 'nope' }, 401));
    await expect(getHealth({ baseUrl: 'https://srv.test', fetchImpl })).rejects.toMatchObject({
      name: 'MddbApiError',
      status: 401,
    });
  });

  it('wraps network errors', async () => {
    const fetchImpl = mockFetch(async () => {
      throw new Error('boom');
    });
    await expect(getHealth({ baseUrl: 'https://srv.test', fetchImpl })).rejects.toMatchObject({
      name: 'MddbApiError',
      status: 0,
    });
  });

  it('wraps non-Error throws', async () => {
    const fetchImpl = mockFetch(async () => {
      throw 'string-error' as unknown as Error;
    });
    await expect(getHealth({ baseUrl: 'https://srv.test', fetchImpl })).rejects.toBeInstanceOf(
      MddbApiError,
    );
  });

  it('reports timeout as MddbApiError with status 0', async () => {
    const fetchImpl = mockFetch(async (_input, init) => {
      return new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () => {
          const err = new Error('aborted');
          err.name = 'AbortError';
          reject(err);
        });
      });
    });
    await expect(
      getHealth({ baseUrl: 'https://srv.test', fetchImpl, timeoutMs: 5 }),
    ).rejects.toMatchObject({ status: 0, message: expect.stringMatching(/timed out/) });
  });
});
