/// Minimal markdown to HTML renderer
/// Supports: bold, italic, inline code, code blocks, links, line breaks
export function renderMarkdown(text: string): string {
  let html = escapeHtml(text);

  // Code blocks: ```...```
  html = html.replace(/```(\w*)\n([\s\S]*?)```/g, (_match, _lang, code) => {
    return `<pre><code>${code.trim()}</code></pre>`;
  });

  // Inline code: `...`
  html = html.replace(/`([^`]+)`/g, '<code>$1</code>');

  // Bold: **...**
  html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');

  // Italic: *...*
  html = html.replace(/(?<!\*)\*(?!\*)(.+?)(?<!\*)\*(?!\*)/g, '<em>$1</em>');

  // Links: [text](url) — only emit an anchor for safe schemes (FE-003).
  // An unsafe scheme (javascript:, data:, vbscript:, blob:, …) would otherwise
  // produce a clickable XSS link in the host page's origin, since the message
  // content comes from the server/LLM (prompt injection). escapeHtml already
  // ran, so the URL can't break out of the href attribute — only the scheme
  // is the risk — hence an allowlist that drops the link but keeps the text.
  html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_match, label, url) => {
    if (!isSafeUrl(url)) {
      return label;
    }
    return `<a href="${url}" target="_blank" rel="noopener noreferrer">${label}</a>`;
  });

  // Line breaks
  html = html.replace(/\n/g, '<br>');

  return html;
}

const SAFE_SCHEME_RE = /^(https?|mailto):/i;

/// isSafeUrl allows only http(s)/mailto URLs or relative references; every
/// other scheme is rejected. Whitespace and control characters are stripped
/// first because browsers ignore them inside a URL scheme (so `java\tscript:`
/// would otherwise slip through).
export function isSafeUrl(raw: string): boolean {
  const url = raw.replace(/[\u0000-\u0020]+/g, "");
  if (
    url.startsWith('/') ||
    url.startsWith('#') ||
    url.startsWith('./') ||
    url.startsWith('../')
  ) {
    return true;
  }
  return SAFE_SCHEME_RE.test(url);
}

function escapeHtml(text: string): string {
  const map: Record<string, string> = {
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#039;',
  };
  return text.replace(/[&<>"']/g, (ch) => map[ch] || ch);
}
