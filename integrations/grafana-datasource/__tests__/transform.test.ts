import { FieldType } from '@grafana/data';
import { toDataFrame } from '../src/transform';
import type { MddbQuery } from '../src/types';

const Q = (queryType: MddbQuery['queryType']): MddbQuery =>
  ({ refId: 'A', queryType }) as MddbQuery;

function values(frame: ReturnType<typeof toDataFrame>, fieldName: string): unknown[] {
  const f = frame.fields.find((field) => field.name === fieldName);
  if (!f) throw new Error(`field ${fieldName} missing`);
  return f.values.toArray ? f.values.toArray() : (f.values as unknown[]);
}

describe('toDataFrame', () => {
  it('renders temporal-histogram as a time/count time-series and skips malformed buckets', () => {
    const frame = toDataFrame(Q('temporal-histogram'), {
      collection: 'blog',
      eventType: 'access',
      buckets: [
        { from: 1700, to: 1800, count: 5 },
        { from: 1800, to: 1900, count: 12 },
        { from: 'bad', count: 1 } as unknown as { from: number; to: number; count: number },
      ],
    });
    expect(frame.name).toBe('blog / access');
    expect(values(frame, 'time')).toEqual([1_700_000, 1_800_000]);
    expect(values(frame, 'count')).toEqual([5, 12]);
    const time = frame.fields.find((f) => f.name === 'time');
    expect(time?.type).toBe(FieldType.time);
  });

  it('renders temporal-hot as a doc table with last-access timestamps in ms', () => {
    const frame = toDataFrame(Q('temporal-hot'), {
      collection: 'blog',
      entries: [{ docId: 'en|blog|a', accessCount: 5, lastAccessAt: 1700 }],
    });
    expect(values(frame, 'docId')).toEqual(['en|blog|a']);
    expect(values(frame, 'accessCount')).toEqual([5]);
    expect(values(frame, 'lastAccessAt')).toEqual([1_700_000]);
  });

  it('renders aggregate buckets as time-series when buckets are present', () => {
    const frame = toDataFrame(Q('aggregate'), {
      collection: 'blog',
      key: 'tags',
      buckets: [{ from: 100, to: 200, count: 3 }],
    });
    expect(values(frame, 'time')).toEqual([100_000]);
    expect(values(frame, 'count')).toEqual([3]);
  });

  it('renders aggregate values as a string/count table when no buckets', () => {
    const frame = toDataFrame(Q('aggregate'), {
      collection: 'blog',
      key: 'tags',
      values: [
        { value: 'rust', count: 7 },
        { value: 'go', count: 4 },
      ],
    });
    expect(values(frame, 'value')).toEqual(['rust', 'go']);
    expect(values(frame, 'count')).toEqual([7, 4]);
  });

  it('renders FTS results into a four-column table and joins highlights with " … "', () => {
    const frame = toDataFrame(Q('fts'), {
      results: [
        {
          key: 'hello',
          lang: 'en_US',
          score: 0.87,
          highlights: ['hello <em>world</em>', 'second hit'],
        },
      ],
    });
    expect(values(frame, 'key')).toEqual(['hello']);
    expect(values(frame, 'highlight')).toEqual(['hello <em>world</em> … second hit']);
  });

  it('renders stats with per-collection rows sorted by name', () => {
    const frame = toDataFrame(Q('stats'), {
      collections: { blog: { documents: 12, revisions: 30 }, docs: { documents: 5, revisions: 5 } },
      vectorEmbeddings: { docs: 5 },
    });
    expect(values(frame, 'collection')).toEqual(['blog', 'docs']);
    expect(values(frame, 'documents')).toEqual([12, 5]);
    expect(values(frame, 'embeddings')).toEqual([0, 5]);
  });

  it('returns an empty frame for unknown query types and handles empty payloads', () => {
    const empty = toDataFrame(
      { refId: 'A', queryType: 'nope' } as unknown as MddbQuery,
      {},
    );
    expect(empty.fields).toEqual([]);
    const noHist = toDataFrame(Q('temporal-histogram'), {});
    expect(values(noHist, 'time')).toEqual([]);
    const noHot = toDataFrame(Q('temporal-hot'), {});
    expect(values(noHot, 'docId')).toEqual([]);
    const noAgg = toDataFrame(Q('aggregate'), {});
    expect(values(noAgg, 'value')).toEqual([]);
    const noFts = toDataFrame(Q('fts'), {});
    expect(values(noFts, 'key')).toEqual([]);
    const noStats = toDataFrame(Q('stats'), {});
    expect(values(noStats, 'collection')).toEqual([]);
  });
});
