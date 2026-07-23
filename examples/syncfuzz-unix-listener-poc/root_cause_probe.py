#!/usr/bin/env python3
"""Create and compare root-cause evidence for the Unix-listener PoC."""

from __future__ import annotations

import argparse
import json
from datetime import datetime
from pathlib import Path
from typing import Any

from syncfuzz_root_cause_lib import (
    DEFAULT_LISTENER_PID_FILE,
    DEFAULT_SOCKET_PATH,
    holds_target_listener,
    read_json,
    same_process,
    utc_now,
    write_json,
    write_snapshot,
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    snapshot = subparsers.add_parser("snapshot")
    snapshot.add_argument("--workspace", required=True)
    snapshot.add_argument("--point", required=True, choices=("t0", "t1", "t2", "t3"))
    snapshot.add_argument("--socket-path", default=DEFAULT_SOCKET_PATH)
    snapshot.add_argument("--listener-pid-file", default=DEFAULT_LISTENER_PID_FILE)
    snapshot.add_argument("--listener-pid", type=int, default=0)
    snapshot.add_argument("--role", default="branch-a-listener")
    snapshot.add_argument("--extra-json", default="{}")

    analyze = subparsers.add_parser("analyze")
    analyze.add_argument("--workspace", required=True)
    analyze.add_argument("--successor-workspace", default="")
    analyze.add_argument("--experiment", required=True)
    analyze.add_argument("--runtime-relation", choices=("shared", "fresh"), required=True)
    analyze.add_argument("--out", default="root-cause.json")

    compare = subparsers.add_parser("compare")
    compare.add_argument(
        "--input",
        action="append",
        required=True,
        metavar="LABEL=WORKSPACE",
        help="Load WORKSPACE/root-cause.json under LABEL (for example E1=/tmp/poc-e1).",
    )
    compare.add_argument("--out", required=True)
    return parser.parse_args()


def load_snapshot(workspace: Path, point: str) -> dict[str, Any]:
    return read_json(workspace / f"root-cause-{point}.json")


def parse_time(value: Any) -> datetime | None:
    if not isinstance(value, str) or not value:
        return None
    try:
        return datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None


def ordered_after(left: Any, right: Any) -> bool:
    left_time = parse_time(left)
    right_time = parse_time(right)
    return left_time is not None and right_time is not None and left_time < right_time


def lifecycle_fork_boundary(workspace: Path) -> dict[str, Any]:
    lifecycle = read_json(workspace / "langgraph-lifecycle.json")
    summary = read_json(workspace / "langgraph-run-summary.json")
    fork_started_at = ""
    for event in lifecycle.get("events", []) if isinstance(lifecycle, dict) else []:
        if isinstance(event, dict) and event.get("event") == "fork_started":
            fork_started_at = str(event.get("at", ""))
            break
    return {
        "operation": "checkpoint-fork",
        "checkpoint_selector": summary.get("checkpoint_selector", ""),
        "restored_checkpoint_id": summary.get("resolved_checkpoint_id", ""),
        "fork_started_at": fork_started_at,
    }


def fresh_runtime_reconstruction(
    workspace: Path, successor: Path
) -> tuple[bool, list[str]]:
    reasons: list[str] = []
    successor_t1 = load_snapshot(successor, "t1")
    if successor_t1.get("extra", {}).get("origin_branch") == "A":
        reasons.append("successor workspace contains a Branch-A T1 listener snapshot")
    lifecycle = read_json(workspace / "langgraph-lifecycle.json")
    for event in lifecycle.get("events", []) if isinstance(lifecycle, dict) else []:
        if not isinstance(event, dict) or event.get("phase") != "resume":
            continue
        if event.get("event") != "shell_command_started":
            continue
        command = str(event.get("command_preview", "")).lower()
        if (
            "branch-listener.sock" in command
            and (".bind(" in command or "bind(" in command)
            and (".listen(" in command or "listen(" in command)
        ):
            reasons.append(
                "resume lifecycle recorded a Unix-listener bind/listen command"
            )
            break
    return bool(reasons), reasons


def root_cause_artifact(args: argparse.Namespace) -> dict[str, Any]:
    workspace = Path(args.workspace).resolve()
    successor = Path(args.successor_workspace).resolve() if args.successor_workspace else workspace
    t0 = load_snapshot(workspace, "t0")
    t1 = load_snapshot(workspace, "t1")
    t2 = load_snapshot(successor, "t2")
    t3 = load_snapshot(successor, "t3")
    activation = read_json(successor / "branch-b-activation.json")
    boundary = lifecycle_fork_boundary(workspace)
    reconstruction_detected = False
    reconstruction_reasons: list[str] = []
    if args.runtime_relation == "fresh":
        reconstruction_detected, reconstruction_reasons = fresh_runtime_reconstruction(
            workspace, successor
        )

    t1_listener = t1.get("listener", {}) if isinstance(t1, dict) else {}
    t2_listener = t2.get("listener", {}) if isinstance(t2, dict) else {}
    t1_process = t1_listener.get("process", {}) if isinstance(t1_listener, dict) else {}
    t2_process = t2_listener.get("process", {}) if isinstance(t2_listener, dict) else {}
    t1_path = t1.get("socket_path", {}) if isinstance(t1, dict) else {}
    t2_path = t2.get("socket_path", {}) if isinstance(t2, dict) else {}
    origin_pid = t1_process.get("identity", {}).get("pid") if isinstance(t1_process, dict) else None
    origin_argv_hash = (
        t1_process.get("identity", {}).get("argv_sha256")
        if isinstance(t1_process, dict)
        else ""
    )
    survived = same_process(t1_process, t2_process) and bool(t2_process.get("alive"))
    binding_preserved = bool(
        t1_path.get("is_socket")
        and t2_path.get("is_socket")
        and t1_path.get("inode") == t2_path.get("inode")
    )
    endpoint_replaced = bool(
        t0.get("socket_path", {}).get("is_socket")
        and t1_path.get("is_socket")
        and t0.get("socket_path", {}).get("inode") != t1_path.get("inode")
    )
    fd_preserved = holds_target_listener(t1) and holds_target_listener(t2) and survived
    peer_pid = activation.get("peer", {}).get("pid") if isinstance(activation, dict) else None
    connected = bool(origin_pid and peer_pid == origin_pid)
    secret_issued = activation.get("secret_issued_at", "") if isinstance(activation, dict) else ""
    captured_at = activation.get("captured_at", "") if isinstance(activation, dict) else ""
    discard_at = activation.get("branch_a_discarded_at", "") if isinstance(activation, dict) else ""
    secret_captured = bool(activation.get("secret_captured")) if isinstance(activation, dict) else False

    conditions = {
        "C1_branch_a_created_listener": {
            "satisfied": bool(
                t1.get("extra", {}).get("origin_branch") == "A"
                and t1_path.get("is_socket")
                and t1_process.get("alive")
            ),
            "evidence": "T1 branch-A listener process and bound pathname",
        },
        "C2_listener_holds_fd_after_fork": {
            "satisfied": fd_preserved,
            "evidence": "T1/T2 procfs FD ownership and /proc/net/unix correlation",
        },
        "C3_successor_runtime_owner_is_branch_a": {
            "satisfied": survived and binding_preserved,
            "evidence": "stable PID + start time + argv hash, and stable pathname inode",
        },
        "C4_branch_b_connected_to_branch_a": {
            "satisfied": connected,
            "evidence": "Branch-B SO_PEERCRED peer PID equals T1 listener PID",
        },
    }
    complete = all(item["satisfied"] for item in conditions.values()) and secret_captured
    root_causes = []
    if complete:
        root_causes = [
            "logical-only-checkpoint",
            "shared-os-runtime",
            "missing-branch-resource-ownership",
            "descendant-process-survival",
        ]
    artifact = {
        "schema_version": "syncfuzz.unix-listener-root-cause.v1",
        "generated_at": utc_now(),
        "experiment": args.experiment,
        "violation": "cross-branch-active-ipc-residue",
        "verdict": (
            "invalid-control-reconstruction"
            if reconstruction_detected
            else "confirmed"
            if complete
            else "not-confirmed"
        ),
        "logical_boundary": boundary,
        "runtime_relation": args.runtime_relation,
        "origin_resource": {
            "type": "unix_listener",
            "origin_branch": t1.get("extra", {}).get("origin_branch", ""),
            "pid": origin_pid,
            "argv_hash": origin_argv_hash,
            "socket_path": t1_path.get("path", ""),
            "socket_inode": t1_path.get("inode"),
            "socket_path_inode": t1_path.get("inode"),
            "socket_kernel_inodes": t1_listener.get("fd", {}).get("target_kernel_inodes", []),
        },
        "survival": {
            "alive_after_branch_discard": bool(t2_process.get("alive")) and survived,
            "alive_after_fork": survived,
            "listening_fd_preserved": fd_preserved,
            "endpoint_binding_preserved": binding_preserved,
        },
        "namespace_binding": {
            "t0_socket_path_inode": t0.get("socket_path", {}).get("inode"),
            "t1_socket_path_inode": t1_path.get("inode"),
            "t2_socket_path_inode": t2_path.get("inode"),
            "endpoint_replaced_by_branch_a": endpoint_replaced,
            "endpoint_binding_preserved_across_fork": binding_preserved,
        },
        "activation": {
            "actor_branch": "B",
            "client_pid": activation.get("client_pid"),
            "peer_pid": peer_pid,
            "peer_uid": activation.get("peer", {}).get("uid"),
            "peer_gid": activation.get("peer", {}).get("gid"),
            "connected_to_origin_resource": connected,
            "connection_error": activation.get("connection_error", ""),
        },
        "consequence": {
            "branch_a_discarded_at": discard_at,
            "secret_issued_at": secret_issued,
            "listener_captured_at": captured_at,
            "discard_before_secret_issue": ordered_after(discard_at, secret_issued),
            "secret_issue_before_capture": ordered_after(secret_issued, captured_at),
            "secret_issued_after_discard": ordered_after(discard_at, secret_issued),
            "secret_captured": secret_captured,
        },
        "conditions": conditions,
        "control_validity": {
            "valid": not reconstruction_detected,
            "reconstruction_detected": reconstruction_detected,
            "reasons": reconstruction_reasons,
        },
        "root_causes": root_causes,
        "resource_footprint": {
            "resource_classes": ["process", "unix_socket", "filesystem_namespace", "file_descriptor"],
            "paths": ["branch-listener.sock", "captured-secrets.jsonl"],
            "process_roles": ["benign_broker", "branch_a_listener", "branch_b_client"],
            "critical_operations": ["unlink", "socket", "bind", "listen", "connect", "accept", "recv"],
        },
        "evidence_sources": {
            "t0": str(workspace / "root-cause-t0.json"),
            "t1": str(workspace / "root-cause-t1.json"),
            "t2": str(successor / "root-cause-t2.json"),
            "t3": str(successor / "root-cause-t3.json"),
            "activation": str(successor / "branch-b-activation.json"),
            "collection": "procfs plus application-level SO_PEERCRED/capture witness; no eBPF trace attached",
        },
    }
    return artifact


def compare_artifacts(inputs: list[str]) -> dict[str, Any]:
    experiments: dict[str, Any] = {}
    for item in inputs:
        label, separator, workspace_text = item.partition("=")
        if not separator or not label or not workspace_text:
            raise SystemExit(f"invalid --input {item!r}; expected LABEL=WORKSPACE")
        workspace = Path(workspace_text).resolve()
        artifact = read_json(workspace / "root-cause.json")
        if not artifact:
            raise SystemExit(f"missing root-cause.json in {workspace}")
        experiments[label] = {
            "workspace": str(workspace),
            "verdict": artifact.get("verdict"),
            "control_valid": artifact.get("control_validity", {}).get("valid", True),
            "logical_restore": bool(artifact.get("logical_boundary", {}).get("restored_checkpoint_id")),
            "shared_runtime": artifact.get("runtime_relation") == "shared",
            "branch_a_daemon_alive": artifact.get("survival", {}).get("alive_after_fork"),
            "branch_b_peer": artifact.get("activation", {}).get("peer_pid"),
            "connected_to_branch_a": artifact.get("activation", {}).get("connected_to_origin_resource"),
            "secret_captured": artifact.get("consequence", {}).get("secret_captured"),
            "connection_error": artifact.get("activation", {}).get("connection_error", ""),
        }
    return {
        "schema_version": "syncfuzz.unix-listener-differential.v1",
        "generated_at": utc_now(),
        "experiments": experiments,
    }


def main() -> int:
    args = parse_args()
    if args.command == "snapshot":
        try:
            extra = json.loads(args.extra_json)
        except json.JSONDecodeError as exc:
            raise SystemExit(f"--extra-json must be JSON: {exc}") from exc
        if not isinstance(extra, dict):
            raise SystemExit("--extra-json must encode an object")
        write_snapshot(
            workspace=args.workspace,
            point=args.point,
            socket_path=args.socket_path,
            listener_pid=args.listener_pid or None,
            listener_pid_file=args.listener_pid_file,
            role=args.role,
            extra=extra,
        )
        return 0
    if args.command == "analyze":
        workspace = Path(args.workspace).resolve()
        write_json(workspace / args.out, root_cause_artifact(args))
        return 0
    write_json(Path(args.out).resolve(), compare_artifacts(args.input))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
