import React, { type ChangeEvent } from 'react';
import { InlineField, Input, SecretInput } from '@grafana/ui';
import type { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import type { MddbDataSourceOptions, MddbSecureJsonData } from '../types';

type Props = DataSourcePluginOptionsEditorProps<MddbDataSourceOptions, MddbSecureJsonData>;

const LABEL_WIDTH = 24;
const INPUT_WIDTH = 50;

export function ConfigEditor(props: Props): JSX.Element {
  const { options, onOptionsChange } = props;
  const jsonData = options.jsonData ?? {};
  const secureJsonFields = options.secureJsonFields ?? {};
  const secureJsonData = (options.secureJsonData ?? {}) as MddbSecureJsonData;

  const onJsonChange = (field: keyof MddbDataSourceOptions) =>
    (event: ChangeEvent<HTMLInputElement>) => {
      onOptionsChange({ ...options, jsonData: { ...jsonData, [field]: event.target.value } });
    };

  const onApiKeyChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      secureJsonData: { ...secureJsonData, apiKey: event.target.value },
    });
  };

  const onApiKeyReset = () => {
    onOptionsChange({
      ...options,
      secureJsonFields: { ...secureJsonFields, apiKey: false },
      secureJsonData: { ...secureJsonData, apiKey: '' },
    });
  };

  return (
    <div data-testid="mddb-config-editor">
      <InlineField label="MDDB URL" labelWidth={LABEL_WIDTH} tooltip="Base URL, no trailing slash.">
        <Input
          width={INPUT_WIDTH}
          value={jsonData.url ?? ''}
          placeholder="https://mddb.tradik.com"
          onChange={onJsonChange('url')}
        />
      </InlineField>
      <InlineField label="Default collection" labelWidth={LABEL_WIDTH}>
        <Input
          width={INPUT_WIDTH}
          value={jsonData.defaultCollection ?? ''}
          placeholder="docs"
          onChange={onJsonChange('defaultCollection')}
        />
      </InlineField>
      <InlineField label="Default language" labelWidth={LABEL_WIDTH}>
        <Input
          width={INPUT_WIDTH}
          value={jsonData.defaultLanguage ?? 'en_US'}
          placeholder="en_US"
          onChange={onJsonChange('defaultLanguage')}
        />
      </InlineField>
      <InlineField label="API key" labelWidth={LABEL_WIDTH} tooltip="Bearer token (vk_…).">
        <SecretInput
          width={INPUT_WIDTH}
          isConfigured={Boolean(secureJsonFields.apiKey)}
          value={secureJsonData.apiKey ?? ''}
          placeholder="vk_********************************"
          onChange={onApiKeyChange}
          onReset={onApiKeyReset}
        />
      </InlineField>
    </div>
  );
}
