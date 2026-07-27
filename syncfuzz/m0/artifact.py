"""M0 artifact IO: the only legal cross-module communication channel."""

from __future__ import annotations

import os
from pathlib import Path
from typing import Iterable, Iterator, TypeVar

from pydantic import BaseModel

from .errors import ContractViolation, DataLossError, sf_assert
from .schema import RunManifest

T = TypeVar("T", bound=BaseModel)

_ARTIFACTS = {
    "manifest": "manifest.json",
    "kevents": "kevents.jsonl",
    "aevents": "aevents.jsonl",
    "memo": "memo.jsonl",
    "ckpt_index": "ckpt_index.jsonl",
    "ckpt_snapshot_map": "ckpt_snapshot_map.jsonl",
    "timeline": "timeline.jsonl",
    "provenance": "provenance.json",
    "hstar": "hstar.jsonl",
    "violations": "violations.jsonl",
    "coverage": "coverage.jsonl",
    "gap": "gap.json",
    "ablation": "ablation.json",
}


class RunDir:
    """Overview: provide strict, atomic access to one PRD-defined run directory."""

    def __init__(self, root: Path) -> None:
        """Overview: create the fixed run subdirectories without interpreting artifacts."""
        self.root = root
        self.root.mkdir(parents=True, exist_ok=True)
        for name in ("snapshots", "replays", "report"):
            (self.root / name).mkdir(exist_ok=True)

    def path(self, name: str) -> Path:
        """Overview: resolve a symbolic artifact name only when it belongs to the contract."""
        sf_assert(name in _ARTIFACTS, f"unknown artifact name: {name!r}")
        return self.root / _ARTIFACTS[name]

    def write(self, name: str, objects: Iterable[BaseModel]) -> int:
        """Overview: atomically write a JSONL artifact and return its record count."""
        path = self.path(name)
        sf_assert(path.suffix == ".jsonl", f"{name} is not a JSONL artifact")
        temporary = path.with_suffix(".jsonl.tmp")
        count = 0
        try:
            with temporary.open("wb") as handle:
                for obj in objects:
                    handle.write(obj.model_dump_json(exclude_none=False).encode("utf-8") + b"\n")
                    count += 1
                handle.flush()
                os.fsync(handle.fileno())
            os.replace(temporary, path)
        except Exception:
            temporary.unlink(missing_ok=True)
            raise
        return count

    def write_one(self, name: str, obj: BaseModel) -> None:
        """Overview: atomically write one JSON-object artifact without JSONL conversion."""
        path = self.path(name)
        sf_assert(path.suffix == ".json", f"{name} is not a single-object artifact")
        temporary = path.with_suffix(".json.tmp")
        try:
            with temporary.open("w", encoding="utf-8") as handle:
                handle.write(obj.model_dump_json(indent=2, exclude_none=False))
                handle.flush()
                os.fsync(handle.fileno())
            os.replace(temporary, path)
        except Exception:
            temporary.unlink(missing_ok=True)
            raise

    def read(self, name: str, model: type[T]) -> Iterator[T]:
        """Overview: yield strict JSONL records and fail at the first malformed line."""
        path = self.path(name)
        sf_assert(path.suffix == ".jsonl", f"{name} is not a JSONL artifact")
        sf_assert(path.exists(), f"missing artifact: {path}")
        with path.open("rb") as handle:
            for line_number, line in enumerate(handle, start=1):
                if not line.strip():
                    continue
                try:
                    yield model.model_validate_json(line)
                except Exception as error:
                    raise ContractViolation(f"{path}:{line_number} bad record: {error}") from error

    def read_one(self, name: str, model: type[T]) -> T:
        """Overview: parse one JSON-object artifact according to its expected Pydantic model."""
        path = self.path(name)
        sf_assert(path.suffix == ".json", f"{name} is not a single-object artifact")
        sf_assert(path.exists(), f"missing artifact: {path}")
        try:
            return model.model_validate_json(path.read_bytes())
        except Exception as error:
            raise ContractViolation(f"{path} bad record: {error}") from error

    def require_lossless(self) -> RunManifest:
        """Overview: reject runs whose recorded kernel trace contains any dropped event."""
        manifest = self.read_one("manifest", RunManifest)
        if manifest.dropped_events > 0:
            raise DataLossError(
                f"run {manifest.run_id}: dropped_events={manifest.dropped_events}; analysis refused",
            )
        return manifest


def open_run(path: str | Path) -> RunDir:
    """Overview: open one run root through the only M0 artifact-directory constructor."""
    return RunDir(Path(path))
