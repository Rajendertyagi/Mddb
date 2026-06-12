// Auth manager for MDDB Panel
// Handles JWT token storage and authentication

import { TOKEN_KEY, LEGACY_TOKEN_KEYS, isValidJwtShape } from './token';

// FE-005: drop stale keys written by older builds so they can never be read
// back as a "current" token.
try {
  LEGACY_TOKEN_KEYS.forEach((k) => localStorage.removeItem(k));
} catch {
  /* storage unavailable */
}

export { isValidJwtShape };

export const authManager = {
  /**
   * Get stored JWT token
   */
  getToken() {
    return localStorage.getItem(TOKEN_KEY);
  },

  /**
   * Store JWT token
   */
  setToken(token) {
    localStorage.setItem(TOKEN_KEY, token);
  },

  /**
   * Clear stored token (logout)
   */
  clearToken() {
    localStorage.removeItem(TOKEN_KEY);
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
