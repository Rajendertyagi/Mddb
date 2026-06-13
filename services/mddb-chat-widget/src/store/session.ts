import type { Message } from './state';

const STORAGE_KEY = 'mddb-chat-session';
const DEFAULT_TTL_MS = 24 * 60 * 60 * 1000; // 24 hours

// FE-004: cap how much conversation history is persisted so a potentially
// sensitive transcript isn't kept in browser storage without bound.
export const MAX_STORED_MESSAGES = 50;

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

// capMessages keeps only the most recent MAX_STORED_MESSAGES entries. Pure and
// exported so it can be unit-tested without a storage backend.
export function capMessages(messages: Message[]): Message[] {
  if (!Array.isArray(messages)) return [];
  return messages.slice(-MAX_STORED_MESSAGES);
}

export function saveSession(sessionId: string, userName: string, messages: Message[] = []): void {
  try {
    const data: StoredSession = {
      sessionId,
      userName,
      lastActive: Date.now(),
      messages: capMessages(messages),
    };
    // FE-004: sessionStorage (per-tab, not persisted to disk) instead of
    // localStorage — the transcript and session id don't survive tab close and
    // aren't shared across the origin.
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(data));
  } catch {
    // storage might be unavailable
  }
}

export function loadSession(): StoredSession | null {
  try {
    // Migrate any session left in localStorage by an older build, then drop it
    // from disk (FE-004).
    let raw = sessionStorage.getItem(STORAGE_KEY);
    if (!raw) {
      const legacy = localStorage.getItem(STORAGE_KEY);
      if (legacy) {
        sessionStorage.setItem(STORAGE_KEY, legacy);
        localStorage.removeItem(STORAGE_KEY);
        raw = legacy;
      }
    }
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
    sessionStorage.removeItem(STORAGE_KEY);
    localStorage.removeItem(STORAGE_KEY); // also clear any legacy on-disk copy
  } catch {
    // ignore
  }
}
