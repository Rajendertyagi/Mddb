/** HTTP fetcher signature — abstracts both Grafana BackendSrv and node/browser fetch. */
export type HttpFetcher = (
  url: string,
  body: unknown,
  headers: Record<string, string>,
) => Promise<{ status: number; data: unknown; bodyText?: string }>;

export interface MddbClientOptions {
  baseUrl: string;
  apiKey?: string;
  fetcher: HttpFetcher;
}

export class MddbHttpError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly body: string,
  ) {
    super(message);
    this.name = 'MddbHttpError';
  }
}

/** Thin wrapper around an injected fetcher; centralises base URL + auth header + error mapping. */
export class MddbClient {
  private readonly baseUrl: string;
  private readonly apiKey: string;
  private readonly fetcher: HttpFetcher;

  constructor(opts: MddbClientOptions) {
    this.baseUrl = (opts.baseUrl || '').replace(/\/+$/, '');
    this.apiKey = (opts.apiKey ?? '').trim();
    this.fetcher = opts.fetcher;
  }

  async post(path: string, body: Record<string, unknown>): Promise<unknown> {
    if (!this.baseUrl) {
      throw new Error('MDDB datasource is missing "url" — set it in the datasource settings.');
    }
    const url = `${this.baseUrl}${path}`;
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    if (this.apiKey) headers.Authorization = `Bearer ${this.apiKey}`;
    const res = await this.fetcher(url, body, headers);
    if (res.status < 200 || res.status >= 300) {
      throw new MddbHttpError(
        `MDDB ${path} failed (HTTP ${res.status})`,
        res.status,
        res.bodyText ?? '',
      );
    }
    return res.data;
  }

  /**
   * Cheap connectivity + auth probe used by /testDatasource. Treats 2xx as healthy,
   * 401/403 as credential failure, 5xx as server failure, and 404/405 as "alive but
   * the probe endpoint isn't supported" — still healthy.
   */
  async ping(): Promise<{ ok: boolean; message: string }> {
    if (!this.baseUrl) {
      return { ok: false, message: 'Missing URL.' };
    }
    const url = `${this.baseUrl}/v1/stats`;
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    if (this.apiKey) headers.Authorization = `Bearer ${this.apiKey}`;
    try {
      const res = await this.fetcher(url, {}, headers);
      if (res.status === 401 || res.status === 403) {
        return { ok: false, message: `Authentication failed (HTTP ${res.status}).` };
      }
      if (res.status >= 500) {
        return { ok: false, message: `MDDB server error (HTTP ${res.status}).` };
      }
      if ((res.status >= 200 && res.status < 300) || res.status === 404 || res.status === 405) {
        return { ok: true, message: 'Connected to MDDB.' };
      }
      return { ok: false, message: `Unexpected response (HTTP ${res.status}).` };
    } catch (err) {
      return { ok: false, message: (err as Error).message };
    }
  }
}
