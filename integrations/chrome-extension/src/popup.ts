import { derivePanelUrl } from './url';
import { loadSettings, loadStatus, CachedStatus, Settings } from './storage';
import { PRIVACY_URL, TERMS_URL } from './constants';
import { formatCount } from './format';

interface PopupElements {
  empty: HTMLElement;
  content: HTMLElement;
  error: HTMLElement;
  errorMessage: HTMLElement;
  badge: HTMLElement;
  serverUrl: HTMLElement;
  status: HTMLElement;
  mode: HTMLElement;
  uptime: HTMLElement;
  totalDocuments: HTMLElement;
  totalRevisions: HTMLElement;
  collectionCount: HTMLElement;
  collectionsList: HTMLElement;
  panelLink: HTMLAnchorElement;
  privacyLink: HTMLAnchorElement;
  termsLink: HTMLAnchorElement;
  settingsLink: HTMLAnchorElement;
  refresh: HTMLButtonElement;
  retry: HTMLButtonElement;
  openOptions: HTMLButtonElement;
}

function $(id: string): HTMLElement {
  const el = document.getElementById(id);
  if (!el) throw new Error(`Missing element #${id}`);
  return el;
}

export function getElements(): PopupElements {
  return {
    empty: $('empty-state'),
    content: $('content'),
    error: $('error-state'),
    errorMessage: $('error-message'),
    badge: $('badge'),
    serverUrl: $('server-url'),
    status: $('status'),
    mode: $('mode'),
    uptime: $('uptime'),
    totalDocuments: $('total-documents'),
    totalRevisions: $('total-revisions'),
    collectionCount: $('collection-count'),
    collectionsList: $('collections-list'),
    panelLink: $('panel-link') as HTMLAnchorElement,
    privacyLink: $('privacy-link') as HTMLAnchorElement,
    termsLink: $('terms-link') as HTMLAnchorElement,
    settingsLink: $('settings-link') as HTMLAnchorElement,
    refresh: $('refresh') as HTMLButtonElement,
    retry: $('retry') as HTMLButtonElement,
    openOptions: $('open-options') as HTMLButtonElement,
  };
}

export function renderEmpty(els: PopupElements): void {
  els.empty.hidden = false;
  els.content.hidden = true;
  els.error.hidden = true;
  els.badge.textContent = '--';
  els.badge.className = 'badge badge--unknown';
}

export function renderError(els: PopupElements, message: string): void {
  els.empty.hidden = true;
  els.content.hidden = true;
  els.error.hidden = false;
  els.errorMessage.textContent = message;
  els.badge.textContent = 'err';
  els.badge.className = 'badge badge--err';
}

export function renderStatus(els: PopupElements, settings: Settings, status: CachedStatus): void {
  els.empty.hidden = true;
  els.error.hidden = true;
  els.content.hidden = false;

  els.serverUrl.textContent = settings.serverUrl;
  els.serverUrl.setAttribute('title', settings.serverUrl);
  els.panelLink.href = derivePanelUrl(settings.serverUrl, settings.panelUrl);

  if (!status.ok) {
    renderError(els, status.message);
    return;
  }

  els.status.textContent = 'connected';
  els.mode.textContent = status.mode || 'unknown';
  els.uptime.textContent = status.uptime || '—';
  els.totalDocuments.textContent = formatCount(status.totalDocuments);
  els.totalRevisions.textContent = formatCount(status.totalRevisions);
  els.collectionCount.textContent = formatCount(status.collectionCount);

  els.badge.textContent = 'ok';
  els.badge.className = 'badge badge--ok';

  els.collectionsList.replaceChildren();
  if (status.topCollections.length === 0) {
    const li = document.createElement('li');
    li.textContent = 'No collections yet.';
    els.collectionsList.appendChild(li);
  } else {
    for (const c of status.topCollections) {
      const li = document.createElement('li');
      const name = document.createElement('span');
      name.textContent = c.name;
      const count = document.createElement('span');
      count.textContent = formatCount(c.documentCount);
      li.append(name, count);
      els.collectionsList.appendChild(li);
    }
  }
}

async function requestRefresh(): Promise<CachedStatus | null> {
  return new Promise((resolve) => {
    chrome.runtime.sendMessage({ type: 'mddb:refresh' }, (resp) => {
      if (chrome.runtime.lastError) {
        resolve(null);
        return;
      }
      resolve((resp?.status ?? null) as CachedStatus | null);
    });
  });
}

export async function init(els = getElements()): Promise<void> {
  els.privacyLink.href = PRIVACY_URL;
  els.termsLink.href = TERMS_URL;

  els.settingsLink.addEventListener('click', (e) => {
    e.preventDefault();
    chrome.runtime.openOptionsPage();
  });
  els.openOptions.addEventListener('click', () => chrome.runtime.openOptionsPage());

  const handleRefresh = async () => {
    const status = await requestRefresh();
    if (status) {
      const settings = await loadSettings();
      renderStatus(els, settings, status);
    }
  };
  els.refresh.addEventListener('click', handleRefresh);
  els.retry.addEventListener('click', handleRefresh);

  const settings = await loadSettings();
  if (!settings.serverUrl) {
    renderEmpty(els);
    return;
  }

  let status = await loadStatus();
  if (!status) {
    status = await requestRefresh();
  }
  if (status) {
    renderStatus(els, settings, status);
  } else {
    renderError(els, 'Could not reach MDDB server. Check settings and try again.');
  }
}
