"""Unit tests for destination_mddb.destination.DestinationMddb."""

from __future__ import annotations

import json
import logging
import unittest
from typing import List
from unittest.mock import MagicMock, patch

from airbyte_cdk.models import (
    AirbyteMessage,
    AirbyteRecordMessage,
    AirbyteStateMessage,
    AirbyteStream,
    ConfiguredAirbyteCatalog,
    ConfiguredAirbyteStream,
    DestinationSyncMode,
    Status,
    SyncMode,
    Type,
)

from destination_mddb.destination import DestinationMddb

LOGGER = logging.getLogger("test")


def _buildCatalog(streamName: str = "users", mode: DestinationSyncMode = DestinationSyncMode.append) -> ConfiguredAirbyteCatalog:
    return ConfiguredAirbyteCatalog(
        streams=[
            ConfiguredAirbyteStream(
                stream=AirbyteStream(
                    name=streamName,
                    json_schema={"type": "object"},
                    supported_sync_modes=[SyncMode.full_refresh],
                ),
                sync_mode=SyncMode.full_refresh,
                destination_sync_mode=mode,
            )
        ]
    )


def _record(stream: str, data: dict, emittedAt: int = 1700000000) -> AirbyteMessage:
    return AirbyteMessage(
        type=Type.RECORD,
        record=AirbyteRecordMessage(stream=stream, data=data, emitted_at=emittedAt),
    )


def _state(payload: dict) -> AirbyteMessage:
    return AirbyteMessage(type=Type.STATE, state=AirbyteStateMessage(data=payload))


class TestSpec(unittest.TestCase):
    def test_specLoadsFromJsonFile(self):
        spec = DestinationMddb().spec(LOGGER)
        connSpec = spec.connectionSpecification
        # Newer airbyte-cdk returns a dict; older returns a pydantic model.
        if hasattr(connSpec, "model_dump"):
            connSpec = connSpec.model_dump()
        elif hasattr(connSpec, "dict"):
            connSpec = connSpec.dict()
        self.assertIn("mddbUrl", connSpec["properties"])
        self.assertIn("apiKey", connSpec["properties"])
        self.assertTrue(connSpec["properties"]["apiKey"]["airbyte_secret"])


class TestCheck(unittest.TestCase):
    def test_succeedsWhenPingPasses(self):
        config = {"mddbUrl": "http://mddb.test"}
        with patch("destination_mddb.destination.MddbClient") as ClientCls:
            ClientCls.return_value.ping.return_value = None
            status = DestinationMddb().check(LOGGER, config)
        self.assertEqual(status.status, Status.SUCCEEDED)

    def test_failsOnAuthError(self):
        config = {"mddbUrl": "http://mddb.test", "apiKey": "bad"}
        with patch("destination_mddb.destination.MddbClient") as ClientCls:
            ClientCls.return_value.ping.side_effect = PermissionError("401")
            status = DestinationMddb().check(LOGGER, config)
        self.assertEqual(status.status, Status.FAILED)
        self.assertIn("Auth", status.message)

    def test_failsOnGenericError(self):
        config = {"mddbUrl": "http://mddb.broken"}
        with patch("destination_mddb.destination.MddbClient") as ClientCls:
            ClientCls.return_value.ping.side_effect = RuntimeError("timeout")
            status = DestinationMddb().check(LOGGER, config)
        self.assertEqual(status.status, Status.FAILED)
        self.assertIn("Cannot reach", status.message)


class TestWrite(unittest.TestCase):
    def _runWrite(self, config, catalog, messages):
        with patch("destination_mddb.destination.MddbClient") as ClientCls:
            client = MagicMock()
            sentDocs: List[dict] = []

            def captureFlush(docs):
                materialised = list(docs)
                sentDocs.extend(materialised)
                return len(materialised)

            client.addBatch.side_effect = captureFlush
            ClientCls.return_value = client
            output = list(DestinationMddb().write(config, catalog, iter(messages)))
            return output, client, sentDocs

    def test_appendsRecordsAndFlushesOnState(self):
        config = {"mddbUrl": "http://mddb.test", "batchSize": 100}
        catalog = _buildCatalog("users")
        messages = [
            _record("users", {"id": "u1", "name": "Alice"}),
            _record("users", {"id": "u2", "name": "Bob"}),
            _state({"cursor": 2}),
        ]
        output, client, sentDocs = self._runWrite(config, catalog, messages)
        self.assertEqual(len(output), 1)
        self.assertEqual(output[0].type, Type.STATE)
        self.assertEqual(len(sentDocs), 2)
        client.addBatch.assert_called()

    def test_flushesAtBatchSize(self):
        config = {"mddbUrl": "http://mddb.test", "batchSize": 2}
        catalog = _buildCatalog("users")
        messages = [
            _record("users", {"id": "u1"}),
            _record("users", {"id": "u2"}),
            _record("users", {"id": "u3"}),
        ]
        _, client, sentDocs = self._runWrite(config, catalog, messages)
        self.assertGreaterEqual(client.addBatch.call_count, 2)
        self.assertEqual(len(sentDocs), 3)

    def test_skipsUnconfiguredStreams(self):
        config = {"mddbUrl": "http://mddb.test"}
        catalog = _buildCatalog("users")
        messages = [
            _record("orders", {"id": "o1"}),
            _record("users", {"id": "u1"}),
        ]
        _, _client, sentDocs = self._runWrite(config, catalog, messages)
        self.assertEqual(len(sentDocs), 1)
        self.assertEqual(sentDocs[0]["key"], "u1")

    def test_overwriteModeIsWarned(self):
        config = {"mddbUrl": "http://mddb.test"}
        catalog = _buildCatalog("users", mode=DestinationSyncMode.overwrite)
        messages = [_record("users", {"id": "u1"})]
        with self.assertLogs("airbyte", level="WARNING") as cm:
            self._runWrite(config, catalog, messages)
        self.assertTrue(any("overwrite mode" in m for m in cm.output))

    def test_passesRecordDataToBuildDocument(self):
        config = {"mddbUrl": "http://mddb.test", "keyField": "email", "language": "pl_PL"}
        catalog = _buildCatalog("users")
        messages = [_record("users", {"email": "a@b.c", "x": 1})]
        _, _client, sentDocs = self._runWrite(config, catalog, messages)
        self.assertEqual(len(sentDocs), 1)
        sentDoc = sentDocs[0]
        self.assertEqual(sentDoc["key"], "a@b.c")
        self.assertEqual(sentDoc["lang"], "pl_PL")
        self.assertEqual(sentDoc["collection"], "users")


if __name__ == "__main__":
    unittest.main()
