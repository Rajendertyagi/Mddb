"""Airbyte Destination connector for MDDB (https://github.com/tradik/mddb)."""

from __future__ import annotations

import json
import logging
from pathlib import Path
from typing import Any, Iterable, List, Mapping, Tuple

from airbyte_cdk.destinations import Destination
from airbyte_cdk.models import (
    AirbyteConnectionStatus,
    AirbyteMessage,
    ConfiguredAirbyteCatalog,
    ConnectorSpecification,
    DestinationSyncMode,
    Status,
    Type,
)

from .client import MddbClient, buildDocument


class DestinationMddb(Destination):
    """Pushes Airbyte records to MDDB via REST /v1/add."""

    def spec(self, logger: logging.Logger) -> ConnectorSpecification:
        specPath = Path(__file__).parent / "spec.json"
        raw = json.loads(specPath.read_text(encoding="utf-8"))
        if hasattr(ConnectorSpecification, "model_validate"):
            return ConnectorSpecification.model_validate(raw)
        if hasattr(ConnectorSpecification, "parse_obj"):
            return ConnectorSpecification.parse_obj(raw)
        # airbyte-cdk >= 1.x exposes ConnectorSpecification as a dataclass; coerce sync-mode strings to enum.
        syncModes = [
            mode if isinstance(mode, DestinationSyncMode) else DestinationSyncMode(mode)
            for mode in raw.get("supported_destination_sync_modes", [])
        ]
        return ConnectorSpecification(
            documentationUrl=raw.get("documentationUrl"),
            connectionSpecification=raw["connectionSpecification"],
            supportsIncremental=raw.get("supportsIncremental"),
            supported_destination_sync_modes=syncModes or None,
        )

    def check(self, logger: logging.Logger, config: Mapping[str, Any]) -> AirbyteConnectionStatus:
        try:
            client = _buildClient(config)
            client.ping()
            return AirbyteConnectionStatus(status=Status.SUCCEEDED)
        except PermissionError as exc:
            return AirbyteConnectionStatus(status=Status.FAILED, message=f"Auth rejected: {exc}")
        except Exception as exc:
            return AirbyteConnectionStatus(status=Status.FAILED, message=f"Cannot reach MDDB: {exc}")

    def write(
        self,
        config: Mapping[str, Any],
        configured_catalog: ConfiguredAirbyteCatalog,
        input_messages: Iterable[AirbyteMessage],
    ) -> Iterable[AirbyteMessage]:
        client = _buildClient(config)
        keyField = config.get("keyField", "id")
        language = config.get("language", "en_US")
        batchSize = int(config.get("batchSize", 100))

        streamModes = {
            stream.stream.name: stream.destination_sync_mode for stream in configured_catalog.streams
        }
        log = logging.getLogger("airbyte")
        for streamName, mode in streamModes.items():
            if mode == DestinationSyncMode.overwrite:
                log.warning(
                    "Stream %r requested overwrite mode; MDDB destination performs upsert-by-key "
                    "(records sharing keys with existing docs are replaced; orphan docs are NOT deleted).",
                    streamName,
                )

        buffer: List[Tuple[str, dict]] = []
        recordCount = 0

        for message in input_messages:
            if message.type == Type.STATE:
                recordCount += _flush(client, buffer)
                yield message
                continue

            if message.type != Type.RECORD or message.record is None:
                continue

            streamName = message.record.stream
            if streamName not in streamModes:
                log.warning("Skipping record from unconfigured stream %r", streamName)
                continue

            document = buildDocument(
                collection=streamName,
                record=message.record.data or {},
                keyField=keyField,
                language=language,
                emittedAt=message.record.emitted_at,
            )
            buffer.append((streamName, document))

            if len(buffer) >= batchSize:
                recordCount += _flush(client, buffer)

        recordCount += _flush(client, buffer)
        log.info("destination-mddb wrote %d record(s) across %d stream(s)", recordCount, len(streamModes))


def _flush(client: MddbClient, buffer: List[Tuple[str, dict]]) -> int:
    if not buffer:
        return 0
    written = client.addBatch(doc for _, doc in buffer)
    buffer.clear()
    return written


def _buildClient(config: Mapping[str, Any]) -> MddbClient:
    return MddbClient(
        baseUrl=config["mddbUrl"],
        apiKey=config.get("apiKey") or None,
        timeoutSeconds=int(config.get("timeoutSeconds", 30)),
        verifySsl=bool(config.get("verifySsl", True)),
    )
