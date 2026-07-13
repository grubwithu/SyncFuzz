#!/usr/bin/env python3
"""Thin MAF-1 wrapper around the official GitHubCopilotAgent shell sample."""

from __future__ import annotations

import argparse
import asyncio
import hashlib
import inspect
import json
import os
import shutil
import sys
import time
import traceback
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

SUMMARY_ARTIFACT = "maf-run-summary.json"
SESSION_ARTIFACT = "maf-session.json"
LIFECYCLE_ARTIFACT = "maf-lifecycle.json"

SUPPORTED_TASKS = {
    "orphan-process",
    "orphan-process-long-delay",
    "persistent-shell-poisoning",
    "unix-listener-residue",
    "file-residue",
    "directory-residue",
    "delete-residue",
    "symlink-residue",
    "rename-residue",
    "mode-residue",
    "append-residue",
    "hardlink-residue",
    "fifo-residue",
    "env-residue",
    "function-residue",
    "cwd-residue",
    "umask-residue",
}

DEFAULT_COPILOT_CLI_CANDIDATES = (
    "copilot",
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run the SyncFuzz MAF GitHub Copilot shell target."
    )
    parser.add_argument("--prompt", default="", help="Inline prompt override.")
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
        "--model",
        default=os.environ.get("COPILOT_MODEL", "").strip(),
        help="Optional provider model override.",
    )
    parser.add_argument(
        "--copilot-timeout",
        type=float,
        default=env_float("MAF_TIMEOUT"),
        help="Optional GitHub Copilot request timeout in seconds.",
    )
    parser.add_argument(
        "--copilot-cli",
        default=os.environ.get("MAF_COPILOT_CLI", "").strip(),
        help="Optional GitHub Copilot CLI path override.",
    )
    parser.add_argument(
        "--session-home",
        default=os.environ.get("MAF_SESSION_HOME", "").strip(),
        help="Optional session home override for the provider runtime.",
    )
    parser.add_argument(
        "--log-level",
        default=os.environ.get("MAF_LOG_LEVEL", "").strip(),
        help="Optional log level override for the provider runtime.",
    )
    parser.add_argument(
        "--summary-artifact",
        default=os.environ.get("MAF_SUMMARY_ARTIFACT", SUMMARY_ARTIFACT),
        help="Summary artifact filename written inside the workspace.",
    )
    parser.add_argument(
        "--session-artifact",
        default=os.environ.get("MAF_SESSION_ARTIFACT", SESSION_ARTIFACT),
        help="Session artifact filename written inside the workspace.",
    )
    parser.add_argument(
        "--lifecycle-artifact",
        default=os.environ.get("MAF_LIFECYCLE_ARTIFACT", LIFECYCLE_ARTIFACT),
        help="Lifecycle artifact filename written inside the workspace.",
    )
    parser.add_argument(
        "--allow-unsupported-task",
        action="store_true",
        default=env_bool("MAF_ALLOW_UNSUPPORTED_TASKS"),
        help="Allow tasks outside the current MAF-1 smoke subset.",
    )
    parser.add_argument(
        "--check",
        action="store_true",
        help="Only validate imports and local prerequisites.",
    )
    return parser.parse_args()


def env_bool(name: str) -> bool:
    value = os.environ.get(name, "").strip().lower()
    return value in {"1", "true", "yes", "on"}


def env_float(name: str) -> float | None:
    value = os.environ.get(name, "").strip()
    if not value:
        return None
    try:
        return float(value)
    except ValueError as exc:
        raise SystemExit(f"{name} must be a number, got {value!r}") from exc


def env_str(name: str) -> str:
    return os.environ.get(name, "").strip()


def now_rfc3339() -> str:
    return datetime.now(timezone.utc).isoformat()


def append_event(events: list[dict[str, Any]], name: str, **details: Any) -> None:
    event = {"timestamp": now_rfc3339(), "event": name}
    if details:
        event["details"] = to_jsonable(details)
    events.append(event)


def to_jsonable(value: Any, depth: int = 0) -> Any:
    if depth > 3:
        return str(value)
    if value is None or isinstance(value, (str, int, float, bool)):
        return value
    if isinstance(value, Path):
        return str(value)
    if isinstance(value, dict):
        return {str(key): to_jsonable(item, depth + 1) for key, item in value.items()}
    if isinstance(value, (list, tuple, set)):
        return [to_jsonable(item, depth + 1) for item in value]
    if hasattr(value, "__dict__"):
        items = {}
        for key, item in vars(value).items():
            if key.startswith("_"):
                continue
            items[str(key)] = to_jsonable(item, depth + 1)
        if items:
            return items
    return str(value)


def sha256_text(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def resolve_workspace(args: argparse.Namespace) -> Path:
    value = args.workspace or os.environ.get("SYNCFUZZ_WORKSPACE", "")
    if not value:
        raise SystemExit("workspace is required via --workspace or SYNCFUZZ_WORKSPACE")
    workspace = Path(value).expanduser()
    if not workspace.is_absolute():
        workspace = workspace.resolve()
    workspace.mkdir(parents=True, exist_ok=True)
    return workspace


def resolve_text(path_value: str) -> str:
    return Path(path_value).read_text(encoding="utf-8").strip()


def resolve_prompt(args: argparse.Namespace) -> tuple[str, str]:
    if args.prompt:
        return args.prompt, "<inline>"
    prompt_file = args.prompt_file or os.environ.get("SYNCFUZZ_PROMPT_FILE", "")
    if prompt_file:
        return resolve_text(prompt_file), prompt_file
    prompt = os.environ.get("SYNCFUZZ_PROMPT", "").strip()
    if prompt:
        return prompt, "<env>"
    raise SystemExit("prompt is required via --prompt, --prompt-file, or SYNCFUZZ_PROMPT")


def resolve_task_file(args: argparse.Namespace) -> str:
    return args.task_file or os.environ.get("SYNCFUZZ_TASK_FILE", "")


def load_task_metadata(task_file: str) -> dict[str, Any]:
    if not task_file:
        return {}
    raw = Path(task_file).read_text(encoding="utf-8")
    data = json.loads(raw)
    if not isinstance(data, dict):
        raise SystemExit(f"unexpected target task payload in {task_file}")
    return data


def ensure_supported_task(
    task_meta: dict[str, Any], allow_unsupported: bool, events: list[dict[str, Any]]
) -> None:
    task_id = str(task_meta.get("task_id", "")).strip()
    if not task_id or allow_unsupported:
        return
    if task_id in SUPPORTED_TASKS:
        append_event(events, "task_supported", task_id=task_id)
        return
    raise SystemExit(
        "task "
        + repr(task_id)
        + " is not in the current MAF-1 smoke subset; supported tasks: "
        + ", ".join(sorted(SUPPORTED_TASKS))
    )


def resolve_copilot_timeout(
    requested: float | None, task_meta: dict[str, Any], events: list[dict[str, Any]]
) -> float | None:
    if requested is not None:
        if requested <= 0:
            raise SystemExit("copilot timeout must be positive")
        append_event(
            events,
            "copilot_timeout_resolved",
            source="arg-or-env",
            timeout_seconds=requested,
        )
        return requested
    timeout_ms = task_meta.get("timeout_ms")
    try:
        timeout_budget_seconds = float(timeout_ms) / 1000.0
    except (TypeError, ValueError):
        timeout_budget_seconds = 0.0
    if timeout_budget_seconds <= 0:
        return None
    # Leave enough budget for the Copilot CLI to unwind the session and for the
    # wrapper to persist lifecycle diagnostics before SyncFuzz kills the process.
    derived = max(5.0, timeout_budget_seconds - 20.0)
    append_event(
        events,
        "copilot_timeout_resolved",
        source="target-timeout-budget",
        timeout_seconds=derived,
        target_timeout_seconds=timeout_budget_seconds,
    )
    return derived


def summarize_provider_config(provider: dict[str, Any] | None) -> dict[str, Any]:
    if not provider:
        return {
            "configured": False,
            "mode": "copilot-default",
            "type": "",
            "base_url": "",
            "base_url_source": "",
            "model_id": "",
            "auth_mode": "",
            "auth_source": "",
        }
    return {
        "configured": True,
        "mode": "custom-openai-compatible",
        "type": str(provider.get("type", "")).strip(),
        "base_url": str(provider.get("base_url", "")).strip(),
        "base_url_source": "",
        "model_id": str(provider.get("model_id", "")).strip(),
        "auth_mode": "api-key" if provider.get("api_key") else "",
        "auth_source": "",
    }


def resolve_provider_config(
    model: str, events: list[dict[str, Any]]
) -> tuple[dict[str, Any] | None, dict[str, Any]]:
    base_url = env_str("COPILOT_PROVIDER_BASE_URL")
    base_url_source = "COPILOT_PROVIDER_BASE_URL" if base_url else ""
    if not base_url:
        base_url = env_str("OPENAI_BASE_URL")
        if base_url:
            base_url_source = "OPENAI_BASE_URL"
    provider_type = env_str("COPILOT_PROVIDER_TYPE") or "openai"
    api_key = env_str("COPILOT_PROVIDER_API_KEY")
    auth_source = "COPILOT_PROVIDER_API_KEY" if api_key else ""
    if not api_key:
        api_key = env_str("OPENAI_API_KEY")
        if api_key:
            auth_source = "OPENAI_API_KEY"

    if not base_url and not api_key and not model:
        summary = summarize_provider_config(None)
        append_event(events, "provider_config_resolved", **summary)
        return None, summary
    if not base_url:
        raise SystemExit(
            "MAF custom provider requires OPENAI_BASE_URL or COPILOT_PROVIDER_BASE_URL."
        )
    if not model:
        raise SystemExit(
            "MAF custom provider requires COPILOT_MODEL."
        )

    provider: dict[str, Any] = {
        "type": provider_type,
        "base_url": base_url,
    }
    if api_key:
        provider["api_key"] = api_key
    if model:
        provider["model_id"] = model

    summary = summarize_provider_config(provider)
    summary["base_url_source"] = base_url_source
    summary["auth_source"] = auth_source
    append_event(events, "provider_config_resolved", **summary)
    return provider, summary


def find_program(program: str) -> str:
    value = program.strip()
    if value:
        candidates = (value,)
    else:
        candidates = DEFAULT_COPILOT_CLI_CANDIDATES
    checked: list[str] = []
    for candidate in candidates:
        if os.path.sep in candidate:
            path = Path(candidate).expanduser()
            checked.append(str(path))
            if path.exists() and os.access(path, os.X_OK):
                return str(path)
            continue
        checked.append(candidate)
        found = shutil.which(candidate)
        if found:
            return found
    if value:
        raise SystemExit(
            "required GitHub Copilot CLI executable was not found: "
            + value
            + ". Set MAF_COPILOT_CLI to the actual copilot executable path."
        )
    raise SystemExit(
        "required GitHub Copilot CLI executable was not found on PATH. "
        "Tried: "
        + ", ".join(checked)
        + ". Install the GitHub Copilot CLI node package or set "
        + "MAF_COPILOT_CLI to the actual copilot executable path."
    )


def import_maf_runtime() -> Any:
    try:
        from agent_framework.github import GitHubCopilotAgent
    except ImportError as exc:  # pragma: no cover - runtime dependency
        raise SystemExit(
            "MAF GitHub Copilot provider is not installed. Install the dependencies "
            "listed in targets/maf_github_copilot_shell/requirements.txt."
        ) from exc
    return GitHubCopilotAgent


def constructor_supports_parameter(cls: Any, name: str) -> bool:
    try:
        signature = inspect.signature(cls)
    except (TypeError, ValueError):
        return False
    for parameter in signature.parameters.values():
        if parameter.kind == inspect.Parameter.VAR_KEYWORD:
            return True
    return name in signature.parameters


def build_agent_kwargs(
    agent_cls: Any,
    instructions: str,
    model: str,
    provider_config: dict[str, Any] | None,
    copilot_timeout: float | None,
    copilot_cli: str,
    session_home: str,
    log_level: str,
    permission_handler: Any,
    pre_tool_use_handler: Any,
) -> dict[str, Any]:
    kwargs: dict[str, Any] = {}
    if constructor_supports_parameter(agent_cls, "instructions"):
        kwargs["instructions"] = instructions
    if constructor_supports_parameter(agent_cls, "name"):
        kwargs["name"] = "syncfuzz-maf"
    if constructor_supports_parameter(agent_cls, "default_options"):
        default_options: dict[str, Any] = {
            "on_permission_request": permission_handler,
            "on_pre_tool_use": pre_tool_use_handler,
        }
        if model:
            default_options["model"] = model
        if provider_config:
            default_options["provider"] = provider_config
        if copilot_timeout is not None:
            default_options["timeout"] = copilot_timeout
        if copilot_cli:
            default_options["cli_path"] = copilot_cli
        if session_home:
            default_options["base_directory"] = session_home
        if log_level:
            default_options["log_level"] = log_level
        kwargs["default_options"] = default_options
    elif constructor_supports_parameter(agent_cls, "on_permission_request"):
        kwargs["on_permission_request"] = permission_handler

    for key in (
        "copilot_cli_path",
        "github_copilot_cli_path",
        "cli_path",
        "copilot_path",
    ):
        if copilot_cli and constructor_supports_parameter(agent_cls, key):
            kwargs[key] = copilot_cli
            break
    for key in ("session_home", "service_session_home"):
        if session_home and constructor_supports_parameter(agent_cls, key):
            kwargs[key] = session_home
            break
    for key in ("log_level", "logging_level"):
        if log_level and constructor_supports_parameter(agent_cls, key):
            kwargs[key] = log_level
            break
    return kwargs


def instantiate_agent(
    agent_cls: Any,
    instructions: str,
    model: str,
    provider_config: dict[str, Any] | None,
    copilot_timeout: float | None,
    copilot_cli: str,
    session_home: str,
    log_level: str,
    permission_handler: Any,
    pre_tool_use_handler: Any,
) -> tuple[Any, dict[str, Any]]:
    primary = build_agent_kwargs(
        agent_cls,
        instructions,
        model,
        provider_config,
        copilot_timeout,
        copilot_cli,
        session_home,
        log_level,
        permission_handler,
        pre_tool_use_handler,
    )
    candidates = [primary]

    minimal = dict(primary)
    minimal.pop("default_options", None)
    minimal.pop("on_permission_request", None)
    if minimal != primary:
        candidates.append(minimal)

    instructions_only = {"instructions": instructions}
    if instructions_only not in candidates:
        candidates.append(instructions_only)

    empty: dict[str, Any] = {}
    if empty not in candidates:
        candidates.append(empty)

    errors: list[str] = []
    for kwargs in candidates:
        try:
            return agent_cls(**kwargs), kwargs
        except TypeError as exc:
            errors.append(f"{kwargs}: {exc}")
    raise RuntimeError(
        "unable to construct GitHubCopilotAgent with supported fallback kwargs: "
        + "; ".join(errors)
    )


def apply_runtime_environment(
    model: str,
    copilot_timeout: float | None,
    copilot_cli: str,
    session_home: str,
    log_level: str,
) -> None:
    if model:
        os.environ.setdefault("COPILOT_MODEL", model)
        os.environ.setdefault("GITHUB_COPILOT_MODEL", model)
    if copilot_timeout is not None:
        os.environ.setdefault("GITHUB_COPILOT_TIMEOUT", str(copilot_timeout))
    if copilot_cli:
        os.environ.setdefault("GITHUB_COPILOT_CLI_PATH", copilot_cli)
    if session_home:
        os.environ.setdefault("GITHUB_COPILOT_BASE_DIRECTORY", session_home)
    if log_level:
        os.environ.setdefault("GITHUB_COPILOT_LOG_LEVEL", log_level)


def apply_provider_environment(provider_config: dict[str, Any] | None, model: str) -> None:
    if not provider_config:
        return
    provider_type = str(provider_config.get("type", "")).strip()
    base_url = str(provider_config.get("base_url", "")).strip()
    api_key = str(provider_config.get("api_key", "")).strip()
    model_id = str(provider_config.get("model_id", "")).strip()

    if provider_type:
        os.environ["COPILOT_PROVIDER_TYPE"] = provider_type
    if base_url:
        os.environ["COPILOT_PROVIDER_BASE_URL"] = base_url
    if api_key:
        os.environ["COPILOT_PROVIDER_API_KEY"] = api_key
    if model:
        os.environ["COPILOT_MODEL"] = model


async def maybe_await(value: Any) -> Any:
    if inspect.isawaitable(value):
        return await value
    return value


async def resolve_run_result(value: Any) -> Any:
    if hasattr(value, "__aiter__"):
        items = []
        async for item in value:
            items.append(item)
        return items
    return await maybe_await(value)


async def call_agent_method(agent: Any, prompt: str) -> tuple[Any, str, str]:
    attempts: list[tuple[str, Any]] = []
    for method_name in ("run", "invoke"):
        method = getattr(agent, method_name, None)
        if callable(method):
            attempts.append((method_name, lambda m=method: m(prompt)))
            attempts.append((method_name + ".input", lambda m=method: m(input=prompt)))
            attempts.append(
                (method_name + ".message", lambda m=method: m(message=prompt))
            )
    errors: list[str] = []
    for label, invocation in attempts:
        try:
            result = invocation()
            return await resolve_run_result(result), label, ""
        except TypeError as exc:
            errors.append(f"{label}: {exc}")
    raise RuntimeError("unable to invoke GitHubCopilotAgent: " + "; ".join(errors))


async def run_agent_once(agent: Any, prompt: str) -> tuple[Any, str, str]:
    if hasattr(agent, "__aenter__") and hasattr(agent, "__aexit__"):
        async with agent:
            return await call_agent_method(agent, prompt)
    if hasattr(agent, "__enter__") and hasattr(agent, "__exit__"):
        with agent:
            return await call_agent_method(agent, prompt)
    return await call_agent_method(agent, prompt)


def extract_text(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, str):
        return value.strip()
    if isinstance(value, list):
        parts = [extract_text(item) for item in value]
        return "\n".join(part for part in parts if part).strip()
    if isinstance(value, dict):
        for key in ("text", "content", "output_text", "message", "messages"):
            if key in value:
                extracted = extract_text(value[key])
                if extracted:
                    return extracted
        return json.dumps(to_jsonable(value), ensure_ascii=True)
    for name in ("output_text", "text", "content", "message"):
        candidate = getattr(value, name, None)
        if candidate is not None:
            extracted = extract_text(candidate)
            if extracted:
                return extracted
    messages = getattr(value, "messages", None)
    if messages is not None:
        extracted = extract_text(messages)
        if extracted:
            return extracted
    return str(value).strip()


def first_non_empty(values: list[tuple[str, Any]]) -> tuple[str, str]:
    for source, value in values:
        if value is None:
            continue
        rendered = str(value).strip()
        if rendered:
            return source, rendered
    return "", ""


def extract_session_metadata(agent: Any, response: Any) -> dict[str, Any]:
    session_values = [
        ("response.service_session_id", getattr(response, "service_session_id", None)),
        (
            "response.session.service_session_id",
            getattr(getattr(response, "session", None), "service_session_id", None),
        ),
        ("agent.service_session_id", getattr(agent, "service_session_id", None)),
        (
            "agent.session.service_session_id",
            getattr(getattr(agent, "session", None), "service_session_id", None),
        ),
        ("response.session_id", getattr(response, "session_id", None)),
        ("agent.session_id", getattr(agent, "session_id", None)),
    ]
    source, session_id = first_non_empty(session_values)
    return {
        "schema_version": "syncfuzz.maf-session.v1",
        "discovered": bool(session_id),
        "source": source,
        "service_session_id": session_id,
    }


def write_json(path: Path, payload: Any) -> None:
    path.write_text(json.dumps(payload, indent=2, ensure_ascii=True) + "\n", encoding="utf-8")


def latest_copilot_log_snapshot(started_at_unix: float | None = None) -> dict[str, Any]:
    copilot_home = env_str("COPILOT_HOME")
    if copilot_home:
        log_dir = Path(copilot_home).expanduser() / "logs"
    else:
        log_dir = Path.home() / ".copilot" / "logs"
    if not log_dir.is_dir():
        return {}

    candidates: list[Path] = []
    for path in log_dir.glob("process-*.log"):
        try:
            stat = path.stat()
        except OSError:
            continue
        if started_at_unix is not None and stat.st_mtime + 5 < started_at_unix:
            continue
        candidates.append(path)
    if not candidates:
        return {}

    latest = max(candidates, key=lambda path: path.stat().st_mtime)
    try:
        lines = latest.read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError:
        return {"copilot_log_path": str(latest)}

    return {
        "copilot_log_path": str(latest),
        "copilot_log_tail": "\n".join(lines[-80:]),
    }


def default_instructions() -> str:
    return (
        "You are a shell-enabled assistant running inside a SyncFuzz workspace. "
        "When the user asks for a shell action, execute it in the current working "
        "directory with the available shell tool instead of only describing what "
        "you would do. Keep the final answer short."
    )


def print_check_success_with_provider(
    cli_path: str, provider_summary: dict[str, Any]
) -> None:
    print("maf github copilot target environment looks ready")
    print(f"copilot_cli: {cli_path}")
    if provider_summary.get("configured"):
        print(
            "provider: "
            + str(provider_summary.get("mode", "")).strip()
            + " "
            + str(provider_summary.get("base_url", "")).strip()
        )
        print(f"auth_mode: {provider_summary.get('auth_mode', '')}")
        print(f"auth_source: {provider_summary.get('auth_source', '')}")
        print(f"model: {provider_summary.get('model_id', '')}")
    else:
        print("provider: copilot-default")


async def run_main(args: argparse.Namespace) -> int:
    events: list[dict[str, Any]] = []
    append_event(events, "wrapper_started", check=args.check)
    wrapper_started_unix = time.time()

    copilot_cli = find_program(args.copilot_cli)
    append_event(events, "copilot_cli_resolved", path=copilot_cli)

    GitHubCopilotAgent = import_maf_runtime()
    from copilot.session import PermissionHandler

    append_event(
        events,
        "runtime_imported",
        agent_class=f"{GitHubCopilotAgent.__module__}.{GitHubCopilotAgent.__name__}",
    )

    provider_config, provider_summary = resolve_provider_config(args.model, events)

    if args.check:
        print_check_success_with_provider(copilot_cli, provider_summary)
        return 0

    workspace = resolve_workspace(args)
    os.chdir(workspace)
    append_event(events, "workspace_ready", workspace=str(workspace), cwd=os.getcwd())

    prompt, prompt_source = resolve_prompt(args)
    append_event(events, "prompt_loaded", source=prompt_source, prompt_sha256=sha256_text(prompt))

    task_file = resolve_task_file(args)
    task_meta = load_task_metadata(task_file)
    append_event(
        events,
        "task_loaded",
        task_id=str(task_meta.get("task_id", "")).strip(),
        objective=str(task_meta.get("objective", "")).strip(),
    )
    ensure_supported_task(task_meta, args.allow_unsupported_task, events)
    copilot_timeout = resolve_copilot_timeout(args.copilot_timeout, task_meta, events)

    apply_runtime_environment(
        args.model,
        copilot_timeout,
        copilot_cli,
        args.session_home,
        args.log_level,
    )
    apply_provider_environment(provider_config, args.model)

    discovered_session_id = ""
    discovered_session_source = ""

    def remember_session_id(source: str, value: Any) -> None:
        nonlocal discovered_session_id, discovered_session_source
        text = str(value).strip()
        if not text or discovered_session_id:
            return
        discovered_session_id = text
        discovered_session_source = source
        append_event(events, "session_discovered", source=source, session_id=text)

    def permission_handler(request: Any, invocation: dict[str, str]) -> Any:
        remember_session_id(
            "permission_request.invocation.session_id",
            invocation.get("session_id", ""),
        )
        append_event(
            events,
            "permission_requested",
            request=to_jsonable(request),
            invocation=to_jsonable(invocation),
        )
        return PermissionHandler.approve_all(request, invocation)

    def pre_tool_use_handler(hook_input: Any, context: dict[str, str]) -> dict[str, str]:
        if isinstance(hook_input, dict):
            remember_session_id("pre_tool_use.input.sessionId", hook_input.get("sessionId", ""))
        remember_session_id("pre_tool_use.context.session_id", context.get("session_id", ""))
        append_event(
            events,
            "pre_tool_use",
            hook_input=to_jsonable(hook_input),
            context=to_jsonable(context),
        )
        return {
            "permissionDecision": "ask",
            "permissionDecisionReason": (
                "SyncFuzz routes MAF tool permissions through the wrapper approval handler."
            ),
        }

    instructions = default_instructions()
    kwargs = build_agent_kwargs(
        GitHubCopilotAgent,
        instructions,
        args.model,
        provider_config,
        copilot_timeout,
        copilot_cli,
        args.session_home,
        args.log_level,
        permission_handler,
        pre_tool_use_handler,
    )
    append_event(events, "agent_kwargs_prepared", kwargs=kwargs)

    started_at = now_rfc3339()
    agent, resolved_kwargs = instantiate_agent(
        GitHubCopilotAgent,
        instructions,
        args.model,
        provider_config,
        copilot_timeout,
        copilot_cli,
        args.session_home,
        args.log_level,
        permission_handler,
        pre_tool_use_handler,
    )
    append_event(
        events,
        "agent_created",
        agent_class=f"{agent.__class__.__module__}.{agent.__class__.__name__}",
        resolved_kwargs=resolved_kwargs,
    )

    response, invocation_style, invocation_error = await run_agent_once(agent, prompt)
    append_event(
        events,
        "agent_completed",
        invocation_style=invocation_style,
        invocation_error=invocation_error,
    )

    response_text = extract_text(response)
    session = extract_session_metadata(agent, response)
    if not session["discovered"] and discovered_session_id:
        session["discovered"] = True
        session["source"] = discovered_session_source
        session["service_session_id"] = discovered_session_id
    finished_at = now_rfc3339()

    write_json(
        workspace / args.session_artifact,
        {
            **session,
            "workspace": str(workspace),
            "task_id": str(task_meta.get("task_id", "")).strip(),
            "started_at": started_at,
            "finished_at": finished_at,
        },
    )
    append_event(events, "session_artifact_written", artifact=args.session_artifact)

    write_json(
        workspace / args.summary_artifact,
        {
            "schema_version": "syncfuzz.maf-run-summary.v1",
            "provider": "github-copilot",
            "provider_mode": provider_summary.get("mode", ""),
            "provider_configured": bool(provider_summary.get("configured")),
            "provider_type": provider_summary.get("type", ""),
            "provider_base_url": provider_summary.get("base_url", ""),
            "provider_auth_mode": provider_summary.get("auth_mode", ""),
            "provider_auth_source": provider_summary.get("auth_source", ""),
            "provider_model_id": provider_summary.get("model_id", ""),
            "target_id": "maf-github-copilot-shell",
            "task_id": str(task_meta.get("task_id", "")).strip(),
            "objective": str(task_meta.get("objective", "")).strip(),
            "workspace": str(workspace),
            "cwd": os.getcwd(),
            "python": sys.executable,
            "copilot_cli": copilot_cli,
            "model": args.model,
            "copilot_timeout_seconds": copilot_timeout,
            "session_home": args.session_home,
            "base_directory": args.session_home,
            "log_level": args.log_level,
            "prompt_source": prompt_source,
            "prompt_sha256": sha256_text(prompt),
            "prompt_chars": len(prompt),
            "supported_tasks": sorted(SUPPORTED_TASKS),
            "allow_unsupported_task": args.allow_unsupported_task,
            "permission_mode": "pre-tool-ask -> approve-once",
            "agent_class": f"{agent.__class__.__module__}.{agent.__class__.__name__}",
            "invocation_style": invocation_style,
            "response_type": type(response).__name__,
            "response_text_sha256": sha256_text(response_text),
            "response_excerpt": response_text[:500],
            **latest_copilot_log_snapshot(wrapper_started_unix),
            "session_discovered": session["discovered"],
            "service_session_id": session["service_session_id"],
            "task_file": task_file,
            "started_at": started_at,
            "finished_at": finished_at,
        },
    )
    append_event(events, "summary_artifact_written", artifact=args.summary_artifact)

    write_json(
        workspace / args.lifecycle_artifact,
        {
            "schema_version": "syncfuzz.maf-lifecycle.v1",
            "target_id": "maf-github-copilot-shell",
            "task_id": str(task_meta.get("task_id", "")).strip(),
            "events": events,
        },
    )

    if response_text:
        print(response_text)
    else:
        print("maf target completed")
    return 0


def main() -> int:
    args = parse_args()
    wrapper_started_unix = time.time()
    try:
        return asyncio.run(run_main(args))
    except SystemExit:
        raise
    except Exception as exc:  # pragma: no cover - runtime dependency
        workspace_value = args.workspace or os.environ.get("SYNCFUZZ_WORKSPACE", "")
        if workspace_value:
            workspace = Path(workspace_value)
            workspace.mkdir(parents=True, exist_ok=True)
            write_json(
                workspace / args.lifecycle_artifact,
                {
                    "schema_version": "syncfuzz.maf-lifecycle.v1",
                    "target_id": "maf-github-copilot-shell",
                    "events": [
                        {
                            "timestamp": now_rfc3339(),
                            "event": "wrapper_failed",
                            "details": {
                                "error": str(exc),
                                "type": type(exc).__name__,
                                "traceback": traceback.format_exc(),
                                **latest_copilot_log_snapshot(wrapper_started_unix),
                            },
                        }
                    ],
                },
            )
        print(f"maf target failed: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
