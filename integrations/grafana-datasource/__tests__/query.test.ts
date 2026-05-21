import { buildRequest, clampLimit, resolveCollection } from '../src/query';
import type { MddbQuery } from '../src/types';

const RANGE = { fromSec: 1_700_000_000, toSec: 1_800_000_000 };
const Q = (extra: Partial<MddbQuery>): MddbQuery =>
  ({ refId: 'A', queryType: 'temporal-histogram', ...extra }) as MddbQuery;

describe('resolveCollection', () => {
  it('uses the query collection when provided', () => {
    expect(resolveCollection({ refId: 'A', collection: 'blog' } as MddbQuery)).toBe('blog');
  });
  it('falls back to the datasource default', () => {
    expect(resolveCollection({ refId: 'A' } as MddbQuery, 'docs')).toBe('docs');
  });
  it('throws when both are empty', () => {
    expect(() => resolveCollection({ refId: 'A' } as MddbQuery)).toThrow(/collection/);
    expect(() => resolveCollection({ refId: 'A', collection: '   ' } as MddbQuery)).toThrow();
  });
});

describe('clampLimit', () => {
  it('falls back to default for invalid/empty inputs', () => {
    expect(clampLimit(undefined, 10)).toBe(10);
    expect(clampLimit(0, 10)).toBe(10);
    expect(clampLimit(Number.NaN, 10)).toBe(10);
    expect(clampLimit(-5, 10)).toBe(10);
  });
  it('clamps to [1, max] and floors', () => {
    expect(clampLimit(5.9, 10)).toBe(5);
    expect(clampLimit(99999, 10, 1000)).toBe(1000);
    expect(clampLimit(1, 10)).toBe(1);
  });
});

describe('buildRequest', () => {
  it('builds /v1/stats for stats queries without needing a collection', () => {
    const req = buildRequest(Q({ queryType: 'stats' }), RANGE);
    expect(req).toEqual({ path: '/v1/stats', body: {} });
  });

  it('builds /v1/temporal/histogram with defaults', () => {
    const req = buildRequest(Q({ collection: 'blog' }), RANGE);
    expect(req.path).toBe('/v1/temporal/histogram');
    expect(req.body).toEqual({
      collection: 'blog',
      eventType: 'access',
      interval: 'day',
      from: RANGE.fromSec,
      to: RANGE.toSec,
    });
  });

  it('uses datasource default collection when query omits it', () => {
    const req = buildRequest(
      Q({ queryType: 'temporal-histogram', eventType: 'create', interval: 'week' }),
      RANGE,
      { collection: 'docs' },
    );
    expect(req.body).toMatchObject({ collection: 'docs', eventType: 'create', interval: 'week' });
  });

  it('builds /v1/temporal/hot with clamped topN', () => {
    const req = buildRequest(
      Q({ queryType: 'temporal-hot', collection: 'docs', topN: 999999 }),
      RANGE,
    );
    expect(req.path).toBe('/v1/temporal/hot');
    expect(req.body).toEqual({ collection: 'docs', topN: 1000, since: RANGE.fromSec });
  });

  it('builds /v1/aggregate when facetKey is set', () => {
    const req = buildRequest(
      Q({ queryType: 'aggregate', collection: 'blog', facetKey: 'tags', topN: 50 }),
      RANGE,
    );
    expect(req).toEqual({
      path: '/v1/aggregate',
      body: { collection: 'blog', key: 'tags', topN: 50 },
    });
  });

  it('throws when aggregate has no facetKey', () => {
    expect(() =>
      buildRequest(Q({ queryType: 'aggregate', collection: 'blog' }), RANGE),
    ).toThrow(/facetKey/);
  });

  it('builds /v1/fts when query string is set', () => {
    const req = buildRequest(
      Q({ queryType: 'fts', collection: 'blog', query: 'machine learning', topN: 5 }),
      RANGE,
    );
    expect(req).toEqual({
      path: '/v1/fts',
      body: { collection: 'blog', query: 'machine learning', limit: 5 },
    });
  });

  it('throws when fts has no query', () => {
    expect(() =>
      buildRequest(Q({ queryType: 'fts', collection: 'blog' }), RANGE),
    ).toThrow(/query/);
  });

  it('throws for unsupported query types', () => {
    expect(() =>
      buildRequest(Q({ queryType: 'nope' as MddbQuery['queryType'], collection: 'x' }), RANGE),
    ).toThrow(/Unsupported/);
  });
});
