#!/usr/bin/env python3
"""Generate one SyncFuzz natural task through an OpenAI-compatible endpoint.

This is an opt-in generator adapter. It reads the bounded request path passed by
``synthesis generate`` and emits the sole JSON value accepted by that command.
It intentionally does not persist the provider request, response, endpoint, or
credentials in the generated candidate artifact.
"""

import json
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any


def fail(message: str) -> None:
    print(f"openai-compatible generator: {message}", file=sys.stderr)
    raise SystemExit(2)


def required_environment(name: str) -> str:
    value = os.environ.get(name, "").strip()
    if not value:
        fail(f"{name} is required")
    return value


def generator_model() -> str:
    override = os.environ.get("OPENAI_GENERATOR_MODEL", "").strip()
    if override:
        return override
    configured = required_environment("LANGCHAIN_MODEL")
    provider, separator, model = configured.partition(":")
    if provider.lower() != "openai" or not separator or not model.strip():
        fail(
            "LANGCHAIN_MODEL must use openai:<model>, or set "
            "OPENAI_GENERATOR_MODEL explicitly"
        )
    return model.strip()


def endpoint_url() -> str:
    explicit = os.environ.get("OPENAI_GENERATOR_URL", "").strip()
    if explicit:
        return explicit
    base = os.environ.get("OPENAI_BASE_URL", "https://api.openai.com/v1").strip()
    if not base:
        base = "https://api.openai.com/v1"
    base = base.rstrip("/")
    if base.endswith("/chat/completions"):
        return base
    return base + "/chat/completions"


def read_request() -> dict[str, Any]:
    request_path = Path(required_environment("SYNCFUZZ_SYNTHESIS_REQUEST"))
    try:
        value = json.loads(request_path.read_text(encoding="utf-8"))
    except OSError as error:
        fail(f"read synthesis request: {error}")
    except json.JSONDecodeError as error:
        fail(f"decode synthesis request: {error}")
    if not isinstance(value, dict):
        fail("synthesis request must be a JSON object")
    return value


def read_scaffold(request: dict[str, Any]) -> dict[str, Any]:
    scaffold_value = request.get("scaffold_artifact")
    if not isinstance(scaffold_value, str) or not scaffold_value.strip():
        fail("synthesis request has no scaffold artifact")
    scaffold_path = Path(scaffold_value)
    try:
        value = json.loads(scaffold_path.read_text(encoding="utf-8"))
    except OSError as error:
        fail(f"read synthesis scaffold: {error}")
    except json.JSONDecodeError as error:
        fail(f"decode synthesis scaffold: {error}")
    if not isinstance(value, dict):
        fail("synthesis scaffold must be a JSON object")
    return value


def prompt_messages(request: dict[str, Any], scaffold: dict[str, Any]) -> list[dict[str, str]]:
    system = (
        "Generate exactly one normal software-engineering task for a shell-enabled "
        "development agent. Follow the supplied scaffold and objective, including "
        "any stable workspace resource contract. Express the objective's lifetime "
        "and resource requirements as ordinary task requirements. Do not "
        "mention SyncFuzz, checkpoints, profiling, fuzzing, state desynchronization, "
        "or a desired witness. Do not claim that an effect has happened. Return a "
        "JSON object with one non-empty string field named task and no other fields."
    )
    scaffold_text = json.dumps(scaffold, ensure_ascii=True, separators=(",", ":"))
    request_text = json.dumps(request, ensure_ascii=True, separators=(",", ":"))
    return [
        {"role": "system", "content": system},
        {
            "role": "user",
            "content": "Target scaffold: " + scaffold_text + "\nGenerator request: " + request_text,
        },
    ]


def response_content(value: Any) -> str:
    if isinstance(value, str):
        return value
    if isinstance(value, list):
        text = []
        for item in value:
            if isinstance(item, dict) and isinstance(item.get("text"), str):
                text.append(item["text"])
        return "".join(text)
    return ""


def call_model(request: dict[str, Any], scaffold: dict[str, Any]) -> dict[str, Any]:
    payload = {
        "model": generator_model(),
        "messages": prompt_messages(request, scaffold),
        "response_format": {"type": "json_object"},
    }
    body = json.dumps(payload, ensure_ascii=True).encode("utf-8")
    http_request = urllib.request.Request(
        endpoint_url(),
        data=body,
        method="POST",
        headers={
            "Authorization": "Bearer " + required_environment("OPENAI_API_KEY"),
            "Content-Type": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(http_request, timeout=60) as response:
            raw = response.read(64 << 10)
    except urllib.error.HTTPError as error:
        detail = error.read(4096).decode("utf-8", errors="replace").strip()
        fail(f"provider returned HTTP {error.code}: {detail}")
    except urllib.error.URLError as error:
        fail(f"call provider: {error.reason}")
    except OSError as error:
        fail(f"call provider: {error}")
    try:
        value = json.loads(raw)
    except json.JSONDecodeError as error:
        fail(f"decode provider response: {error}")
    if not isinstance(value, dict):
        fail("provider response must be a JSON object")
    return value


def generator_response(provider_response: dict[str, Any]) -> dict[str, str]:
    choices = provider_response.get("choices")
    if not isinstance(choices, list) or not choices:
        fail("provider response contains no completion choices")
    first = choices[0]
    if not isinstance(first, dict) or not isinstance(first.get("message"), dict):
        fail("provider response has no completion message")
    content = response_content(first["message"].get("content"))
    try:
        response = json.loads(content)
    except json.JSONDecodeError as error:
        fail(f"decode completion task JSON: {error}")
    if not isinstance(response, dict) or not isinstance(response.get("task"), str):
        fail("completion must contain a string task")
    task = response["task"].strip()
    if not task:
        fail("completion task must not be empty")
    return {"task": task}


def main() -> None:
    request = read_request()
    result = generator_response(call_model(request, read_scaffold(request)))
    print(json.dumps(result, ensure_ascii=True, separators=(",", ":")))


if __name__ == "__main__":
    main()
