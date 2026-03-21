export interface Message {
  role: 'user' | 'assistant';
  content: string;
  timestamp: number;
}

export interface WidgetState {
  isOpen: boolean;
  isConnected: boolean;
  isJoined: boolean;
  isStreaming: boolean;
  isQueued: boolean;
  queuePosition: number;
  sessionId: string | null;
  userName: string | null;
  messages: Message[];
  error: string | null;
}

type Listener = (state: WidgetState) => void;

export class Store {
  private state: WidgetState;
  private listeners: Set<Listener> = new Set();

  constructor() {
    this.state = {
      isOpen: false,
      isConnected: false,
      isJoined: false,
      isStreaming: false,
      isQueued: false,
      queuePosition: 0,
      sessionId: null,
      userName: null,
      messages: [],
      error: null,
    };
  }

  getState(): WidgetState {
    return this.state;
  }

  update(partial: Partial<WidgetState>): void {
    this.state = { ...this.state, ...partial };
    this.notify();
  }

  addMessage(role: 'user' | 'assistant', content: string): void {
    this.state = {
      ...this.state,
      messages: [
        ...this.state.messages,
        { role, content, timestamp: Date.now() },
      ],
    };
    this.notify();
  }

  appendToLastAssistant(chunk: string): void {
    const messages = [...this.state.messages];
    const last = messages[messages.length - 1];
    if (last && last.role === 'assistant') {
      messages[messages.length - 1] = {
        ...last,
        content: last.content + chunk,
      };
    } else {
      messages.push({ role: 'assistant', content: chunk, timestamp: Date.now() });
    }
    this.state = { ...this.state, messages };
    this.notify();
  }

  subscribe(listener: Listener): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private notify(): void {
    for (const listener of this.listeners) {
      listener(this.state);
    }
  }
}
