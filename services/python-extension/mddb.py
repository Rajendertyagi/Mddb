"""
MDDB Python Client - HTTP REST client for MDDB (Markdown Database).

Zero external dependencies - uses only Python stdlib.

Usage (TCP):
    from mddb import MDDB

    db = MDDB.connect('localhost:11023', 'read')
    db = db.collection('blog')
    doc = db.get('homepage', 'en_GB')

Usage (Unix Domain Socket, MDDB 2.9.12+):
    db = MDDB.connect('unix:/tmp/mddb.sock', 'read')
    db = db.collection('blog')
    doc = db.get('homepage', 'en_GB')
"""

import http.client
import json
import socket
import urllib.error
import urllib.request


class _UnixHTTPConnection(http.client.HTTPConnection):
    """HTTPConnection that dials a Unix Domain Socket instead of TCP."""

    def __init__(self, socket_path: str, timeout=None):
        super().__init__('localhost', timeout=timeout)
        self._socket_path = socket_path

    def connect(self):
        sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        if self.timeout is not None and self.timeout is not socket._GLOBAL_DEFAULT_TIMEOUT:
            sock.settimeout(self.timeout)
        sock.connect(self._socket_path)
        self.sock = sock


class _UnixHTTPHandler(urllib.request.HTTPHandler):
    """urllib HTTP handler that routes every request through a UDS path."""

    def __init__(self, socket_path: str):
        super().__init__()
        self._socket_path = socket_path

    def http_open(self, req):
        sp = self._socket_path
        return self.do_open(lambda host, timeout=None: _UnixHTTPConnection(sp, timeout), req)


class MDDB:
    def __init__(self):
        self._base = ''
        self._mode = 'read'
        self._collection = ''
        self._env = {}
        self._opener = urllib.request.build_opener()

    @staticmethod
    def connect(addr: str, mode: str = 'read') -> 'MDDB':
        """Connect to MDDB server.

        addr forms:
          * 'host:port'        — plain TCP HTTP (default MDDB listener)
          * 'unix:/path.sock'  — Unix Domain Socket (MDDB 2.9.12+)

        mode: 'read' (default) or 'write'.
        """
        db = MDDB()
        db._mode = mode
        if addr.startswith('unix:'):
            # Tolerate both 'unix:/path' and 'unix:///path' forms.
            path = addr[len('unix:'):]
            if path.startswith('//'):
                path = path[2:]
            db._base = 'http://localhost/v1'
            db._opener = urllib.request.build_opener(_UnixHTTPHandler(path))
        else:
            db._base = f'http://{addr}/v1'
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

    # --- Batch operations ---

    def add_batch(self, documents: list) -> dict:
        """Add/update multiple documents in a single batch.
        Each document should be a dict with keys: key, lang, contentMd, meta (optional).
        """
        if self._mode == 'read':
            raise RuntimeError('read-only client')
        return self._post('/add-batch', {
            'collection': self._collection,
            'documents': documents,
        })

    def ingest(self, documents: list, options: dict = None) -> dict:
        """Bulk ingest documents with URL key derivation, frontmatter extraction, and dedup.
        Each document should be a dict with keys: url, key (optional), lang, contentMd,
        meta (optional), extractFrontmatter (optional), scrapedAt (optional), scraper (optional).
        Options: skipDuplicates, skipEmbeddings, skipFts, skipWebhooks,
                 autoConfigureCollection, saveRevision.
        """
        if self._mode == 'read':
            raise RuntimeError('read-only client')
        payload = {
            'collection': self._collection,
            'documents': documents,
        }
        if options:
            payload['options'] = options
        return self._post('/ingest', payload)

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

    # --- Import URL ---

    def import_url(self, url: str, lang: str, key: str = None,
                   meta: dict = None, ttl: int = 0) -> dict:
        """Import a markdown document from URL."""
        if self._mode == 'read':
            raise RuntimeError('read-only client')
        payload = {
            'collection': self._collection,
            'url': url,
            'lang': lang,
        }
        if key:
            payload['key'] = key
        if meta:
            payload['meta'] = meta
        if ttl > 0:
            payload['ttl'] = ttl
        return self._post('/import-url', payload)

    # --- TTL ---

    def set_ttl(self, key: str, lang: str, ttl: int) -> dict:
        """Set or remove TTL on a document (0 = remove TTL)."""
        if self._mode == 'read':
            raise RuntimeError('read-only client')
        return self._post('/set-ttl', {
            'collection': self._collection,
            'key': key,
            'lang': lang,
            'ttl': ttl,
        })

    # --- Full-text search ---

    def fts_search(self, query: str, limit: int = 50) -> dict:
        """Full-text search by content."""
        return self._post('/fts', {
            'collection': self._collection,
            'query': query,
            'limit': limit,
        })

    # --- Webhooks ---

    def register_webhook(self, url: str, events: list,
                         collection: str = None) -> dict:
        """Register a webhook for document events."""
        if self._mode == 'read':
            raise RuntimeError('read-only client')
        payload = {'url': url, 'events': events}
        if collection:
            payload['collection'] = collection
        return self._post('/webhooks', payload)

    def list_webhooks(self) -> list:
        """List all registered webhooks."""
        return self._get('/webhooks')

    def delete_webhook(self, webhook_id: str) -> dict:
        """Delete a webhook by ID."""
        if self._mode == 'read':
            raise RuntimeError('read-only client')
        return self._post('/webhooks/delete', {'id': webhook_id})

    # --- Schema validation ---

    def set_schema(self, schema: dict) -> dict:
        """Set a JSON schema for the active collection."""
        if self._mode == 'read':
            raise RuntimeError('read-only client')
        return self._post('/schema/set', {
            'collection': self._collection,
            'schema': schema,
        })

    def get_schema(self) -> dict:
        """Get the schema for the active collection."""
        return self._post('/schema/get', {
            'collection': self._collection,
        })

    def delete_schema(self) -> dict:
        """Delete the schema for the active collection."""
        if self._mode == 'read':
            raise RuntimeError('read-only client')
        return self._post('/schema/delete', {
            'collection': self._collection,
        })

    def list_schemas(self) -> list:
        """List all schemas across all collections."""
        return self._post('/schema/list', {})

    def validate(self, meta: dict) -> dict:
        """Validate metadata against the collection schema."""
        return self._post('/validate', {
            'collection': self._collection,
            'meta': meta,
        })

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
            with self._opener.open(req) as resp:
                return json.loads(resp.read().decode('utf-8'))
        except urllib.error.HTTPError as e:
            body = e.read().decode('utf-8')
            raise RuntimeError(f'MDDB API error ({e.code}): {body}') from e
        except urllib.error.URLError as e:
            raise RuntimeError(f'Connection error: {e.reason}') from e
