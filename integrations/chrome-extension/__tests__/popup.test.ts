import { getElements, init, renderEmpty, renderError, renderStatus } from '../src/popup';
import { saveSettings, saveStatus } from '../src/storage';
import { PRIVACY_URL, TERMS_URL } from '../src/constants';

const popupHtml = `
  <header><img alt="" /></header>
  <span id="badge" class="badge badge--unknown">--</span>
  <section id="empty-state" hidden><button id="open-options" type="button">x</button></section>
  <section id="content" hidden>
    <dd id="server-url" title=""></dd>
    <dd id="status"></dd>
    <dd id="mode"></dd>
    <dd id="uptime"></dd>
    <dd id="total-documents">--</dd>
    <dd id="total-revisions">--</dd>
    <dd id="collection-count">--</dd>
    <ul id="collections-list"></ul>
    <a id="panel-link" href="#"></a>
    <button id="refresh" type="button">r</button>
  </section>
  <section id="error-state" hidden>
    <p id="error-message" role="alert"></p>
    <button id="retry" type="button">retry</button>
  </section>
  <footer>
    <a id="settings-link" href="#">Settings</a>
    <a id="privacy-link" href="#">P</a>
    <a id="terms-link" href="#">T</a>
  </footer>
`;

describe('popup view', () => {
  beforeEach(() => {
    document.body.innerHTML = popupHtml;
  });

  it('renderEmpty shows the empty state and resets badge', () => {
    const els = getElements();
    renderEmpty(els);
    expect(els.empty.hidden).toBe(false);
    expect(els.content.hidden).toBe(true);
    expect(els.badge.className).toContain('badge--unknown');
  });

  it('renderError shows the error state', () => {
    const els = getElements();
    renderError(els, 'oops');
    expect(els.error.hidden).toBe(false);
    expect(els.errorMessage.textContent).toBe('oops');
    expect(els.badge.className).toContain('badge--err');
  });

  it('renderStatus paints success stats', () => {
    const els = getElements();
    renderStatus(
      els,
      {
        serverUrl: 'https://srv.test',
        apiKey: '',
        panelUrl: '',
        refreshIntervalSeconds: 60,
      },
      {
        ok: true,
        fetchedAt: 1,
        totalDocuments: 1234,
        totalRevisions: 2_500_000,
        collectionCount: 3,
        mode: 'wr',
        uptime: '1h',
        topCollections: [
          { name: 'blog', documentCount: 1234 },
          { name: 'docs', documentCount: 30 },
        ],
      },
    );
    expect(els.totalDocuments.textContent).toBe('1.2k');
    expect(els.totalRevisions.textContent).toBe('2.5M');
    expect(els.collectionCount.textContent).toBe('3');
    expect(els.panelLink.href).toContain(':3000');
    expect(els.collectionsList.children.length).toBe(2);
  });

  it('renderStatus renders empty collections placeholder', () => {
    const els = getElements();
    renderStatus(
      els,
      { serverUrl: 'https://srv.test', apiKey: '', panelUrl: '', refreshIntervalSeconds: 60 },
      {
        ok: true,
        fetchedAt: 0,
        totalDocuments: 0,
        totalRevisions: 0,
        collectionCount: 0,
        mode: 'wr',
        uptime: '',
        topCollections: [],
      },
    );
    expect(els.collectionsList.textContent).toMatch(/No collections/);
    expect(els.uptime.textContent).toBe('—');
  });

  it('renderStatus delegates to error when status is not ok', () => {
    const els = getElements();
    renderStatus(
      els,
      { serverUrl: 'https://srv.test', apiKey: '', panelUrl: '', refreshIntervalSeconds: 60 },
      { ok: false, fetchedAt: 0, message: 'bad' },
    );
    expect(els.error.hidden).toBe(false);
    expect(els.errorMessage.textContent).toBe('bad');
  });
});

describe('popup init', () => {
  beforeEach(() => {
    document.body.innerHTML = popupHtml;
  });

  it('shows empty state when no server configured', async () => {
    await init();
    const els = getElements();
    expect(els.empty.hidden).toBe(false);
    expect(els.privacyLink.href).toBe(PRIVACY_URL);
    expect(els.termsLink.href).toBe(TERMS_URL);
  });

  it('renders cached status when available', async () => {
    await saveSettings({
      serverUrl: 'https://srv.test',
      apiKey: '',
      panelUrl: '',
      refreshIntervalSeconds: 60,
    });
    await saveStatus({
      ok: true,
      fetchedAt: 0,
      totalDocuments: 7,
      totalRevisions: 9,
      collectionCount: 1,
      mode: 'wr',
      uptime: '10s',
      topCollections: [{ name: 'blog', documentCount: 7 }],
    });
    await init();
    const els = getElements();
    expect(els.content.hidden).toBe(false);
    expect(els.totalDocuments.textContent).toBe('7');
  });

  it('requests refresh when no status is cached', async () => {
    await saveSettings({
      serverUrl: 'https://srv.test',
      apiKey: '',
      panelUrl: '',
      refreshIntervalSeconds: 60,
    });
    chrome.runtime.sendMessage = jest.fn((_msg: unknown, cb?: (resp: unknown) => void) => {
      if (cb)
        cb({
          status: {
            ok: true,
            fetchedAt: 0,
            totalDocuments: 1,
            totalRevisions: 1,
            collectionCount: 1,
            mode: 'wr',
            uptime: '1s',
            topCollections: [],
          },
        });
    }) as unknown as typeof chrome.runtime.sendMessage;

    await init();
    const els = getElements();
    expect(els.content.hidden).toBe(false);
    expect(els.totalDocuments.textContent).toBe('1');
  });

  it('shows error when refresh fails to return a status', async () => {
    await saveSettings({
      serverUrl: 'https://srv.test',
      apiKey: '',
      panelUrl: '',
      refreshIntervalSeconds: 60,
    });
    chrome.runtime.sendMessage = jest.fn((_msg: unknown, cb?: (resp: unknown) => void) => {
      if (cb) cb({ status: null });
    }) as unknown as typeof chrome.runtime.sendMessage;

    await init();
    const els = getElements();
    expect(els.error.hidden).toBe(false);
  });

  it('handles sendMessage lastError gracefully', async () => {
    await saveSettings({
      serverUrl: 'https://srv.test',
      apiKey: '',
      panelUrl: '',
      refreshIntervalSeconds: 60,
    });
    chrome.runtime.sendMessage = jest.fn((_msg: unknown, cb?: (resp: unknown) => void) => {
      (chrome.runtime as { lastError?: unknown }).lastError = { message: 'down' };
      if (cb) cb(undefined);
    }) as unknown as typeof chrome.runtime.sendMessage;

    await init();
    const els = getElements();
    expect(els.error.hidden).toBe(false);
  });

  it('wires settings, refresh and retry buttons', async () => {
    await init();
    const els = getElements();
    chrome.runtime.openOptionsPage = jest.fn();
    els.settingsLink.click();
    els.openOptions.click();
    expect(chrome.runtime.openOptionsPage).toHaveBeenCalledTimes(2);

    chrome.runtime.sendMessage = jest.fn((_msg: unknown, cb?: (resp: unknown) => void) => {
      if (cb) cb({ status: null });
    }) as unknown as typeof chrome.runtime.sendMessage;
    els.refresh.click();
    els.retry.click();
  });
});

describe('popup helpers', () => {
  it('getElements throws when an id is missing', () => {
    document.body.innerHTML = '';
    expect(() => getElements()).toThrow(/Missing element/);
  });
});
