"""Unit tests for destination_mddb.client."""

from __future__ import annotations

import json
import unittest
from unittest.mock import MagicMock, patch

from destination_mddb.client import (
    MddbClient,
    _extractKey,
    _flattenToStringLists,
    _hashFallback,
    _renderContentMd,
    _scalarToString,
    _stringifyValue,
    buildDocument,
)


class TestKeyExtraction(unittest.TestCase):
    def test_extractsConfiguredField(self):
        self.assertEqual(_extractKey({"id": "abc"}, "id"), "abc")

    def test_castsNonStringToString(self):
        self.assertEqual(_extractKey({"id": 42}, "id"), "42")

    def test_fallsBackToHashWhenMissing(self):
        record = {"name": "alice"}
        key = _extractKey(record, "id")
        self.assertEqual(key, _hashFallback(record))
        self.assertEqual(len(key), 40)

    def test_fallsBackToHashWhenEmptyString(self):
        key = _extractKey({"id": "   "}, "id")
        self.assertEqual(len(key), 40)

    def test_fallsBackToHashWhenNull(self):
        key = _extractKey({"id": None, "name": "bob"}, "id")
        self.assertEqual(len(key), 40)

    def test_hashIsDeterministic(self):
        record = {"b": 2, "a": 1}
        self.assertEqual(_hashFallback(record), _hashFallback({"a": 1, "b": 2}))


class TestStringification(unittest.TestCase):
    def test_intToString(self):
        self.assertEqual(_scalarToString(42), "42")

    def test_floatToString(self):
        self.assertEqual(_scalarToString(3.14), "3.14")

    def test_boolLowercase(self):
        self.assertEqual(_scalarToString(True), "true")
        self.assertEqual(_scalarToString(False), "false")

    def test_dictBecomesJson(self):
        self.assertEqual(_scalarToString({"k": "v"}), '{"k": "v"}')

    def test_listInsideScalarBecomesJson(self):
        self.assertEqual(_scalarToString([1, 2]), "[1, 2]")

    def test_stringifyScalar(self):
        self.assertEqual(_stringifyValue("hello"), ["hello"])

    def test_stringifyList(self):
        self.assertEqual(_stringifyValue([1, "two", 3]), ["1", "two", "3"])

    def test_stringifyListDropsNone(self):
        self.assertEqual(_stringifyValue([1, None, 2]), ["1", "2"])


class TestFlatten(unittest.TestCase):
    def test_basicFlatten(self):
        result = _flattenToStringLists({"id": 1, "name": "alice", "active": True})
        self.assertEqual(result, {"id": ["1"], "name": ["alice"], "active": ["true"]})

    def test_dropsNoneValues(self):
        result = _flattenToStringLists({"id": 1, "deletedAt": None})
        self.assertNotIn("deletedAt", result)

    def test_nestedDictBecomesJsonString(self):
        result = _flattenToStringLists({"meta": {"x": 1}})
        self.assertEqual(result["meta"], ['{"x": 1}'])


class TestContentMd(unittest.TestCase):
    def test_wrapsInJsonFence(self):
        md = _renderContentMd({"a": 1}, emittedAt=None)
        self.assertIn("```json", md)
        self.assertIn('"a": 1', md)
        self.assertTrue(md.endswith("```\n"))

    def test_emittedAtComment(self):
        md = _renderContentMd({"a": 1}, emittedAt=1700000000)
        self.assertIn("<!-- emittedAt=1700000000 -->", md)


class TestBuildDocument(unittest.TestCase):
    def test_assemblesFullDocument(self):
        doc = buildDocument(
            collection="users",
            record={"id": "u-1", "name": "Alice"},
            keyField="id",
            language="en_US",
            emittedAt=123,
        )
        self.assertEqual(doc["collection"], "users")
        self.assertEqual(doc["key"], "u-1")
        self.assertEqual(doc["lang"], "en_US")
        self.assertEqual(doc["meta"]["name"], ["Alice"])
        self.assertIn("emittedAt=123", doc["contentMd"])

    def test_keyFallsBackToHashWhenFieldMissing(self):
        doc = buildDocument(
            collection="users",
            record={"name": "Bob"},
            keyField="id",
            language="en_US",
        )
        self.assertEqual(len(doc["key"]), 40)


class TestMddbClient(unittest.TestCase):
    def setUp(self):
        self.client = MddbClient(baseUrl="http://mddb.test/", apiKey="vk_test")

    def test_normalisesBaseUrl(self):
        self.assertEqual(self.client.baseUrl, "http://mddb.test")

    def test_setsBearerHeader(self):
        self.assertEqual(self.client.session.headers["Authorization"], "Bearer vk_test")

    def test_noAuthHeaderWhenKeyMissing(self):
        c = MddbClient(baseUrl="http://x", apiKey=None)
        self.assertNotIn("Authorization", c.session.headers)

    def test_pingHits404Gracefully(self):
        with patch.object(self.client.session, "post") as post:
            post.return_value = MagicMock(status_code=404, ok=False, text="not found")
            self.client.ping()
            post.assert_called_once()
            url = post.call_args[0][0]
            self.assertEqual(url, "http://mddb.test/v1/search")

    def test_pingHits405Gracefully(self):
        with patch.object(self.client.session, "post") as post:
            post.return_value = MagicMock(status_code=405, ok=False, text="method not allowed")
            self.client.ping()

    def test_pingSurfacesAuthFailure(self):
        with patch.object(self.client.session, "post") as post:
            post.return_value = MagicMock(status_code=401, ok=False, text="nope")
            with self.assertRaises(PermissionError):
                self.client.ping()

    def test_pingSurfacesServerError(self):
        with patch.object(self.client.session, "post") as post:
            post.return_value = MagicMock(status_code=500, ok=False, text="boom")
            with self.assertRaises(RuntimeError):
                self.client.ping()

    def test_addDocumentSendsJson(self):
        with patch.object(self.client.session, "post") as post:
            post.return_value = MagicMock(status_code=200, ok=True, text="{}")
            self.client.addDocument({"collection": "c", "key": "k"})
            body = json.loads(post.call_args[1]["data"])
            self.assertEqual(body, {"collection": "c", "key": "k"})

    def test_addDocumentRaisesOnError(self):
        with patch.object(self.client.session, "post") as post:
            post.return_value = MagicMock(status_code=422, ok=False, text="bad payload")
            with self.assertRaises(RuntimeError):
                self.client.addDocument({"key": "k"})

    def test_addBatchCountsAccepted(self):
        with patch.object(self.client, "addDocument") as add:
            count = self.client.addBatch([{"key": "1"}, {"key": "2"}, {"key": "3"}])
            self.assertEqual(count, 3)
            self.assertEqual(add.call_count, 3)


if __name__ == "__main__":
    unittest.main()
