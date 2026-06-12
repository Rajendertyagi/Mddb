import React, { type ChangeEvent, type ReactElement } from 'react';
import { InlineField, Input, Select } from '@grafana/ui';
import type { QueryEditorProps, SelectableValue } from '@grafana/data';
import { MddbDataSource } from '../datasource';
import type {
  MddbDataSourceOptions,
  MddbEventType,
  MddbHistogramInterval,
  MddbQuery,
  MddbQueryType,
} from '../types';

type Props = QueryEditorProps<MddbDataSource, MddbQuery, MddbDataSourceOptions>;

const QUERY_TYPES: Array<SelectableValue<MddbQueryType>> = [
  { value: 'temporal-histogram', label: 'Temporal histogram (time-series)' },
  { value: 'temporal-hot', label: 'Hot documents (table)' },
  { value: 'aggregate', label: 'Metadata aggregate (facet/histogram)' },
  { value: 'fts', label: 'Full-text search (table)' },
  { value: 'stats', label: 'Database stats (table)' },
];

const INTERVALS: Array<SelectableValue<MddbHistogramInterval>> = [
  { value: 'day', label: 'Day' },
  { value: 'week', label: 'Week' },
  { value: 'month', label: 'Month' },
];

const EVENT_TYPES: Array<SelectableValue<MddbEventType>> = [
  { value: 'access', label: 'Access' },
  { value: 'create', label: 'Create' },
  { value: 'update', label: 'Update' },
];

const LABEL_WIDTH = 18;
const INPUT_WIDTH = 30;

export function QueryEditor(props: Props): ReactElement {
  const { query, onChange, onRunQuery } = props;

  const update = (patch: Partial<MddbQuery>) => {
    onChange({ ...query, ...patch });
  };
  const runOnBlur = () => onRunQuery();
  const onText = (field: keyof MddbQuery) => (event: ChangeEvent<HTMLInputElement>) => {
    update({ [field]: event.target.value } as Partial<MddbQuery>);
  };
  const onNumber = (field: keyof MddbQuery) => (event: ChangeEvent<HTMLInputElement>) => {
    const n = Number.parseInt(event.target.value, 10);
    update({ [field]: Number.isFinite(n) ? n : undefined } as Partial<MddbQuery>);
  };

  const queryType = query.queryType ?? 'temporal-histogram';
  const showCollection = queryType !== 'stats';
  const showInterval = queryType === 'temporal-histogram';
  const showEventType = queryType === 'temporal-histogram';
  const showLimit = queryType !== 'stats';
  const showFacetKey = queryType === 'aggregate';
  const showQueryString = queryType === 'fts';

  return (
    <div data-testid="mddb-query-editor">
      <InlineField label="Query type" labelWidth={LABEL_WIDTH}>
        <Select
          width={INPUT_WIDTH}
          options={QUERY_TYPES}
          value={QUERY_TYPES.find((o) => o.value === queryType) ?? QUERY_TYPES[0]}
          onChange={(opt) => {
            update({ queryType: (opt.value ?? 'temporal-histogram') as MddbQueryType });
            onRunQuery();
          }}
        />
      </InlineField>

      {showCollection && (
        <InlineField label="Collection" labelWidth={LABEL_WIDTH}>
          <Input
            width={INPUT_WIDTH}
            value={query.collection ?? ''}
            placeholder="leave empty to use datasource default"
            onChange={onText('collection')}
            onBlur={runOnBlur}
          />
        </InlineField>
      )}

      {showInterval && (
        <InlineField label="Interval" labelWidth={LABEL_WIDTH}>
          <Select
            width={INPUT_WIDTH}
            options={INTERVALS}
            value={INTERVALS.find((o) => o.value === (query.interval ?? 'day'))}
            onChange={(opt) => {
              update({ interval: (opt.value ?? 'day') as MddbHistogramInterval });
              onRunQuery();
            }}
          />
        </InlineField>
      )}

      {showEventType && (
        <InlineField label="Event type" labelWidth={LABEL_WIDTH}>
          <Select
            width={INPUT_WIDTH}
            options={EVENT_TYPES}
            value={EVENT_TYPES.find((o) => o.value === (query.eventType ?? 'access'))}
            onChange={(opt) => {
              update({ eventType: (opt.value ?? 'access') as MddbEventType });
              onRunQuery();
            }}
          />
        </InlineField>
      )}

      {showFacetKey && (
        <InlineField label="Facet key" labelWidth={LABEL_WIDTH}>
          <Input
            width={INPUT_WIDTH}
            value={query.facetKey ?? ''}
            placeholder="tags"
            onChange={onText('facetKey')}
            onBlur={runOnBlur}
          />
        </InlineField>
      )}

      {showQueryString && (
        <InlineField label="Query" labelWidth={LABEL_WIDTH}>
          <Input
            width={INPUT_WIDTH}
            value={query.query ?? ''}
            placeholder="machine learning"
            onChange={onText('query')}
            onBlur={runOnBlur}
          />
        </InlineField>
      )}

      {showLimit && (
        <InlineField label="Limit / topN" labelWidth={LABEL_WIDTH}>
          <Input
            width={INPUT_WIDTH}
            type="number"
            value={query.topN ?? ''}
            placeholder="10"
            onChange={onNumber('topN')}
            onBlur={runOnBlur}
          />
        </InlineField>
      )}
    </div>
  );
}
