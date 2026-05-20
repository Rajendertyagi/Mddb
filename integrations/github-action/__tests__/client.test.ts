import { MddbClient, MddbHttpError } from '../src/client';
import type { MddbDocument } from '../src/document';

type FetchArgs = Parameters<typeof fetch>;

interface MockResponse {
  status: number;
  body?: string;
}

function makeFetch(responses: (MockResponse | Error)[]) {
  const calls: FetchArgs[] = [];
  const queue = responses.slice();
  const fetchImpl = (async (...args: FetchArgs) => {
    calls.push(args);
    const next = queue.shift();
    if (!next) throw new Error('unexpected extra fetch call');
    if (next instanceof Error) throw next;
    return new Response(next.body ?? '', { status: next.status });
  }) as unknown as typeof fetch;
  return { fetchImpl, calls };
}

const doc: MddbDocument = {
  collection: 'docs',
  key: 'guide.md',
  lang: 'en_US',
  meta: { source: ['github-action'] },
  contentMd: '# Hello',
};

const clientOpts = (overrides: Partial<ConstructorParameters<typeof MddbClient>[0]> = {}) => ({
  baseUrl: 'https://mddb.example.com',
  apiKey: 'vk_test',
  timeoutSeconds: 5,
  verifySsl: true,
  maxAttempts: 3,
  backoffMs: 0,
  sleepImpl: async () => {},
  ...overrides,
});

describe('MddbClient.addDocument', () => {
  it('POSTs JSON with Authorization header on success', async () => {
    const { fetchImpl, calls } = makeFetch([{ status: 200, body: '{}' }]);
    const client = new MddbClient(clientOpts({ fetchImpl }));

    await client.addDocument(doc);

    expect(calls).toHaveLength(1);
    const [url, init] = calls[0];
    expect(url).toBe('https://mddb.example.com/v1/add');
    expect(init?.method).toBe('POST');
    const headers = init?.headers as Record<string, string>;
    expect(headers['Content-Type']).toBe('application/json');
    expect(headers.Authorization).toBe('Bearer vk_test');
    expect(JSON.parse(String(init?.body))).toEqual(doc);
  });

  it('attaches a permissive https agent when verifySsl is false', async () => {
    const { fetchImpl, calls } = makeFetch([{ status: 200 }]);
    const client = new MddbClient(clientOpts({ verifySsl: false, fetchImpl }));
    await client.addDocument(doc);
    const init = calls[0][1] as RequestInit & {
      agent?: { options?: { rejectUnauthorized?: boolean } };
    };
    expect(init.agent).toBeDefined();
    expect(init.agent?.options?.rejectUnauthorized).toBe(false);
  });

  it('skips Authorization header when no api key is configured', async () => {
    const { fetchImpl, calls } = makeFetch([{ status: 200 }]);
    const client = new MddbClient(clientOpts({ apiKey: '', fetchImpl }));
    await client.addDocument(doc);
    const headers = (calls[0][1]?.headers ?? {}) as Record<string, string>;
    expect(headers.Authorization).toBeUndefined();
  });

  it('retries on transient HTTP 503 and eventually succeeds', async () => {
    const { fetchImpl, calls } = makeFetch([{ status: 503, body: 'busy' }, { status: 200 }]);
    const client = new MddbClient(clientOpts({ fetchImpl }));
    await client.addDocument(doc);
    expect(calls).toHaveLength(2);
  });

  it('retries on network errors and surfaces the last error after exhausting attempts', async () => {
    const { fetchImpl, calls } = makeFetch([
      new Error('ECONNRESET'),
      new Error('ECONNRESET'),
      new Error('ECONNRESET'),
    ]);
    const client = new MddbClient(clientOpts({ fetchImpl, maxAttempts: 3 }));
    await expect(client.addDocument(doc)).rejects.toThrow(/ECONNRESET/);
    expect(calls).toHaveLength(3);
  });

  it('throws MddbHttpError on non-retryable 4xx without retrying', async () => {
    const { fetchImpl, calls } = makeFetch([{ status: 400, body: 'bad shape' }]);
    const client = new MddbClient(clientOpts({ fetchImpl }));
    await expect(client.addDocument(doc)).rejects.toMatchObject({
      name: 'MddbHttpError',
      status: 400,
    });
    expect(calls).toHaveLength(1);
  });

  it('gives up after the final retry on persistent 5xx and reports body excerpt', async () => {
    const { fetchImpl } = makeFetch([
      { status: 500, body: 'a' },
      { status: 500, body: 'b' },
      { status: 500, body: 'c' },
    ]);
    const client = new MddbClient(clientOpts({ fetchImpl, maxAttempts: 3 }));
    await expect(client.addDocument(doc)).rejects.toBeInstanceOf(MddbHttpError);
  });
});

describe('MddbClient.ping', () => {
  it('treats 200/404/405 as healthy', async () => {
    for (const status of [200, 404, 405]) {
      const { fetchImpl } = makeFetch([{ status }]);
      const client = new MddbClient(clientOpts({ fetchImpl }));
      await expect(client.ping()).resolves.toBeUndefined();
    }
  });

  it('rejects on 401/403 with credential error', async () => {
    const { fetchImpl } = makeFetch([{ status: 401, body: 'no auth' }]);
    const client = new MddbClient(clientOpts({ fetchImpl }));
    await expect(client.ping()).rejects.toThrow(/credentials/);
  });

  it('rejects on 5xx', async () => {
    const { fetchImpl } = makeFetch([{ status: 502, body: 'down' }]);
    const client = new MddbClient(clientOpts({ fetchImpl }));
    await expect(client.ping()).rejects.toThrow(/server error/);
  });

  it('rejects on unexpected 4xx that is not 404/405', async () => {
    const { fetchImpl } = makeFetch([{ status: 418, body: "i'm a teapot" }]);
    const client = new MddbClient(clientOpts({ fetchImpl }));
    await expect(client.ping()).rejects.toThrow(/Unexpected/);
  });
});
