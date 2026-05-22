import { getStats, MddbApiError } from './client';
import { CachedStatus, EMPTY_SETTINGS, Settings, loadSettings, saveStatus } from './storage';
import { formatBadge } from './format';

export interface RefreshDeps {
  loadSettingsFn?: typeof loadSettings;
  saveStatusFn?: typeof saveStatus;
  getStatsFn?: typeof getStats;
  setBadgeText?: (text: string) => Promise<void>;
  setBadgeColor?: (color: string) => Promise<void>;
  setBadgeTitle?: (title: string) => Promise<void>;
  now?: () => number;
}

export function topCollections(
  collections: Array<{ name: string; documentCount: number }>,
  limit = 5,
): Array<{ name: string; documentCount: number }> {
  return [...collections]
    .filter((c) => c && typeof c.name === 'string')
    .sort((a, b) => (b.documentCount ?? 0) - (a.documentCount ?? 0))
    .slice(0, limit)
    .map((c) => ({ name: c.name, documentCount: c.documentCount ?? 0 }));
}

async function applyBadge(
  deps: RefreshDeps,
  text: string,
  color: string,
  title: string,
): Promise<void> {
  if (deps.setBadgeText) await deps.setBadgeText(text);
  if (deps.setBadgeColor) await deps.setBadgeColor(color);
  if (deps.setBadgeTitle) await deps.setBadgeTitle(title);
}

export async function refreshStats(deps: RefreshDeps = {}): Promise<CachedStatus | null> {
  const loadFn = deps.loadSettingsFn ?? loadSettings;
  const saveFn = deps.saveStatusFn ?? saveStatus;
  const statsFn = deps.getStatsFn ?? getStats;
  const now = deps.now ?? Date.now;

  let settings: Settings;
  try {
    settings = await loadFn();
  } catch {
    settings = { ...EMPTY_SETTINGS };
  }

  if (!settings.serverUrl) {
    await applyBadge(deps, '', '#666666', 'MDDB Browser — not configured');
    return null;
  }

  try {
    const stats = await statsFn({
      baseUrl: settings.serverUrl,
      apiKey: settings.apiKey || undefined,
    });
    const status: CachedStatus = {
      ok: true,
      fetchedAt: now(),
      totalDocuments: stats.totalDocuments ?? 0,
      totalRevisions: stats.totalRevisions ?? 0,
      collectionCount: stats.collections?.length ?? 0,
      mode: stats.mode ?? 'unknown',
      uptime: stats.uptime ?? '',
      topCollections: topCollections(stats.collections ?? []),
    };
    await saveFn(status);
    await applyBadge(
      deps,
      formatBadge(status.totalDocuments),
      '#1f7a8c',
      `MDDB — ${status.totalDocuments} document(s) across ${status.collectionCount} collection(s)`,
    );
    return status;
  } catch (err) {
    const message =
      err instanceof MddbApiError
        ? err.status === 401 || err.status === 403
          ? 'Authentication failed — check your API key.'
          : err.status > 0
            ? `Server returned ${err.status}.`
            : err.message
        : err instanceof Error
          ? err.message
          : 'Unknown error';
    const status: CachedStatus = { ok: false, fetchedAt: now(), message };
    await saveFn(status);
    await applyBadge(deps, '!', '#9b1e1e', `MDDB error: ${message}`);
    return status;
  }
}
