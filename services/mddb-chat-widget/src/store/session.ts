const STORAGE_KEY = 'mddb-chat-session';

interface StoredSession {
  sessionId: string;
  userName: string;
  lastActive: number;
}

export function saveSession(sessionId: string, userName: string): void {
  try {
    const data: StoredSession = {
      sessionId,
      userName,
      lastActive: Date.now(),
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

    // Expire after 1 hour
    if (Date.now() - data.lastActive > 60 * 60 * 1000) {
      clearSession();
      return null;
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
