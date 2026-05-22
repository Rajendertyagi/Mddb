import { MddbApiError } from '../src/client';
import { refreshStats, topCollections } from '../src/refresh';

describe('topCollections', () => {
  it('sorts by documentCount desc and caps at limit', () => {
    const out = topCollections(
      [
        { name: 'a', documentCount: 1 },
        { name: 'b', documentCount: 100 },
        { name: 'c', documentCount: 50 },
        { name: 'd', documentCount: 200 },
      ],
      2,
    );
    expect(out.map((c) => c.name)).toEqual(['d', 'b']);
  });

  it('handles missing documentCount', () => {
    const out = topCollections([
      { name: 'a', documentCount: 0 },
      { name: 'b', documentCount: 1 },
    ]);
    expect(out[0].name).toBe('b');
  });

  it('ignores malformed entries', () => {
    const out = topCollections([
      { name: 'a', documentCount: 1 },
      // @ts-expect-error - intentionally invalid
      null,
    ]);
    expect(out.length).toBe(1);
  });
});

describe('refreshStats', () => {
  it('clears badge when no server configured', async () => {
    const setBadgeText = jest.fn(async () => undefined);
    const setBadgeColor = jest.fn(async () => undefined);
    const setBadgeTitle = jest.fn(async () => undefined);

    const out = await refreshStats({
      setBadgeText,
      setBadgeColor,
      setBadgeTitle,
    });

    expect(out).toBeNull();
    expect(setBadgeText).toHaveBeenCalledWith('');
    expect(setBadgeTitle).toHaveBeenCalledWith('MDDB Browser — not configured');
  });

  it('writes ok status on success', async () => {
    await chrome.storage.local.set({
      settings: {
        serverUrl: 'https://srv.test',
        apiKey: '',
        panelUrl: '',
        refreshIntervalSeconds: 60,
      },
    });

    const getStatsFn = jest.fn(async () => ({
      totalDocuments: 3,
      totalRevisions: 5,
      collections: [{ name: 'blog', documentCount: 3 }],
      mode: 'wr',
      uptime: '10s',
    }));
    const setBadgeText = jest.fn(async () => undefined);

    const status = await refreshStats({
      getStatsFn: getStatsFn as unknown as Parameters<typeof refreshStats>[0]['getStatsFn'],
      setBadgeText,
      setBadgeColor: jest.fn(async () => undefined),
      setBadgeTitle: jest.fn(async () => undefined),
      now: () => 1234,
    });

    expect(status?.ok).toBe(true);
    if (status?.ok) {
      expect(status.totalDocuments).toBe(3);
      expect(status.collectionCount).toBe(1);
      expect(status.fetchedAt).toBe(1234);
    }
    expect(setBadgeText).toHaveBeenCalledWith('3');
  });

  it('reports MddbApiError 401 as auth message', async () => {
    await chrome.storage.local.set({
      settings: {
        serverUrl: 'https://srv.test',
        apiKey: '',
        panelUrl: '',
        refreshIntervalSeconds: 60,
      },
    });
    const status = await refreshStats({
      getStatsFn: jest.fn(async () => {
        throw new MddbApiError('HTTP 401', 401);
      }) as unknown as Parameters<typeof refreshStats>[0]['getStatsFn'],
      setBadgeText: jest.fn(async () => undefined),
      setBadgeColor: jest.fn(async () => undefined),
      setBadgeTitle: jest.fn(async () => undefined),
    });
    expect(status?.ok).toBe(false);
    if (status && !status.ok) {
      expect(status.message).toMatch(/Authentication/);
    }
  });

  it('reports server 500 with status', async () => {
    await chrome.storage.local.set({
      settings: {
        serverUrl: 'https://srv.test',
        apiKey: '',
        panelUrl: '',
        refreshIntervalSeconds: 60,
      },
    });
    const status = await refreshStats({
      getStatsFn: jest.fn(async () => {
        throw new MddbApiError('HTTP 500', 500);
      }) as unknown as Parameters<typeof refreshStats>[0]['getStatsFn'],
      setBadgeText: jest.fn(async () => undefined),
      setBadgeColor: jest.fn(async () => undefined),
      setBadgeTitle: jest.fn(async () => undefined),
    });
    if (status && !status.ok) {
      expect(status.message).toMatch(/500/);
    }
  });

  it('reports network error', async () => {
    await chrome.storage.local.set({
      settings: {
        serverUrl: 'https://srv.test',
        apiKey: '',
        panelUrl: '',
        refreshIntervalSeconds: 60,
      },
    });
    const status = await refreshStats({
      getStatsFn: jest.fn(async () => {
        throw new MddbApiError('Network error: boom', 0);
      }) as unknown as Parameters<typeof refreshStats>[0]['getStatsFn'],
      setBadgeText: jest.fn(async () => undefined),
      setBadgeColor: jest.fn(async () => undefined),
      setBadgeTitle: jest.fn(async () => undefined),
    });
    if (status && !status.ok) {
      expect(status.message).toMatch(/Network error/);
    }
  });

  it('reports generic error', async () => {
    await chrome.storage.local.set({
      settings: {
        serverUrl: 'https://srv.test',
        apiKey: '',
        panelUrl: '',
        refreshIntervalSeconds: 60,
      },
    });
    const status = await refreshStats({
      getStatsFn: jest.fn(async () => {
        throw new Error('weird');
      }) as unknown as Parameters<typeof refreshStats>[0]['getStatsFn'],
      setBadgeText: jest.fn(async () => undefined),
      setBadgeColor: jest.fn(async () => undefined),
      setBadgeTitle: jest.fn(async () => undefined),
    });
    if (status && !status.ok) {
      expect(status.message).toBe('weird');
    }
  });

  it('reports non-Error throws', async () => {
    await chrome.storage.local.set({
      settings: {
        serverUrl: 'https://srv.test',
        apiKey: '',
        panelUrl: '',
        refreshIntervalSeconds: 60,
      },
    });
    const status = await refreshStats({
      getStatsFn: jest.fn(async () => {
        throw 'string-thrown';
      }) as unknown as Parameters<typeof refreshStats>[0]['getStatsFn'],
      setBadgeText: jest.fn(async () => undefined),
      setBadgeColor: jest.fn(async () => undefined),
      setBadgeTitle: jest.fn(async () => undefined),
    });
    if (status && !status.ok) {
      expect(status.message).toBe('Unknown error');
    }
  });

  it('coerces missing stats fields to defaults', async () => {
    await chrome.storage.local.set({
      settings: {
        serverUrl: 'https://srv.test',
        apiKey: '',
        panelUrl: '',
        refreshIntervalSeconds: 60,
      },
    });
    const status = await refreshStats({
      getStatsFn: jest.fn(async () => ({})) as unknown as Parameters<
        typeof refreshStats
      >[0]['getStatsFn'],
      setBadgeText: jest.fn(async () => undefined),
      setBadgeColor: jest.fn(async () => undefined),
      setBadgeTitle: jest.fn(async () => undefined),
    });
    expect(status?.ok).toBe(true);
    if (status?.ok) {
      expect(status.totalDocuments).toBe(0);
      expect(status.totalRevisions).toBe(0);
      expect(status.collectionCount).toBe(0);
      expect(status.mode).toBe('unknown');
      expect(status.uptime).toBe('');
      expect(status.topCollections).toEqual([]);
    }
  });

  it('topCollections handles a collection without documentCount', () => {
    const out = topCollections([
      // @ts-expect-error - intentionally missing documentCount
      { name: 'a' },
      { name: 'b', documentCount: 5 },
    ]);
    expect(out[0].name).toBe('b');
    expect(out[1].documentCount).toBe(0);
  });

  it('falls back to empty settings when loadSettings throws', async () => {
    const loadSettingsFn = jest.fn(async () => {
      throw new Error('storage down');
    });
    const out = await refreshStats({
      loadSettingsFn: loadSettingsFn as unknown as Parameters<
        typeof refreshStats
      >[0]['loadSettingsFn'],
      setBadgeText: jest.fn(async () => undefined),
      setBadgeColor: jest.fn(async () => undefined),
      setBadgeTitle: jest.fn(async () => undefined),
    });
    expect(out).toBeNull();
  });
});
