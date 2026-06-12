import type { MddbQuery, MddbQueryType } from './types';

export interface TimeRangeSeconds {
  fromSec: number;
  toSec: number;
}

export interface BuiltRequest {
  path: string;
  body: Record<string, unknown>;
}

/** Resolve the effective collection, falling back to a datasource default. */
export function resolveCollection(query: MddbQuery, fallback?: string): string {
  const c = (query.collection ?? '').trim() || (fallback ?? '').trim();
  if (!c) {
    throw new Error('Query is missing "collection" and no defaultCollection is configured.');
  }
  return c;
}

/** Clamp topN/limit to a sensible range. */
export function clampLimit(value: number | undefined, fallback: number, max = 1000): number {
  const n = Number.isFinite(value) && (value as number) > 0 ? (value as number) : fallback;
  return Math.min(Math.max(1, Math.floor(n)), max);
}

/** Build the MDDB HTTP request (path + JSON body) for a query. */
export function buildRequest(
  query: MddbQuery,
  range: TimeRangeSeconds,
  defaults: { collection?: string; language?: string } = {},
): BuiltRequest {
  const queryType: MddbQueryType = query.queryType ?? 'temporal-histogram';

  if (queryType === 'stats') {
    return { path: '/v1/stats', body: {} };
  }

  const collection = resolveCollection(query, defaults.collection);

  if (queryType === 'temporal-histogram') {
    return {
      path: '/v1/temporal/histogram',
      body: {
        collection,
        eventType: query.eventType ?? 'access',
        interval: query.interval ?? 'day',
        from: range.fromSec,
        to: range.toSec,
      },
    };
  }

  if (queryType === 'temporal-hot') {
    return {
      path: '/v1/temporal/hot',
      body: {
        collection,
        topN: clampLimit(query.topN, 10, 1000),
        since: range.fromSec,
      },
    };
  }

  if (queryType === 'aggregate') {
    const key = (query.facetKey ?? '').trim();
    if (!key) {
      throw new Error('Aggregate query requires "facetKey".');
    }
    return {
      path: '/v1/aggregate',
      body: {
        collection,
        key,
        topN: clampLimit(query.topN, 25, 1000),
      },
    };
  }

  if (queryType === 'fts') {
    const q = (query.query ?? '').trim();
    if (!q) {
      throw new Error('FTS query requires "query".');
    }
    return {
      path: '/v1/fts',
      body: {
        collection,
        query: q,
        limit: clampLimit(query.topN, 25, 1000),
      },
    };
  }

  throw new Error(`Unsupported query type: ${String(queryType)}`);
}
