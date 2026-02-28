"""
MDDB Python Client - HTTP REST client for MDDB (Markdown Database).

Zero external dependencies - uses only Python stdlib.

Usage:
    from mddb import MDDB

    db = MDDB.connect('localhost:11023', 'read')
    db = db.collection('blog')
    doc = db.get('homepage', 'en_GB')
"""

import json
import urllib.request
import urllib.error


class MDDB:
    def __init__(self):
        self._base = ''
        self._mode = 'read'
        self._collection = ''
        self._env = {}

    @staticmethod
    def connect(addr: str, mode: str = 'read') -> 'MDDB':
        """Connect to MDDB server. Mode: 'read' or 'write'."""
        db = MDDB()
        db._base = f'http://{addr}/v1'
        db._mode = mode
        return db

    def collection(self, name: str) -> 'MDDB':
        """Set active collection."""
        self._collection = name
        return self

    def env(self, key: str, value: str) -> 'MDDB':
        """Set template variable for get() requests."""
        self._env[key] = value
        return self

    # --- Document operations ---

    def get(self, key: str, lang: str) -> dict:
        """Get a document by key and language."""
        return self._post('/get', {
            'collection': self._collection,
            'key': key,
            'lang': lang,
            'env': self._env,
        })

    def add(self, key: str, lang: str, meta: dict, content_md: str) -> dict:
        """Add or update a document."""
        if self._mode == 'read':
            raise RuntimeError('read-only client')
        return self._post('/add', {
            'collection': self._collection,
            'key': key,
            'lang': lang,
            'meta': meta,
            'contentMd': content_md,
        })

    def search(self, meta_key: str, meta_val: str, sort: str = 'addedAt',
               asc: bool = True, limit: int = 100) -> list:
        """Search documents by metadata."""
        return self._post('/search', {
            'collection': self._collection,
            'filterMeta': {meta_key: [meta_val]},
            'sort': sort,
            'asc': asc,
            'limit': limit,
            'offset': 0,
        })

    def delete(self, key: str, lang: str) -> dict:
        """Delete a document."""
        if self._mode == 'read':
            raise RuntimeError('read-only client')
        return self._post('/delete', {
            'collection': self._collection,
            'key': key,
            'lang': lang,
        })

    # --- Vector search ---

    def vector_search(self, query: str, top_k: int = 5, threshold: float = 0.0,
                      include_content: bool = False, filter_meta: dict = None) -> dict:
        """Semantic search using AI embeddings."""
        payload = {
            'collection': self._collection,
            'query': query,
            'topK': top_k,
            'threshold': threshold,
            'includeContent': include_content,
        }
        if filter_meta:
            payload['filterMeta'] = filter_meta
        return self._post('/vector-search', payload)

    def vector_reindex(self, force: bool = False) -> dict:
        """Re-embed all documents in the active collection."""
        if self._mode == 'read':
            raise RuntimeError('read-only client')
        return self._post('/vector-reindex', {
            'collection': self._collection,
            'force': force,
        })

    def vector_stats(self) -> dict:
        """Get embedding/vector statistics."""
        return self._get('/vector-stats')

    # --- Server operations ---

    def stats(self) -> dict:
        """Get server statistics."""
        return self._get('/stats')

    def backup(self, filename: str = None) -> dict:
        """Create database backup."""
        path = '/backup'
        if filename:
            path += f'?to={filename}'
        return self._get(path)

    def export(self, fmt: str = 'ndjson', filter_meta: dict = None) -> str:
        """Export documents from collection."""
        payload = {
            'collection': self._collection,
            'format': fmt,
        }
        if filter_meta:
            payload['filterMeta'] = filter_meta
        return self._post('/export', payload)

    # --- HTTP helpers ---

    def _post(self, path: str, payload: dict):
        data = json.dumps(payload).encode('utf-8')
        req = urllib.request.Request(
            self._base + path,
            data=data,
            headers={'Content-Type': 'application/json'},
            method='POST',
        )
        return self._do(req)

    def _get(self, path: str):
        req = urllib.request.Request(self._base + path, method='GET')
        return self._do(req)

    def _do(self, req):
        try:
            with urllib.request.urlopen(req) as resp:
                return json.loads(resp.read().decode('utf-8'))
        except urllib.error.HTTPError as e:
            body = e.read().decode('utf-8')
            raise RuntimeError(f'MDDB API error ({e.code}): {body}') from e
        except urllib.error.URLError as e:
            raise RuntimeError(f'Connection error: {e.reason}') from e
