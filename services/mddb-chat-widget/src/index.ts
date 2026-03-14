import { ChatWidget, type WidgetOptions } from './widget';

function init(): void {
  // Find the script tag that loaded us
  const scripts = document.querySelectorAll('script[data-server]');
  const script = scripts[scripts.length - 1] as HTMLScriptElement | undefined;

  if (!script) {
    console.error('[mddb-chat] Missing data-server attribute on script tag');
    return;
  }

  const server = script.getAttribute('data-server');
  if (!server) {
    console.error('[mddb-chat] data-server is required');
    return;
  }

  const options: WidgetOptions = {
    server,
    scenario: script.getAttribute('data-scenario') || 'assistant',
    theme: (script.getAttribute('data-theme') as 'light' | 'dark') || 'light',
    accent: script.getAttribute('data-accent') || undefined,
    position:
      (script.getAttribute('data-position') as 'bottom-right' | 'bottom-left') ||
      'bottom-right',
  };

  // Create host element
  const host = document.createElement('div');
  host.id = 'mddb-chat-widget';
  document.body.appendChild(host);

  // Initialize widget
  const widget = new ChatWidget(host, options);

  // Expose globally for programmatic control
  (window as any).__mddbChat = widget;
}

// Auto-init when DOM is ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', init);
} else {
  init();
}
