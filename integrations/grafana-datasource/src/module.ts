import { DataSourcePlugin } from '@grafana/data';
import { MddbDataSource } from './datasource';
import { ConfigEditor } from './components/ConfigEditor';
import { QueryEditor } from './components/QueryEditor';
import type { MddbDataSourceOptions, MddbQuery } from './types';

export const plugin = new DataSourcePlugin<MddbDataSource, MddbQuery, MddbDataSourceOptions>(
  MddbDataSource,
)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor);
