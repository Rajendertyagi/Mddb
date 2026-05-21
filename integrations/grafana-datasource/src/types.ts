import type { DataQuery, DataSourceJsonData } from '@grafana/data';

/** Persistent datasource configuration shown in the Grafana datasource settings UI. */
export interface MddbDataSourceOptions extends DataSourceJsonData {
  /** Base URL of the MDDB instance (no trailing slash). */
  url?: string;
  /** Default collection used by queries that don't override it. */
  defaultCollection?: string;
  /** Default document language used by queries that don't override it. */
  defaultLanguage?: string;
}

/** Secret config fields (encrypted at rest by Grafana). */
export interface MddbSecureJsonData {
  apiKey?: string;
}

/** Query types exposed by the MDDB datasource. */
export type MddbQueryType =
  | 'temporal-histogram'
  | 'temporal-hot'
  | 'aggregate'
  | 'fts'
  | 'stats';

export type MddbHistogramInterval = 'day' | 'week' | 'month';
export type MddbEventType = 'create' | 'update' | 'access';

export interface MddbQuery extends DataQuery {
  queryType: MddbQueryType;
  collection?: string;
  /** Histogram interval — used by temporal-histogram. */
  interval?: MddbHistogramInterval;
  /** Event type — used by temporal-histogram and temporal-query. */
  eventType?: MddbEventType;
  /** Limit / topN — used by fts, temporal-hot, aggregate. */
  topN?: number;
  /** FTS query string. */
  query?: string;
  /** Metadata key for /v1/aggregate facet/histogram. */
  facetKey?: string;
}

export const DEFAULT_QUERY: Partial<MddbQuery> = {
  queryType: 'temporal-histogram',
  interval: 'day',
  eventType: 'access',
  topN: 10,
};

/** Shape returned by /v1/temporal/histogram. */
export interface TemporalHistogramResponse {
  collection?: string;
  eventType?: MddbEventType;
  interval?: MddbHistogramInterval;
  buckets: { from: number; to: number; count: number }[];
}

/** Shape returned by /v1/temporal/hot. */
export interface TemporalHotResponse {
  collection?: string;
  entries: {
    docId: string;
    accessCount: number;
    lastAccessAt: number;
    document?: unknown;
  }[];
}

/** Shape returned by /v1/aggregate. */
export interface AggregateResponse {
  collection?: string;
  key?: string;
  values?: { value: string; count: number }[];
  buckets?: { from: number; to: number; count: number }[];
}

/** Shape returned by /v1/fts. */
export interface FtsResponse {
  results: Array<{
    key: string;
    lang: string;
    score: number;
    meta?: Record<string, string[]>;
    highlights?: string[];
  }>;
  total?: number;
}

/** Shape returned by /v1/stats. */
export interface StatsResponse {
  uptimeSeconds?: number;
  databaseSizeBytes?: number;
  collections?: Record<string, { documents: number; revisions: number }>;
  vectorEmbeddings?: Record<string, number>;
}
