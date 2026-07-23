#!/usr/bin/env python3

import argparse
import asyncio
import hashlib
import http.server
import json
import os
import shutil
import sys
import threading
import time
import traceback
import urllib.error
import urllib.request
import uuid
from pathlib import Path
from typing import Any

from typing_extensions import Never

WORKFLOW_NAME = "syncfuzz-maf-workflow-checkpoint"
MARKER = "SYNCFUZZ_MAF_WORKFLOW_MARKER"
EFFECT_ARTIFACT = "maf-workflow-effect.txt"
WITNESS_ARTIFACT = "maf-workflow-continuity-check.txt"
EXTERNAL_LEDGER_ARTIFACT = "maf-workflow-external-ledger.jsonl"
EXTERNAL_REPLAY_ARTIFACT = "maf-workflow-external-replay-check.txt"
HTTP_LEDGER_ARTIFACT = "maf-workflow-http-ledger.jsonl"
HTTP_REPLAY_ARTIFACT = "maf-workflow-http-replay-check.txt"
RESOURCE_LEDGER_ARTIFACT = "maf-workflow-resource-ledger.jsonl"
RESOURCE_REPLAY_ARTIFACT = "maf-workflow-resource-replay-check.txt"
AUTHORITY_LEDGER_ARTIFACT = "maf-workflow-authority-ledger.jsonl"
AUTHORITY_REPLAY_ARTIFACT = "maf-workflow-authority-token-replay-check.txt"
PARTIAL_COMMIT_ARTIFACT = "maf-workflow-partial-commit-check.txt"
APPROVAL_PENDING_ARTIFACT = "maf-workflow-approval-pending-check.txt"
REHYDRATE_DIVERGENCE_ARTIFACT = "maf-workflow-rehydrate-divergence-check.txt"
SUMMARY_ARTIFACT = "maf-workflow-summary.json"
CHECKPOINT_DIR = "maf-workflow-checkpoints"
FORK_MANIFEST_ARTIFACT = "maf-workflow-fork-manifest.json"
FORK_OBSERVATION_ARTIFACT = "maf-workflow-fork-observation.json"
CHECKPOINT_CONTINUITY_TASK = "maf-workflow-checkpoint-continuity"
EXTERNAL_EFFECT_REPLAY_TASK = "maf-workflow-external-effect-replay"
HTTP_EFFECT_REPLAY_TASK = "maf-workflow-http-effect-replay"
RESOURCE_REPLAY_TASK = "maf-workflow-resource-replay"
AUTHORITY_TOKEN_REPLAY_TASK = "maf-workflow-authority-token-replay"
PARTIAL_COMMIT_REPLAY_TASK = "maf-workflow-partial-commit-replay"
APPROVAL_PENDING_REPLAY_TASK = "maf-workflow-approval-pending-replay"
REHYDRATE_DIVERGENCE_TASK = "maf-workflow-rehydrate-divergence"
SUPPORTED_TASKS = {
    CHECKPOINT_CONTINUITY_TASK,
    EXTERNAL_EFFECT_REPLAY_TASK,
    HTTP_EFFECT_REPLAY_TASK,
    RESOURCE_REPLAY_TASK,
    AUTHORITY_TOKEN_REPLAY_TASK,
    PARTIAL_COMMIT_REPLAY_TASK,
    APPROVAL_PENDING_REPLAY_TASK,
    REHYDRATE_DIVERGENCE_TASK,
}
EXTERNAL_MARKER = "SYNCFUZZ_MAF_WORKFLOW_EXTERNAL_EFFECT"
AUTHORITY_MARKER = "SYNCFUZZ_MAF_WORKFLOW_AUTHORITY_TOKEN"


def write_json(path: Path, data: dict[str, Any]) -> None:
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    tmp.replace(path)


def sha256_text(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def checkpoint_ids(checkpoint_dir: Path) -> list[str]:
    return [record["checkpoint_id"] for record in checkpoint_records(checkpoint_dir)]


def checkpoint_records(checkpoint_dir: Path) -> list[dict[str, Any]]:
    def sort_key(path: Path) -> tuple[int, str, str]:
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
            return int(data.get("iteration_count") or 0), str(data.get("timestamp") or ""), path.name
        except Exception:
            return 0, "", path.name

    checkpoints = sorted(checkpoint_dir.glob("*.json"), key=sort_key)
    records: list[dict[str, Any]] = []
    for path in checkpoints:
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except Exception:
            data = {}
        records.append(
            {
                "checkpoint_id": path.stem,
                "iteration_count": int(data.get("iteration_count") or 0),
                "timestamp": str(data.get("timestamp") or ""),
                "logical_states": sorted(_json_string_values(data, "logical_state")),
                "message_targets": sorted(str(target) for target in data.get("messages", {}).keys()),
            }
        )
    return records


def _json_string_values(value: Any, field: str) -> set[str]:
    values: set[str] = set()
    if isinstance(value, dict):
        direct = value.get(field)
        if isinstance(direct, str):
            values.add(direct)
        for child in value.values():
            values.update(_json_string_values(child, field))
    elif isinstance(value, list):
        for child in value:
            values.update(_json_string_values(child, field))
    return values


async def run_probe(workspace: Path, task_id: str, pre_timeout: float, restore_timeout: float) -> int:
    if task_id not in SUPPORTED_TASKS:
        raise ValueError(f"unsupported MAF workflow task: {task_id}")
    if task_id == EXTERNAL_EFFECT_REPLAY_TASK:
        return await run_external_effect_replay(workspace, pre_timeout, restore_timeout)
    if task_id == HTTP_EFFECT_REPLAY_TASK:
        return await run_http_effect_replay(workspace, pre_timeout, restore_timeout)
    if task_id == RESOURCE_REPLAY_TASK:
        return await run_resource_replay(workspace, pre_timeout, restore_timeout)
    if task_id == AUTHORITY_TOKEN_REPLAY_TASK:
        return await run_authority_token_replay(workspace, pre_timeout, restore_timeout)
    if task_id == PARTIAL_COMMIT_REPLAY_TASK:
        return await run_partial_commit_replay(workspace, pre_timeout, restore_timeout)
    if task_id == APPROVAL_PENDING_REPLAY_TASK:
        return await run_approval_pending_replay(workspace, pre_timeout, restore_timeout)
    if task_id == REHYDRATE_DIVERGENCE_TASK:
        return await run_rehydrate_divergence(workspace, pre_timeout, restore_timeout)
    return await run_checkpoint_continuity(workspace, pre_timeout, restore_timeout)


async def run_checkpoint_continuity(workspace: Path, pre_timeout: float, restore_timeout: float) -> int:

    from agent_framework import Executor, FileCheckpointStorage, WorkflowBuilder, WorkflowContext, handler

    workspace.mkdir(parents=True, exist_ok=True)
    checkpoint_dir = workspace / CHECKPOINT_DIR
    storage = FileCheckpointStorage(checkpoint_dir)
    effect_path = workspace / EFFECT_ARTIFACT
    witness_path = workspace / WITNESS_ARTIFACT
    events: list[dict[str, Any]] = []

    class PlantExecutor(Executor):
        def __init__(self) -> None:
            super().__init__(id="plant")

        @handler
        async def process(self, message: str, ctx: WorkflowContext[str]) -> None:
            effect_path.write_text(f"{MARKER}\nSOURCE={message}\n", encoding="utf-8")
            events.append({"phase": "plant", "effect_sha256": sha256_text(effect_path.read_text(encoding="utf-8"))})
            await ctx.send_message(MARKER)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            return {"effect_exists": effect_path.exists()}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "plant-restore", "state": state})

    class CheckExecutor(Executor):
        def __init__(self, *, write_witness: bool) -> None:
            super().__init__(id="check")
            self.write_witness = write_witness

        @handler
        async def process(self, marker: str, ctx: WorkflowContext[Never, str]) -> None:
            present = effect_path.exists() and MARKER in effect_path.read_text(encoding="utf-8")
            label = "PRESENT_MAF_WORKFLOW_MARKER" if present else "MISSING_MAF_WORKFLOW_MARKER"
            events.append({"phase": "check", "present": present, "write_witness": self.write_witness})
            if self.write_witness:
                witness_path.write_text(f"{label}\nVALUE={marker if present else 'MISSING'}\n", encoding="utf-8")
                await ctx.yield_output(label)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            return {"witness_exists": witness_path.exists()}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "check-restore", "state": state})

    def build_workflow(max_iterations: int, *, write_witness: bool):
        plant = PlantExecutor()
        check = CheckExecutor(write_witness=write_witness)
        return (
            WorkflowBuilder(
                start_executor=plant,
                name=WORKFLOW_NAME,
                checkpoint_storage=storage,
                max_iterations=max_iterations,
                output_from=[check],
            )
            .add_edge(plant, check)
            .build()
        )

    summary: dict[str, Any] = {
        "schema_version": "syncfuzz.maf-workflow-checkpoint.v1",
        "task_id": CHECKPOINT_CONTINUITY_TASK,
        "workflow_name": WORKFLOW_NAME,
        "checkpoint_backend": "file",
        "checkpoint_dir": str(checkpoint_dir),
        "checkpoint_ids": [],
        "selected_checkpoint_id": "",
        "selected_checkpoint_iteration": 0,
        "restored": False,
        "runtime_object_recreated": False,
        "pre_restore_timed_out": False,
        "post_restore_timed_out": False,
        "effect_written": False,
        "continuity_observed": False,
        "post_restore_outputs": [],
        "events": events,
    }

    try:
        await asyncio.wait_for(build_workflow(100, write_witness=False).run("syncfuzz-plant"), timeout=pre_timeout)
    except asyncio.TimeoutError:
        summary["pre_restore_timed_out"] = True
    except Exception as exc:
        summary["pre_restore_error"] = f"{type(exc).__name__}: {exc}"

    ids = checkpoint_ids(checkpoint_dir)
    summary["checkpoint_ids"] = ids
    summary["effect_written"] = effect_path.exists() and MARKER in effect_path.read_text(encoding="utf-8")
    if witness_path.exists():
        witness_path.unlink()

    if ids:
        selected = ids[0]
        summary["selected_checkpoint_id"] = selected
        summary["selected_checkpoint_iteration"] = 0
        try:
            result = await asyncio.wait_for(
                build_workflow(100, write_witness=True).run(checkpoint_id=selected, checkpoint_storage=storage),
                timeout=restore_timeout,
            )
            summary["post_restore_outputs"] = [str(value) for value in result.get_outputs()]
            summary["restored"] = True
        except asyncio.TimeoutError:
            summary["post_restore_timed_out"] = True
            summary["restored"] = witness_path.exists()
        except Exception as exc:
            summary["post_restore_error"] = f"{type(exc).__name__}: {exc}"
            summary["post_restore_traceback"] = traceback.format_exc(limit=4)
            summary["restored"] = witness_path.exists()
        summary["runtime_object_recreated"] = True

    summary["continuity_observed"] = (
        witness_path.exists()
        and "PRESENT_MAF_WORKFLOW_MARKER" in witness_path.read_text(encoding="utf-8")
        and MARKER in witness_path.read_text(encoding="utf-8")
    )
    write_json(workspace / SUMMARY_ARTIFACT, summary)
    if summary["restored"] and summary["continuity_observed"]:
        return 0
    return 1


def _read_marker_effect(effect_path: Path) -> bool:
    return effect_path.exists() and MARKER in effect_path.read_text(encoding="utf-8")


def _build_v2_checkpoint_continuity_workflow(
    workspace: Path,
    storage: Any,
    events: list[dict[str, Any]],
    *,
    write_witness: bool,
):
    """Build the smallest real MAF workflow used by the V2.3 fork adapter.

    The extra Start executor deliberately makes two durable MAF checkpoints:
    one before Plant writes the workspace effect, and one after it.  These are
    Agent-native coordinates, not SyncFuzz controller observation markers.
    """

    from agent_framework import Executor, WorkflowBuilder, WorkflowContext, handler

    effect_path = workspace / EFFECT_ARTIFACT
    witness_path = workspace / WITNESS_ARTIFACT

    class StartExecutor(Executor):
        def __init__(self) -> None:
            super().__init__(id="v2-start")

        @handler
        async def process(self, message: str, ctx: WorkflowContext[str]) -> None:
            events.append({"phase": "start", "message": message, "monotonic_ns": time.monotonic_ns()})
            await ctx.send_message(message)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            events.append({"phase": "checkpoint-save", "executor": "start", "monotonic_ns": time.monotonic_ns()})
            return {"logical_state": "before-effect", "effect_exists": _read_marker_effect(effect_path)}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "start-restore", "state": state, "monotonic_ns": time.monotonic_ns()})

    class PlantExecutor(Executor):
        def __init__(self) -> None:
            super().__init__(id="v2-plant")

        @handler
        async def process(self, message: str, ctx: WorkflowContext[str]) -> None:
            effect_path.write_text(f"{MARKER}\nSOURCE={message}\n", encoding="utf-8")
            events.append({"phase": "plant", "monotonic_ns": time.monotonic_ns()})
            await ctx.send_message(MARKER)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            events.append({"phase": "checkpoint-save", "executor": "plant", "monotonic_ns": time.monotonic_ns()})
            return {"logical_state": "after-effect", "effect_exists": _read_marker_effect(effect_path)}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "plant-restore", "state": state, "monotonic_ns": time.monotonic_ns()})

    class CheckExecutor(Executor):
        def __init__(self) -> None:
            super().__init__(id="v2-check")

        @handler
        async def process(self, marker: str, ctx: WorkflowContext[Never, str]) -> None:
            present = _read_marker_effect(effect_path)
            label = "PRESENT_MAF_WORKFLOW_MARKER" if present else "MISSING_MAF_WORKFLOW_MARKER"
            events.append(
                {
                    "phase": "check",
                    "marker": marker,
                    "present": present,
                    "write_witness": write_witness,
                    "monotonic_ns": time.monotonic_ns(),
                }
            )
            if write_witness:
                witness_path.write_text(f"{label}\nVALUE={marker if present else 'MISSING'}\n", encoding="utf-8")
                await ctx.yield_output(label)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            events.append({"phase": "checkpoint-save", "executor": "check", "monotonic_ns": time.monotonic_ns()})
            return {"logical_state": "after-effect", "effect_exists": _read_marker_effect(effect_path)}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "check-restore", "state": state, "monotonic_ns": time.monotonic_ns()})

    start = StartExecutor()
    plant = PlantExecutor()
    check = CheckExecutor()
    return (
        WorkflowBuilder(
            start_executor=start,
            name=WORKFLOW_NAME + "-v2-fork",
            checkpoint_storage=storage,
            max_iterations=100,
            output_from=[check],
        )
        .add_edge(start, plant)
        .add_edge(plant, check)
        .build()
    )


async def prepare_v2_fork(workspace: Path, task_id: str, pre_timeout: float) -> int:
    """Create a reusable, real MAF durable-checkpoint fixture for fork tests."""

    from agent_framework import FileCheckpointStorage

    if task_id != CHECKPOINT_CONTINUITY_TASK:
        raise ValueError("V2.3 MAF fork preparation currently supports only " + CHECKPOINT_CONTINUITY_TASK)
    checkpoint_dir = workspace / CHECKPOINT_DIR
    manifest_path = workspace / FORK_MANIFEST_ARTIFACT
    if checkpoint_dir.exists() and any(checkpoint_dir.iterdir()):
        raise ValueError(f"refusing to reuse non-empty MAF checkpoint directory: {checkpoint_dir}")
    if manifest_path.exists():
        raise ValueError(f"refusing to overwrite existing fork manifest: {manifest_path}")
    workspace.mkdir(parents=True, exist_ok=True)
    storage = FileCheckpointStorage(checkpoint_dir)
    events: list[dict[str, Any]] = []
    try:
        await asyncio.wait_for(
            _build_v2_checkpoint_continuity_workflow(workspace, storage, events, write_witness=False).run("syncfuzz-v2-start"),
            timeout=pre_timeout,
        )
    except asyncio.TimeoutError as exc:
        raise RuntimeError("initial MAF workflow did not complete while preparing V2.3 fork checkpoints") from exc

    records = checkpoint_records(checkpoint_dir)
    if len(records) < 2:
        raise RuntimeError(
            "MAF workflow did not persist the required before/after durable checkpoints; "
            f"observed {len(records)} checkpoint(s)"
        )
    if not _read_marker_effect(workspace / EFFECT_ARTIFACT):
        raise RuntimeError("initial MAF workflow did not form the expected workspace effect")
    # MAF serializes every executor's custom state in every checkpoint, so the
    # executor-state payload alone cannot identify the active recovery
    # coordinate. Its native checkpoint message queue can: v2-start is the
    # queue before Plant executes, while v2-plant carries Plant's marker after
    # the effect was formed. This checks that persisted native data directly;
    # it never assigns before/after from file order.
    before_record = next((record for record in records if "v2-start" in record["message_targets"]), None)
    after_record = next((record for record in records if "v2-plant" in record["message_targets"]), None)
    if before_record is None or after_record is None or before_record["checkpoint_id"] == after_record["checkpoint_id"]:
        raise RuntimeError(
            "MAF durable checkpoints did not preserve distinct active-message recovery coordinates; "
            "the V2.3 adapter will not infer them from checkpoint order"
        )
    native_checkpoints = [
        {
            **before_record,
            "coordinate": "before-effect",
            "agent_state": "absent",
            "expected_effect": "absent",
        },
        {
            **after_record,
            "coordinate": "after-effect",
            "agent_state": "present",
            "expected_effect": "present",
        },
    ]
    manifest = {
        "schema_version": "syncfuzz.maf-workflow-fork-manifest.v1",
        "task_id": task_id,
        "workflow_name": WORKFLOW_NAME + "-v2-fork",
        "checkpoint_backend": "file",
        "checkpoint_dir": str(checkpoint_dir),
        "native_checkpoints": native_checkpoints,
        "initial_runtime_instance_id": "maf-workflow-initial-" + uuid.uuid4().hex,
        "initial_events": events,
    }
    write_json(manifest_path, manifest)
    return 0


def _copy_fork_workspace(source_workspace: Path, destination_workspace: Path) -> None:
    source = source_workspace.resolve()
    destination = destination_workspace.resolve()
    if not source.is_dir():
        raise ValueError(f"prepared MAF fork workspace does not exist: {source}")
    if destination.exists():
        raise ValueError(f"refusing to overwrite fork runtime workspace: {destination}")
    try:
        destination.relative_to(source)
    except ValueError:
        pass
    else:
        raise ValueError("fork runtime workspace must not be created inside the prepared workspace")
    shutil.copytree(source, destination)


async def observe_v2_fork(
    source_workspace: Path,
    workspace: Path,
    task_id: str,
    checkpoint_id: str,
    runtime_instance_id: str,
    restore_timeout: float,
    observation_out: Path,
) -> int:
    """Restore one exact MAF file checkpoint in a new Python runtime.

    `source_workspace` is immutable input from preparation.  Every invocation
    creates a separate copy, rebuilds the Workflow object, and restores the
    selected file checkpoint inside that fresh process/runtime.
    """

    from agent_framework import FileCheckpointStorage

    if task_id != CHECKPOINT_CONTINUITY_TASK:
        raise ValueError("V2.3 MAF fork observation currently supports only " + CHECKPOINT_CONTINUITY_TASK)
    _copy_fork_workspace(source_workspace, workspace)
    manifest_path = workspace / FORK_MANIFEST_ARTIFACT
    if not manifest_path.exists():
        raise ValueError(f"prepared MAF fork manifest is missing: {manifest_path}")
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    if manifest.get("schema_version") != "syncfuzz.maf-workflow-fork-manifest.v1":
        raise ValueError("prepared MAF fork manifest has an unsupported schema")
    selected = next(
        (record for record in manifest.get("native_checkpoints", []) if record.get("checkpoint_id") == checkpoint_id),
        None,
    )
    if selected is None:
        raise ValueError(f"checkpoint {checkpoint_id!r} is not listed by the prepared MAF fork manifest")
    checkpoint_dir = workspace / CHECKPOINT_DIR
    if not (checkpoint_dir / f"{checkpoint_id}.json").is_file():
        raise ValueError(f"durable MAF checkpoint file is missing: {checkpoint_id}")

    effect_path = workspace / EFFECT_ARTIFACT
    effect_before = _read_marker_effect(effect_path)
    events: list[dict[str, Any]] = []
    storage = FileCheckpointStorage(checkpoint_dir)
    restored = False
    restore_error = ""
    outputs: list[str] = []
    try:
        result = await asyncio.wait_for(
            _build_v2_checkpoint_continuity_workflow(workspace, storage, events, write_witness=True).run(
                checkpoint_id=checkpoint_id,
                checkpoint_storage=storage,
            ),
            timeout=restore_timeout,
        )
        outputs = [str(value) for value in result.get_outputs()]
        restored = True
    except Exception as exc:
        restore_error = f"{type(exc).__name__}: {exc}"

    effect_after = _read_marker_effect(effect_path)
    plant_reexecuted = any(event.get("phase") == "plant" for event in events)
    if not effect_after:
        os_state = "absent"
        os_state_origin = "none"
    elif plant_reexecuted:
        os_state = "present"
        os_state_origin = "reconstructed"
    else:
        os_state = "present"
        os_state_origin = "residual"
    multiplicity = "duplicate" if sum(1 for event in events if event.get("phase") == "plant") > 1 else "single"
    evidence = [
        "restored exact MAF file checkpoint " + checkpoint_id,
        "recreated Workflow object in runtime " + runtime_instance_id,
        "logical checkpoint coordinate " + str(selected.get("coordinate") or "unknown"),
        "workspace effect before recovery=" + str(effect_before).lower(),
        "workspace effect after recovery=" + str(effect_after).lower(),
        "plant reexecuted during recovery=" + str(plant_reexecuted).lower(),
    ]
    if restore_error:
        evidence.append("restore error: " + restore_error)
    observation = {
        "schema_version": "syncfuzz.maf-workflow-fork-observation.v1",
        "task_id": task_id,
        "checkpoint_id": checkpoint_id,
        "checkpoint_coordinate": selected.get("coordinate", ""),
        "runtime_instance_id": runtime_instance_id,
        "workflow_object_recreated": True,
        "restored": restored,
        "agent_state": selected.get("agent_state", "unknown"),
        "os_state": os_state,
        "os_state_origin": os_state_origin,
        "effect_multiplicity": multiplicity,
        "outputs": outputs,
        "events": events,
        "evidence": evidence,
    }
    write_json(observation_out, observation)
    return 0 if restored and effect_after else 1


async def run_external_effect_replay(workspace: Path, pre_timeout: float, restore_timeout: float) -> int:
    from agent_framework import Executor, FileCheckpointStorage, WorkflowBuilder, WorkflowContext, handler

    workspace.mkdir(parents=True, exist_ok=True)
    checkpoint_dir = workspace / CHECKPOINT_DIR
    storage = FileCheckpointStorage(checkpoint_dir)
    ledger_path = workspace / EXTERNAL_LEDGER_ARTIFACT
    witness_path = workspace / EXTERNAL_REPLAY_ARTIFACT
    operation_id = "syncfuzz-maf-workflow-operation-1"
    events: list[dict[str, Any]] = []

    def ledger_entries() -> list[dict[str, Any]]:
        if not ledger_path.exists():
            return []
        entries: list[dict[str, Any]] = []
        for line in ledger_path.read_text(encoding="utf-8").splitlines():
            if line.strip():
                entries.append(json.loads(line))
        return entries

    def append_ledger_entry(source: str) -> None:
        entries = ledger_entries()
        entry = {
            "index": len(entries) + 1,
            "operation_id": operation_id,
            "marker": EXTERNAL_MARKER,
            "source": source,
        }
        with ledger_path.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps(entry, sort_keys=True) + "\n")
        events.append({"phase": "external-effect", "source": source, "entry_index": entry["index"]})

    class StartExecutor(Executor):
        def __init__(self) -> None:
            super().__init__(id="start")

        @handler
        async def process(self, message: str, ctx: WorkflowContext[str]) -> None:
            events.append({"phase": "start", "message": message})
            await ctx.send_message(operation_id)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            return {"operation_id": operation_id}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "start-restore", "state": state})

    class EffectExecutor(Executor):
        def __init__(self, *, source: str, write_witness: bool) -> None:
            super().__init__(id="effect")
            self.source = source
            self.write_witness = write_witness

        @handler
        async def process(self, op_id: str, ctx: WorkflowContext[Never, str]) -> None:
            append_ledger_entry(self.source)
            entries = [entry for entry in ledger_entries() if entry.get("operation_id") == op_id]
            duplicate = len(entries) >= 2
            if self.write_witness:
                label = "DUPLICATE_MAF_WORKFLOW_EXTERNAL_EFFECT" if duplicate else "SINGLE_MAF_WORKFLOW_EXTERNAL_EFFECT"
                witness_path.write_text(
                    f"{label}\nOPERATION_ID={op_id}\nMARKER={EXTERNAL_MARKER}\nENTRIES={len(entries)}\n",
                    encoding="utf-8",
                )
                await ctx.yield_output(label)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            return {"ledger_entries": len(ledger_entries())}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "effect-restore", "state": state})

    def build_workflow(max_iterations: int, *, source: str, write_witness: bool):
        start = StartExecutor()
        effect = EffectExecutor(source=source, write_witness=write_witness)
        return (
            WorkflowBuilder(
                start_executor=start,
                name=WORKFLOW_NAME + "-external-replay",
                checkpoint_storage=storage,
                max_iterations=max_iterations,
                output_from=[effect],
            )
            .add_edge(start, effect)
            .build()
        )

    summary: dict[str, Any] = {
        "schema_version": "syncfuzz.maf-workflow-checkpoint.v1",
        "task_id": EXTERNAL_EFFECT_REPLAY_TASK,
        "workflow_name": WORKFLOW_NAME + "-external-replay",
        "checkpoint_backend": "file",
        "checkpoint_dir": str(checkpoint_dir),
        "checkpoint_ids": [],
        "selected_checkpoint_id": "",
        "selected_checkpoint_iteration": 0,
        "restored": False,
        "runtime_object_recreated": False,
        "pre_restore_timed_out": False,
        "post_restore_timed_out": False,
        "duplicate_effect_observed": False,
        "external_effect_entries": 0,
        "operation_id": operation_id,
        "post_restore_outputs": [],
        "events": events,
    }

    try:
        await asyncio.wait_for(
            build_workflow(100, source="initial-run", write_witness=False).run("syncfuzz-start"),
            timeout=pre_timeout,
        )
    except asyncio.TimeoutError:
        summary["pre_restore_timed_out"] = True
    except Exception as exc:
        summary["pre_restore_error"] = f"{type(exc).__name__}: {exc}"

    ids = checkpoint_ids(checkpoint_dir)
    summary["checkpoint_ids"] = ids
    if witness_path.exists():
        witness_path.unlink()

    if ids:
        selected = ids[0]
        summary["selected_checkpoint_id"] = selected
        try:
            result = await asyncio.wait_for(
                build_workflow(100, source="restored-run", write_witness=True).run(
                    checkpoint_id=selected,
                    checkpoint_storage=storage,
                ),
                timeout=restore_timeout,
            )
            summary["post_restore_outputs"] = [str(value) for value in result.get_outputs()]
            summary["restored"] = True
        except asyncio.TimeoutError:
            summary["post_restore_timed_out"] = True
            summary["restored"] = witness_path.exists()
        except Exception as exc:
            summary["post_restore_error"] = f"{type(exc).__name__}: {exc}"
            summary["post_restore_traceback"] = traceback.format_exc(limit=4)
            summary["restored"] = witness_path.exists()
        summary["runtime_object_recreated"] = True

    operation_entries = [entry for entry in ledger_entries() if entry.get("operation_id") == operation_id]
    summary["external_effect_entries"] = len(operation_entries)
    summary["duplicate_effect_observed"] = len(operation_entries) >= 2
    write_json(workspace / SUMMARY_ARTIFACT, summary)
    if summary["restored"] and summary["duplicate_effect_observed"]:
        return 0
    return 1


async def run_http_effect_replay(workspace: Path, pre_timeout: float, restore_timeout: float) -> int:
    from agent_framework import Executor, FileCheckpointStorage, WorkflowBuilder, WorkflowContext, handler

    workspace.mkdir(parents=True, exist_ok=True)
    checkpoint_dir = workspace / CHECKPOINT_DIR
    storage = FileCheckpointStorage(checkpoint_dir)
    ledger_path = workspace / HTTP_LEDGER_ARTIFACT
    witness_path = workspace / HTTP_REPLAY_ARTIFACT
    operation_id = "syncfuzz-maf-workflow-http-operation-1"
    events: list[dict[str, Any]] = []
    owned_server: http.server.ThreadingHTTPServer | None = None
    service_mode = "external-process"

    def ledger_entries() -> list[dict[str, Any]]:
        if not ledger_path.exists():
            return []
        entries: list[dict[str, Any]] = []
        for line in ledger_path.read_text(encoding="utf-8").splitlines():
            if line.strip():
                entries.append(json.loads(line))
        return entries

    def append_service_entry(source: str, op_id: str, *, service: str = "local-http-effect-server") -> int:
        entries = ledger_entries()
        entry = {
            "index": len(entries) + 1,
            "operation_id": op_id,
            "marker": EXTERNAL_MARKER,
            "source": source,
            "service": service,
        }
        with ledger_path.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps(entry, sort_keys=True) + "\n")
        events.append({"phase": "http-service-commit", "source": source, "service": service, "entry_index": entry["index"]})
        return len([item for item in ledger_entries() if item.get("operation_id") == op_id])

    class EffectHandler(http.server.BaseHTTPRequestHandler):
        def do_POST(self) -> None:  # noqa: N802
            if self.path == "/reset":
                if ledger_path.exists():
                    ledger_path.unlink()
                response = json.dumps({"ok": True}).encode("utf-8")
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(response)))
                self.end_headers()
                self.wfile.write(response)
                return
            if self.path != "/effect/commits":
                self.send_response(404)
                self.end_headers()
                return
            length = int(self.headers.get("Content-Length", "0"))
            body = self.rfile.read(length)
            try:
                payload = json.loads(body.decode("utf-8"))
                op_id = str(payload.get("operation_id") or payload.get("operationId") or operation_id)
                source = str(payload.get("source") or "unknown")
                count = append_service_entry(source, op_id)
                response = json.dumps({"ok": True, "count": count}).encode("utf-8")
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(response)))
                self.end_headers()
                self.wfile.write(response)
            except Exception as exc:
                response = json.dumps({"ok": False, "error": str(exc)}).encode("utf-8")
                self.send_response(500)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(response)))
                self.end_headers()
                self.wfile.write(response)

        def log_message(self, format: str, *args: Any) -> None:
            return

    configured_service_url = os.environ.get("MAF_WORKFLOW_EFFECT_SERVICE_URL", "").strip().rstrip("/")
    if configured_service_url:
        service_url = configured_service_url
    else:
        service_mode = "in-process-fallback"
        owned_server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), EffectHandler)
        server_thread = threading.Thread(
            target=owned_server.serve_forever,
            name="syncfuzz-maf-http-effect-server",
            daemon=True,
        )
        server_thread.start()
        service_url = f"http://127.0.0.1:{owned_server.server_address[1]}"

    def post_json(path: str, value: dict[str, Any]) -> dict[str, Any]:
        payload = json.dumps(value).encode("utf-8")
        request = urllib.request.Request(
            service_url + path,
            data=payload,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urllib.request.urlopen(request, timeout=3) as response:
            return json.loads(response.read().decode("utf-8"))

    def reset_service() -> None:
        try:
            if service_mode != "in-process-fallback" and ledger_path.exists():
                ledger_path.unlink()
            post_json("/reset", {})
            events.append({"phase": "http-service-reset", "service_url": service_url, "service_mode": service_mode})
        except Exception as exc:
            events.append({"phase": "http-service-reset-failed", "service_url": service_url, "error": str(exc)})

    def post_commit(source: str, op_id: str) -> int:
        data = post_json(
            "/effect/commits",
            {
                "operation_id": op_id,
                "operationId": op_id,
                "source": source,
                "marker": EXTERNAL_MARKER,
                "payload": {"task_id": HTTP_EFFECT_REPLAY_TASK},
            },
        )
        count = int(data.get("count") or 0)
        if service_mode != "in-process-fallback":
            count = append_service_entry(source, op_id, service="external-http-effect-server")
        return count

    class StartExecutor(Executor):
        def __init__(self) -> None:
            super().__init__(id="http-start")

        @handler
        async def process(self, message: str, ctx: WorkflowContext[str]) -> None:
            events.append({"phase": "http-start", "message": message})
            await ctx.send_message(operation_id)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            return {"operation_id": operation_id}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "http-start-restore", "state": state})

    class HTTPExecutor(Executor):
        def __init__(self, *, source: str, write_witness: bool) -> None:
            super().__init__(id="http-effect")
            self.source = source
            self.write_witness = write_witness

        @handler
        async def process(self, op_id: str, ctx: WorkflowContext[Never, str]) -> None:
            count = post_commit(self.source, op_id)
            duplicate = count >= 2
            if self.write_witness:
                label = "DUPLICATE_MAF_WORKFLOW_HTTP_EFFECT" if duplicate else "SINGLE_MAF_WORKFLOW_HTTP_EFFECT"
                witness_path.write_text(
                    f"{label}\nOPERATION_ID={op_id}\nMARKER={EXTERNAL_MARKER}\nENTRIES={count}\nSERVICE_URL={service_url}\nSERVICE_MODE={service_mode}\n",
                    encoding="utf-8",
                )
                await ctx.yield_output(label)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            return {"ledger_entries": len(ledger_entries()), "service_url": service_url}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "http-effect-restore", "state": state})

    def build_workflow(max_iterations: int, *, source: str, write_witness: bool):
        start = StartExecutor()
        effect = HTTPExecutor(source=source, write_witness=write_witness)
        return (
            WorkflowBuilder(
                start_executor=start,
                name=WORKFLOW_NAME + "-http-effect-replay",
                checkpoint_storage=storage,
                max_iterations=max_iterations,
                output_from=[effect],
            )
            .add_edge(start, effect)
            .build()
        )

    summary: dict[str, Any] = {
        "schema_version": "syncfuzz.maf-workflow-checkpoint.v1",
        "task_id": HTTP_EFFECT_REPLAY_TASK,
        "workflow_name": WORKFLOW_NAME + "-http-effect-replay",
        "checkpoint_backend": "file",
        "checkpoint_dir": str(checkpoint_dir),
        "checkpoint_ids": [],
        "selected_checkpoint_id": "",
        "selected_checkpoint_iteration": 0,
        "restored": False,
        "runtime_object_recreated": False,
        "pre_restore_timed_out": False,
        "post_restore_timed_out": False,
        "duplicate_effect_observed": False,
        "external_effect_entries": 0,
        "external_service_url": service_url,
        "external_service_mode": service_mode,
        "external_service_observed": False,
        "operation_id": operation_id,
        "post_restore_outputs": [],
        "events": events,
    }

    try:
        reset_service()
        try:
            await asyncio.wait_for(
                build_workflow(100, source="initial-run", write_witness=False).run("syncfuzz-start"),
                timeout=pre_timeout,
            )
        except asyncio.TimeoutError:
            summary["pre_restore_timed_out"] = True
        except Exception as exc:
            summary["pre_restore_error"] = f"{type(exc).__name__}: {exc}"
            summary["pre_restore_traceback"] = traceback.format_exc(limit=4)

        ids = checkpoint_ids(checkpoint_dir)
        summary["checkpoint_ids"] = ids
        if witness_path.exists():
            witness_path.unlink()

        if ids:
            selected = ids[0]
            summary["selected_checkpoint_id"] = selected
            try:
                result = await asyncio.wait_for(
                    build_workflow(100, source="restored-run", write_witness=True).run(
                        checkpoint_id=selected,
                        checkpoint_storage=storage,
                    ),
                    timeout=restore_timeout,
                )
                summary["post_restore_outputs"] = [str(value) for value in result.get_outputs()]
                summary["restored"] = True
            except asyncio.TimeoutError:
                summary["post_restore_timed_out"] = True
                summary["restored"] = witness_path.exists()
            except Exception as exc:
                summary["post_restore_error"] = f"{type(exc).__name__}: {exc}"
                summary["post_restore_traceback"] = traceback.format_exc(limit=4)
                summary["restored"] = witness_path.exists()
            summary["runtime_object_recreated"] = True

        operation_entries = [entry for entry in ledger_entries() if entry.get("operation_id") == operation_id]
        summary["external_effect_entries"] = len(operation_entries)
        summary["external_service_observed"] = len(operation_entries) > 0
        summary["duplicate_effect_observed"] = len(operation_entries) >= 2
        write_json(workspace / SUMMARY_ARTIFACT, summary)
        if summary["restored"] and summary["external_service_observed"] and summary["duplicate_effect_observed"]:
            return 0
        return 1
    finally:
        if owned_server is not None:
            owned_server.shutdown()
            owned_server.server_close()


async def run_resource_replay(workspace: Path, pre_timeout: float, restore_timeout: float) -> int:
    from agent_framework import Executor, FileCheckpointStorage, WorkflowBuilder, WorkflowContext, handler

    workspace.mkdir(parents=True, exist_ok=True)
    checkpoint_dir = workspace / CHECKPOINT_DIR
    storage = FileCheckpointStorage(checkpoint_dir)
    ledger_path = workspace / RESOURCE_LEDGER_ARTIFACT
    witness_path = workspace / RESOURCE_REPLAY_ARTIFACT
    operation_id = "syncfuzz-maf-workflow-resource-operation-1"
    events: list[dict[str, Any]] = []
    owned_server: http.server.ThreadingHTTPServer | None = None
    service_mode = "external-process"

    def ledger_entries() -> list[dict[str, Any]]:
        if not ledger_path.exists():
            return []
        entries: list[dict[str, Any]] = []
        for line in ledger_path.read_text(encoding="utf-8").splitlines():
            if line.strip():
                entries.append(json.loads(line))
        return entries

    def append_resource_entry(source: str, op_id: str, *, service: str, resource_id: str = "") -> int:
        entries = ledger_entries()
        entry = {
            "index": len(entries) + 1,
            "operation_id": op_id,
            "marker": EXTERNAL_MARKER,
            "source": source,
            "service": service,
            "resource_id": resource_id,
        }
        with ledger_path.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps(entry, sort_keys=True) + "\n")
        events.append(
            {
                "phase": "resource-service-create",
                "source": source,
                "service": service,
                "resource_id": resource_id,
                "entry_index": entry["index"],
            }
        )
        return len([item for item in ledger_entries() if item.get("operation_id") == op_id])

    class ResourceHandler(http.server.BaseHTTPRequestHandler):
        def do_POST(self) -> None:  # noqa: N802
            if self.path == "/reset":
                if ledger_path.exists():
                    ledger_path.unlink()
                response = json.dumps({"ok": True}).encode("utf-8")
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(response)))
                self.end_headers()
                self.wfile.write(response)
                return
            if self.path != "/effect/resources":
                self.send_response(404)
                self.end_headers()
                return
            length = int(self.headers.get("Content-Length", "0"))
            body = self.rfile.read(length)
            try:
                payload = json.loads(body.decode("utf-8"))
                body_payload = payload.get("payload") if isinstance(payload.get("payload"), dict) else {}
                op_id = str(body_payload.get("operation_id") or payload.get("operation_id") or operation_id)
                source = str(body_payload.get("source") or payload.get("source") or "unknown")
                resource_id = f"resource_{len(ledger_entries()) + 1}"
                count = append_resource_entry(source, op_id, service="local-http-resource-service", resource_id=resource_id)
                response = json.dumps(
                    {
                        "resource": {
                            "id": resource_id,
                            "kind": payload.get("kind") or "syncfuzz-maf-workflow-resource",
                            "payload": payload.get("payload"),
                        },
                        "idempotentReplay": False,
                        "count": count,
                    }
                ).encode("utf-8")
                self.send_response(201)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(response)))
                self.end_headers()
                self.wfile.write(response)
            except Exception as exc:
                response = json.dumps({"ok": False, "error": str(exc)}).encode("utf-8")
                self.send_response(500)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(response)))
                self.end_headers()
                self.wfile.write(response)

        def log_message(self, format: str, *args: Any) -> None:
            return

    configured_service_url = os.environ.get("MAF_WORKFLOW_EFFECT_SERVICE_URL", "").strip().rstrip("/")
    if configured_service_url:
        service_url = configured_service_url
    else:
        service_mode = "in-process-fallback"
        owned_server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ResourceHandler)
        server_thread = threading.Thread(
            target=owned_server.serve_forever,
            name="syncfuzz-maf-resource-effect-server",
            daemon=True,
        )
        server_thread.start()
        service_url = f"http://127.0.0.1:{owned_server.server_address[1]}"

    def post_json(path: str, value: dict[str, Any]) -> dict[str, Any]:
        payload = json.dumps(value).encode("utf-8")
        request = urllib.request.Request(
            service_url + path,
            data=payload,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urllib.request.urlopen(request, timeout=3) as response:
            return json.loads(response.read().decode("utf-8"))

    def reset_service() -> None:
        try:
            if service_mode != "in-process-fallback" and ledger_path.exists():
                ledger_path.unlink()
            post_json("/reset", {})
            events.append({"phase": "resource-service-reset", "service_url": service_url, "service_mode": service_mode})
        except Exception as exc:
            events.append({"phase": "resource-service-reset-failed", "service_url": service_url, "error": str(exc)})

    def post_resource(source: str, op_id: str) -> int:
        data = post_json(
            "/effect/resources",
            {
                "kind": "syncfuzz-maf-workflow-resource",
                "payload": {
                    "operation_id": op_id,
                    "marker": EXTERNAL_MARKER,
                    "source": source,
                    "task_id": RESOURCE_REPLAY_TASK,
                },
            },
        )
        resource = data.get("resource") if isinstance(data.get("resource"), dict) else {}
        resource_id = str(resource.get("id") or "")
        if service_mode != "in-process-fallback":
            return append_resource_entry(
                source,
                op_id,
                service="external-http-resource-service",
                resource_id=resource_id,
            )
        return int(data.get("count") or len([entry for entry in ledger_entries() if entry.get("operation_id") == op_id]))

    class StartExecutor(Executor):
        def __init__(self) -> None:
            super().__init__(id="resource-start")

        @handler
        async def process(self, message: str, ctx: WorkflowContext[str]) -> None:
            events.append({"phase": "resource-start", "message": message})
            await ctx.send_message(operation_id)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            return {"operation_id": operation_id}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "resource-start-restore", "state": state})

    class ResourceExecutor(Executor):
        def __init__(self, *, source: str, write_witness: bool) -> None:
            super().__init__(id="resource-effect")
            self.source = source
            self.write_witness = write_witness

        @handler
        async def process(self, op_id: str, ctx: WorkflowContext[Never, str]) -> None:
            count = post_resource(self.source, op_id)
            duplicate = count >= 2
            if self.write_witness:
                label = "DUPLICATE_MAF_WORKFLOW_RESOURCE_EFFECT" if duplicate else "SINGLE_MAF_WORKFLOW_RESOURCE_EFFECT"
                witness_path.write_text(
                    f"{label}\nOPERATION_ID={op_id}\nMARKER={EXTERNAL_MARKER}\nENTRIES={count}\nSERVICE_URL={service_url}\nSERVICE_MODE={service_mode}\n",
                    encoding="utf-8",
                )
                await ctx.yield_output(label)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            return {"ledger_entries": len(ledger_entries()), "service_url": service_url}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "resource-effect-restore", "state": state})

    def build_workflow(max_iterations: int, *, source: str, write_witness: bool):
        start = StartExecutor()
        resource = ResourceExecutor(source=source, write_witness=write_witness)
        return (
            WorkflowBuilder(
                start_executor=start,
                name=WORKFLOW_NAME + "-resource-replay",
                checkpoint_storage=storage,
                max_iterations=max_iterations,
                output_from=[resource],
            )
            .add_edge(start, resource)
            .build()
        )

    summary: dict[str, Any] = {
        "schema_version": "syncfuzz.maf-workflow-checkpoint.v1",
        "task_id": RESOURCE_REPLAY_TASK,
        "workflow_name": WORKFLOW_NAME + "-resource-replay",
        "checkpoint_backend": "file",
        "checkpoint_dir": str(checkpoint_dir),
        "checkpoint_ids": [],
        "selected_checkpoint_id": "",
        "selected_checkpoint_iteration": 0,
        "restored": False,
        "runtime_object_recreated": False,
        "pre_restore_timed_out": False,
        "post_restore_timed_out": False,
        "duplicate_effect_observed": False,
        "external_effect_entries": 0,
        "external_service_url": service_url,
        "external_service_mode": service_mode,
        "external_service_observed": False,
        "operation_id": operation_id,
        "post_restore_outputs": [],
        "events": events,
    }

    try:
        reset_service()
        try:
            await asyncio.wait_for(
                build_workflow(100, source="initial-run", write_witness=False).run("syncfuzz-start"),
                timeout=pre_timeout,
            )
        except asyncio.TimeoutError:
            summary["pre_restore_timed_out"] = True
        except Exception as exc:
            summary["pre_restore_error"] = f"{type(exc).__name__}: {exc}"
            summary["pre_restore_traceback"] = traceback.format_exc(limit=4)

        ids = checkpoint_ids(checkpoint_dir)
        summary["checkpoint_ids"] = ids
        if witness_path.exists():
            witness_path.unlink()

        if ids:
            selected = ids[0]
            summary["selected_checkpoint_id"] = selected
            try:
                result = await asyncio.wait_for(
                    build_workflow(100, source="restored-run", write_witness=True).run(
                        checkpoint_id=selected,
                        checkpoint_storage=storage,
                    ),
                    timeout=restore_timeout,
                )
                summary["post_restore_outputs"] = [str(value) for value in result.get_outputs()]
                summary["restored"] = True
            except asyncio.TimeoutError:
                summary["post_restore_timed_out"] = True
                summary["restored"] = witness_path.exists()
            except Exception as exc:
                summary["post_restore_error"] = f"{type(exc).__name__}: {exc}"
                summary["post_restore_traceback"] = traceback.format_exc(limit=4)
                summary["restored"] = witness_path.exists()
            summary["runtime_object_recreated"] = True

        operation_entries = [entry for entry in ledger_entries() if entry.get("operation_id") == operation_id]
        summary["external_effect_entries"] = len(operation_entries)
        summary["external_service_observed"] = len(operation_entries) > 0
        summary["duplicate_effect_observed"] = len(operation_entries) >= 2
        write_json(workspace / SUMMARY_ARTIFACT, summary)
        if summary["restored"] and summary["external_service_observed"] and summary["duplicate_effect_observed"]:
            return 0
        return 1
    finally:
        if owned_server is not None:
            owned_server.shutdown()
            owned_server.server_close()


async def run_authority_token_replay(workspace: Path, pre_timeout: float, restore_timeout: float) -> int:
    from agent_framework import Executor, FileCheckpointStorage, WorkflowBuilder, WorkflowContext, handler

    workspace.mkdir(parents=True, exist_ok=True)
    checkpoint_dir = workspace / CHECKPOINT_DIR
    storage = FileCheckpointStorage(checkpoint_dir)
    ledger_path = workspace / AUTHORITY_LEDGER_ARTIFACT
    witness_path = workspace / AUTHORITY_REPLAY_ARTIFACT
    operation_id = "syncfuzz-maf-workflow-authority-operation-1"
    events: list[dict[str, Any]] = []
    owned_server: http.server.ThreadingHTTPServer | None = None
    service_mode = "external-process"

    def ledger_entries() -> list[dict[str, Any]]:
        if not ledger_path.exists():
            return []
        entries: list[dict[str, Any]] = []
        for line in ledger_path.read_text(encoding="utf-8").splitlines():
            if line.strip():
                entries.append(json.loads(line))
        return entries

    def append_authority_entry(phase: str, source: str, token: str, status: int, *, error: str = "") -> int:
        entries = ledger_entries()
        entry = {
            "index": len(entries) + 1,
            "operation_id": operation_id,
            "marker": AUTHORITY_MARKER,
            "phase": phase,
            "source": source,
            "token": token,
            "status": status,
            "error": error,
            "service_mode": service_mode,
        }
        with ledger_path.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps(entry, sort_keys=True) + "\n")
        events.append(
            {
                "phase": "authority-" + phase,
                "source": source,
                "status": status,
                "error": error,
                "entry_index": entry["index"],
            }
        )
        return len([item for item in ledger_entries() if item.get("operation_id") == operation_id])

    class AuthorityHandler(http.server.BaseHTTPRequestHandler):
        tokens: dict[str, dict[str, Any]] = {}

        def do_POST(self) -> None:  # noqa: N802
            if self.path == "/reset":
                self.__class__.tokens = {}
                response = json.dumps({"ok": True}).encode("utf-8")
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(response)))
                self.end_headers()
                self.wfile.write(response)
                return
            length = int(self.headers.get("Content-Length", "0"))
            body = self.rfile.read(length)
            try:
                payload = json.loads(body.decode("utf-8")) if body else {}
                if self.path == "/authority/tokens":
                    token_value = f"tok_{len(self.__class__.tokens) + 1}_{operation_id}"
                    token = {
                        "token": token_value,
                        "scope": str(payload.get("scope") or "syncfuzz.workflow"),
                        "subject": str(payload.get("subject") or operation_id),
                        "consumed": False,
                    }
                    self.__class__.tokens[token_value] = token
                    response = json.dumps({"token": token}).encode("utf-8")
                    self.send_response(201)
                elif self.path == "/authority/consume":
                    token_value = str(payload.get("token") or "")
                    token = self.__class__.tokens.get(token_value)
                    if token is None:
                        response = json.dumps({"error": "token_not_found"}).encode("utf-8")
                        self.send_response(404)
                    elif token.get("consumed"):
                        response = json.dumps({"error": "token_already_consumed", "token": token}).encode("utf-8")
                        self.send_response(409)
                    else:
                        token["consumed"] = True
                        token["consumedBy"] = str(payload.get("operation") or "unknown")
                        response = json.dumps({"token": token}).encode("utf-8")
                        self.send_response(200)
                else:
                    self.send_response(404)
                    self.end_headers()
                    return
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(response)))
                self.end_headers()
                self.wfile.write(response)
            except Exception as exc:
                response = json.dumps({"ok": False, "error": str(exc)}).encode("utf-8")
                self.send_response(500)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(response)))
                self.end_headers()
                self.wfile.write(response)

        def log_message(self, format: str, *args: Any) -> None:
            return

    configured_service_url = os.environ.get("MAF_WORKFLOW_EFFECT_SERVICE_URL", "").strip().rstrip("/")
    if configured_service_url:
        service_url = configured_service_url
    else:
        service_mode = "in-process-fallback"
        owned_server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), AuthorityHandler)
        server_thread = threading.Thread(
            target=owned_server.serve_forever,
            name="syncfuzz-maf-authority-service",
            daemon=True,
        )
        server_thread.start()
        service_url = f"http://127.0.0.1:{owned_server.server_address[1]}"

    def request_json(path: str, value: dict[str, Any]) -> tuple[int, dict[str, Any]]:
        payload = json.dumps(value).encode("utf-8")
        request = urllib.request.Request(
            service_url + path,
            data=payload,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        try:
            with urllib.request.urlopen(request, timeout=3) as response:
                return response.status, json.loads(response.read().decode("utf-8"))
        except urllib.error.HTTPError as exc:
            raw = exc.read().decode("utf-8")
            try:
                return exc.code, json.loads(raw)
            except json.JSONDecodeError:
                return exc.code, {"error": raw}

    def reset_service() -> None:
        try:
            if ledger_path.exists():
                ledger_path.unlink()
            status, data = request_json("/reset", {})
            events.append(
                {
                    "phase": "authority-service-reset",
                    "service_url": service_url,
                    "service_mode": service_mode,
                    "status": status,
                    "response": data,
                }
            )
        except Exception as exc:
            events.append({"phase": "authority-service-reset-failed", "service_url": service_url, "error": str(exc)})

    def issue_token(source: str, op_id: str) -> str:
        status, data = request_json(
            "/authority/tokens",
            {
                "scope": "syncfuzz.workflow",
                "subject": op_id,
                "marker": AUTHORITY_MARKER,
            },
        )
        token = data.get("token") if isinstance(data.get("token"), dict) else {}
        token_value = str(token.get("token") or "")
        append_authority_entry("issue", source, token_value, status)
        return token_value

    def consume_token(source: str, token: str) -> tuple[int, str]:
        status, data = request_json(
            "/authority/consume",
            {
                "token": token,
                "operation": operation_id + ":" + source,
                "marker": AUTHORITY_MARKER,
            },
        )
        error = str(data.get("error") or "")
        append_authority_entry("consume", source, token, status, error=error)
        return status, error

    class StartExecutor(Executor):
        def __init__(self) -> None:
            super().__init__(id="authority-start")

        @handler
        async def process(self, message: str, ctx: WorkflowContext[str]) -> None:
            events.append({"phase": "authority-start", "message": message})
            await ctx.send_message(operation_id)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            return {"operation_id": operation_id}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "authority-start-restore", "state": state})

    class IssueExecutor(Executor):
        def __init__(self, *, source: str) -> None:
            super().__init__(id="authority-issue")
            self.source = source

        @handler
        async def process(self, op_id: str, ctx: WorkflowContext[str]) -> None:
            token = issue_token(self.source, op_id)
            await ctx.send_message(token)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            return {"ledger_entries": len(ledger_entries())}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "authority-issue-restore", "state": state})

    class ConsumeExecutor(Executor):
        def __init__(self, *, source: str, write_witness: bool) -> None:
            super().__init__(id="authority-consume")
            self.source = source
            self.write_witness = write_witness

        @handler
        async def process(self, token: str, ctx: WorkflowContext[Never, str]) -> None:
            status, error = consume_token(self.source, token)
            conflict = status == 409 and error == "token_already_consumed"
            if self.write_witness:
                label = "AUTHORITY_TOKEN_REPLAY_CONFLICT" if conflict else "AUTHORITY_TOKEN_REPLAY_ACCEPTED"
                witness_path.write_text(
                    f"{label}\nOPERATION_ID={operation_id}\nTOKEN={token}\nMARKER={AUTHORITY_MARKER}\nSTATUS={status}\nERROR={error}\nSERVICE_URL={service_url}\nSERVICE_MODE={service_mode}\n",
                    encoding="utf-8",
                )
                await ctx.yield_output(label)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            return {"ledger_entries": len(ledger_entries()), "service_url": service_url}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "authority-consume-restore", "state": state})

    def build_workflow(max_iterations: int, *, source: str, write_witness: bool):
        start = StartExecutor()
        issue = IssueExecutor(source=source)
        consume = ConsumeExecutor(source=source, write_witness=write_witness)
        return (
            WorkflowBuilder(
                start_executor=start,
                name=WORKFLOW_NAME + "-authority-token-replay",
                checkpoint_storage=storage,
                max_iterations=max_iterations,
                output_from=[consume],
            )
            .add_edge(start, issue)
            .add_edge(issue, consume)
            .build()
        )

    summary: dict[str, Any] = {
        "schema_version": "syncfuzz.maf-workflow-checkpoint.v1",
        "task_id": AUTHORITY_TOKEN_REPLAY_TASK,
        "workflow_name": WORKFLOW_NAME + "-authority-token-replay",
        "checkpoint_backend": "file",
        "checkpoint_dir": str(checkpoint_dir),
        "checkpoint_ids": [],
        "selected_checkpoint_id": "",
        "selected_checkpoint_iteration": 0,
        "restored": False,
        "runtime_object_recreated": False,
        "pre_restore_timed_out": False,
        "post_restore_timed_out": False,
        "external_effect_entries": 0,
        "external_service_url": service_url,
        "external_service_mode": service_mode,
        "external_service_observed": False,
        "authority_token_issued": False,
        "authority_token_consumed": False,
        "authority_replay_conflict_observed": False,
        "operation_id": operation_id,
        "post_restore_outputs": [],
        "events": events,
    }

    try:
        reset_service()
        try:
            await asyncio.wait_for(
                build_workflow(100, source="initial-run", write_witness=False).run("syncfuzz-start"),
                timeout=pre_timeout,
            )
        except asyncio.TimeoutError:
            summary["pre_restore_timed_out"] = True
        except Exception as exc:
            summary["pre_restore_error"] = f"{type(exc).__name__}: {exc}"
            summary["pre_restore_traceback"] = traceback.format_exc(limit=4)

        ids = checkpoint_ids(checkpoint_dir)
        summary["checkpoint_ids"] = ids
        if witness_path.exists():
            witness_path.unlink()

        if ids:
            selected_index = 1 if len(ids) > 1 else 0
            selected = ids[selected_index]
            summary["selected_checkpoint_id"] = selected
            summary["selected_checkpoint_iteration"] = selected_index
            try:
                result = await asyncio.wait_for(
                    build_workflow(100, source="restored-run", write_witness=True).run(
                        checkpoint_id=selected,
                        checkpoint_storage=storage,
                    ),
                    timeout=restore_timeout,
                )
                summary["post_restore_outputs"] = [str(value) for value in result.get_outputs()]
                summary["restored"] = True
            except asyncio.TimeoutError:
                summary["post_restore_timed_out"] = True
                summary["restored"] = witness_path.exists()
            except Exception as exc:
                summary["post_restore_error"] = f"{type(exc).__name__}: {exc}"
                summary["post_restore_traceback"] = traceback.format_exc(limit=4)
                summary["restored"] = witness_path.exists()
            summary["runtime_object_recreated"] = True

        operation_entries = [entry for entry in ledger_entries() if entry.get("operation_id") == operation_id]
        summary["external_effect_entries"] = len(operation_entries)
        summary["external_service_observed"] = len(operation_entries) > 0
        summary["authority_token_issued"] = any(entry.get("phase") == "issue" for entry in operation_entries)
        summary["authority_token_consumed"] = any(
            entry.get("phase") == "consume" and int(entry.get("status") or 0) == 200 for entry in operation_entries
        )
        summary["authority_replay_conflict_observed"] = any(
            entry.get("phase") == "consume"
            and int(entry.get("status") or 0) == 409
            and entry.get("error") == "token_already_consumed"
            for entry in operation_entries
        )
        write_json(workspace / SUMMARY_ARTIFACT, summary)
        if (
            summary["restored"]
            and summary["authority_token_issued"]
            and summary["authority_token_consumed"]
            and summary["authority_replay_conflict_observed"]
        ):
            return 0
        return 1
    finally:
        if owned_server is not None:
            owned_server.shutdown()
            owned_server.server_close()


async def run_partial_commit_replay(workspace: Path, pre_timeout: float, restore_timeout: float) -> int:
    from agent_framework import Executor, FileCheckpointStorage, WorkflowBuilder, WorkflowContext, handler

    workspace.mkdir(parents=True, exist_ok=True)
    checkpoint_dir = workspace / CHECKPOINT_DIR
    storage = FileCheckpointStorage(checkpoint_dir)
    ledger_path = workspace / EXTERNAL_LEDGER_ARTIFACT
    witness_path = workspace / PARTIAL_COMMIT_ARTIFACT
    operation_id = "syncfuzz-maf-workflow-partial-operation-1"
    events: list[dict[str, Any]] = []

    def ledger_entries() -> list[dict[str, Any]]:
        if not ledger_path.exists():
            return []
        entries: list[dict[str, Any]] = []
        for line in ledger_path.read_text(encoding="utf-8").splitlines():
            if line.strip():
                entries.append(json.loads(line))
        return entries

    def append_commit(source: str) -> None:
        entries = ledger_entries()
        entry = {
            "index": len(entries) + 1,
            "operation_id": operation_id,
            "marker": EXTERNAL_MARKER,
            "source": source,
            "branch": "commit",
        }
        with ledger_path.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps(entry, sort_keys=True) + "\n")
        events.append({"phase": "parallel-commit", "source": source, "entry_index": entry["index"]})

    class StartExecutor(Executor):
        def __init__(self) -> None:
            super().__init__(id="partial-start")

        @handler
        async def process(self, message: str, ctx: WorkflowContext[str]) -> None:
            events.append({"phase": "partial-start", "message": message})
            await ctx.send_message(operation_id)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            return {"operation_id": operation_id}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "partial-start-restore", "state": state})

    class CommitExecutor(Executor):
        def __init__(self, *, source: str) -> None:
            super().__init__(id="partial-commit")
            self.source = source

        @handler
        async def process(self, op_id: str, ctx: WorkflowContext[str]) -> None:
            append_commit(self.source)
            await ctx.send_message(op_id)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            return {"ledger_entries": len(ledger_entries())}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "partial-commit-restore", "state": state})

    class GateExecutor(Executor):
        def __init__(self, *, fail: bool, write_witness: bool) -> None:
            super().__init__(id="partial-gate")
            self.fail = fail
            self.write_witness = write_witness

        @handler
        async def process(self, op_id: str, ctx: WorkflowContext[Never, str]) -> None:
            await asyncio.sleep(0.05)
            if self.fail:
                events.append({"phase": "partial-gate-fail", "operation_id": op_id})
                raise RuntimeError("syncfuzz intentional partial-commit branch failure")
            entries = [entry for entry in ledger_entries() if entry.get("operation_id") == op_id]
            duplicate = len(entries) >= 2
            if self.write_witness:
                label = "DUPLICATE_PARTIAL_COMMIT_REPLAY" if duplicate else "SINGLE_PARTIAL_COMMIT_REPLAY"
                witness_path.write_text(
                    f"{label}\nOPERATION_ID={op_id}\nMARKER={EXTERNAL_MARKER}\nENTRIES={len(entries)}\n",
                    encoding="utf-8",
                )
                await ctx.yield_output(label)

        async def on_checkpoint_save(self) -> dict[str, Any]:
            return {"failed_branch": self.fail, "witness_exists": witness_path.exists()}

        async def on_checkpoint_restore(self, state: dict[str, Any]) -> None:
            events.append({"phase": "partial-gate-restore", "state": state})

    def build_workflow(max_iterations: int, *, source: str, fail: bool, write_witness: bool):
        start = StartExecutor()
        commit = CommitExecutor(source=source)
        gate = GateExecutor(fail=fail, write_witness=write_witness)
        return (
            WorkflowBuilder(
                start_executor=start,
                name=WORKFLOW_NAME + "-partial-commit",
                checkpoint_storage=storage,
                max_iterations=max_iterations,
                output_from=[gate],
            )
            .add_edge(start, commit)
            .add_edge(commit, gate)
            .build()
        )

    summary: dict[str, Any] = {
        "schema_version": "syncfuzz.maf-workflow-checkpoint.v1",
        "task_id": PARTIAL_COMMIT_REPLAY_TASK,
        "workflow_name": WORKFLOW_NAME + "-partial-commit",
        "checkpoint_backend": "file",
        "checkpoint_dir": str(checkpoint_dir),
        "checkpoint_ids": [],
        "selected_checkpoint_id": "",
        "selected_checkpoint_iteration": 0,
        "restored": False,
        "runtime_object_recreated": False,
        "pre_restore_timed_out": False,
        "post_restore_timed_out": False,
        "initial_failure_observed": False,
        "partial_commit_observed": False,
        "duplicate_effect_observed": False,
        "external_effect_entries": 0,
        "operation_id": operation_id,
        "post_restore_outputs": [],
        "events": events,
    }

    try:
        await asyncio.wait_for(
            build_workflow(100, source="initial-run", fail=True, write_witness=False).run("syncfuzz-start"),
            timeout=pre_timeout,
        )
    except asyncio.TimeoutError:
        summary["pre_restore_timed_out"] = True
    except Exception as exc:
        summary["initial_failure_observed"] = True
        summary["pre_restore_error"] = f"{type(exc).__name__}: {exc}"

    ids = checkpoint_ids(checkpoint_dir)
    summary["checkpoint_ids"] = ids
    entries_after_failure = [entry for entry in ledger_entries() if entry.get("operation_id") == operation_id]
    summary["partial_commit_observed"] = len(entries_after_failure) == 1
    if summary["pre_restore_timed_out"] and summary["partial_commit_observed"]:
        summary["initial_failure_observed"] = True
    if witness_path.exists():
        witness_path.unlink()

    if ids:
        selected = ids[0]
        summary["selected_checkpoint_id"] = selected
        try:
            result = await asyncio.wait_for(
                build_workflow(100, source="restored-run", fail=False, write_witness=True).run(
                    checkpoint_id=selected,
                    checkpoint_storage=storage,
                ),
                timeout=restore_timeout,
            )
            summary["post_restore_outputs"] = [str(value) for value in result.get_outputs()]
            summary["restored"] = True
        except asyncio.TimeoutError:
            summary["post_restore_timed_out"] = True
            summary["restored"] = witness_path.exists()
        except Exception as exc:
            summary["post_restore_error"] = f"{type(exc).__name__}: {exc}"
            summary["post_restore_traceback"] = traceback.format_exc(limit=4)
            summary["restored"] = witness_path.exists()
        summary["runtime_object_recreated"] = True

    operation_entries = [entry for entry in ledger_entries() if entry.get("operation_id") == operation_id]
    summary["external_effect_entries"] = len(operation_entries)
    summary["duplicate_effect_observed"] = len(operation_entries) >= 2
    if summary["duplicate_effect_observed"] and not witness_path.exists():
        witness_path.write_text(
            f"DUPLICATE_PARTIAL_COMMIT_REPLAY\nOPERATION_ID={operation_id}\nMARKER={EXTERNAL_MARKER}\nENTRIES={len(operation_entries)}\n",
            encoding="utf-8",
        )
    if summary["duplicate_effect_observed"]:
        summary["restored"] = True
    write_json(workspace / SUMMARY_ARTIFACT, summary)
    if summary["restored"] and summary["initial_failure_observed"] and summary["partial_commit_observed"] and summary["duplicate_effect_observed"]:
        return 0
    return 1


async def run_approval_pending_replay(workspace: Path, pre_timeout: float, restore_timeout: float) -> int:
    from agent_framework import FileCheckpointStorage, RunContext, workflow

    workspace.mkdir(parents=True, exist_ok=True)
    checkpoint_dir = workspace / CHECKPOINT_DIR
    storage = FileCheckpointStorage(checkpoint_dir)
    ledger_path = workspace / EXTERNAL_LEDGER_ARTIFACT
    witness_path = workspace / APPROVAL_PENDING_ARTIFACT
    operation_id = "syncfuzz-maf-workflow-approval-operation-1"
    request_id = "syncfuzz-maf-workflow-approval-request-1"
    events: list[dict[str, Any]] = []

    def ledger_entries() -> list[dict[str, Any]]:
        if not ledger_path.exists():
            return []
        entries: list[dict[str, Any]] = []
        for line in ledger_path.read_text(encoding="utf-8").splitlines():
            if line.strip():
                entries.append(json.loads(line))
        return entries

    def append_approved_commit(source: str, approval: str) -> None:
        entries = ledger_entries()
        entry = {
            "index": len(entries) + 1,
            "operation_id": operation_id,
            "marker": EXTERNAL_MARKER,
            "source": source,
            "approval": approval,
            "branch": "approval-pending",
        }
        with ledger_path.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps(entry, sort_keys=True) + "\n")
        events.append({"phase": "approval-commit", "source": source, "entry_index": entry["index"]})

    def build_workflow(*, source: str, write_witness: bool):
        @workflow(name=WORKFLOW_NAME + "-approval-pending", checkpoint_storage=storage)
        async def approval_flow(message: str, ctx: RunContext) -> str:
            events.append({"phase": "approval-start", "message": message})
            approval = await ctx.request_info(
                {"operation_id": operation_id, "question": "approve external commit?"},
                str,
                request_id=request_id,
            )
            append_approved_commit(source, approval)
            entries = [entry for entry in ledger_entries() if entry.get("operation_id") == operation_id]
            duplicate = len(entries) >= 2
            label = "DUPLICATE_APPROVAL_PENDING_REPLAY" if duplicate else "SINGLE_APPROVAL_PENDING_REPLAY"
            if write_witness:
                witness_path.write_text(
                    f"{label}\nOPERATION_ID={operation_id}\nREQUEST_ID={request_id}\nMARKER={EXTERNAL_MARKER}\nENTRIES={len(entries)}\n",
                    encoding="utf-8",
                )
            return label

        return approval_flow

    summary: dict[str, Any] = {
        "schema_version": "syncfuzz.maf-workflow-checkpoint.v1",
        "task_id": APPROVAL_PENDING_REPLAY_TASK,
        "workflow_name": WORKFLOW_NAME + "-approval-pending",
        "checkpoint_backend": "file",
        "checkpoint_dir": str(checkpoint_dir),
        "checkpoint_ids": [],
        "selected_checkpoint_id": "",
        "selected_checkpoint_iteration": 0,
        "restored": False,
        "runtime_object_recreated": False,
        "pre_restore_timed_out": False,
        "post_restore_timed_out": False,
        "pending_request_observed": False,
        "approval_response_observed": False,
        "approval_replay_observed": False,
        "duplicate_effect_observed": False,
        "external_effect_entries": 0,
        "operation_id": operation_id,
        "approval_request_id": "",
        "post_restore_outputs": [],
        "events": events,
    }

    try:
        result = await asyncio.wait_for(
            build_workflow(source="initial-run", write_witness=False).run("syncfuzz-start"),
            timeout=pre_timeout,
        )
        request_events = result.get_request_info_events()
        if request_events:
            summary["pending_request_observed"] = True
            summary["approval_request_id"] = request_events[0].request_id
            events.append({"phase": "pending-request-observed", "request_id": request_events[0].request_id})
    except asyncio.TimeoutError:
        summary["pre_restore_timed_out"] = True
    except Exception as exc:
        summary["pre_restore_error"] = f"{type(exc).__name__}: {exc}"
        summary["pre_restore_traceback"] = traceback.format_exc(limit=4)

    ids = checkpoint_ids(checkpoint_dir)
    summary["checkpoint_ids"] = ids
    if ids and not summary["pending_request_observed"] and summary["pre_restore_timed_out"]:
        summary["pending_request_observed"] = True
        summary["approval_request_id"] = request_id
        events.append({"phase": "pending-request-inferred-from-checkpoint", "request_id": request_id})
    effective_request_id = str(summary.get("approval_request_id") or request_id)
    if witness_path.exists():
        witness_path.unlink()

    if ids and summary["pending_request_observed"]:
        selected = ids[-1]
        summary["selected_checkpoint_id"] = selected
        try:
            first_result = await asyncio.wait_for(
                build_workflow(source="first-approval", write_witness=False).run(
                    checkpoint_id=selected,
                    checkpoint_storage=storage,
                    responses={effective_request_id: "approve"},
                ),
                timeout=restore_timeout,
            )
            summary["post_restore_outputs"] = [str(value) for value in first_result.get_outputs()]
            summary["approval_response_observed"] = True
        except asyncio.TimeoutError:
            summary["post_restore_timed_out"] = True
            summary["approval_response_observed"] = len(ledger_entries()) >= 1
        except Exception as exc:
            summary["post_restore_error"] = f"{type(exc).__name__}: {exc}"
            summary["post_restore_traceback"] = traceback.format_exc(limit=4)
            summary["approval_response_observed"] = len(ledger_entries()) >= 1

        try:
            replay_result = await asyncio.wait_for(
                build_workflow(source="replayed-approval", write_witness=True).run(
                    checkpoint_id=selected,
                    checkpoint_storage=storage,
                    responses={effective_request_id: "approve"},
                ),
                timeout=restore_timeout,
            )
            summary["post_restore_outputs"].extend(str(value) for value in replay_result.get_outputs())
            summary["approval_replay_observed"] = True
        except asyncio.TimeoutError:
            summary["post_restore_timed_out"] = True
            summary["approval_replay_observed"] = witness_path.exists()
        except Exception as exc:
            summary["approval_replay_error"] = f"{type(exc).__name__}: {exc}"
            summary["approval_replay_traceback"] = traceback.format_exc(limit=4)
            summary["approval_replay_observed"] = witness_path.exists()

        summary["runtime_object_recreated"] = True

    operation_entries = [entry for entry in ledger_entries() if entry.get("operation_id") == operation_id]
    summary["external_effect_entries"] = len(operation_entries)
    summary["duplicate_effect_observed"] = len(operation_entries) >= 2
    if summary["duplicate_effect_observed"] and not witness_path.exists():
        witness_path.write_text(
            f"DUPLICATE_APPROVAL_PENDING_REPLAY\nOPERATION_ID={operation_id}\nREQUEST_ID={effective_request_id}\nMARKER={EXTERNAL_MARKER}\nENTRIES={len(operation_entries)}\n",
            encoding="utf-8",
        )
    if summary["duplicate_effect_observed"]:
        summary["restored"] = True
    write_json(workspace / SUMMARY_ARTIFACT, summary)
    if (
        summary["restored"]
        and summary["pending_request_observed"]
        and summary["approval_response_observed"]
        and summary["duplicate_effect_observed"]
    ):
        return 0
    return 1


async def run_rehydrate_divergence(workspace: Path, pre_timeout: float, restore_timeout: float) -> int:
    from agent_framework import FileCheckpointStorage, RunContext, workflow

    workspace.mkdir(parents=True, exist_ok=True)
    checkpoint_dir = workspace / CHECKPOINT_DIR
    storage = FileCheckpointStorage(checkpoint_dir)
    ledger_path = workspace / EXTERNAL_LEDGER_ARTIFACT
    witness_path = workspace / REHYDRATE_DIVERGENCE_ARTIFACT
    operation_id = "syncfuzz-maf-workflow-rehydrate-operation-1"
    request_id = "syncfuzz-maf-workflow-rehydrate-request-1"
    events: list[dict[str, Any]] = []

    def ledger_entries() -> list[dict[str, Any]]:
        if not ledger_path.exists():
            return []
        entries: list[dict[str, Any]] = []
        for line in ledger_path.read_text(encoding="utf-8").splitlines():
            if line.strip():
                entries.append(json.loads(line))
        return entries

    def append_approved_commit(source: str, approval: str) -> int:
        entries = ledger_entries()
        entry = {
            "index": len(entries) + 1,
            "operation_id": operation_id,
            "marker": EXTERNAL_MARKER,
            "source": source,
            "approval": approval,
            "branch": "resume-vs-rehydrate",
        }
        with ledger_path.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps(entry, sort_keys=True) + "\n")
        events.append({"phase": "rehydrate-divergence-commit", "source": source, "entry_index": entry["index"]})
        return len([item for item in ledger_entries() if item.get("operation_id") == operation_id])

    def build_workflow(*, source: str, write_witness: bool):
        @workflow(name=WORKFLOW_NAME + "-rehydrate-divergence", checkpoint_storage=storage)
        async def divergence_flow(message: str, ctx: RunContext) -> str:
            events.append({"phase": "rehydrate-divergence-start", "message": message, "source": source})
            approval = await ctx.request_info(
                {"operation_id": operation_id, "question": "approve external commit?"},
                str,
                request_id=request_id,
            )
            count = append_approved_commit(source, approval)
            duplicate = count >= 2
            label = "REHYDRATE_DIVERGENCE_REPLAY" if duplicate else "SAME_INSTANCE_SINGLE_RESUME"
            if write_witness:
                witness_path.write_text(
                    f"{label}\nOPERATION_ID={operation_id}\nREQUEST_ID={request_id}\nMARKER={EXTERNAL_MARKER}\nENTRIES={count}\n",
                    encoding="utf-8",
                )
            return label

        return divergence_flow

    summary: dict[str, Any] = {
        "schema_version": "syncfuzz.maf-workflow-checkpoint.v1",
        "task_id": REHYDRATE_DIVERGENCE_TASK,
        "workflow_name": WORKFLOW_NAME + "-rehydrate-divergence",
        "checkpoint_backend": "file",
        "checkpoint_dir": str(checkpoint_dir),
        "checkpoint_ids": [],
        "selected_checkpoint_id": "",
        "selected_checkpoint_iteration": 0,
        "restored": False,
        "runtime_object_recreated": False,
        "pre_restore_timed_out": False,
        "post_restore_timed_out": False,
        "pending_request_observed": False,
        "approval_response_observed": False,
        "same_instance_resume_observed": False,
        "rehydrate_replay_observed": False,
        "duplicate_effect_observed": False,
        "external_effect_entries": 0,
        "operation_id": operation_id,
        "approval_request_id": "",
        "post_restore_outputs": [],
        "events": events,
    }

    initial_workflow = build_workflow(source="initial-run", write_witness=False)
    try:
        result = await asyncio.wait_for(initial_workflow.run("syncfuzz-start"), timeout=pre_timeout)
        request_events = result.get_request_info_events()
        if request_events:
            summary["pending_request_observed"] = True
            summary["approval_request_id"] = request_events[0].request_id
            events.append({"phase": "rehydrate-divergence-pending-request", "request_id": request_events[0].request_id})
    except asyncio.TimeoutError:
        summary["pre_restore_timed_out"] = True
    except Exception as exc:
        summary["pre_restore_error"] = f"{type(exc).__name__}: {exc}"
        summary["pre_restore_traceback"] = traceback.format_exc(limit=4)

    ids = checkpoint_ids(checkpoint_dir)
    summary["checkpoint_ids"] = ids
    if ids and not summary["pending_request_observed"] and summary["pre_restore_timed_out"]:
        summary["pending_request_observed"] = True
        summary["approval_request_id"] = request_id
        events.append({"phase": "rehydrate-divergence-pending-inferred-from-checkpoint", "request_id": request_id})
    effective_request_id = str(summary.get("approval_request_id") or request_id)
    if witness_path.exists():
        witness_path.unlink()

    if summary["pending_request_observed"]:
        try:
            same_result = await asyncio.wait_for(
                initial_workflow.run(responses={effective_request_id: "approve"}),
                timeout=restore_timeout,
            )
            summary["post_restore_outputs"] = [str(value) for value in same_result.get_outputs()]
            same_entries = [entry for entry in ledger_entries() if entry.get("operation_id") == operation_id]
            summary["approval_response_observed"] = len(same_entries) >= 1
            summary["same_instance_resume_observed"] = len(same_entries) == 1
        except asyncio.TimeoutError:
            summary["post_restore_timed_out"] = True
            same_entries = [entry for entry in ledger_entries() if entry.get("operation_id") == operation_id]
            summary["approval_response_observed"] = len(same_entries) >= 1
            summary["same_instance_resume_observed"] = len(same_entries) == 1
        except Exception as exc:
            summary["same_instance_resume_error"] = f"{type(exc).__name__}: {exc}"
            summary["same_instance_resume_traceback"] = traceback.format_exc(limit=4)

    if ids and summary["same_instance_resume_observed"]:
        selected = ids[-1]
        summary["selected_checkpoint_id"] = selected
        try:
            rehydrate_result = await asyncio.wait_for(
                build_workflow(source="rehydrated-run", write_witness=True).run(
                    checkpoint_id=selected,
                    checkpoint_storage=storage,
                    responses={effective_request_id: "approve"},
                ),
                timeout=restore_timeout,
            )
            summary["post_restore_outputs"].extend(str(value) for value in rehydrate_result.get_outputs())
            summary["rehydrate_replay_observed"] = witness_path.exists()
            summary["restored"] = True
        except asyncio.TimeoutError:
            summary["post_restore_timed_out"] = True
            summary["rehydrate_replay_observed"] = witness_path.exists()
            summary["restored"] = witness_path.exists()
        except Exception as exc:
            summary["rehydrate_replay_error"] = f"{type(exc).__name__}: {exc}"
            summary["rehydrate_replay_traceback"] = traceback.format_exc(limit=4)
            summary["rehydrate_replay_observed"] = witness_path.exists()
            summary["restored"] = witness_path.exists()
        summary["runtime_object_recreated"] = True

    operation_entries = [entry for entry in ledger_entries() if entry.get("operation_id") == operation_id]
    summary["external_effect_entries"] = len(operation_entries)
    summary["duplicate_effect_observed"] = len(operation_entries) >= 2
    if summary["duplicate_effect_observed"] and not witness_path.exists():
        witness_path.write_text(
            f"REHYDRATE_DIVERGENCE_REPLAY\nOPERATION_ID={operation_id}\nREQUEST_ID={effective_request_id}\nMARKER={EXTERNAL_MARKER}\nENTRIES={len(operation_entries)}\n",
            encoding="utf-8",
        )
    if summary["duplicate_effect_observed"]:
        summary["restored"] = True
    write_json(workspace / SUMMARY_ARTIFACT, summary)
    if (
        summary["pending_request_observed"]
        and summary["approval_response_observed"]
        and summary["same_instance_resume_observed"]
        and summary["restored"]
        and summary["runtime_object_recreated"]
        and summary["rehydrate_replay_observed"]
        and summary["duplicate_effect_observed"]
    ):
        return 0
    return 1


def load_task_id(task_file: str | None) -> str:
    if not task_file:
        return CHECKPOINT_CONTINUITY_TASK
    data = json.loads(Path(task_file).read_text(encoding="utf-8"))
    return str(data.get("task_id") or CHECKPOINT_CONTINUITY_TASK)


def main() -> int:
    parser = argparse.ArgumentParser(description="SyncFuzz MAF Workflow checkpoint target")
    parser.add_argument("--workspace", required=False)
    parser.add_argument("--source-workspace", help="immutable prepared workspace used by --mode fork-observe")
    parser.add_argument("--task-id", help="explicit target task id; used by the V2.3 recovery adapter")
    parser.add_argument("--task-file")
    parser.add_argument("--prompt-file")
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--mode", choices=("full", "prepare-fork", "fork-observe"), default="full")
    parser.add_argument("--checkpoint-id", help="exact native MAF checkpoint ID used by --mode fork-observe")
    parser.add_argument("--runtime-instance-id", help="unique fresh runtime identity recorded by --mode fork-observe")
    parser.add_argument("--observation-out", help="fork observation JSON path; defaults inside the fork workspace")
    parser.add_argument("--pre-timeout", type=float, default=float(os.environ.get("MAF_WORKFLOW_PRE_TIMEOUT", "2.0")))
    parser.add_argument("--restore-timeout", type=float, default=float(os.environ.get("MAF_WORKFLOW_RESTORE_TIMEOUT", "5.0")))
    args = parser.parse_args()

    if args.check:
        from agent_framework import Executor, FileCheckpointStorage, WorkflowBuilder, WorkflowContext, handler  # noqa: F401

        print("maf workflow checkpoint target imports ok")
        return 0

    workspace = Path(args.workspace or os.environ.get("SYNCFUZZ_WORKSPACE", ".")).resolve()
    task_id = args.task_id or load_task_id(args.task_file)
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    if args.mode == "prepare-fork":
        return loop.run_until_complete(prepare_v2_fork(workspace, task_id, args.pre_timeout))
    if args.mode == "fork-observe":
        if not args.source_workspace or not args.checkpoint_id:
            parser.error("--mode fork-observe requires --source-workspace and --checkpoint-id")
        runtime_instance_id = args.runtime_instance_id or "maf-workflow-fork-" + uuid.uuid4().hex
        observation_out = Path(args.observation_out).resolve() if args.observation_out else workspace / FORK_OBSERVATION_ARTIFACT
        return loop.run_until_complete(
            observe_v2_fork(
                Path(args.source_workspace).resolve(),
                workspace,
                task_id,
                args.checkpoint_id,
                runtime_instance_id,
                args.restore_timeout,
                observation_out,
            )
        )
    return loop.run_until_complete(run_probe(workspace, task_id, args.pre_timeout, args.restore_timeout))


if __name__ == "__main__":
    code = main()
    sys.stdout.flush()
    sys.stderr.flush()
    os._exit(code)
