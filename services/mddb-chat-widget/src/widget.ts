import { WsClient } from './ws/client';
import type { WsIncoming } from './ws/protocol';
import { Store, type WidgetState, type Message } from './store/state';
import { saveSession, loadSession, clearSession } from './store/session';
import { sanitizeInput } from './utils/sanitize';
import { renderMarkdown } from './utils/markdown';
import styles from './styles/widget.css?inline';

export interface WidgetOptions {
  server: string;
  scenario?: string;
  theme?: 'light' | 'dark';
  accent?: string;
  position?: 'bottom-right' | 'bottom-left';
}

const CHAT_ICON_SVG = `<svg viewBox="0 0 24 24"><path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm0 14H6l-2 2V4h16v12z"/></svg>`;
const SEND_ICON_SVG = `<svg viewBox="0 0 24 24"><path d="M2.01 21L23 12 2.01 3 2 10l15 2-15 2z"/></svg>`;
const CLOSE_ICON = '\u00D7';

export class ChatWidget {
  private store: Store;
  private ws: WsClient;
  private options: WidgetOptions;
  private root: ShadowRoot;
  private container: HTMLDivElement;

  // DOM references
  private windowEl!: HTMLDivElement;
  private messagesEl!: HTMLDivElement;
  private inputEl!: HTMLTextAreaElement;
  private sendBtn!: HTMLButtonElement;
  private nameFormEl!: HTMLDivElement;
  private chatViewEl!: HTMLDivElement;
  private typingEl!: HTMLDivElement;
  private queueEl!: HTMLDivElement;
  private errorEl!: HTMLDivElement;

  constructor(hostEl: HTMLElement, options: WidgetOptions) {
    this.options = options;
    this.store = new Store();

    // Create shadow DOM
    this.root = hostEl.attachShadow({ mode: 'closed' });
    this.container = document.createElement('div');
    this.root.appendChild(this.container);

    // Inject styles
    const styleEl = document.createElement('style');
    styleEl.textContent = styles;
    this.root.appendChild(styleEl);

    // Apply custom accent
    if (options.accent) {
      this.container.style.setProperty('--mddb-accent', options.accent);
    }

    // WebSocket
    this.ws = new WsClient(
      options.server,
      (msg) => this.handleMessage(msg),
      (connected) => this.store.update({ isConnected: connected }),
    );

    // Subscribe to state changes
    this.store.subscribe((state) => this.render(state));

    // Build UI
    this.buildUI();

    // Try to resume session
    const saved = loadSession();
    if (saved) {
      this.store.update({ userName: saved.userName });
    }
  }

  private buildUI(): void {
    // Chat icon button
    const iconBtn = document.createElement('button');
    iconBtn.className = 'mddb-chat-icon';
    iconBtn.innerHTML = CHAT_ICON_SVG;
    iconBtn.addEventListener('click', () => this.toggle());
    if (this.options.position === 'bottom-left') {
      iconBtn.style.left = '20px';
      iconBtn.style.right = 'auto';
    }
    this.container.appendChild(iconBtn);

    // Chat window
    this.windowEl = document.createElement('div');
    this.windowEl.className = 'mddb-chat-window hidden';
    if (this.options.position === 'bottom-left') {
      this.windowEl.style.left = '20px';
      this.windowEl.style.right = 'auto';
    }

    // Header
    const header = document.createElement('div');
    header.className = 'mddb-header';
    header.innerHTML = `
      <div>
        <div class="mddb-header-title">Chat Support</div>
        <div class="mddb-header-status">Online</div>
      </div>
    `;
    const closeBtn = document.createElement('button');
    closeBtn.className = 'mddb-header-close';
    closeBtn.textContent = CLOSE_ICON;
    closeBtn.addEventListener('click', () => this.toggle());
    header.appendChild(closeBtn);
    this.windowEl.appendChild(header);

    // Error bar
    this.errorEl = document.createElement('div');
    this.errorEl.className = 'mddb-error';
    this.errorEl.style.display = 'none';
    this.windowEl.appendChild(this.errorEl);

    // Queue notice
    this.queueEl = document.createElement('div');
    this.queueEl.className = 'mddb-queue-notice';
    this.queueEl.style.display = 'none';
    this.windowEl.appendChild(this.queueEl);

    // Name form
    this.nameFormEl = document.createElement('div');
    this.nameFormEl.className = 'mddb-name-form';
    this.nameFormEl.innerHTML = `
      <h3>Welcome!</h3>
      <p>Enter your name to start chatting</p>
    `;
    const nameInput = document.createElement('input');
    nameInput.className = 'mddb-name-input';
    nameInput.type = 'text';
    nameInput.placeholder = 'Your name...';
    nameInput.maxLength = 50;

    const nameSubmit = document.createElement('button');
    nameSubmit.className = 'mddb-name-submit';
    nameSubmit.textContent = 'Start Chat';

    const submitName = () => {
      const name = sanitizeInput(nameInput.value, 50);
      if (name) this.join(name);
    };

    nameSubmit.addEventListener('click', submitName);
    nameInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') submitName();
    });

    this.nameFormEl.appendChild(nameInput);
    this.nameFormEl.appendChild(nameSubmit);
    this.windowEl.appendChild(this.nameFormEl);

    // Chat view (messages + input)
    this.chatViewEl = document.createElement('div');
    this.chatViewEl.style.display = 'none';
    this.chatViewEl.style.flex = '1';
    this.chatViewEl.style.flexDirection = 'column';
    this.chatViewEl.style.overflow = 'hidden';

    this.messagesEl = document.createElement('div');
    this.messagesEl.className = 'mddb-messages';

    this.typingEl = document.createElement('div');
    this.typingEl.className = 'mddb-typing';
    this.typingEl.style.display = 'none';
    this.typingEl.innerHTML = `
      <div class="mddb-typing-dot"></div>
      <div class="mddb-typing-dot"></div>
      <div class="mddb-typing-dot"></div>
    `;

    const inputArea = document.createElement('div');
    inputArea.className = 'mddb-input-area';

    this.inputEl = document.createElement('textarea');
    this.inputEl.className = 'mddb-input';
    this.inputEl.placeholder = 'Type a message...';
    this.inputEl.rows = 1;
    this.inputEl.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        this.sendMessage();
      }
    });
    this.inputEl.addEventListener('input', () => {
      this.inputEl.style.height = 'auto';
      this.inputEl.style.height = Math.min(this.inputEl.scrollHeight, 80) + 'px';
    });

    this.sendBtn = document.createElement('button');
    this.sendBtn.className = 'mddb-send-btn';
    this.sendBtn.innerHTML = SEND_ICON_SVG;
    this.sendBtn.addEventListener('click', () => this.sendMessage());

    inputArea.appendChild(this.inputEl);
    inputArea.appendChild(this.sendBtn);

    // Powered by
    const powered = document.createElement('div');
    powered.className = 'mddb-powered';
    powered.textContent = 'Powered by MDDB';

    this.chatViewEl.appendChild(this.messagesEl);
    this.chatViewEl.appendChild(this.typingEl);
    this.chatViewEl.appendChild(inputArea);
    this.chatViewEl.appendChild(powered);
    this.windowEl.appendChild(this.chatViewEl);

    this.container.appendChild(this.windowEl);
  }

  private toggle(): void {
    const isOpen = !this.store.getState().isOpen;
    this.store.update({ isOpen });

    if (isOpen && !this.ws.isConnected) {
      this.ws.connect();
    }
  }

  private join(name: string): void {
    this.store.update({ userName: name });

    if (!this.ws.isConnected) {
      this.ws.connect();
      // Wait for connection, then join
      const unsub = this.store.subscribe((state) => {
        if (state.isConnected) {
          unsub();
          this.ws.send({
            type: 'join',
            name,
            scenario: this.options.scenario || 'assistant',
          });
        }
      });
    } else {
      this.ws.send({
        type: 'join',
        name,
        scenario: this.options.scenario || 'assistant',
      });
    }
  }

  private sendMessage(): void {
    const state = this.store.getState();
    if (state.isStreaming || !state.isJoined) return;

    const content = sanitizeInput(this.inputEl.value, 2000);
    if (!content) return;

    this.inputEl.value = '';
    this.inputEl.style.height = 'auto';

    this.store.addMessage('user', content);
    this.store.update({ isStreaming: true });

    this.ws.send({ type: 'message', content });
  }

  private handleMessage(msg: WsIncoming): void {
    switch (msg.type) {
      case 'session':
        this.store.update({
          isJoined: true,
          isQueued: false,
          sessionId: msg.id,
        });
        saveSession(msg.id, this.store.getState().userName || '');
        break;

      case 'queued':
        this.store.update({
          isQueued: true,
          queuePosition: msg.position,
        });
        break;

      case 'chunk':
        this.store.appendToLastAssistant(msg.content);
        break;

      case 'done':
        this.store.update({ isStreaming: false });
        break;

      case 'error':
        this.store.update({
          error: msg.message,
          isStreaming: false,
        });
        // Clear error after 5s
        setTimeout(() => this.store.update({ error: null }), 5000);
        break;

      case 'pong':
        break;
    }
  }

  private render(state: WidgetState): void {
    // Window visibility
    this.windowEl.classList.toggle('hidden', !state.isOpen);

    // Name form vs chat view
    if (state.isJoined) {
      this.nameFormEl.style.display = 'none';
      this.chatViewEl.style.display = 'flex';
    } else {
      this.nameFormEl.style.display = 'flex';
      this.chatViewEl.style.display = 'none';
    }

    // Queue notice
    if (state.isQueued) {
      this.queueEl.style.display = 'block';
      this.queueEl.textContent = `You are #${state.queuePosition} in queue. Please wait...`;
    } else {
      this.queueEl.style.display = 'none';
    }

    // Error
    if (state.error) {
      this.errorEl.style.display = 'block';
      this.errorEl.textContent = state.error;
    } else {
      this.errorEl.style.display = 'none';
    }

    // Messages
    this.renderMessages(state.messages);

    // Typing indicator
    this.typingEl.style.display = state.isStreaming ? 'flex' : 'none';

    // Input state
    this.sendBtn.disabled = state.isStreaming || !state.isJoined;
    this.inputEl.disabled = state.isStreaming || !state.isJoined;
  }

  private renderMessages(messages: Message[]): void {
    // Only re-render if message count changed
    const currentCount = this.messagesEl.children.length;
    if (currentCount === messages.length && messages.length > 0) {
      // Update last message content (for streaming)
      const last = messages[messages.length - 1];
      const lastEl = this.messagesEl.lastElementChild as HTMLDivElement;
      if (lastEl && last.role === 'assistant') {
        lastEl.innerHTML = renderMarkdown(last.content);
      }
      this.scrollToBottom();
      return;
    }

    this.messagesEl.innerHTML = '';
    for (const msg of messages) {
      const el = document.createElement('div');
      el.className = `mddb-message ${msg.role}`;
      el.innerHTML = msg.role === 'assistant'
        ? renderMarkdown(msg.content)
        : this.escapeHtml(msg.content);
      this.messagesEl.appendChild(el);
    }
    this.scrollToBottom();
  }

  private scrollToBottom(): void {
    this.messagesEl.scrollTop = this.messagesEl.scrollHeight;
  }

  private escapeHtml(text: string): string {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  destroy(): void {
    this.ws.disconnect();
    clearSession();
  }
}
