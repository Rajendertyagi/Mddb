import {
  clearStatus,
  DEFAULT_REFRESH_SECONDS,
  loadSettings,
  loadStatus,
  saveSettings,
  saveStatus,
} from '../src/storage';

describe('storage helpers', () => {
  it('returns empty settings when nothing stored', async () => {
    const s = await loadSettings();
    expect(s.serverUrl).toBe('');
    expect(s.apiKey).toBe('');
    expect(s.panelUrl).toBe('');
    expect(s.refreshIntervalSeconds).toBe(DEFAULT_REFRESH_SECONDS);
  });

  it('round-trips settings', async () => {
    await saveSettings({
      serverUrl: 'https://mddb.example.com',
      apiKey: 'mk_secret',
      panelUrl: 'https://panel.example.com',
      refreshIntervalSeconds: 120,
    });
    const s = await loadSettings();
    expect(s).toEqual({
      serverUrl: 'https://mddb.example.com',
      apiKey: 'mk_secret',
      panelUrl: 'https://panel.example.com',
      refreshIntervalSeconds: 120,
    });
  });

  it('coerces invalid refresh interval', async () => {
    await chrome.storage.local.set({
      settings: {
        serverUrl: 'https://x.test',
        apiKey: '',
        panelUrl: '',
        refreshIntervalSeconds: -10,
      },
    });
    const s = await loadSettings();
    expect(s.refreshIntervalSeconds).toBe(DEFAULT_REFRESH_SECONDS);
  });

  it('round-trips success status', async () => {
    await saveStatus({
      ok: true,
      fetchedAt: 123,
      totalDocuments: 5,
      totalRevisions: 7,
      collectionCount: 1,
      mode: 'wr',
      uptime: '1h',
      topCollections: [{ name: 'blog', documentCount: 5 }],
    });
    const s = await loadStatus();
    expect(s?.ok).toBe(true);
    if (s?.ok) {
      expect(s.totalDocuments).toBe(5);
      expect(s.topCollections[0].name).toBe('blog');
    }
  });

  it('returns null when no status cached', async () => {
    expect(await loadStatus()).toBeNull();
  });

  it('clears cached status', async () => {
    await saveStatus({ ok: false, fetchedAt: 1, message: 'err' });
    await clearStatus();
    expect(await loadStatus()).toBeNull();
  });
});
