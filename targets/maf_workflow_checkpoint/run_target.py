#!/usr/bin/env python3

import argparse
import asyncio
import hashlib
import json
import os
import sys
import traceback
from pathlib import Path
from typing import Any

from typing_extensions import Never

WORKFLOW_NAME = "syncfuzz-maf-workflow-checkpoint"
MARKER = "SYNCFUZZ_MAF_WORKFLOW_MARKER"
EFFECT_ARTIFACT = "maf-workflow-effect.txt"
WITNESS_ARTIFACT = "maf-workflow-continuity-check.txt"
EXTERNAL_LEDGER_ARTIFACT = "maf-workflow-external-ledger.jsonl"
EXTERNAL_REPLAY_ARTIFACT = "maf-workflow-external-replay-check.txt"
SUMMARY_ARTIFACT = "maf-workflow-summary.json"
CHECKPOINT_DIR = "maf-workflow-checkpoints"
CHECKPOINT_CONTINUITY_TASK = "maf-workflow-checkpoint-continuity"
EXTERNAL_EFFECT_REPLAY_TASK = "maf-workflow-external-effect-replay"
SUPPORTED_TASKS = {CHECKPOINT_CONTINUITY_TASK, EXTERNAL_EFFECT_REPLAY_TASK}
EXTERNAL_MARKER = "SYNCFUZZ_MAF_WORKFLOW_EXTERNAL_EFFECT"


def write_json(path: Path, data: dict[str, Any]) -> None:
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    tmp.replace(path)


def sha256_text(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def checkpoint_ids(checkpoint_dir: Path) -> list[str]:
    checkpoints = sorted(checkpoint_dir.glob("*.json"), key=lambda path: path.stat().st_mtime_ns)
    return [path.stem for path in checkpoints]


async def run_probe(workspace: Path, task_id: str, pre_timeout: float, restore_timeout: float) -> int:
    if task_id not in SUPPORTED_TASKS:
        raise ValueError(f"unsupported MAF workflow task: {task_id}")
    if task_id == EXTERNAL_EFFECT_REPLAY_TASK:
        return await run_external_effect_replay(workspace, pre_timeout, restore_timeout)
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
        selected = ids[-1]
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
        selected = ids[-1]
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


def load_task_id(task_file: str | None) -> str:
    if not task_file:
        return CHECKPOINT_CONTINUITY_TASK
    data = json.loads(Path(task_file).read_text(encoding="utf-8"))
    return str(data.get("task_id") or CHECKPOINT_CONTINUITY_TASK)


def main() -> int:
    parser = argparse.ArgumentParser(description="SyncFuzz MAF Workflow checkpoint target")
    parser.add_argument("--workspace", required=False)
    parser.add_argument("--task-file")
    parser.add_argument("--prompt-file")
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--pre-timeout", type=float, default=float(os.environ.get("MAF_WORKFLOW_PRE_TIMEOUT", "2.0")))
    parser.add_argument("--restore-timeout", type=float, default=float(os.environ.get("MAF_WORKFLOW_RESTORE_TIMEOUT", "5.0")))
    args = parser.parse_args()

    if args.check:
        from agent_framework import Executor, FileCheckpointStorage, WorkflowBuilder, WorkflowContext, handler  # noqa: F401

        print("maf workflow checkpoint target imports ok")
        return 0

    workspace = Path(args.workspace or os.environ.get("SYNCFUZZ_WORKSPACE", ".")).resolve()
    task_id = load_task_id(args.task_file)
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    return loop.run_until_complete(run_probe(workspace, task_id, args.pre_timeout, args.restore_timeout))


if __name__ == "__main__":
    code = main()
    sys.stdout.flush()
    sys.stderr.flush()
    os._exit(code)
