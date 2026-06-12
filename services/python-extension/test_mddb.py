"""Tests for the MDDB Python client. Run: python3 -m unittest -v

Covers INT-010: backup() query encoding and request timeouts.
"""

import urllib.parse
import unittest

import mddb


class TestBackupEncoding(unittest.TestCase):
    def _capture_backup_path(self, filename=None):
        client = mddb.MDDB()
        captured = {}
        client._get = lambda path: captured.setdefault("path", path) or {}
        if filename is None:
            client.backup()
        else:
            client.backup(filename)
        return captured["path"]

    def test_backup_filename_is_percent_encoded(self):
        # INT-010: query-param injection / path traversal must be neutralised.
        path = self._capture_backup_path("../../etc/passwd&x=1 y")
        self.assertIn("?to=", path)
        q = path.split("?to=", 1)[1]
        for bad in ("&", " ", "/"):
            self.assertNotIn(bad, q, f"raw {bad!r} must be encoded")
        # Encoding is reversible — the server receives the exact intended value.
        self.assertEqual(urllib.parse.unquote(q), "../../etc/passwd&x=1 y")

    def test_backup_without_filename_has_no_query(self):
        self.assertEqual(self._capture_backup_path(), "/backup")


class TestTimeout(unittest.TestCase):
    def test_do_passes_timeout_to_open(self):
        client = mddb.MDDB()
        client._timeout = 7
        seen = {}

        class FakeResp:
            def __enter__(self):
                return self

            def __exit__(self, *exc):
                return False

            def read(self):
                return b"{}"

        class FakeOpener:
            def open(self, req, timeout=None):
                seen["timeout"] = timeout
                return FakeResp()

        client._opener = FakeOpener()
        self.assertEqual(client._do(object()), {})
        self.assertEqual(seen["timeout"], 7)

    def test_default_timeout_is_set(self):
        self.assertEqual(mddb.MDDB()._timeout, mddb.DEFAULT_TIMEOUT)


if __name__ == "__main__":
    unittest.main()
