import {
  DataSourceApi,
  type DataQueryRequest,
  type DataQueryResponse,
  type DataSourceInstanceSettings,
  type ScopedVars,
} from '@grafana/data';
import { getBackendSrv, getTemplateSrv } from '@grafana/runtime';
import { MddbClient, type HttpFetcher } from './client';
import { buildRequest, type TimeRangeSeconds } from './query';
import { toDataFrame } from './transform';
import type { MddbDataSourceOptions, MddbQuery } from './types';
import { DEFAULT_QUERY } from './types';

export interface MddbDataSourceDeps {
  fetcher?: HttpFetcher;
  templateInterpolate?: (raw: string, scopedVars?: ScopedVars) => string;
}

/** Build the default Grafana-backed HTTP fetcher (uses BackendSrv proxy). */
/* istanbul ignore next — exercised only when running inside Grafana; tests inject fetcher. */
function defaultFetcher(): HttpFetcher {
  return (url, body, headers) =>
    new Promise((resolve) => {
      const obs = getBackendSrv().fetch<unknown>({
        url,
        method: 'POST',
        headers,
        data: body,
        showErrorAlert: false,
      });
      // BackendSrv's fetch returns an Observable from a different `rxjs` copy
      // than ours, so we consume it with `.subscribe` directly to dodge the
      // dual-rxjs type incompatibility.
      const sub = obs.subscribe({
        next: (res: { status: number; data: unknown }) => {
          resolve({ status: res.status, data: res.data, bodyText: JSON.stringify(res.data) });
          sub.unsubscribe();
        },
        error: (err: unknown) => {
          const e = err as { status?: number; data?: unknown; statusText?: string };
          resolve({
            status: e.status ?? 0,
            data: e.data ?? null,
            bodyText: e.statusText ?? (err as Error).message,
          });
        },
      });
    });
}

/* istanbul ignore next — exercised only when running inside Grafana; tests inject templateInterpolate. */
function defaultInterpolate(raw: string, scopedVars?: ScopedVars): string {
  return getTemplateSrv().replace(raw, scopedVars);
}

export class MddbDataSource extends DataSourceApi<MddbQuery, MddbDataSourceOptions> {
  private readonly client: MddbClient;
  private readonly defaultCollection?: string;
  private readonly defaultLanguage: string;
  private readonly interpolate: (raw: string, scopedVars?: ScopedVars) => string;

  constructor(
    instanceSettings: DataSourceInstanceSettings<MddbDataSourceOptions>,
    deps: MddbDataSourceDeps = {},
  ) {
    super(instanceSettings);
    const json = instanceSettings.jsonData ?? {};
    const secureFields =
      (instanceSettings as unknown as { secureJsonFields?: Record<string, boolean> })
        .secureJsonFields ?? {};
    this.defaultCollection = json.defaultCollection;
    this.defaultLanguage = json.defaultLanguage ?? 'en_US';
    this.client = new MddbClient({
      uid: instanceSettings.uid,
      hasApiKey: Boolean(secureFields.apiKey),
      fetcher: deps.fetcher ?? defaultFetcher(),
    });
    this.interpolate = deps.templateInterpolate ?? defaultInterpolate;
  }

  /**
   * Apply dashboard variables / scoped vars to every interpolatable field on
   * the query. Grafana calls this automatically before `query()`, so do NOT
   * call it again inside `query()` — that would re-interpolate already-resolved
   * values and corrupt anything containing a literal `$...`.
   */
  applyTemplateVariables(query: MddbQuery, scopedVars: ScopedVars): MddbQuery {
    return {
      ...query,
      collection: query.collection
        ? this.interpolate(query.collection, scopedVars)
        : query.collection,
      query: query.query ? this.interpolate(query.query, scopedVars) : query.query,
      facetKey: query.facetKey ? this.interpolate(query.facetKey, scopedVars) : query.facetKey,
    };
  }

  getDefaultQuery(): Partial<MddbQuery> {
    return DEFAULT_QUERY;
  }

  filterQuery(query: MddbQuery): boolean {
    return !query.hide;
  }

  async query(request: DataQueryRequest<MddbQuery>): Promise<DataQueryResponse> {
    const range: TimeRangeSeconds = {
      fromSec: Math.floor(request.range.from.valueOf() / 1000),
      toSec: Math.floor(request.range.to.valueOf() / 1000),
    };
    const tasks = request.targets
      .filter((target) => this.filterQuery(target))
      .map(async (target) => {
        // Grafana already invoked applyTemplateVariables() on `target` for us.
        const { path, body } = buildRequest(target, range, {
          collection: this.defaultCollection,
          language: this.defaultLanguage,
        });
        const payload = await this.client.post(path, body);
        return toDataFrame(target, payload);
      });
    const data = await Promise.all(tasks);
    return { data };
  }

  async testDatasource(): Promise<{ status: 'success' | 'error'; message: string }> {
    const probe = await this.client.ping();
    return { status: probe.ok ? 'success' : 'error', message: probe.message };
  }
}
