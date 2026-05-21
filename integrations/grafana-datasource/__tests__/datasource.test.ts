import type { DataQueryRequest, DataSourceInstanceSettings, ScopedVars } from '@grafana/data';
import { MddbDataSource } from '../src/datasource';
import type { HttpFetcher } from '../src/client';
import type { MddbDataSourceOptions, MddbQuery, MddbSecureJsonData } from '../src/types';

function makeFetcher(map: Record<string, { status: number; data: unknown }>): {
  fetcher: HttpFetcher;
  calls: Array<{ url: string; body: any }>;
} {
  const calls: Array<{ url: string; body: any }> = [];
  const fetcher: HttpFetcher = async (url, body) => {
    calls.push({ url, body });
    const match = Object.keys(map).find((k) => url.endsWith(k));
    if (!match) {
      return { status: 404, data: null, bodyText: 'no mock' };
    }
    return map[match];
  };
  return { fetcher, calls };
}

function makeSettings(
  opts: Partial<MddbDataSourceOptions> = {},
  secure: Partial<MddbSecureJsonData> = {},
): DataSourceInstanceSettings<MddbDataSourceOptions> {
  return {
    id: 1,
    uid: 'mddb-uid',
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
    // @ts-expect-error - Grafana puts secureJsonData on the instance settings in tests
    secureJsonData: secure,
  };
}

function makeRequest(targets: MddbQuery[], scopedVars: ScopedVars = {}): DataQueryRequest<MddbQuery> {
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
      templateInterpolate: (raw, vars) => `${raw}::${(vars as any)?.tag?.value ?? ''}`,
    });
    const out = ds.applyTemplateVariables(
      { refId: 'A', queryType: 'fts', collection: 'c', query: '$tag', facetKey: 'tags' } as MddbQuery,
      { tag: { text: 't', value: 'rust' } },
    );
    expect(out).toMatchObject({
      collection: 'c::rust',
      query: '$tag::rust',
      facetKey: 'tags::rust',
    });
  });

  it('query() dispatches per target and returns one DataFrame each', async () => {
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
      'http://localhost:11023/v1/temporal/histogram',
      'http://localhost:11023/v1/stats',
    ]);
    expect(calls[0].body).toMatchObject({ collection: 'docs', from: 1_700_000_000, to: 1_800_000_000 });
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

    const bad = new MddbDataSource(makeSettings(), {
      fetcher: makeFetcher({ '/v1/stats': { status: 401, data: {} } }).fetcher,
    });
    const res = await bad.testDatasource();
    expect(res.status).toBe('error');
    expect(res.message).toMatch(/Authentication/);
  });
});
