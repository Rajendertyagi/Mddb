/**
 * Thin HTTP client that talks to MDDB *through* a Grafana datasource proxy route.
 *
 * Reasoning: Grafana never ships `secureJsonData` (e.g. the API key) back to the
 * frontend after save — only `secureJsonFields.apiKey: true` (a boolean). The
 * Bearer token therefore has to be injected on Grafana's server side, via a
 * `routes` entry in `plugin.json`. This client just picks one of two configured
 * proxy paths (`auth` when an API key is set, `noauth` otherwise) and builds
 * `/api/datasources/proxy/uid/<uid>/<routePath><mddbPath>`. The configured
 * route copies the request to `{{ .JsonData.url }}{{ mddbPath }}` and adds the
 * `Authorization: Bearer {{ .SecureJsonData.apiKey }}` header when applicable.
 */
export type HttpFetcher = (
  url: string,
  body: unknown,
  headers: Record<string, string>,
) => Promise<{ status: number; data: unknown; bodyText?: string }>;

export interface MddbClientOptions {
  /** Grafana datasource UID — used to build the proxy URL. */
  uid: string;
  /** True if `secureJsonFields.apiKey === true`. Chooses `auth` vs `noauth` route. */
  hasApiKey: boolean;
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

export class MddbClient {
  private readonly uid: string;
  private readonly routePath: 'auth' | 'noauth';
  private readonly fetcher: HttpFetcher;

  constructor(opts: MddbClientOptions) {
    if (!opts.uid) {
      throw new Error('MddbClient requires a Grafana datasource UID.');
    }
    this.uid = opts.uid;
    this.routePath = opts.hasApiKey ? 'auth' : 'noauth';
    this.fetcher = opts.fetcher;
  }

  /** Build the Grafana proxy URL for a given MDDB endpoint path. */
  proxyUrl(path: string): string {
    const normalised = path.startsWith('/') ? path : `/${path}`;
    return `/api/datasources/proxy/uid/${this.uid}/${this.routePath}${normalised}`;
  }

  async post(path: string, body: Record<string, unknown>): Promise<unknown> {
    const url = this.proxyUrl(path);
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
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
   * Cheap connectivity + auth probe used by `/testDatasource`. Treats 2xx as
   * healthy, 401/403 as credential failure, 5xx as server failure, and 404/405
   * as "alive but the probe endpoint isn't supported" — still healthy.
   */
  async ping(): Promise<{ ok: boolean; message: string }> {
    const url = this.proxyUrl('/v1/stats');
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
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
