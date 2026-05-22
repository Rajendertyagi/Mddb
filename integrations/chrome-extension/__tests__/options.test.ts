import { MddbApiError } from '../src/client';
import { buildSettings, getElements, init, parseRefresh } from '../src/options';

const optionsHtml = `
  <form id="settings-form">
    <input id="mddb-url" />
    <input id="api-key" />
    <input id="panel-url" />
    <input id="refresh-interval" />
    <button id="test-button" type="button">test</button>
    <p id="status-message"></p>
  </form>
`;

describe('parseRefresh', () => {
  it.each([
    ['', 60],
    ['0', 0],
    ['-5', 0],
    ['10', 30],
    ['90', 90],
    ['99999', 3600],
    ['not-a-number', 60],
  ])('parses %s -> %d', (input, expected) => {
    expect(parseRefresh(input)).toBe(expected);
  });
});

describe('buildSettings', () => {
  beforeEach(() => {
    document.body.innerHTML = optionsHtml;
  });

  it('normalizes URLs and trims fields', () => {
    const els = getElements();
    els.url.value = ' https://srv.test/ ';
    els.apiKey.value = '  mk_secret  ';
    els.panel.value = 'https://panel.test/';
    els.refresh.value = '120';
    const out = buildSettings(els);
    expect(out.serverUrl).toBe('https://srv.test');
    expect(out.apiKey).toBe('mk_secret');
    expect(out.panelUrl).toBe('https://panel.test');
    expect(out.refreshIntervalSeconds).toBe(120);
  });

  it('handles empty panel URL', () => {
    const els = getElements();
    els.url.value = 'https://srv.test';
    els.refresh.value = '60';
    const out = buildSettings(els);
    expect(out.panelUrl).toBe('');
  });
});

describe('options init', () => {
  beforeEach(() => {
    document.body.innerHTML = optionsHtml;
  });

  it('saves settings on submit', async () => {
    await init();
    const els = getElements();
    els.url.value = 'https://srv.test';
    els.refresh.value = '60';

    chrome.permissions.request = jest.fn(
      async () => true,
    ) as unknown as typeof chrome.permissions.request;
    els.form.dispatchEvent(new Event('submit', { cancelable: true }));
    await new Promise((r) => setTimeout(r, 0));
    expect(els.status.textContent).toMatch(/Settings saved/);
  });

  it('reports permission denial', async () => {
    await init();
    const els = getElements();
    els.url.value = 'https://srv.test';
    els.refresh.value = '60';
    chrome.permissions.request = jest.fn(
      async () => false,
    ) as unknown as typeof chrome.permissions.request;
    els.form.dispatchEvent(new Event('submit', { cancelable: true }));
    await new Promise((r) => setTimeout(r, 0));
    expect(els.status.textContent).toMatch(/Permission denied/);
  });

  it('handles permission promise rejection', async () => {
    await init();
    const els = getElements();
    els.url.value = 'https://srv.test';
    els.refresh.value = '60';
    chrome.permissions.request = jest.fn(async () => {
      throw new Error('nope');
    }) as unknown as typeof chrome.permissions.request;
    els.form.dispatchEvent(new Event('submit', { cancelable: true }));
    await new Promise((r) => setTimeout(r, 0));
    expect(els.status.textContent).toMatch(/Permission denied/);
  });

  it('reports validation error on submit', async () => {
    await init();
    const els = getElements();
    els.url.value = 'not a url';
    els.form.dispatchEvent(new Event('submit', { cancelable: true }));
    await new Promise((r) => setTimeout(r, 0));
    expect(els.status.textContent).toMatch(/valid absolute URL/);
  });

  it('tests a connection successfully', async () => {
    await init();
    const els = getElements();
    els.url.value = 'https://srv.test';
    els.refresh.value = '60';
    const originalFetch = globalThis.fetch;
    globalThis.fetch = jest.fn(
      async () =>
        new Response(JSON.stringify({ status: 'healthy', mode: 'wr' }), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        }),
    ) as unknown as typeof fetch;
    els.testButton.click();
    await new Promise((r) => setTimeout(r, 0));
    expect(els.status.textContent).toMatch(/OK/);
    globalThis.fetch = originalFetch;
  });

  it('reports 401 from test connection', async () => {
    await init();
    const els = getElements();
    els.url.value = 'https://srv.test';
    els.refresh.value = '60';
    const originalFetch = globalThis.fetch;
    globalThis.fetch = jest.fn(
      async () => new Response('', { status: 401 }),
    ) as unknown as typeof fetch;
    els.testButton.click();
    await new Promise((r) => setTimeout(r, 0));
    expect(els.status.textContent).toMatch(/authentication failed/);
    globalThis.fetch = originalFetch;
  });

  it('reports server error from test connection', async () => {
    await init();
    const els = getElements();
    els.url.value = 'https://srv.test';
    els.refresh.value = '60';
    const originalFetch = globalThis.fetch;
    globalThis.fetch = jest.fn(
      async () => new Response('', { status: 500 }),
    ) as unknown as typeof fetch;
    els.testButton.click();
    await new Promise((r) => setTimeout(r, 0));
    expect(els.status.textContent).toMatch(/500/);
    globalThis.fetch = originalFetch;
  });

  it('reports network error from test connection', async () => {
    await init();
    const els = getElements();
    els.url.value = 'https://srv.test';
    els.refresh.value = '60';
    const originalFetch = globalThis.fetch;
    globalThis.fetch = jest.fn(async () => {
      throw new MddbApiError('Network error: boom', 0);
    }) as unknown as typeof fetch;
    els.testButton.click();
    await new Promise((r) => setTimeout(r, 0));
    expect(els.status.textContent).toMatch(/Network|boom/i);
    globalThis.fetch = originalFetch;
  });

  it('reports unknown error from test connection', async () => {
    await init();
    const els = getElements();
    els.url.value = 'not a url';
    els.testButton.click();
    await new Promise((r) => setTimeout(r, 0));
    expect(els.status.textContent).toMatch(/valid absolute URL/);
  });

  it('getElements throws when DOM nodes missing', () => {
    document.body.innerHTML = '';
    expect(() => getElements()).toThrow(/Missing element/);
  });
});
