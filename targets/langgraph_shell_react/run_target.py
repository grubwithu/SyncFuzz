#!/usr/bin/env python3
"""Minimal official create_agent + ShellToolMiddleware target for SyncFuzz."""

from __future__ import annotations

import argparse
import json
import os
import sys
import uuid
from pathlib import Path
from typing import Any

SHELL_REQUIRED_TASKS = {
    "orphan-process",
    "orphan-process-long-delay",
    "persistent-shell-poisoning",
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
        "--checkpoint-index",
        type=int,
        default=int(os.environ.get("SYNCFUZZ_LANGGRAPH_CHECKPOINT_INDEX", "-1")),
        help="Optional replay/fork checkpoint index into state history."
        " 0 means most recent checkpoint.",
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
    return parser.parse_args()


def env_bool(name: str) -> bool:
    value = os.environ.get(name, "").strip().lower()
    return value in {"1", "true", "yes", "on"}


def import_langchain_runtime() -> tuple[Any, Any, Any, Any, Any, Any, Any]:
    try:
        from langchain.agents import create_agent
    except ImportError as exc:  # pragma: no cover - runtime dependency
        raise SystemExit(
            "langchain is not installed. Install the dependencies listed in "
            "targets/langgraph_shell_react/requirements.txt."
        ) from exc

    try:
        from langgraph.checkpoint.memory import InMemorySaver
    except ImportError:  # pragma: no cover - compatibility fallback
        try:
            from langgraph.checkpoint.memory import MemorySaver as InMemorySaver
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
    ShellToolMiddleware: Any, workspace: Path, execution_policy: Any
) -> Any:
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
    return instantiate_with_fallbacks(ShellToolMiddleware, candidates)


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
    fork_config = agent.update_state(
        state.config,
        values={"messages": [HumanMessage(content=fork_user_message)]},
    )
    return agent.invoke(None, config=fork_config)


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
    user_message: str = "",
) -> dict[str, Any]:
    state = resolve_history_state(history, index)
    return {
        "operation": operation,
        "requested": True,
        "checkpoint_index": index,
        "checkpoint_id": extract_checkpoint_id(state) if state is not None else "",
        "available_history": len(history),
        "user_message": user_message,
        "messages": summarize_messages(result_messages(result)),
    }


def summarize_messages(messages: list[Any]) -> list[dict[str, Any]]:
    summary: list[dict[str, Any]] = []
    for message in messages[-6:]:
        role = message_role(message)
        content = getattr(message, "content", "")
        if isinstance(content, list):
            text = json.dumps(content, ensure_ascii=False)
        else:
            text = str(content)
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
        HumanMessage,
        ShellToolMiddleware,
        HostExecutionPolicy,
        DockerExecutionPolicy,
        CodexSandboxExecutionPolicy,
    ) = import_langchain_runtime()

    execution_policy = build_execution_policy(
        args,
        HostExecutionPolicy,
        DockerExecutionPolicy,
        CodexSandboxExecutionPolicy,
    )
    middleware = [
        build_shell_middleware(ShellToolMiddleware, workspace, execution_policy),
    ]
    checkpointer = InMemorySaver()
    system_prompt = args.system_prompt or default_system_prompt(workspace, task_path)
    agent = create_agent(
        model=model,
        tools=[],
        middleware=middleware,
        checkpointer=checkpointer,
        system_prompt=system_prompt,
        name="syncfuzz-langgraph-shell-react",
    )

    result, config = invoke_agent(agent, prompt, thread_id)
    history = collect_state_history(agent, config)
    replay_result = None
    if args.replay:
        replay_result = maybe_replay(agent, history, args.checkpoint_index)
        if replay_result is None:
            raise SystemExit(
                f"replay requested but checkpoint index {args.checkpoint_index} is unavailable"
            )
    fork_result = maybe_fork(
        agent,
        HumanMessage,
        history,
        args.checkpoint_index,
        args.fork_user_message.strip(),
    )
    if args.fork_user_message.strip() and fork_result is None:
        raise SystemExit(
            f"fork requested but checkpoint index {args.checkpoint_index} is unavailable"
        )

    openai_endpoint = openai_base_url() if provider == "openai" else ""
    openai_endpoint_source = ""
    if provider == "openai":
        openai_endpoint_source = "OPENAI_BASE_URL" if openai_endpoint else "provider-default"

    messages = result_messages(result)
    replay_messages = result_messages(replay_result)
    fork_messages = result_messages(fork_result)
    tool_messages = tool_message_count(messages)
    ai_tool_calls = ai_tool_call_count(messages)
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
        "workspace": str(workspace),
        "task_file": task_path,
        "task_id": str(task_contract.get("task_id", "")),
        "prompt_file": args.prompt_file or os.environ.get("SYNCFUZZ_PROMPT_FILE", ""),
        "require_tool_use": require_tool_use,
        "tool_use_observed": tool_messages > 0,
        "tool_message_count": tool_messages,
        "ai_tool_call_count": ai_tool_calls,
        "validation_error": validation_error,
        "checkpoint_index": args.checkpoint_index,
        "replay_requested": args.replay,
        "fork_requested": bool(args.fork_user_message.strip()),
        "history_count": len(history),
        "checkpoint_ids": [
            item["checkpoint_id"] for item in summarize_history(history) if item["checkpoint_id"]
        ],
        "result": summarize_messages(messages),
        "replay_result": summarize_messages(replay_messages),
        "fork_result": summarize_messages(fork_messages),
    }
    history_summary = summarize_history(history)
    write_json(workspace / args.history_artifact, history_summary)
    write_json(workspace / args.summary_artifact, summary)
    if args.replay:
        write_json(
            workspace / args.replay_artifact,
            summarize_operation_result(
                "replay", history, args.checkpoint_index, replay_result
            ),
        )
    if args.fork_user_message.strip():
        write_json(
            workspace / args.fork_artifact,
            summarize_operation_result(
                "fork",
                history,
                args.checkpoint_index,
                fork_result,
                args.fork_user_message.strip(),
            ),
        )

    if validation_error:
        print(validation_error, file=sys.stderr)
        return 3

    final_messages = summary["result"]
    if final_messages:
        print(final_messages[-1]["content"])
    return 0


if __name__ == "__main__":
    sys.exit(main())
