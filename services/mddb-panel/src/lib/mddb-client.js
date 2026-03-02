/**
 * MDDB API Client
 * Simple client for interacting with MDDB HTTP API
 */

import { authManager } from './auth';

const API_BASE = import.meta.env.MODE === 'production'
  ? `http://${import.meta.env.VITE_MDBB_SERVER || 'localhost:11023'}/v1`
  : '/v1';

class MDDBClient {
  constructor(baseUrl = API_BASE) {
    this.baseUrl = baseUrl;
  }

  async request(endpoint, options = {}) {
    const url = `${this.baseUrl}${endpoint}`;
    const token = authManager.getToken();

    const config = {
      headers: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
      ...options,
    };

    // Add authentication header if token exists
    if (token) {
      config.headers['Authorization'] = `Bearer ${token}`;
    }

    try {
      const response = await fetch(url, config);

      // Handle 401 Unauthorized - clear token and reload
      if (response.status === 401) {
        authManager.clearToken();
        window.location.reload();
        throw new Error('Unauthorized - please login again');
      }

      if (!response.ok) {
        const error = await response.text();
        throw new Error(`API Error (${response.status}): ${error}`);
      }

      return await response.json();
    } catch (error) {
      console.error('MDDB API Error:', error);
      throw error;
    }
  }

  /**
   * Get server statistics
   */
  async getStats() {
    return this.request('/stats', { method: 'GET' });
  }

  /**
   * Search documents in a collection
   */
  async search({ collection, filterMeta = {}, sort = 'addedAt', asc = false, limit = 100 }) {
    return this.request('/search', {
      method: 'POST',
      body: JSON.stringify({
        collection,
        filterMeta,
        sort,
        asc,
        limit,
      }),
    });
  }

  /**
   * Get a specific document
   */
  async getDocument({ collection, key, lang, env = {} }) {
    return this.request('/get', {
      method: 'POST',
      body: JSON.stringify({
        collection,
        key,
        lang,
        env,
      }),
    });
  }

  /**
   * Add or update a document
   */
  async addDocument({ collection, key, lang, meta = {}, contentMd }) {
    return this.request('/add', {
      method: 'POST',
      body: JSON.stringify({
        collection,
        key,
        lang,
        meta,
        contentMd,
      }),
    });
  }

  /**
   * Export documents
   */
  async export({ collection, filterMeta = {}, format = 'ndjson' }) {
    return this.request('/export', {
      method: 'POST',
      body: JSON.stringify({
        collection,
        filterMeta,
        format,
      }),
    });
  }

  /**
   * Create database backup
   */
  async backup(filename) {
    const url = `${this.baseUrl}/backup${filename ? `?to=${filename}` : ''}`;
    const response = await fetch(url, { method: 'GET' });
    
    if (!response.ok) {
      throw new Error(`Backup failed: ${response.statusText}`);
    }
    
    return response.json();
  }

  /**
   * Truncate old revisions
   */
  async truncate({ collection, keepRevs = 3, dropCache = true }) {
    return this.request('/truncate', {
      method: 'POST',
      body: JSON.stringify({
        collection,
        keepRevs,
        dropCache,
      }),
    });
  }

  /**
   * Delete a single document
   */
  async deleteDocument({ collection, key, lang }) {
    return this.request('/delete', {
      method: 'POST',
      body: JSON.stringify({
        collection,
        key,
        lang,
      }),
    });
  }

  /**
   * Vector/semantic search
   */
  async vectorSearch({ collection, query, topK = 5, threshold = 0.0, filterMeta = {}, includeContent = false }) {
    return this.request('/vector-search', {
      method: 'POST',
      body: JSON.stringify({
        collection,
        query,
        topK,
        threshold,
        filterMeta,
        includeContent,
      }),
    });
  }

  /**
   * Re-embed documents in a collection
   */
  async vectorReindex({ collection, force = false }) {
    return this.request('/vector-reindex', {
      method: 'POST',
      body: JSON.stringify({ collection, force }),
    });
  }

  /**
   * Get vector/embedding statistics
   */
  async vectorStats() {
    return this.request('/vector-stats', { method: 'GET' });
  }

  /**
   * Import document from URL
   */
  async importURL({ collection, url, lang, key, meta = {}, ttl = 0 }) {
    const body = { collection, url, lang };
    if (key) body.key = key;
    if (Object.keys(meta).length > 0) body.meta = meta;
    if (ttl > 0) body.ttl = ttl;
    return this.request('/import-url', {
      method: 'POST',
      body: JSON.stringify(body),
    });
  }

  /**
   * Set TTL on a document
   */
  async setTTL({ collection, key, lang, ttl }) {
    return this.request('/set-ttl', {
      method: 'POST',
      body: JSON.stringify({ collection, key, lang, ttl }),
    });
  }

  /**
   * Full-text search
   */
  async ftsSearch({ collection, query, limit = 50 }) {
    return this.request('/fts', {
      method: 'POST',
      body: JSON.stringify({ collection, query, limit }),
    });
  }

  /**
   * Register a webhook
   */
  async registerWebhook({ url, events, collection }) {
    const body = { url, events };
    if (collection) body.collection = collection;
    return this.request('/webhooks', {
      method: 'POST',
      body: JSON.stringify(body),
    });
  }

  /**
   * List all webhooks
   */
  async listWebhooks() {
    return this.request('/webhooks', { method: 'GET' });
  }

  /**
   * Delete a webhook
   */
  async deleteWebhook(id) {
    return this.request('/webhooks/delete', {
      method: 'POST',
      body: JSON.stringify({ id }),
    });
  }

  /**
   * Delete entire collection
   */
  async deleteCollection({ collection }) {
    return this.request('/delete-collection', {
      method: 'POST',
      body: JSON.stringify({
        collection,
      }),
    });
  }

  // ---- System & Configuration Methods ----

  /**
   * Get system information
   */
  async getSystemInfo() {
    return this.request('/system/info', { method: 'GET' });
  }

  /**
   * Get server configuration
   */
  async getConfig() {
    return this.request('/config', { method: 'GET' });
  }

  /**
   * Get MCP configuration YAML
   */
  async getMCPConfig() {
    return this.request('/mcp/config', { method: 'GET' });
  }

  /**
   * Get all API endpoints
   */
  async getEndpoints() {
    return this.request('/endpoints', { method: 'GET' });
  }

  // ---- User Management Methods ----

  /**
   * List all users (admin only)
   */
  async listUsers() {
    return this.request('/auth/users', { method: 'GET' });
  }

  // ---- Group Management Methods ----

  /**
   * Create a new group
   */
  async createGroup({ name, description, members = [] }) {
    return this.request('/auth/groups', {
      method: 'POST',
      body: JSON.stringify({ name, description, members }),
    });
  }

  /**
   * List all groups
   */
  async listGroups() {
    return this.request('/auth/groups', { method: 'GET' });
  }

  /**
   * Get group details
   */
  async getGroup(name) {
    return this.request(`/auth/groups/${name}`, { method: 'GET' });
  }

  /**
   * Update group
   */
  async updateGroup(name, { description, members }) {
    return this.request(`/auth/groups/${name}`, {
      method: 'PUT',
      body: JSON.stringify({ description, members }),
    });
  }

  /**
   * Delete group
   */
  async deleteGroup(name) {
    return this.request(`/auth/groups/${name}`, { method: 'DELETE' });
  }

  /**
   * Set group permission
   */
  async setGroupPermission({ groupName, collection, read, write, admin }) {
    return this.request('/auth/group-permissions', {
      method: 'POST',
      body: JSON.stringify({ groupName, collection, read, write, admin }),
    });
  }

  /**
   * Get group permissions
   */
  async getGroupPermissions(groupName) {
    return this.request(`/auth/group-permissions?group=${groupName}`, {
      method: 'GET',
    });
  }
}

export default new MDDBClient();
