// Auth manager for MDDB Panel
// Handles JWT token storage and authentication

import { TOKEN_KEY, LEGACY_TOKEN_KEYS, isValidJwtShape } from './token';

// FE-004: the JWT lives in sessionStorage, not localStorage — it is gone when
// the tab closes and never written to disk, so a single XSS can no longer
// exfiltrate a long-lived admin token, and it is invisible to other tabs /
// browser extensions on the origin.
//
// On startup: clear stale keys (FE-005), and migrate any token left in
// localStorage by an older build into sessionStorage so users stay logged in
// across the upgrade — then scrub it from disk.
try {
  LEGACY_TOKEN_KEYS.forEach((k) => {
    localStorage.removeItem(k);
    sessionStorage.removeItem(k);
  });
  const legacyToken = localStorage.getItem(TOKEN_KEY);
  if (legacyToken) {
    if (!sessionStorage.getItem(TOKEN_KEY)) {
      sessionStorage.setItem(TOKEN_KEY, legacyToken);
    }
    localStorage.removeItem(TOKEN_KEY);
  }
} catch {
  /* storage unavailable */
}

export { isValidJwtShape };

export const authManager = {
  /**
   * Get stored JWT token
   */
  getToken() {
    return sessionStorage.getItem(TOKEN_KEY);
  },

  /**
   * Store JWT token
   */
  setToken(token) {
    sessionStorage.setItem(TOKEN_KEY, token);
  },

  /**
   * Clear stored token (logout)
   */
  clearToken() {
    sessionStorage.removeItem(TOKEN_KEY);
  },

  /**
   * Check if user is authenticated
   */
  isAuthenticated() {
    return !!this.getToken();
  },

  /**
   * Login with username and password
   * @param {string} username
   * @param {string} password
   * @returns {Promise<{token: string, expiresAt: number}>}
   */
  async login(username, password) {
    const resp = await fetch('/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    });

    if (!resp.ok) {
      const error = await resp.text();
      throw new Error(`Login failed: ${error}`);
    }

    const data = await resp.json();
    this.setToken(data.token);
    return data;
  },

  /**
   * Logout and clear token
   */
  logout() {
    this.clearToken();
    window.location.reload();
  },
};
