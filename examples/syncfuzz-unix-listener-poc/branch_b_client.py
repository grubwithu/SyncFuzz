#!/usr/bin/env python3
"""Trusted Branch-B client with a peer-identity and timing witness."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import secrets
import signal
import socket
import struct
import time
from pathlib import Path
from typing import Any

from syncfuzz_root_cause_lib import (
    process_snapshot,
    read_pid_file,
    utc_now,
    write_json,
    write_snapshot,
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--mode", choices=("original", "cleanup", "fresh-runtime", "benign"), required=True)
    parser.add_argument("--socket-path", default="branch-listener.sock")
    parser.add_argument("--listener-pid-file", default="branch-listener-pid.txt")
    return parser.parse_args()


def listener_alive(pid: int | None) -> bool:
    if pid is None:
        return False
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return bool(process_snapshot(pid).get("alive"))


def terminate_listener(pid: int | None) -> dict[str, Any]:
    result: dict[str, Any] = {"requested_at": utc_now(), "pid": pid, "sent": False}
    if pid is None:
        result["reason"] = "listener PID file was absent"
        return result
    try:
        os.kill(pid, signal.SIGTERM)
        result["sent"] = True
    except ProcessLookupError:
        result["reason"] = "listener already exited"
        return result
    for _ in range(40):
        if not listener_alive(pid):
            break
        time.sleep(0.05)
    result["alive_after_cleanup"] = listener_alive(pid)
    return result


def captured_secret(secret: str) -> tuple[bool, str]:
    captured_path = Path("captured-secrets.jsonl")
    if not captured_path.exists():
        return False, ""
    try:
        lines = captured_path.read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError:
        return False, ""
    for line in reversed(lines):
        try:
            record = json.loads(line)
        except json.JSONDecodeError:
            continue
        if secret in str(record.get("payload", "")):
            return True, str(record.get("captured_at", ""))
    return False, ""


def main() -> int:
    args = parse_args()
    workspace = Path.cwd()
    listener_pid = read_pid_file(workspace / args.listener_pid_file)
    discard_at = utc_now()
    write_snapshot(
        workspace=workspace,
        point="t2",
        socket_path=args.socket_path,
        listener_pid=listener_pid,
        listener_pid_file=args.listener_pid_file,
        role="branch-a-listener",
        extra={
            "observation_phase": "branch-b-after-checkpoint-fork-before-activation",
            "branch_b_client_pid": os.getpid(),
            "branch_a_discarded_at": discard_at,
        },
    )

    cleanup = terminate_listener(listener_pid) if args.mode == "cleanup" else {}
    secret = "sk-syncfuzz-poc-" + secrets.token_urlsafe(18)
    secret_issued_at = utc_now()
    Path("branch-b-api-key.txt").write_text(secret + "\n", encoding="utf-8")
    payload = json.dumps(
        {
            "api_key": secret,
            "branch": "B",
            "created_after_branch_a_discard": True,
            "event": "normal_tool_sends_to_fixed_socket",
        },
        sort_keys=True,
    ).encode("utf-8")

    peer: dict[str, int] = {}
    response = ""
    connection_error = ""
    client: socket.socket | None = None
    try:
        client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        client.settimeout(3)
        client.connect(args.socket_path)
        raw_peer = client.getsockopt(
            socket.SOL_SOCKET, socket.SO_PEERCRED, struct.calcsize("3i")
        )
        peer_pid, peer_uid, peer_gid = struct.unpack("3i", raw_peer)
        peer = {"pid": peer_pid, "uid": peer_uid, "gid": peer_gid}
        client.sendall(payload)
        client.shutdown(socket.SHUT_WR)
        response = client.recv(1024).decode("utf-8", "replace").strip()
    except OSError as exc:
        connection_error = f"{type(exc).__name__}: {exc}"
    finally:
        if client is not None:
            client.close()
    time.sleep(0.35)
    secret_captured, captured_at = captured_secret(secret)
    activation = {
        "schema_version": "syncfuzz.unix-listener-activation.v1",
        "mode": args.mode,
        "client_pid": os.getpid(),
        "branch_a_discarded_at": discard_at,
        "secret_issued_at": secret_issued_at,
        "secret_sha256": hashlib.sha256(secret.encode("utf-8")).hexdigest(),
        "peer": peer,
        "response": response,
        "connection_error": connection_error,
        "cleanup": cleanup,
        "secret_captured": secret_captured,
        "captured_at": captured_at,
    }
    write_json(workspace / "branch-b-activation.json", activation)
    write_snapshot(
        workspace=workspace,
        point="t3",
        socket_path=args.socket_path,
        listener_pid=listener_pid,
        listener_pid_file=args.listener_pid_file,
        role="branch-a-listener",
        extra={
            "observation_phase": "branch-b-after-trusted-client-activation",
            "branch_b_client_pid": os.getpid(),
            "peer": peer,
            "secret_captured": secret_captured,
            "connection_error": connection_error,
        },
    )
    verdict = "PRESENT_BRANCH_LISTENER_SECRET_LEAKAGE" if secret_captured else "NO_BRANCH_LISTENER_SECRET_LEAKAGE"
    proof = "\n".join(
        (
            verdict,
            f"mode={args.mode}",
            f"response={response}",
            f"connection_error={connection_error}",
            f"peer_pid={peer.get('pid', '')}",
            "secret_file=branch-b-api-key.txt",
            "captured_file=captured-secrets.jsonl",
            f"captured_contains_branch_b_secret={str(secret_captured).lower()}",
        )
    ) + "\n"
    Path("branch-b-secret-leak-check.txt").write_text(proof, encoding="utf-8")
    Path("proof-send.txt").write_text(proof, encoding="utf-8")
    print(proof, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
