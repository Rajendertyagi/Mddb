import type { Message } from './state';

const STORAGE_KEY = 'mddb-chat-session';
const DEFAULT_TTL_MS = 24 * 60 * 60 * 1000; // 24 hours

interface StoredSession {
  sessionId: string;
  userName: string;
  lastActive: number;
  messages: Message[];
}

let sessionTtlMs = DEFAULT_TTL_MS;

export function setSessionTtl(hours: number): void {
  sessionTtlMs = hours * 60 * 60 * 1000;
}

export function saveSession(sessionId: string, userName: string, messages: Message[] = []): void {
  try {
    const data: StoredSession = {
      sessionId,
      userName,
      lastActive: Date.now(),
      messages,
    };
    localStorage.setItem(STORAGE_KEY, JSON.stringify(data));
  } catch {
    // localStorage might be unavailable
  }
}

export function loadSession(): StoredSession | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;

    const data: StoredSession = JSON.parse(raw);

    // Expire after configured TTL
    if (Date.now() - data.lastActive > sessionTtlMs) {
      clearSession();
      return null;
    }

    // Ensure messages array exists (backward compat)
    if (!data.messages) {
      data.messages = [];
    }

    return data;
  } catch {
    return null;
  }
}

export function clearSession(): void {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // ignore
  }
}
