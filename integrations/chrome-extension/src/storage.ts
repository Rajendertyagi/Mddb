export interface Settings {
  serverUrl: string;
  apiKey: string;
  panelUrl: string;
  refreshIntervalSeconds: number;
}

export interface CachedStats {
  fetchedAt: number;
  totalDocuments: number;
  totalRevisions: number;
  collectionCount: number;
  mode: string;
  uptime: string;
  topCollections: Array<{ name: string; documentCount: number }>;
  ok: true;
}

export interface CachedError {
  fetchedAt: number;
  ok: false;
  message: string;
}

export type CachedStatus = CachedStats | CachedError;

export const SETTINGS_KEY = 'settings';
export const STATUS_KEY = 'status';
export const DEFAULT_REFRESH_SECONDS = 60;

export const EMPTY_SETTINGS: Settings = {
  serverUrl: '',
  apiKey: '',
  panelUrl: '',
  refreshIntervalSeconds: DEFAULT_REFRESH_SECONDS,
};

export async function loadSettings(): Promise<Settings> {
  const raw = await chrome.storage.local.get(SETTINGS_KEY);
  const stored = (raw[SETTINGS_KEY] ?? {}) as Partial<Settings>;
  return {
    serverUrl: typeof stored.serverUrl === 'string' ? stored.serverUrl : '',
    apiKey: typeof stored.apiKey === 'string' ? stored.apiKey : '',
    panelUrl: typeof stored.panelUrl === 'string' ? stored.panelUrl : '',
    refreshIntervalSeconds:
      typeof stored.refreshIntervalSeconds === 'number' &&
      Number.isFinite(stored.refreshIntervalSeconds) &&
      stored.refreshIntervalSeconds >= 0
        ? stored.refreshIntervalSeconds
        : DEFAULT_REFRESH_SECONDS,
  };
}

export async function saveSettings(settings: Settings): Promise<void> {
  await chrome.storage.local.set({ [SETTINGS_KEY]: settings });
}

export async function loadStatus(): Promise<CachedStatus | null> {
  const raw = await chrome.storage.local.get(STATUS_KEY);
  const cached = raw[STATUS_KEY];
  if (!cached || typeof cached !== 'object') return null;
  return cached as CachedStatus;
}

export async function saveStatus(status: CachedStatus): Promise<void> {
  await chrome.storage.local.set({ [STATUS_KEY]: status });
}

export async function clearStatus(): Promise<void> {
  await chrome.storage.local.remove(STATUS_KEY);
}
