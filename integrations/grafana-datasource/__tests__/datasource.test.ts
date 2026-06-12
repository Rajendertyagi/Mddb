import type { DataQueryRequest, DataSourceInstanceSettings, ScopedVars } from '@grafana/data';
import { MddbDataSource } from '../src/datasource';
import type { HttpFetcher } from '../src/client';
import type { MddbDataSourceOptions, MddbQuery } from '../src/types';

const UID = 'mddb-uid';

function makeFetcher(map: Record<string, { status: number; data: unknown }>): {
  fetcher: HttpFetcher;
  calls: Array<{ url: string; body: unknown }>;
} {
  const calls: Array<{ url: string; body: unknown }> = [];
  const fetcher: HttpFetcher = async (url, body) => {
    calls.push({ url, body });
    const match = Object.keys(map).find((k) => url.endsWith(k));
    if (!match) return { status: 404, data: null, bodyText: 'no mock' };
    return map[match];
  };
  return { fetcher, calls };
}

function makeSettings(
  opts: Partial<MddbDataSourceOptions> = {},
  secureJsonFields: Record<string, boolean> = {},
): DataSourceInstanceSettings<MddbDataSourceOptions> {
  return {
    id: 1,
    uid: UID,
    type: 'tradik-mddb-datasource',
    name: 'MDDB',
    access: 'proxy',
    readOnly: false,
    meta: {} as DataSourceInstanceSettings<MddbDataSourceOptions>['meta'],
    jsonData: {
      url: 'http://localhost:11023',
      defaultCollection: 'docs',
      defaultLanguage: 'en_US',
      ...opts,
    },
    // Grafana attaches secureJsonFields on instance settings — booleans only,
    // never the secrets themselves. Cast widens the type for the test stub.
    ...({ secureJsonFields } as Record<string, unknown>),
  };
}

function makeRequest(
  targets: MddbQuery[],
  scopedVars: ScopedVars = {},
): DataQueryRequest<MddbQuery> {
  return {
    requestId: 'r',
    timezone: 'utc',
    interval: '1m',
    intervalMs: 60_000,
    range: {
      from: { valueOf: () => 1_700_000_000_000 } as DataQueryRequest<MddbQuery>['range']['from'],
      to: { valueOf: () => 1_800_000_000_000 } as DataQueryRequest<MddbQuery>['range']['to'],
      raw: { from: 'now-1h', to: 'now' },
    },
    scopedVars,
    targets,
    startTime: Date.now(),
    app: 'panel-editor',
  } as DataQueryRequest<MddbQuery>;
}

describe('MddbDataSource', () => {
  it('getDefaultQuery + filterQuery work as expected', () => {
    const { fetcher } = makeFetcher({});
    const ds = new MddbDataSource(makeSettings(), { fetcher });
    expect(ds.getDefaultQuery().queryType).toBe('temporal-histogram');
    expect(ds.filterQuery({ refId: 'A' } as MddbQuery)).toBe(true);
    expect(ds.filterQuery({ refId: 'A', hide: true } as MddbQuery)).toBe(false);
  });

  it('applyTemplateVariables interpolates collection / query / facetKey only', () => {
    const { fetcher } = makeFetcher({});
    const ds = new MddbDataSource(makeSettings(), {
      fetcher,
      templateInterpolate: (raw, vars) =>
        `${raw}::${(vars as Record<string, { value?: string }>)?.tag?.value ?? ''}`,
    });
    const out = ds.applyTemplateVariables(
      {
        refId: 'A',
        queryType: 'fts',
        collection: 'c',
        query: '$tag',
        facetKey: 'tags',
      } as MddbQuery,
      { tag: { text: 't', value: 'rust' } },
    );
    expect(out).toMatchObject({
      collection: 'c::rust',
      query: '$tag::rust',
      facetKey: 'tags::rust',
    });
  });

  it('applyTemplateVariables leaves undefined / empty fields untouched', () => {
    const { fetcher } = makeFetcher({});
    const ds = new MddbDataSource(makeSettings(), {
      fetcher,
      templateInterpolate: () => 'TOUCHED',
    });
    const out = ds.applyTemplateVariables({ refId: 'A', queryType: 'stats' } as MddbQuery, {});
    expect(out.collection).toBeUndefined();
    expect(out.query).toBeUndefined();
    expect(out.facetKey).toBeUndefined();
  });

  it('falls back gracefully when jsonData and secureJsonFields are missing', () => {
    const { fetcher } = makeFetcher({});
    const minimal = {
      id: 1,
      uid: UID,
      type: 'tradik-mddb-datasource',
      name: 'MDDB',
      access: 'proxy',
      readOnly: false,
      meta: {} as DataSourceInstanceSettings<MddbDataSourceOptions>['meta'],
      jsonData: {},
    } as DataSourceInstanceSettings<MddbDataSourceOptions>;
    const ds = new MddbDataSource(minimal, { fetcher });
    // No throw on construct; defaults filled in.
    expect(ds.getDefaultQuery().queryType).toBe('temporal-histogram');
  });

  it('uses the Grafana-backed defaults when no deps are injected', () => {
    // `jsonData` undefined → forces the `?? {}` branch in the constructor.
    // `deps` omitted entirely → forces `defaultFetcher()` / `defaultInterpolate`,
    // both istanbul-ignored. We only assert the constructor doesn't blow up.
    const bare = {
      id: 1,
      uid: UID,
      type: 'tradik-mddb-datasource',
      name: 'MDDB',
      access: 'proxy',
      readOnly: false,
      meta: {} as DataSourceInstanceSettings<MddbDataSourceOptions>['meta'],
    } as unknown as DataSourceInstanceSettings<MddbDataSourceOptions>;
    expect(() => new MddbDataSource(bare)).not.toThrow();
  });

  it('routes via /noauth proxy path when no API key is configured', async () => {
    const { fetcher, calls } = makeFetcher({
      '/v1/temporal/histogram': {
        status: 200,
        data: { collection: 'docs', eventType: 'access', buckets: [{ from: 1700, count: 1 }] },
      },
    });
    const ds = new MddbDataSource(makeSettings({}, {}), { fetcher });
    await ds.query(makeRequest([{ refId: 'A', queryType: 'temporal-histogram' } as MddbQuery]));
    expect(calls[0].url).toBe(`/api/datasources/proxy/uid/${UID}/noauth/v1/temporal/histogram`);
  });

  it('routes via /auth proxy path when secureJsonFields.apiKey is set', async () => {
    const { fetcher, calls } = makeFetcher({
      '/v1/stats': { status: 200, data: { collections: {} } },
    });
    const ds = new MddbDataSource(makeSettings({}, { apiKey: true }), { fetcher });
    await ds.query(makeRequest([{ refId: 'A', queryType: 'stats' } as MddbQuery]));
    expect(calls[0].url).toBe(`/api/datasources/proxy/uid/${UID}/auth/v1/stats`);
  });

  it('query() dispatches per target, hides hidden ones, and returns one DataFrame each', async () => {
    const { fetcher, calls } = makeFetcher({
      '/v1/temporal/histogram': {
        status: 200,
        data: { collection: 'docs', eventType: 'access', buckets: [{ from: 1700, count: 1 }] },
      },
      '/v1/stats': { status: 200, data: { collections: {} } },
    });
    const ds = new MddbDataSource(makeSettings(), { fetcher });
    const res = await ds.query(
      makeRequest([
        { refId: 'A', queryType: 'temporal-histogram' } as MddbQuery,
        { refId: 'B', queryType: 'stats' } as MddbQuery,
        { refId: 'H', queryType: 'stats', hide: true } as MddbQuery,
      ]),
    );
    expect(res.data).toHaveLength(2);
    expect(calls.map((c) => c.url)).toEqual([
      `/api/datasources/proxy/uid/${UID}/noauth/v1/temporal/histogram`,
      `/api/datasources/proxy/uid/${UID}/noauth/v1/stats`,
    ]);
    expect(calls[0].body).toMatchObject({
      collection: 'docs',
      from: 1_700_000_000,
      to: 1_800_000_000,
    });
  });

  it('does NOT re-interpolate inside query() — Grafana already did that', async () => {
    const { fetcher, calls } = makeFetcher({
      '/v1/temporal/histogram': { status: 200, data: { buckets: [] } },
    });
    let interpolateCalls = 0;
    const ds = new MddbDataSource(makeSettings(), {
      fetcher,
      templateInterpolate: (raw) => {
        interpolateCalls += 1;
        return raw;
      },
    });
    // The DataSourceApi contract: Grafana invokes applyTemplateVariables once
    // per target before query(). Calling query() directly (without going
    // through applyTemplateVariables first) must NOT trigger a second pass.
    await ds.query(
      makeRequest([
        { refId: 'A', queryType: 'temporal-histogram', collection: '$col' } as MddbQuery,
      ]),
    );
    expect(interpolateCalls).toBe(0);
    expect(calls[0].body).toMatchObject({ collection: '$col' });
  });

  it('query() propagates errors from the client', async () => {
    const { fetcher } = makeFetcher({
      '/v1/temporal/histogram': { status: 500, data: null },
    });
    const ds = new MddbDataSource(makeSettings(), { fetcher });
    await expect(
      ds.query(makeRequest([{ refId: 'A', queryType: 'temporal-histogram' } as MddbQuery])),
    ).rejects.toThrow(/HTTP 500/);
  });

  it('testDatasource maps ping into Grafana status', async () => {
    const ok = new MddbDataSource(makeSettings(), {
      fetcher: makeFetcher({ '/v1/stats': { status: 200, data: {} } }).fetcher,
    });
    await expect(ok.testDatasource()).resolves.toEqual({
      status: 'success',
      message: 'Connected to MDDB.',
    });

    const bad = new MddbDataSource(makeSettings({}, { apiKey: true }), {
      fetcher: makeFetcher({ '/v1/stats': { status: 401, data: {} } }).fetcher,
    });
    const res = await bad.testDatasource();
    expect(res.status).toBe('error');
    expect(res.message).toMatch(/Authentication/);
  });
});
