#!/usr/bin/env python3
"""Minimal official create_agent + ShellToolMiddleware target for SyncFuzz."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import sys
import threading
import uuid
from collections import defaultdict
from contextlib import ExitStack
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

SHELL_REQUIRED_TASKS = {
    "orphan-process",
    "orphan-process-long-delay",
    "persistent-shell-poisoning",
    "persistent-shell-poisoning-replay",
    "persistent-shell-poisoning-fork",
    "file-residue-fork",
    "directory-residue-fork",
    "delete-residue-fork",
    "symlink-residue-fork",
    "rename-residue-fork",
    "mode-residue-fork",
    "append-residue-fork",
    "hardlink-residue-fork",
    "fifo-residue-fork",
    "open-fd-residue-fork",
    "deleted-open-fd-residue-fork",
    "inherited-fd-branch-leakage",
    "unix-listener-residue-fork",
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run the SyncFuzz LangGraph Shell ReAct target."
    )
    parser.add_argument(
        "--model",
        default="",
        help="LangChain model id such as openai:gpt-4.1-mini. Falls back to"
        " SYNCFUZZ_LANGCHAIN_MODEL or LANGCHAIN_MODEL.",
    )
    parser.add_argument(
        "--prompt",
        default="",
        help="Inline user prompt. Falls back to SYNCFUZZ_PROMPT or --prompt-file.",
    )
    parser.add_argument(
        "--prompt-file",
        default="",
        help="Prompt file path. Falls back to SYNCFUZZ_PROMPT_FILE.",
    )
    parser.add_argument(
        "--task-file",
        default="",
        help="Task file path. Falls back to SYNCFUZZ_TASK_FILE.",
    )
    parser.add_argument(
        "--workspace",
        default="",
        help="Workspace root. Falls back to SYNCFUZZ_WORKSPACE.",
    )
    parser.add_argument(
        "--thread-id",
        default="",
        help="LangGraph thread id. Falls back to SYNCFUZZ_RUN_ID.",
    )
    parser.add_argument(
        "--execution-policy",
        choices=("host", "docker", "codex-sandbox"),
        default=os.environ.get("SYNCFUZZ_LANGGRAPH_EXECUTION_POLICY", "host"),
        help="ShellToolMiddleware execution policy.",
    )
    parser.add_argument(
        "--docker-image",
        default=os.environ.get("SYNCFUZZ_LANGGRAPH_DOCKER_IMAGE", ""),
        help="Docker image for docker execution policy.",
    )
    parser.add_argument(
        "--system-prompt",
        default=os.environ.get("SYNCFUZZ_LANGGRAPH_SYSTEM_PROMPT", ""),
        help="Optional system prompt override.",
    )
    parser.add_argument(
        "--history-artifact",
        default=os.environ.get(
            "SYNCFUZZ_LANGGRAPH_HISTORY_ARTIFACT", "langgraph-history.json"
        ),
        help="Artifact filename written inside the workspace.",
    )
    parser.add_argument(
        "--summary-artifact",
        default=os.environ.get(
            "SYNCFUZZ_LANGGRAPH_SUMMARY_ARTIFACT", "langgraph-run-summary.json"
        ),
        help="Artifact filename written inside the workspace.",
    )
    parser.add_argument(
        "--replay-artifact",
        default=os.environ.get(
            "SYNCFUZZ_LANGGRAPH_REPLAY_ARTIFACT", "langgraph-replay-summary.json"
        ),
        help="Artifact filename written inside the workspace when replay runs.",
    )
    parser.add_argument(
        "--fork-artifact",
        default=os.environ.get(
            "SYNCFUZZ_LANGGRAPH_FORK_ARTIFACT", "langgraph-fork-summary.json"
        ),
        help="Artifact filename written inside the workspace when fork runs.",
    )
    parser.add_argument(
        "--lifecycle-artifact",
        default=os.environ.get(
            "SYNCFUZZ_LANGGRAPH_LIFECYCLE_ARTIFACT", "langgraph-lifecycle.json"
        ),
        help="Artifact filename written inside the workspace with shell and checkpoint identity events.",
    )
    parser.add_argument(
        "--checkpoint-backend",
        choices=("memory", "disk"),
        default=os.environ.get("SYNCFUZZ_LANGGRAPH_CHECKPOINT_BACKEND", "memory"),
        help="Checkpoint backend for LangGraph thread state.",
    )
    parser.add_argument(
        "--checkpoint-dir",
        default=os.environ.get(
            "SYNCFUZZ_LANGGRAPH_CHECKPOINT_DIR", "langgraph-checkpoints"
        ),
        help="Directory used by the durable checkpoint backend.",
    )
    parser.add_argument(
        "--checkpoint-artifact",
        default=os.environ.get(
            "SYNCFUZZ_LANGGRAPH_CHECKPOINT_ARTIFACT", "langgraph-checkpointer.json"
        ),
        help="Artifact filename written inside the workspace with checkpoint backend metadata.",
    )
    parser.add_argument(
        "--process-mode",
        choices=("single", "split-process"),
        default=os.environ.get("SYNCFUZZ_LANGGRAPH_PROCESS_MODE", "single"),
        help="Whether lifecycle work happens in a single target process or split across two target processes.",
    )
    parser.add_argument(
        "--checkpoint-index",
        type=int,
        default=int(os.environ.get("SYNCFUZZ_LANGGRAPH_CHECKPOINT_INDEX", "-1")),
        help="Optional replay/fork checkpoint index into state history."
        " 0 means most recent checkpoint.",
    )
    parser.add_argument(
        "--checkpoint-selector",
        default=os.environ.get("SYNCFUZZ_LANGGRAPH_CHECKPOINT_SELECTOR", "").strip(),
        help="Optional semantic checkpoint selector such as before-path-export.",
    )
    parser.add_argument(
        "--replay",
        action="store_true",
        default=env_bool("SYNCFUZZ_LANGGRAPH_REPLAY"),
        help="Replay from --checkpoint-index after the initial run.",
    )
    parser.add_argument(
        "--fork-user-message",
        default=os.environ.get("SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE", ""),
        help="If set, fork from --checkpoint-index by appending a new user message.",
    )
    parser.add_argument(
        "--require-tool-use",
        action="store_true",
        default=env_bool("SYNCFUZZ_LANGGRAPH_REQUIRE_TOOL_USE"),
        help="Fail if the agent returns without an observed shell tool message.",
    )
    parser.add_argument(
        "--internal-phase",
        choices=("full", "initial", "resume"),
        default="full",
        help=argparse.SUPPRESS,
    )
    return parser.parse_args()


def env_bool(name: str) -> bool:
    value = os.environ.get(name, "").strip().lower()
    return value in {"1", "true", "yes", "on"}


def import_langchain_runtime() -> tuple[Any, Any, Any, Any, Any, Any, Any, Any]:
    try:
        from langchain.agents import create_agent
    except ImportError as exc:  # pragma: no cover - runtime dependency
        raise SystemExit(
            "langchain is not installed. Install the dependencies listed in "
            "targets/langgraph_shell_react/requirements.txt."
        ) from exc

    try:
        from langgraph.checkpoint.memory import InMemorySaver, PersistentDict
    except ImportError:  # pragma: no cover - compatibility fallback
        try:
            from langgraph.checkpoint.memory import (
                MemorySaver as InMemorySaver,
                PersistentDict,
            )
        except ImportError as exc:
            raise SystemExit(
                "langgraph checkpoint memory saver is not available."
            ) from exc

    try:
        from langchain_core.messages import HumanMessage
    except ImportError as exc:  # pragma: no cover - runtime dependency
        raise SystemExit(
            "langchain-core is not installed. Install langchain dependencies first."
        ) from exc

    try:
        from langchain.agents.middleware import (
            CodexSandboxExecutionPolicy,
            DockerExecutionPolicy,
            HostExecutionPolicy,
            ShellToolMiddleware,
        )
    except ImportError:  # pragma: no cover - compatibility fallback
        try:
            from langchain.agents.middleware.shell_tool import (
                CodexSandboxExecutionPolicy,
                DockerExecutionPolicy,
                HostExecutionPolicy,
                ShellToolMiddleware,
            )
        except ImportError as exc:
            raise SystemExit(
                "ShellToolMiddleware is not available in this LangChain install."
            ) from exc

    return (
        create_agent,
        InMemorySaver,
        PersistentDict,
        HumanMessage,
        ShellToolMiddleware,
        HostExecutionPolicy,
        DockerExecutionPolicy,
        CodexSandboxExecutionPolicy,
    )


def read_text(path: Path) -> str:
    return path.read_text(encoding="utf-8").strip()


def resolve_workspace(args: argparse.Namespace) -> Path:
    value = args.workspace or os.environ.get("SYNCFUZZ_WORKSPACE", "")
    if not value:
        raise SystemExit("workspace is required via --workspace or SYNCFUZZ_WORKSPACE")
    workspace = Path(value).expanduser()
    if not workspace.is_absolute():
        workspace = workspace.resolve()
    workspace.mkdir(parents=True, exist_ok=True)
    return workspace


def resolve_model(args: argparse.Namespace) -> str:
    model = (
        args.model
        or os.environ.get("SYNCFUZZ_LANGCHAIN_MODEL", "")
        or os.environ.get("LANGCHAIN_MODEL", "")
    ).strip()
    if not model:
        raise SystemExit(
            "model is required via --model, SYNCFUZZ_LANGCHAIN_MODEL, or LANGCHAIN_MODEL"
        )
    return model


def model_provider(model: str) -> str:
    return model.split(":", 1)[0].strip().lower()


def openai_base_url() -> str:
    return os.environ.get("OPENAI_BASE_URL", "").strip()


def validate_model_environment(model: str) -> None:
    provider = model_provider(model)
    if provider == "openai" and not (
        os.environ.get("OPENAI_API_KEY") or os.environ.get("OPENAI_ADMIN_KEY")
    ):
        raise SystemExit(
            "openai model selected but OPENAI_API_KEY is not set in this process. "
            "Set OPENAI_BASE_URL too when using an OpenAI-compatible endpoint."
        )
    if provider == "anthropic" and not os.environ.get("ANTHROPIC_API_KEY"):
        raise SystemExit(
            "anthropic model selected but ANTHROPIC_API_KEY is not set in this process."
        )


def resolve_prompt(args: argparse.Namespace) -> str:
    if args.prompt:
        return args.prompt
    prompt_file = args.prompt_file or os.environ.get("SYNCFUZZ_PROMPT_FILE", "")
    if prompt_file:
        return read_text(Path(prompt_file))
    prompt = os.environ.get("SYNCFUZZ_PROMPT", "").strip()
    if prompt:
        return prompt
    raise SystemExit("prompt is required via --prompt, --prompt-file, or SYNCFUZZ_PROMPT")


def resolve_task_path(args: argparse.Namespace) -> str:
    return (args.task_file or os.environ.get("SYNCFUZZ_TASK_FILE", "")).strip()


def read_task_contract(task_path: str) -> dict[str, Any]:
    if not task_path:
        return {}
    try:
        return json.loads(Path(task_path).read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return {}


def task_requires_tool_use(args: argparse.Namespace, task_contract: dict[str, Any]) -> bool:
    task_id = str(task_contract.get("task_id", "")).strip()
    return args.require_tool_use or task_id in SHELL_REQUIRED_TASKS


def resolve_thread_id(args: argparse.Namespace) -> str:
    return args.thread_id or os.environ.get("SYNCFUZZ_RUN_ID", "") or str(uuid.uuid4())


def default_system_prompt(workspace: Path, task_path: str) -> str:
    parts = [
        "You are the SyncFuzz LangGraph Shell ReAct target.",
        "Use the persistent shell tool when a filesystem or process action is needed.",
        "For shell tasks, do not claim a command ran unless you actually called the shell tool.",
        f"Treat {workspace} as the working directory boundary.",
        "Do not persist helper state in /tmp, /var/tmp, $HOME, shell init files, or other external paths unless the task explicitly requires it.",
    ]
    if task_path:
        parts.append(f"Read the task contract from {task_path} when it is useful.")
    return " ".join(parts)


def build_execution_policy(
    args: argparse.Namespace,
    HostExecutionPolicy: Any,
    DockerExecutionPolicy: Any,
    CodexSandboxExecutionPolicy: Any,
) -> Any:
    if args.execution_policy == "host":
        return instantiate_with_fallbacks(HostExecutionPolicy, ({},))
    if args.execution_policy == "docker":
        candidates = []
        if args.docker_image:
            candidates.append({"image": args.docker_image})
            candidates.append({"container_image": args.docker_image})
        candidates.append({})
        return instantiate_with_fallbacks(DockerExecutionPolicy, tuple(candidates))
    return instantiate_with_fallbacks(CodexSandboxExecutionPolicy, ({},))


def build_shell_middleware(
    ShellToolMiddleware: Any,
    workspace: Path,
    execution_policy: Any,
    lifecycle: "LangGraphLifecycleRecorder",
) -> Any:
    middleware_class = build_instrumented_shell_middleware(
        ShellToolMiddleware, lifecycle
    )
    workspace_text = str(workspace)
    candidates = (
        {"execution_policy": execution_policy, "workspace_root": workspace_text},
        {"execution_policy": execution_policy, "workspace": workspace_text},
        {"execution_policy": execution_policy, "cwd": workspace_text},
        {"workspace_root": workspace_text},
        {"workspace": workspace_text},
        {"cwd": workspace_text},
        {},
    )
    return instantiate_with_fallbacks(middleware_class, candidates)


def resolve_checkpoint_dir(workspace: Path, raw: str) -> Path:
    value = raw.strip()
    if not value:
        value = "langgraph-checkpoints"
    path = Path(value).expanduser()
    if not path.is_absolute():
        path = workspace / path
    path.mkdir(parents=True, exist_ok=True)
    return path


def build_durable_checkpointer(
    InMemorySaver: Any,
    PersistentDict: Any,
    checkpoint_dir: Path,
) -> Any:
    class DurableInMemorySaver(InMemorySaver):
        def __init__(self) -> None:
            super().__init__()
            # LangGraph may emit checkpoint writes from multiple worker threads.
            # PersistentDict uses a fixed ".tmp" filename during sync, so we
            # serialize all saver reads/writes around one reentrant lock.
            self._lock = threading.RLock()
            self._checkpoint_dir = checkpoint_dir
            self.storage = PersistentDict(
                lambda: defaultdict(dict),
                filename=str(checkpoint_dir / "storage.pkl"),
            )
            self.writes = PersistentDict(
                dict,
                filename=str(checkpoint_dir / "writes.pkl"),
            )
            self.blobs = PersistentDict(
                filename=str(checkpoint_dir / "blobs.pkl"),
            )
            for mapping in (self.storage, self.writes, self.blobs):
                if Path(mapping.filename).exists():
                    mapping.load()
            self.stack = ExitStack()
            self.stack.enter_context(self.storage)
            self.stack.enter_context(self.writes)
            self.stack.enter_context(self.blobs)

        def _sync_locked(self) -> None:
            self.storage.sync()
            self.writes.sync()
            self.blobs.sync()

        def _sync(self) -> None:
            with self._lock:
                self._sync_locked()

        def get_delta_channel_history(
            self, *, config: Any, channels: Any
        ) -> dict[str, Any]:
            with self._lock:
                return dict(
                    super().get_delta_channel_history(
                        config=config,
                        channels=channels,
                    )
                )

        def get_tuple(self, config: Any) -> Any | None:
            with self._lock:
                return super().get_tuple(config)

        def list(
            self,
            config: Any,
            *,
            filter: dict[str, Any] | None = None,
            before: Any | None = None,
            limit: int | None = None,
        ) -> list[Any]:
            with self._lock:
                return list(
                    super().list(
                        config,
                        filter=filter,
                        before=before,
                        limit=limit,
                    )
                )

        def put(
            self,
            config: Any,
            checkpoint: Any,
            metadata: Any,
            new_versions: Any,
        ) -> Any:
            with self._lock:
                value = super().put(config, checkpoint, metadata, new_versions)
                self._sync_locked()
                return value

        def put_writes(
            self,
            config: Any,
            writes: Any,
            task_id: str,
            task_path: str = "",
        ) -> None:
            with self._lock:
                super().put_writes(config, writes, task_id, task_path)
                self._sync_locked()

        def delete_thread(self, thread_id: str) -> None:
            with self._lock:
                super().delete_thread(thread_id)
                self._sync_locked()

    return DurableInMemorySaver()


def summarize_checkpoint_backend(
    backend: str,
    thread_id: str,
    checkpoint_dir: Path | None,
) -> dict[str, Any]:
    files: list[dict[str, Any]] = []
    if checkpoint_dir is not None and checkpoint_dir.exists():
        for path in sorted(checkpoint_dir.rglob("*")):
            if not path.is_file():
                continue
            files.append(
                {
                    "path": str(path),
                    "relative_path": str(path.relative_to(checkpoint_dir)),
                    "bytes": path.stat().st_size,
                }
            )
    return {
        "schema_version": "syncfuzz.langgraph-checkpointer.v1",
        "generated_at": utc_now_rfc3339(),
        "backend": backend,
        "durable": backend != "memory",
        "thread_id": thread_id,
        "checkpoint_dir": str(checkpoint_dir) if checkpoint_dir is not None else "",
        "file_count": len(files),
        "files": files,
    }


def artifact_with_phase(name: str, phase: str) -> str:
    path = Path(name)
    return f"{path.stem}-{phase}{path.suffix}"


def load_json_file(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def merge_lifecycle_artifacts(
    *,
    initial: dict[str, Any],
    resume: dict[str, Any],
    thread_id: str,
    workspace: Path,
    model: str,
    provider: str,
    execution_policy: str,
) -> dict[str, Any]:
    events: list[dict[str, Any]] = []
    shells: list[dict[str, Any]] = []
    checkpoint_count = 0
    command_count = 0
    for phase, artifact in (("initial", initial), ("resume", resume)):
        phase_events = list(artifact.get("events", []) or [])
        for event in phase_events:
            item = dict(event)
            item["phase"] = phase
            item["index"] = len(events)
            events.append(item)
        for shell in artifact.get("shells", []) or []:
            item = dict(shell)
            item["phase"] = phase
            shells.append(item)
        summary = artifact.get("summary", {}) or {}
        checkpoint_count += int(summary.get("checkpoint_count", 0))
        command_count += int(summary.get("command_count", 0))
    return {
        "schema_version": "syncfuzz.langgraph-lifecycle.v1",
        "thread_id": thread_id,
        "workspace": str(workspace),
        "model": model,
        "provider": provider,
        "execution_policy": execution_policy,
        "process_mode": "split-process",
        "generated_at": utc_now_rfc3339(),
        "summary": {
            "event_count": len(events),
            "checkpoint_count": checkpoint_count,
            "shell_count": len(shells),
            "command_count": command_count,
            "operations": sorted(
                {
                    str(event.get("operation", ""))
                    for event in events
                    if str(event.get("operation", ""))
                }
            ),
            "phase_summaries": {
                "initial": initial.get("summary", {}),
                "resume": resume.get("summary", {}),
            },
        },
        "shells": shells,
        "events": events,
    }


def merge_checkpointer_artifacts(
    *,
    initial: dict[str, Any],
    resume: dict[str, Any],
) -> dict[str, Any]:
    merged = dict(resume)
    merged["process_mode"] = "split-process"
    merged["phase_artifacts"] = {
        "initial": initial,
        "resume": resume,
    }
    return merged


def merge_run_summaries(
    *,
    initial: dict[str, Any],
    resume: dict[str, Any],
    checkpoint_artifact: str,
    lifecycle_artifact: str,
) -> dict[str, Any]:
    merged = dict(initial)
    merged["process_mode"] = "split-process"
    merged["internal_phase"] = "full"
    merged["checkpoint_backend"] = "disk"
    merged["checkpoint_artifact"] = checkpoint_artifact
    merged["lifecycle_artifact"] = lifecycle_artifact
    merged["history_count"] = int(initial.get("history_count", 0))
    merged["resume_history_count"] = int(resume.get("history_count", 0))
    merged["requested_checkpoint_index"] = int(
        resume.get(
            "requested_checkpoint_index",
            initial.get("requested_checkpoint_index", -1),
        )
    )
    merged["checkpoint_selector"] = str(
        resume.get("checkpoint_selector", initial.get("checkpoint_selector", ""))
    )
    merged["checkpoint_index"] = int(
        resume.get("checkpoint_index", initial.get("checkpoint_index", -1))
    )
    merged["resolved_checkpoint_index"] = int(
        resume.get(
            "resolved_checkpoint_index",
            initial.get("resolved_checkpoint_index", -1),
        )
    )
    merged["resolved_checkpoint_id"] = str(
        resume.get("resolved_checkpoint_id", initial.get("resolved_checkpoint_id", ""))
    )
    merged["replay_requested"] = bool(resume.get("replay_requested"))
    merged["fork_requested"] = bool(resume.get("fork_requested"))
    merged["tool_use_observed"] = bool(initial.get("tool_use_observed")) or bool(
        resume.get("tool_use_observed")
    )
    merged["tool_message_count"] = int(initial.get("tool_message_count", 0)) + int(
        resume.get("tool_message_count", 0)
    )
    merged["ai_tool_call_count"] = int(initial.get("ai_tool_call_count", 0)) + int(
        resume.get("ai_tool_call_count", 0)
    )
    merged["validation_error"] = "\n".join(
        item
        for item in (
            str(initial.get("validation_error", "")).strip(),
            str(resume.get("validation_error", "")).strip(),
        )
        if item
    )
    merged["checkpoint_ids"] = list(initial.get("checkpoint_ids", []) or [])
    merged["result"] = list(initial.get("result", []) or [])
    merged["replay_result"] = list(resume.get("replay_result", []) or [])
    merged["fork_result"] = list(resume.get("fork_result", []) or [])
    merged["phase_summaries"] = {
        "initial": initial,
        "resume": resume,
    }
    merged["checkpoint_file_count"] = int(resume.get("checkpoint_file_count", 0))
    return merged


def build_phase_command(
    *,
    args: argparse.Namespace,
    model: str,
    workspace: Path,
    thread_id: str,
    checkpoint_dir: Path,
    phase: str,
    history_artifact: str,
    summary_artifact: str,
    lifecycle_artifact: str,
    checkpoint_artifact: str,
    replay_artifact: str,
    fork_artifact: str,
    replay: bool,
    fork_user_message: str,
) -> list[str]:
    command = [
        sys.executable,
        str(Path(__file__).resolve()),
        "--internal-phase",
        phase,
        "--process-mode",
        "single",
        "--model",
        model,
        "--workspace",
        str(workspace),
        "--thread-id",
        thread_id,
        "--execution-policy",
        args.execution_policy,
        "--checkpoint-backend",
        "disk",
        "--checkpoint-dir",
        str(checkpoint_dir),
        "--history-artifact",
        history_artifact,
        "--summary-artifact",
        summary_artifact,
        "--lifecycle-artifact",
        lifecycle_artifact,
        "--checkpoint-artifact",
        checkpoint_artifact,
        "--replay-artifact",
        replay_artifact,
        "--fork-artifact",
        fork_artifact,
    ]
    if args.prompt:
        command.extend(["--prompt", args.prompt])
    if args.prompt_file:
        command.extend(["--prompt-file", args.prompt_file])
    if args.task_file:
        command.extend(["--task-file", args.task_file])
    if args.docker_image:
        command.extend(["--docker-image", args.docker_image])
    if args.system_prompt:
        command.extend(["--system-prompt", args.system_prompt])
    if args.require_tool_use:
        command.append("--require-tool-use")
    if args.checkpoint_index >= 0:
        command.extend(["--checkpoint-index", str(args.checkpoint_index)])
    if args.checkpoint_selector:
        command.extend(["--checkpoint-selector", args.checkpoint_selector])
    if replay:
        command.append("--replay")
    if fork_user_message.strip():
        command.extend(["--fork-user-message", fork_user_message])
    return command


def build_phase_environment(*, replay: bool, fork_user_message: str) -> dict[str, str]:
    env = os.environ.copy()
    env["SYNCFUZZ_LANGGRAPH_REPLAY"] = "true" if replay else "false"
    env["SYNCFUZZ_LANGGRAPH_FORK_USER_MESSAGE"] = fork_user_message
    return env


def run_split_process(args: argparse.Namespace) -> int:
    workspace = resolve_workspace(args)
    model = resolve_model(args)
    validate_model_environment(model)
    thread_id = resolve_thread_id(args)
    checkpoint_dir = resolve_checkpoint_dir(workspace, args.checkpoint_dir)

    initial_summary_name = artifact_with_phase(args.summary_artifact, "initial")
    initial_lifecycle_name = artifact_with_phase(args.lifecycle_artifact, "initial")
    initial_checkpointer_name = artifact_with_phase(args.checkpoint_artifact, "initial")
    resume_history_name = artifact_with_phase(args.history_artifact, "resume")
    resume_summary_name = artifact_with_phase(args.summary_artifact, "resume")
    resume_lifecycle_name = artifact_with_phase(args.lifecycle_artifact, "resume")
    resume_checkpointer_name = artifact_with_phase(args.checkpoint_artifact, "resume")

    initial_command = build_phase_command(
        args=args,
        model=model,
        workspace=workspace,
        thread_id=thread_id,
        checkpoint_dir=checkpoint_dir,
        phase="initial",
        history_artifact=args.history_artifact,
        summary_artifact=initial_summary_name,
        lifecycle_artifact=initial_lifecycle_name,
        checkpoint_artifact=initial_checkpointer_name,
        replay_artifact=args.replay_artifact,
        fork_artifact=args.fork_artifact,
        replay=False,
        fork_user_message="",
    )
    initial_result = subprocess.run(
        initial_command,
        check=False,
        env=build_phase_environment(replay=False, fork_user_message=""),
    )
    if initial_result.returncode != 0:
        return int(initial_result.returncode)

    resume_command = build_phase_command(
        args=args,
        model=model,
        workspace=workspace,
        thread_id=thread_id,
        checkpoint_dir=checkpoint_dir,
        phase="resume",
        history_artifact=resume_history_name,
        summary_artifact=resume_summary_name,
        lifecycle_artifact=resume_lifecycle_name,
        checkpoint_artifact=resume_checkpointer_name,
        replay_artifact=args.replay_artifact,
        fork_artifact=args.fork_artifact,
        replay=args.replay,
        fork_user_message=args.fork_user_message,
    )
    resume_result = subprocess.run(
        resume_command,
        check=False,
        env=build_phase_environment(
            replay=args.replay,
            fork_user_message=args.fork_user_message,
        ),
    )
    if resume_result.returncode != 0:
        return int(resume_result.returncode)

    initial_summary = load_json_file(workspace / initial_summary_name)
    resume_summary = load_json_file(workspace / resume_summary_name)
    initial_lifecycle = load_json_file(workspace / initial_lifecycle_name)
    resume_lifecycle = load_json_file(workspace / resume_lifecycle_name)
    initial_checkpointer = load_json_file(workspace / initial_checkpointer_name)
    resume_checkpointer = load_json_file(workspace / resume_checkpointer_name)

    merged_lifecycle = merge_lifecycle_artifacts(
        initial=initial_lifecycle,
        resume=resume_lifecycle,
        thread_id=thread_id,
        workspace=workspace,
        model=model,
        provider=model_provider(model),
        execution_policy=args.execution_policy,
    )
    write_json(
        workspace / args.lifecycle_artifact,
        merged_lifecycle,
    )
    merged_checkpointer = merge_checkpointer_artifacts(
        initial=initial_checkpointer,
        resume=resume_checkpointer,
    )
    write_json(
        workspace / args.checkpoint_artifact,
        merged_checkpointer,
    )
    merged_summary = merge_run_summaries(
        initial=initial_summary,
        resume=resume_summary,
        checkpoint_artifact=args.checkpoint_artifact,
        lifecycle_artifact=args.lifecycle_artifact,
    )
    merged_summary["lifecycle_event_count"] = int(
        merged_lifecycle["summary"]["event_count"]
    )
    merged_summary["shell_identity_summary"] = merged_lifecycle["summary"]
    merged_summary["checkpoint_file_count"] = int(merged_checkpointer["file_count"])
    write_json(
        workspace / args.summary_artifact,
        merged_summary,
    )
    return 0


def utc_now_rfc3339() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def sha256_text(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


class LangGraphLifecycleRecorder:
    def __init__(
        self,
        *,
        workspace: Path,
        thread_id: str,
        model: str,
        provider: str,
        execution_policy: str,
    ) -> None:
        self.workspace = str(workspace)
        self.thread_id = thread_id
        self.model = model
        self.provider = provider
        self.execution_policy = execution_policy
        self._events: list[dict[str, Any]] = []
        self._shells: dict[int, dict[str, Any]] = {}
        self._seen_checkpoints: set[str] = set()
        self._current_operation = ""

    def record(self, event: str, **fields: Any) -> None:
        item: dict[str, Any] = {
            "index": len(self._events),
            "at": utc_now_rfc3339(),
            "event": event,
        }
        if self._current_operation:
            item["operation"] = self._current_operation
        for key, value in fields.items():
            if value is None:
                continue
            item[key] = value
        self._events.append(item)

    def begin_operation(self, name: str, **fields: Any) -> None:
        self._current_operation = name
        self.record(f"{name}_started", **fields)

    def complete_operation(self, name: str, messages: list[Any]) -> None:
        self.record(
            f"{name}_completed",
            message_count=len(messages),
            tool_message_count=tool_message_count(messages),
            ai_tool_call_count=ai_tool_call_count(messages),
        )
        if self._current_operation == name:
            self._current_operation = ""

    def note_history(self, history_summary: list[dict[str, Any]], source: str) -> None:
        self.record("history_collected", source=source, checkpoint_count=len(history_summary))
        for item in history_summary:
            index = int(item.get("index", -1))
            checkpoint_id = str(item.get("checkpoint_id", ""))
            checkpoint_key = checkpoint_id or (
                f"index:{index}:messages:{int(item.get('message_count', 0))}"
            )
            if checkpoint_key in self._seen_checkpoints:
                continue
            self._seen_checkpoints.add(checkpoint_key)
            self.record(
                "checkpoint_created",
                source=source,
                checkpoint_key=checkpoint_key,
                checkpoint_id=checkpoint_id,
                checkpoint_index=index,
                message_count=int(item.get("message_count", 0)),
                next=list(item.get("next", []) or []),
            )

    def ensure_shell(self, session: Any, *, reason: str) -> dict[str, Any]:
        key = id(session)
        item = self._shells.get(key)
        pid = self._shell_pid(session)
        if item is None:
            item = {
                "shell_session_id": f"shell-{len(self._shells) + 1}",
                "workspace": self._shell_workspace(session),
                "shell_command": self._shell_command(session),
                "execution_policy": type(getattr(session, "_policy", None)).__name__,
                "created_at": utc_now_rfc3339(),
                "created_reason": reason,
                "current_pid": pid,
                "pids": [pid] if pid is not None else [],
                "restart_count": 0,
                "tool_call_count": 0,
                "destroyed": False,
            }
            self._shells[key] = item
            self.record(
                "shell_created",
                reason=reason,
                shell_session_id=item["shell_session_id"],
                shell_pid=pid,
                workspace=item["workspace"],
                shell_command=item["shell_command"],
            )
            return item
        if pid is not None and pid not in item["pids"]:
            item["pids"].append(pid)
        item["current_pid"] = pid
        return item

    def note_shell_reused(self, session: Any, *, reason: str) -> None:
        item = self.ensure_shell(session, reason=reason)
        self.record(
            "shell_reused",
            reason=reason,
            shell_session_id=item["shell_session_id"],
            shell_pid=item.get("current_pid"),
        )

    def note_shell_destroyed(self, session: Any, *, reason: str) -> None:
        item = self.ensure_shell(session, reason=reason)
        item["destroyed"] = True
        item["destroyed_at"] = utc_now_rfc3339()
        self.record(
            "shell_destroyed",
            reason=reason,
            shell_session_id=item["shell_session_id"],
            shell_pid=item.get("current_pid"),
        )

    def note_shell_restarted(
        self, session: Any, *, previous_pid: int | None, reason: str
    ) -> None:
        item = self.ensure_shell(session, reason=reason)
        item["restart_count"] += 1
        self.record(
            "shell_restarted",
            reason=reason,
            shell_session_id=item["shell_session_id"],
            previous_pid=previous_pid,
            shell_pid=item.get("current_pid"),
        )

    def note_shell_command_started(
        self, session: Any, *, tool_call_id: str | None, command: str
    ) -> tuple[str, int | None]:
        item = self.ensure_shell(session, reason="tool-command")
        item["tool_call_count"] += 1
        command_preview = command[:500]
        self.record(
            "shell_command_started",
            shell_session_id=item["shell_session_id"],
            shell_pid=item.get("current_pid"),
            tool_call_id=tool_call_id,
            command_preview=command_preview,
            command_sha256=sha256_text(command),
        )
        return str(item["shell_session_id"]), item.get("current_pid")

    def note_shell_command_finished(
        self,
        session: Any,
        *,
        shell_session_id: str,
        previous_pid: int | None,
        tool_call_id: str | None,
        result: Any,
    ) -> None:
        item = self.ensure_shell(session, reason="tool-command")
        artifact = getattr(result, "artifact", {}) or {}
        self.record(
            "shell_command_finished",
            shell_session_id=shell_session_id,
            previous_pid=previous_pid,
            shell_pid=item.get("current_pid"),
            tool_call_id=tool_call_id,
            status=getattr(result, "status", ""),
            exit_code=artifact.get("exit_code"),
            timed_out=artifact.get("timed_out"),
            truncated_by_lines=artifact.get("truncated_by_lines"),
            truncated_by_bytes=artifact.get("truncated_by_bytes"),
        )
        if previous_pid != item.get("current_pid"):
            reason = (
                "command-timeout"
                if artifact.get("timed_out")
                else "shell-process-replaced"
            )
            self.note_shell_restarted(session, previous_pid=previous_pid, reason=reason)

    def artifact(self) -> dict[str, Any]:
        events = list(self._events)
        shells = sorted(
            self._shells.values(),
            key=lambda item: str(item.get("shell_session_id", "")),
        )
        summary = {
            "event_count": len(events),
            "checkpoint_count": len(self._seen_checkpoints),
            "shell_count": len(shells),
            "shell_create_count": sum(1 for event in events if event["event"] == "shell_created"),
            "shell_reuse_count": sum(1 for event in events if event["event"] == "shell_reused"),
            "shell_restart_count": sum(1 for event in events if event["event"] == "shell_restarted"),
            "shell_destroy_count": sum(1 for event in events if event["event"] == "shell_destroyed"),
            "command_count": sum(1 for event in events if event["event"] == "shell_command_started"),
            "operations": sorted(
                {
                    str(event.get("operation", ""))
                    for event in events
                    if str(event.get("operation", ""))
                }
            ),
        }
        return {
            "schema_version": "syncfuzz.langgraph-lifecycle.v1",
            "thread_id": self.thread_id,
            "workspace": self.workspace,
            "model": self.model,
            "provider": self.provider,
            "execution_policy": self.execution_policy,
            "generated_at": utc_now_rfc3339(),
            "summary": summary,
            "shells": shells,
            "events": events,
        }

    @staticmethod
    def _shell_pid(session: Any) -> int | None:
        process = getattr(session, "_process", None)
        pid = getattr(process, "pid", None)
        return int(pid) if isinstance(pid, int) else None

    @staticmethod
    def _shell_workspace(session: Any) -> str:
        workspace = getattr(session, "_workspace", None)
        return str(workspace) if workspace is not None else ""

    @staticmethod
    def _shell_command(session: Any) -> list[str]:
        command = getattr(session, "_command", ())
        return [str(item) for item in command]


def build_instrumented_shell_middleware(
    base: Any, lifecycle: LangGraphLifecycleRecorder
) -> Any:
    class InstrumentedShellToolMiddleware(base):
        def _get_or_create_resources(self, state: Any) -> Any:
            existing = state.get("shell_session_resources")
            resources = super()._get_or_create_resources(state)
            if existing is not None:
                lifecycle.note_shell_reused(resources.session, reason="before_agent")
            else:
                lifecycle.ensure_shell(resources.session, reason="before_agent")
            return resources

        def after_agent(self, state: Any, runtime: Any) -> None:
            resources = state.get("shell_session_resources")
            if resources is not None:
                lifecycle.note_shell_destroyed(resources.session, reason="after_agent")
            return super().after_agent(state, runtime)

        def _run_shell_tool(
            self,
            resources: Any,
            payload: dict[str, Any],
            *,
            tool_call_id: str | None,
        ) -> Any:
            session = resources.session
            if payload.get("restart"):
                previous_pid = lifecycle._shell_pid(session)
                lifecycle.record(
                    "shell_restart_requested",
                    shell_pid=previous_pid,
                    tool_call_id=tool_call_id,
                )
                result = super()._run_shell_tool(
                    resources,
                    payload,
                    tool_call_id=tool_call_id,
                )
                lifecycle.note_shell_restarted(
                    session,
                    previous_pid=previous_pid,
                    reason="tool-restart",
                )
                return result

            command = payload.get("command") or ""
            shell_session_id, previous_pid = lifecycle.note_shell_command_started(
                session,
                tool_call_id=tool_call_id,
                command=str(command),
            )
            result = super()._run_shell_tool(
                resources,
                payload,
                tool_call_id=tool_call_id,
            )
            lifecycle.note_shell_command_finished(
                session,
                shell_session_id=shell_session_id,
                previous_pid=previous_pid,
                tool_call_id=tool_call_id,
                result=result,
            )
            return result

    return InstrumentedShellToolMiddleware


def instantiate_with_fallbacks(factory: Any, candidates: tuple[dict[str, Any], ...]) -> Any:
    last_error: Exception | None = None
    for kwargs in candidates:
        try:
            return factory(**kwargs)
        except TypeError as exc:
            last_error = exc
    if last_error is None:
        raise TypeError(f"failed to instantiate {factory}")
    raise last_error


def invoke_agent(agent: Any, prompt: str, thread_id: str) -> tuple[Any, dict[str, Any]]:
    config = {"configurable": {"thread_id": thread_id}}
    result = agent.invoke(
        {"messages": [{"role": "user", "content": prompt}]},
        config=config,
    )
    return result, config


def collect_state_history(agent: Any, config: dict[str, Any]) -> list[Any]:
    try:
        return list(agent.get_state_history(config))
    except Exception:  # pragma: no cover - runtime behavior
        return []


def resolve_history_state(history: list[Any], index: int) -> Any | None:
    if index < 0 or index >= len(history):
        return None
    return history[index]


def resolve_operation_checkpoint(
    history: list[Any], checkpoint_index: int, checkpoint_selector: str
) -> tuple[int, str]:
    selector = checkpoint_selector.strip()
    if selector:
        resolved = resolve_checkpoint_selector(history, selector)
        if resolved < 0:
            raise SystemExit(
                f"checkpoint selector {selector!r} did not match any usable checkpoint"
            )
        return resolved, selector
    return checkpoint_index, ""


def resolve_checkpoint_selector(history: list[Any], selector: str) -> int:
    if selector == "before-path-export":
        return checkpoint_before_path_export(history)
    if selector == "before-file-drop":
        return checkpoint_before_file_drop(history)
    if selector == "before-directory-create":
        return checkpoint_before_directory_create(history)
    if selector == "before-file-delete":
        return checkpoint_before_file_delete(history)
    if selector == "before-symlink-create":
        return checkpoint_before_symlink_create(history)
    if selector == "before-file-rename":
        return checkpoint_before_file_rename(history)
    if selector == "before-file-chmod":
        return checkpoint_before_file_chmod(history)
    if selector == "before-file-append":
        return checkpoint_before_file_append(history)
    if selector == "before-hardlink-create":
        return checkpoint_before_hardlink_create(history)
    if selector == "before-fifo-create":
        return checkpoint_before_fifo_create(history)
    if selector == "before-open-fd-hold":
        return checkpoint_before_open_fd_hold(history)
    if selector == "before-deleted-open-fd-hold":
        return checkpoint_before_deleted_open_fd_hold(history)
    if selector == "before-inherited-fd-leak-holder":
        return checkpoint_before_inherited_fd_leak_holder(history)
    if selector == "before-unix-listener-launch":
        return checkpoint_before_unix_listener_launch(history)
    raise SystemExit(f"unsupported checkpoint selector: {selector}")


def checkpoint_before_path_export(history: list[Any]) -> int:
    saw_export_in_newer_state = False
    for index in range(len(history) - 1, -1, -1):
        has_export = state_has_path_export(history[index])
        if not saw_export_in_newer_state and has_export:
            candidate = index + 1
            if candidate >= len(history):
                return -1
            return candidate
        saw_export_in_newer_state = saw_export_in_newer_state or has_export
    return -1


def checkpoint_before_file_drop(history: list[Any]) -> int:
    saw_file_drop_in_newer_state = False
    for index in range(len(history) - 1, -1, -1):
        has_file_drop = state_has_file_drop(history[index])
        if not saw_file_drop_in_newer_state and has_file_drop:
            candidate = index + 1
            if candidate >= len(history):
                return -1
            return candidate
        saw_file_drop_in_newer_state = saw_file_drop_in_newer_state or has_file_drop
    return -1


def checkpoint_before_file_delete(history: list[Any]) -> int:
    saw_file_delete_in_newer_state = False
    for index in range(len(history) - 1, -1, -1):
        has_file_delete = state_has_file_delete(history[index])
        if not saw_file_delete_in_newer_state and has_file_delete:
            candidate = index + 1
            if candidate >= len(history):
                return -1
            return candidate
        saw_file_delete_in_newer_state = saw_file_delete_in_newer_state or has_file_delete
    return -1


def checkpoint_before_directory_create(history: list[Any]) -> int:
    saw_directory_create_in_newer_state = False
    for index in range(len(history) - 1, -1, -1):
        has_directory_create = state_has_directory_create(history[index])
        if not saw_directory_create_in_newer_state and has_directory_create:
            candidate = index + 1
            if candidate >= len(history):
                return -1
            return candidate
        saw_directory_create_in_newer_state = (
            saw_directory_create_in_newer_state or has_directory_create
        )
    return -1


def checkpoint_before_symlink_create(history: list[Any]) -> int:
    saw_symlink_create_in_newer_state = False
    for index in range(len(history) - 1, -1, -1):
        has_symlink_create = state_has_symlink_create(history[index])
        if not saw_symlink_create_in_newer_state and has_symlink_create:
            candidate = index + 1
            if candidate >= len(history):
                return -1
            return candidate
        saw_symlink_create_in_newer_state = (
            saw_symlink_create_in_newer_state or has_symlink_create
        )
    return -1


def checkpoint_before_file_rename(history: list[Any]) -> int:
    saw_file_rename_in_newer_state = False
    for index in range(len(history) - 1, -1, -1):
        has_file_rename = state_has_file_rename(history[index])
        if not saw_file_rename_in_newer_state and has_file_rename:
            candidate = index + 1
            if candidate >= len(history):
                return -1
            return candidate
        saw_file_rename_in_newer_state = saw_file_rename_in_newer_state or has_file_rename
    return -1


def checkpoint_before_file_chmod(history: list[Any]) -> int:
    saw_file_chmod_in_newer_state = False
    for index in range(len(history) - 1, -1, -1):
        has_file_chmod = state_has_file_chmod(history[index])
        if not saw_file_chmod_in_newer_state and has_file_chmod:
            candidate = index + 1
            if candidate >= len(history):
                return -1
            return candidate
        saw_file_chmod_in_newer_state = saw_file_chmod_in_newer_state or has_file_chmod
    return -1


def checkpoint_before_file_append(history: list[Any]) -> int:
    saw_file_append_in_newer_state = False
    for index in range(len(history) - 1, -1, -1):
        has_file_append = state_has_file_append(history[index])
        if not saw_file_append_in_newer_state and has_file_append:
            candidate = index + 1
            if candidate >= len(history):
                return -1
            return candidate
        saw_file_append_in_newer_state = saw_file_append_in_newer_state or has_file_append
    return -1


def checkpoint_before_hardlink_create(history: list[Any]) -> int:
    saw_hardlink_create_in_newer_state = False
    for index in range(len(history) - 1, -1, -1):
        has_hardlink_create = state_has_hardlink_create(history[index])
        if not saw_hardlink_create_in_newer_state and has_hardlink_create:
            candidate = index + 1
            if candidate >= len(history):
                return -1
            return candidate
        saw_hardlink_create_in_newer_state = (
            saw_hardlink_create_in_newer_state or has_hardlink_create
        )
    return -1


def checkpoint_before_fifo_create(history: list[Any]) -> int:
    saw_fifo_create_in_newer_state = False
    for index in range(len(history) - 1, -1, -1):
        has_fifo_create = state_has_fifo_create(history[index])
        if not saw_fifo_create_in_newer_state and has_fifo_create:
            candidate = index + 1
            if candidate >= len(history):
                return -1
            return candidate
        saw_fifo_create_in_newer_state = saw_fifo_create_in_newer_state or has_fifo_create
    return -1


def checkpoint_before_open_fd_hold(history: list[Any]) -> int:
    saw_open_fd_in_newer_state = False
    for index in range(len(history) - 1, -1, -1):
        has_open_fd = state_has_open_fd_hold(history[index])
        if not saw_open_fd_in_newer_state and has_open_fd:
            candidate = index + 1
            if candidate >= len(history):
                return -1
            return candidate
        saw_open_fd_in_newer_state = saw_open_fd_in_newer_state or has_open_fd
    return -1


def checkpoint_before_deleted_open_fd_hold(history: list[Any]) -> int:
    saw_deleted_open_fd_in_newer_state = False
    for index in range(len(history) - 1, -1, -1):
        has_deleted_open_fd = state_has_deleted_open_fd_hold(history[index])
        if not saw_deleted_open_fd_in_newer_state and has_deleted_open_fd:
            candidate = index + 1
            if candidate >= len(history):
                return -1
            return candidate
        saw_deleted_open_fd_in_newer_state = (
            saw_deleted_open_fd_in_newer_state or has_deleted_open_fd
        )
    return -1


def checkpoint_before_inherited_fd_leak_holder(history: list[Any]) -> int:
    saw_inherited_fd_leak_in_newer_state = False
    for index in range(len(history) - 1, -1, -1):
        has_inherited_fd_leak = state_has_inherited_fd_leak_holder(history[index])
        if not saw_inherited_fd_leak_in_newer_state and has_inherited_fd_leak:
            candidate = index + 1
            if candidate >= len(history):
                return -1
            return candidate
        saw_inherited_fd_leak_in_newer_state = (
            saw_inherited_fd_leak_in_newer_state or has_inherited_fd_leak
        )
    return -1


def checkpoint_before_unix_listener_launch(history: list[Any]) -> int:
    saw_unix_listener_launch_in_newer_state = False
    for index in range(len(history) - 1, -1, -1):
        has_unix_listener_launch = state_has_unix_listener_launch(history[index])
        if not saw_unix_listener_launch_in_newer_state and has_unix_listener_launch:
            candidate = index + 1
            if candidate >= len(history):
                return -1
            return candidate
        saw_unix_listener_launch_in_newer_state = (
            saw_unix_listener_launch_in_newer_state or has_unix_listener_launch
        )
    return -1


def state_has_path_export(state: Any) -> bool:
    values = getattr(state, "values", {}) or {}
    messages = values.get("messages", []) or []
    for message in messages:
        for command in shell_commands_from_message(message):
            if "export PATH=" in command and (
                "workspace-bin" in command or "attacker-bin" in command
            ):
                return True
    return False


def state_has_file_drop(state: Any) -> bool:
    values = getattr(state, "values", {}) or {}
    messages = values.get("messages", []) or []
    for message in messages:
        for command in shell_commands_from_message(message):
            normalized = normalize_shell_command(command)
            if command_writes_workspace_file(normalized, "branch-note.txt"):
                return True
    return False


def state_has_file_delete(state: Any) -> bool:
    values = getattr(state, "values", {}) or {}
    messages = values.get("messages", []) or []
    for message in messages:
        for command in shell_commands_from_message(message):
            normalized = normalize_shell_command(command)
            if command_deletes_workspace_file(normalized, "branch-delete-note.txt"):
                return True
    return False


def state_has_directory_create(state: Any) -> bool:
    values = getattr(state, "values", {}) or {}
    messages = values.get("messages", []) or []
    for message in messages:
        for command in shell_commands_from_message(message):
            normalized = normalize_shell_command(command)
            if command_creates_workspace_directory(normalized, "branch-dir"):
                return True
    return False


def state_has_symlink_create(state: Any) -> bool:
    values = getattr(state, "values", {}) or {}
    messages = values.get("messages", []) or []
    for message in messages:
        for command in shell_commands_from_message(message):
            normalized = normalize_shell_command(command)
            if command_creates_workspace_symlink(normalized, "branch-link.txt"):
                return True
    return False


def state_has_file_rename(state: Any) -> bool:
    values = getattr(state, "values", {}) or {}
    messages = values.get("messages", []) or []
    for message in messages:
        for command in shell_commands_from_message(message):
            normalized = normalize_shell_command(command)
            if command_renames_workspace_file(
                normalized, "branch-rename-src.txt", "branch-rename-dst.txt"
            ):
                return True
    return False


def state_has_file_chmod(state: Any) -> bool:
    values = getattr(state, "values", {}) or {}
    messages = values.get("messages", []) or []
    for message in messages:
        for command in shell_commands_from_message(message):
            normalized = normalize_shell_command(command)
            if command_changes_workspace_file_mode(
                normalized, "branch-mode-note.txt", "000"
            ):
                return True
    return False


def state_has_file_append(state: Any) -> bool:
    values = getattr(state, "values", {}) or {}
    messages = values.get("messages", []) or []
    for message in messages:
        for command in shell_commands_from_message(message):
            normalized = normalize_shell_command(command)
            if command_appends_workspace_file(
                normalized, "branch-append-note.txt"
            ):
                return True
    return False


def state_has_hardlink_create(state: Any) -> bool:
    values = getattr(state, "values", {}) or {}
    messages = values.get("messages", []) or []
    for message in messages:
        for command in shell_commands_from_message(message):
            normalized = normalize_shell_command(command)
            if command_creates_workspace_hardlink(normalized, "branch-hardlink.txt"):
                return True
    return False


def state_has_fifo_create(state: Any) -> bool:
    values = getattr(state, "values", {}) or {}
    messages = values.get("messages", []) or []
    for message in messages:
        for command in shell_commands_from_message(message):
            normalized = normalize_shell_command(command)
            if command_creates_workspace_fifo(normalized, "branch-fifo"):
                return True
    return False


def state_has_open_fd_hold(state: Any) -> bool:
    values = getattr(state, "values", {}) or {}
    messages = values.get("messages", []) or []
    for message in messages:
        for command in shell_commands_from_message(message):
            normalized = normalize_shell_command(command)
            if command_opens_workspace_fd(normalized, "branch-fd-note.txt"):
                return True
    return False


def state_has_deleted_open_fd_hold(state: Any) -> bool:
    values = getattr(state, "values", {}) or {}
    messages = values.get("messages", []) or []
    for message in messages:
        for command in shell_commands_from_message(message):
            normalized = normalize_shell_command(command)
            if command_opens_deleted_workspace_fd(
                normalized, "branch-deleted-fd-note.txt"
            ):
                return True
    return False


def state_has_inherited_fd_leak_holder(state: Any) -> bool:
    values = getattr(state, "values", {}) or {}
    messages = values.get("messages", []) or []
    for message in messages:
        for command in shell_commands_from_message(message):
            normalized = normalize_shell_command(command)
            if command_opens_deleted_workspace_fd(
                normalized, "branch-inherited-fd-secret.txt"
            ):
                return True
    return False


def state_has_unix_listener_launch(state: Any) -> bool:
    values = getattr(state, "values", {}) or {}
    messages = values.get("messages", []) or []
    for message in messages:
        for command in shell_commands_from_message(message):
            normalized = normalize_shell_command(command)
            if command_launches_unix_listener(normalized):
                return True
    return False


def normalize_shell_command(command: str) -> str:
    return " ".join(command.strip().lower().replace("\\", "/").split())


def command_writes_workspace_file(command: str, filename: str) -> bool:
    filename = filename.lower()
    for marker in (">>", ">"):
        index = command.find(marker)
        if index >= 0 and filename in command[index:]:
            return True
    if "touch " in command and filename in command:
        return True
    if "tee " in command and filename in command:
        return True
    return False


def command_deletes_workspace_file(command: str, filename: str) -> bool:
    filename = filename.lower()
    if "rm " not in command and "unlink " not in command:
        return False
    return (
        command.endswith(" " + filename)
        or ("/" + filename) in command
        or (" " + filename + " ") in command
        or (" " + filename + ";") in command
        or (" " + filename + " &&") in command
        or (" " + filename + " ||") in command
    )


def command_creates_workspace_directory(command: str, filename: str) -> bool:
    filename = filename.lower()
    if "mkdir " not in command and "install -d " not in command:
        return False
    return (
        command.endswith(" " + filename)
        or ("/" + filename) in command
        or (" " + filename + " ") in command
    )


def command_creates_workspace_symlink(command: str, filename: str) -> bool:
    filename = filename.lower()
    if "ln -s" not in command:
        return False
    return (
        command.endswith(" " + filename)
        or ("/" + filename) in command
        or (" " + filename + " ") in command
    )


def command_renames_workspace_file(command: str, old_name: str, new_name: str) -> bool:
    old_name = old_name.lower()
    new_name = new_name.lower()
    if "mv " not in command and "rename " not in command:
        return False
    return (
        (command.endswith(" " + new_name) or (" " + new_name + " ") in command or ("/" + new_name) in command)
        and ((" " + old_name + " ") in command or ("/" + old_name + " ") in command or command.startswith("mv " + old_name + " "))
    )


def command_changes_workspace_file_mode(command: str, filename: str, mode: str) -> bool:
    filename = filename.lower()
    mode = mode.lower().strip()
    if "chmod " not in command:
        return False
    return (
        ("chmod " + mode + " ") in command or ("chmod 0" + mode + " ") in command
    ) and (
        command.endswith(" " + filename)
        or ("/" + filename) in command
        or (" " + filename + " ") in command
    )


def command_appends_workspace_file(command: str, filename: str) -> bool:
    filename = filename.lower()
    return (">>" in command and filename in command) or (
        "tee -a " in command and filename in command
    )


def command_creates_workspace_hardlink(command: str, filename: str) -> bool:
    filename = filename.lower()
    if "ln " not in command or "ln -s" in command:
        return False
    return (
        command.endswith(" " + filename)
        or ("/" + filename) in command
        or (" " + filename + " ") in command
    )


def command_creates_workspace_fifo(command: str, filename: str) -> bool:
    filename = filename.lower()
    if "mkfifo " not in command:
        return False
    return (
        command.endswith(" " + filename)
        or ("/" + filename) in command
        or (" " + filename + " ") in command
    )


def command_opens_workspace_fd(command: str, filename: str) -> bool:
    filename = filename.lower()
    return f"exec 9<{filename}" in command or (
        "exec 9<" in command and ("/" + filename) in command
    )


def command_opens_deleted_workspace_fd(command: str, filename: str) -> bool:
    return command_opens_workspace_fd(command, filename) and command_deletes_workspace_file(
        command, filename
    )


def command_launches_unix_listener(command: str) -> bool:
    return (
        "branch-listener.sock" in command
        and ("socket.af_unix" in command or "af_unix" in command)
        and (".bind(" in command or "bind(" in command)
        and (".listen(" in command or "listen(" in command)
    )


def shell_commands_from_message(message: Any) -> list[str]:
    commands: list[str] = []
    tool_calls = getattr(message, "tool_calls", None) or []
    if not tool_calls:
        additional = getattr(message, "additional_kwargs", {}) or {}
        tool_calls = additional.get("tool_calls", []) or []
    for call in tool_calls:
        if isinstance(call, dict):
            name = str(call.get("name") or call.get("function", {}).get("name", ""))
            args = call.get("args")
            if args is None:
                args = call.get("arguments")
            if args is None and isinstance(call.get("function"), dict):
                args = call["function"].get("arguments")
        else:
            name = str(getattr(call, "name", ""))
            args = getattr(call, "args", "")
        if name != "shell":
            continue
        if not isinstance(args, str):
            try:
                args = json.dumps(args, ensure_ascii=False)
            except TypeError:
                args = str(args)
        commands.append(extract_shell_command(args))
    return commands


def extract_shell_command(raw: str) -> str:
    trimmed = raw.strip()
    if not trimmed:
        return ""
    try:
        decoded = json.loads(trimmed)
    except json.JSONDecodeError:
        return trimmed
    if isinstance(decoded, dict):
        command = decoded.get("command")
        if isinstance(command, str):
            return command
    return trimmed


def maybe_replay(agent: Any, history: list[Any], index: int) -> Any | None:
    state = resolve_history_state(history, index)
    if state is None:
        return None
    return agent.invoke(None, config=state.config)


def maybe_fork(
    agent: Any,
    HumanMessage: Any,
    history: list[Any],
    index: int,
    fork_user_message: str,
) -> Any | None:
    if not fork_user_message:
        return None
    state = resolve_history_state(history, index)
    if state is None:
        return None
    return agent.invoke(
        {"messages": [HumanMessage(content=fork_user_message)]},
        config=state.config,
    )


def extract_checkpoint_id(state: Any) -> str:
    try:
        configurable = state.config.get("configurable", {})
    except AttributeError:
        return ""
    return str(configurable.get("checkpoint_id", ""))


def result_messages(result: Any | None) -> list[Any]:
    messages = (result or {}).get("messages", []) if result else []
    return list(messages or [])


def message_role(message: Any) -> str:
    return str(getattr(message, "type", message.__class__.__name__.lower()))


def tool_message_count(messages: list[Any]) -> int:
    return sum(1 for message in messages if message_role(message) == "tool")


def ai_tool_call_count(messages: list[Any]) -> int:
    count = 0
    for message in messages:
        tool_calls = getattr(message, "tool_calls", None) or []
        if not tool_calls:
            additional = getattr(message, "additional_kwargs", {}) or {}
            tool_calls = additional.get("tool_calls", []) or []
        count += len(tool_calls)
    return count


def summarize_history(history: list[Any]) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    for index, state in enumerate(history):
        values = getattr(state, "values", {}) or {}
        messages = values.get("messages", [])
        items.append(
            {
                "index": index,
                "checkpoint_id": extract_checkpoint_id(state),
                "next": list(getattr(state, "next", ()) or ()),
                "message_count": len(messages),
                "messages": summarize_messages(messages),
            }
        )
    return items


def summarize_operation_result(
    operation: str,
    history: list[Any],
    index: int,
    result: Any | None,
    selector: str = "",
    user_message: str = "",
) -> dict[str, Any]:
    state = resolve_history_state(history, index)
    return {
        "operation": operation,
        "requested": True,
        "checkpoint_selector": selector,
        "checkpoint_index": index,
        "checkpoint_id": extract_checkpoint_id(state) if state is not None else "",
        "available_history": len(history),
        "user_message": user_message,
        "messages": summarize_messages(result_messages(result)),
    }


def message_content_text(message: Any) -> str:
    content = getattr(message, "content", "")
    if isinstance(content, list):
        return json.dumps(content, ensure_ascii=False)
    return str(content)


def summarize_messages(messages: list[Any]) -> list[dict[str, Any]]:
    summary: list[dict[str, Any]] = []
    for message in messages[-6:]:
        role = message_role(message)
        text = message_content_text(message)
        item = {
            "role": role,
            "content": text[:500],
        }
        tool_calls = summarize_tool_calls(message)
        if tool_calls:
            item["tool_calls"] = tool_calls
        summary.append(item)
    return summary


def summarize_tool_calls(message: Any) -> list[dict[str, str]]:
    tool_calls = getattr(message, "tool_calls", None) or []
    if not tool_calls:
        additional = getattr(message, "additional_kwargs", {}) or {}
        tool_calls = additional.get("tool_calls", []) or []
    summary: list[dict[str, str]] = []
    for call in tool_calls[:4]:
        if isinstance(call, dict):
            name = str(call.get("name") or call.get("function", {}).get("name", ""))
            args = call.get("args")
            if args is None:
                args = call.get("arguments")
            if args is None and isinstance(call.get("function"), dict):
                args = call["function"].get("arguments")
        else:
            name = str(getattr(call, "name", ""))
            args = getattr(call, "args", "")
        summary.append(
            {
                "name": name,
                "args": json.dumps(args, ensure_ascii=False)[:500]
                if not isinstance(args, str)
                else args[:500],
            }
        )
    return summary


def write_json(path: Path, value: Any) -> None:
    path.write_text(json.dumps(value, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")


def main() -> int:
    args = parse_args()
    if (
        args.internal_phase == "full"
        and args.process_mode == "split-process"
        and (args.replay or args.fork_user_message.strip())
    ):
        return run_split_process(args)
    workspace = resolve_workspace(args)
    model = resolve_model(args)
    provider = model_provider(model)
    validate_model_environment(model)
    prompt = resolve_prompt(args)
    task_path = resolve_task_path(args)
    task_contract = read_task_contract(task_path)
    require_tool_use = task_requires_tool_use(args, task_contract)
    thread_id = resolve_thread_id(args)
    (
        create_agent,
        InMemorySaver,
        PersistentDict,
        HumanMessage,
        ShellToolMiddleware,
        HostExecutionPolicy,
        DockerExecutionPolicy,
        CodexSandboxExecutionPolicy,
    ) = import_langchain_runtime()

    lifecycle = LangGraphLifecycleRecorder(
        workspace=workspace,
        thread_id=thread_id,
        model=model,
        provider=provider,
        execution_policy=args.execution_policy,
    )
    lifecycle.record(
        "target_started",
        task_id=str(task_contract.get("task_id", "")).strip(),
        require_tool_use=require_tool_use,
        process_mode=args.process_mode,
        internal_phase=args.internal_phase,
    )
    execution_policy = build_execution_policy(
        args,
        HostExecutionPolicy,
        DockerExecutionPolicy,
        CodexSandboxExecutionPolicy,
    )
    middleware = [
        build_shell_middleware(
            ShellToolMiddleware, workspace, execution_policy, lifecycle
        ),
    ]
    checkpoint_dir = None
    if args.checkpoint_backend == "disk":
        checkpoint_dir = resolve_checkpoint_dir(workspace, args.checkpoint_dir)
        checkpointer = build_durable_checkpointer(
            InMemorySaver,
            PersistentDict,
            checkpoint_dir,
        )
    else:
        checkpointer = InMemorySaver()
    lifecycle.record(
        "checkpointer_configured",
        backend=args.checkpoint_backend,
        durable=args.checkpoint_backend != "memory",
        checkpoint_dir=str(checkpoint_dir) if checkpoint_dir is not None else "",
    )
    system_prompt = args.system_prompt or default_system_prompt(workspace, task_path)
    agent = create_agent(
        model=model,
        tools=[],
        middleware=middleware,
        checkpointer=checkpointer,
        system_prompt=system_prompt,
        name="syncfuzz-langgraph-shell-react",
    )

    result = None
    config = {"configurable": {"thread_id": thread_id}}
    if args.internal_phase != "resume":
        lifecycle.begin_operation(
            "initial_run",
            prompt_sha256=sha256_text(prompt),
            prompt_preview=prompt[:500],
        )
        try:
            result, config = invoke_agent(agent, prompt, thread_id)
        finally:
            lifecycle.complete_operation("initial_run", result_messages(result))
    else:
        lifecycle.begin_operation(
            "resume_load",
            replay_requested=args.replay,
            fork_requested=bool(args.fork_user_message.strip()),
        )
        lifecycle.complete_operation("resume_load", [])
    history = collect_state_history(agent, config)
    if args.internal_phase == "resume" and not history:
        raise SystemExit(
            f"resume requested but no stored history was available for thread {thread_id}"
        )
    history_summary = summarize_history(history)
    history_source = "resume-load" if args.internal_phase == "resume" else "initial-run"
    lifecycle.note_history(history_summary, history_source)
    write_json(workspace / args.history_artifact, history_summary)
    selected_checkpoint_index, checkpoint_selector = resolve_operation_checkpoint(
        history, args.checkpoint_index, args.checkpoint_selector
    )
    selected_state = resolve_history_state(history, selected_checkpoint_index)
    replay_result = None
    if args.replay:
        lifecycle.begin_operation(
            "replay",
            checkpoint_index=selected_checkpoint_index,
            checkpoint_selector=checkpoint_selector,
            checkpoint_id=extract_checkpoint_id(selected_state),
        )
        try:
            replay_result = maybe_replay(agent, history, selected_checkpoint_index)
        finally:
            lifecycle.complete_operation("replay", result_messages(replay_result))
        if replay_result is None:
            raise SystemExit(
                f"replay requested but checkpoint index {selected_checkpoint_index} is unavailable"
            )
        if selected_state is not None:
            replay_history = collect_state_history(agent, selected_state.config)
            lifecycle.note_history(summarize_history(replay_history), "replay")
    fork_result = None
    if args.fork_user_message.strip():
        lifecycle.begin_operation(
            "fork",
            checkpoint_index=selected_checkpoint_index,
            checkpoint_selector=checkpoint_selector,
            checkpoint_id=extract_checkpoint_id(selected_state),
            fork_user_message=args.fork_user_message.strip()[:500],
        )
        try:
            fork_result = maybe_fork(
                agent,
                HumanMessage,
                history,
                selected_checkpoint_index,
                args.fork_user_message.strip(),
            )
        finally:
            lifecycle.complete_operation("fork", result_messages(fork_result))
        if fork_result is None:
            raise SystemExit(
                f"fork requested but checkpoint index {selected_checkpoint_index} is unavailable"
            )
        if selected_state is not None:
            fork_history = collect_state_history(agent, selected_state.config)
            lifecycle.note_history(summarize_history(fork_history), "fork")

    openai_endpoint = openai_base_url() if provider == "openai" else ""
    openai_endpoint_source = ""
    if provider == "openai":
        openai_endpoint_source = "OPENAI_BASE_URL" if openai_endpoint else "provider-default"

    messages = result_messages(result)
    replay_messages = result_messages(replay_result)
    fork_messages = result_messages(fork_result)
    observed_messages = (
        replay_messages + fork_messages
        if args.internal_phase == "resume"
        else messages
    )
    tool_messages = tool_message_count(observed_messages)
    ai_tool_calls = ai_tool_call_count(observed_messages)
    validation_error = ""
    if require_tool_use and tool_messages == 0:
        validation_error = (
            "required shell tool use was not observed; the model returned without "
            "executing ShellToolMiddleware"
        )

    summary = {
        "target": "langgraph-shell-react",
        "model": model,
        "model_provider": provider,
        "openai_base_url": openai_endpoint,
        "openai_base_url_source": openai_endpoint_source,
        "thread_id": thread_id,
        "execution_policy": args.execution_policy,
        "process_mode": args.process_mode,
        "internal_phase": args.internal_phase,
        "checkpoint_backend": args.checkpoint_backend,
        "checkpoint_dir": str(checkpoint_dir) if checkpoint_dir is not None else "",
        "checkpoint_artifact": args.checkpoint_artifact,
        "workspace": str(workspace),
        "task_file": task_path,
        "task_id": str(task_contract.get("task_id", "")),
        "prompt_file": args.prompt_file or os.environ.get("SYNCFUZZ_PROMPT_FILE", ""),
        "require_tool_use": require_tool_use,
        "tool_use_observed": tool_messages > 0,
        "tool_message_count": tool_messages,
        "ai_tool_call_count": ai_tool_calls,
        "validation_error": validation_error,
        "requested_checkpoint_index": args.checkpoint_index,
        "checkpoint_selector": checkpoint_selector,
        "checkpoint_index": selected_checkpoint_index,
        "resolved_checkpoint_index": selected_checkpoint_index,
        "resolved_checkpoint_id": extract_checkpoint_id(selected_state)
        if selected_state is not None
        else "",
        "replay_requested": args.replay,
        "fork_requested": bool(args.fork_user_message.strip()),
        "history_count": len(history),
        "lifecycle_artifact": args.lifecycle_artifact,
        "checkpoint_ids": [
            item["checkpoint_id"] for item in summarize_history(history) if item["checkpoint_id"]
        ],
        "result": summarize_messages(messages),
        "replay_result": summarize_messages(replay_messages),
        "fork_result": summarize_messages(fork_messages),
    }
    lifecycle.record(
        "target_completed",
        validation_error=validation_error,
        replay_requested=args.replay,
        fork_requested=bool(args.fork_user_message.strip()),
    )
    lifecycle_artifact = lifecycle.artifact()
    summary["lifecycle_event_count"] = lifecycle_artifact["summary"]["event_count"]
    summary["shell_identity_summary"] = lifecycle_artifact["summary"]
    checkpointer_artifact = summarize_checkpoint_backend(
        args.checkpoint_backend,
        thread_id,
        checkpoint_dir,
    )
    summary["checkpoint_file_count"] = checkpointer_artifact["file_count"]
    write_json(workspace / args.lifecycle_artifact, lifecycle_artifact)
    write_json(workspace / args.checkpoint_artifact, checkpointer_artifact)
    write_json(workspace / args.summary_artifact, summary)
    if args.replay:
        write_json(
            workspace / args.replay_artifact,
            summarize_operation_result(
                "replay",
                history,
                selected_checkpoint_index,
                replay_result,
                selector=checkpoint_selector,
            ),
        )
    if args.fork_user_message.strip():
        write_json(
            workspace / args.fork_artifact,
            summarize_operation_result(
                "fork",
                history,
                selected_checkpoint_index,
                fork_result,
                selector=checkpoint_selector,
                user_message=args.fork_user_message.strip(),
            ),
        )

    if validation_error:
        print(validation_error, file=sys.stderr)
        return 3

    final_messages_raw = fork_messages or replay_messages or messages
    if final_messages_raw:
        print(message_content_text(final_messages_raw[-1]))
    return 0


if __name__ == "__main__":
    sys.exit(main())
