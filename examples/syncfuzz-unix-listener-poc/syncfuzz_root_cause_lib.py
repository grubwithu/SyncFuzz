#!/usr/bin/env python3
"""Small, dependency-free Linux probes used by the Unix-listener PoC.

The probes deliberately collect identifiers rather than payloads.  They are
not an eBPF replacement: they provide the minimal process, pathname, and file
descriptor evidence required to explain this one motivation example.
"""

from __future__ import annotations

import hashlib
import json
import os
import stat
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


DEFAULT_SOCKET_PATH = "branch-listener.sock"
DEFAULT_LISTENER_PID_FILE = "branch-listener-pid.txt"


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="microseconds").replace(
        "+00:00", "Z"
    )


def write_json(path: Path, value: Any) -> None:
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def read_json(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return {}
    return value if isinstance(value, dict) else {}


def sha256_text(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8", "surrogateescape")).hexdigest()


def read_pid_file(path: Path) -> int | None:
    try:
        value = path.read_text(encoding="utf-8").strip()
        pid = int(value)
    except (OSError, ValueError):
        return None
    return pid if pid > 0 else None


def _read_proc_stat(pid: int) -> dict[str, Any]:
    path = Path("/proc") / str(pid) / "stat"
    try:
        raw = path.read_text(encoding="utf-8")
        close = raw.rfind(")")
        if close < 0:
            raise ValueError("missing comm terminator")
        fields = raw[close + 2 :].split()
        # fields[0] is process state (Linux stat field 3); starttime is field 22.
        return {
            "state": fields[0],
            "ppid": int(fields[1]),
            "pgrp": int(fields[2]),
            "session": int(fields[3]),
            "start_time_ticks": int(fields[19]),
        }
    except (OSError, ValueError, IndexError):
        return {}


def _read_cmdline(pid: int) -> list[str]:
    try:
        raw = (Path("/proc") / str(pid) / "cmdline").read_bytes()
    except OSError:
        return []
    return [part.decode("utf-8", "replace") for part in raw.split(b"\0") if part]


def _read_cgroup(pid: int) -> list[str]:
    try:
        return (Path("/proc") / str(pid) / "cgroup").read_text(
            encoding="utf-8", errors="replace"
        ).splitlines()
    except OSError:
        return []


def _read_exe(pid: int) -> str:
    try:
        return os.readlink(Path("/proc") / str(pid) / "exe")
    except OSError:
        return ""


def process_snapshot(pid: int | None, *, lineage_depth: int = 8) -> dict[str, Any]:
    if pid is None:
        return {"observed": False, "alive": False}
    stat_fields = _read_proc_stat(pid)
    if not stat_fields:
        return {"observed": False, "pid": pid, "alive": False}
    argv = _read_cmdline(pid)
    identity = {
        "pid": pid,
        "start_time_ticks": stat_fields["start_time_ticks"],
        "argv_sha256": sha256_text("\0".join(argv)),
    }
    result: dict[str, Any] = {
        "observed": True,
        "alive": stat_fields["state"] != "Z",
        "identity": identity,
        "state": stat_fields["state"],
        "ppid": stat_fields["ppid"],
        "pgrp": stat_fields["pgrp"],
        "session": stat_fields["session"],
        "executable": _read_exe(pid),
        "argv": argv,
        "cgroup": _read_cgroup(pid),
    }
    lineage: list[dict[str, Any]] = []
    current = result
    for _ in range(lineage_depth):
        parent = current.get("ppid")
        if not isinstance(parent, int) or parent <= 1:
            break
        parent_stat = _read_proc_stat(parent)
        if not parent_stat:
            break
        parent_argv = _read_cmdline(parent)
        lineage.append(
            {
                "pid": parent,
                "ppid": parent_stat["ppid"],
                "start_time_ticks": parent_stat["start_time_ticks"],
                "argv_sha256": sha256_text("\0".join(parent_argv)),
            }
        )
        current = {"ppid": parent_stat["ppid"]}
    result["parent_lineage"] = lineage
    return result


def socket_path_snapshot(workspace: Path, socket_path: str) -> dict[str, Any]:
    path = Path(socket_path)
    if not path.is_absolute():
        path = workspace / path
    try:
        details = path.lstat()
    except OSError as exc:
        return {
            "observed": False,
            "path": str(path),
            "exists": False,
            "error": exc.strerror or type(exc).__name__,
        }
    return {
        "observed": True,
        "path": str(path),
        "exists": True,
        "is_socket": stat.S_ISSOCK(details.st_mode),
        "inode": details.st_ino,
        "mode": format(stat.S_IMODE(details.st_mode), "04o"),
        "uid": details.st_uid,
        "gid": details.st_gid,
        # A successful application client connection is recorded separately.
        "connectability": "not-probed-by-procfs-snapshot",
    }


def unix_socket_entries(workspace: Path, socket_path: str) -> list[dict[str, Any]]:
    candidates = {socket_path, str(workspace / socket_path)}
    try:
        lines = Path("/proc/net/unix").read_text(
            encoding="utf-8", errors="replace"
        ).splitlines()[1:]
    except OSError:
        return []
    entries: list[dict[str, Any]] = []
    for line in lines:
        fields = line.split(maxsplit=7)
        if len(fields) < 7:
            continue
        path = fields[7] if len(fields) > 7 else ""
        if path not in candidates:
            continue
        try:
            inode = int(fields[6])
        except ValueError:
            continue
        entries.append(
            {
                "kernel_inode": inode,
                "flags": fields[3],
                "socket_type": fields[4],
                "state": fields[5],
                "path": path,
            }
        )
    return entries


def listener_fd_snapshot(pid: int | None, target_entries: list[dict[str, Any]]) -> dict[str, Any]:
    if pid is None:
        return {"observed": False, "target_fd_held": False, "fds": []}
    fd_dir = Path("/proc") / str(pid) / "fd"
    fdinfo_dir = Path("/proc") / str(pid) / "fdinfo"
    fds: list[dict[str, Any]] = []
    try:
        paths = sorted(fd_dir.iterdir(), key=lambda item: int(item.name))
    except OSError:
        return {"observed": False, "pid": pid, "target_fd_held": False, "fds": []}
    target_kernel_inodes = {
        int(entry["kernel_inode"])
        for entry in target_entries
        if isinstance(entry.get("kernel_inode"), int)
    }
    target_fds: list[dict[str, Any]] = []
    for fd_path in paths:
        try:
            target = os.readlink(fd_path)
        except OSError:
            continue
        info: dict[str, str] = {}
        try:
            for line in (fdinfo_dir / fd_path.name).read_text(
                encoding="utf-8", errors="replace"
            ).splitlines():
                key, sep, value = line.partition(":")
                if sep:
                    info[key.strip()] = value.strip()
        except OSError:
            pass
        item: dict[str, Any] = {"fd": int(fd_path.name), "target": target, "fdinfo": info}
        if target.startswith("socket:[") and target.endswith("]"):
            try:
                item["kernel_inode"] = int(target[8:-1])
            except ValueError:
                pass
        fds.append(item)
        if item.get("kernel_inode") in target_kernel_inodes:
            target_fds.append(item)
    return {
        "observed": True,
        "pid": pid,
        "target_kernel_inodes": sorted(target_kernel_inodes),
        "target_fd_held": bool(target_fds),
        "target_fds": target_fds,
        "fds": fds,
    }


def write_snapshot(
    *,
    workspace: Path | str,
    point: str,
    socket_path: str = DEFAULT_SOCKET_PATH,
    listener_pid: int | None = None,
    listener_pid_file: str = DEFAULT_LISTENER_PID_FILE,
    role: str = "branch-a-listener",
    extra: dict[str, Any] | None = None,
) -> dict[str, Any]:
    root = Path(workspace).resolve()
    if listener_pid is None:
        listener_pid = read_pid_file(root / listener_pid_file)
    path = socket_path_snapshot(root, socket_path)
    entries = unix_socket_entries(root, socket_path)
    process = process_snapshot(listener_pid)
    fd = listener_fd_snapshot(listener_pid, entries)
    snapshot = {
        "schema_version": "syncfuzz.unix-listener-procfs-snapshot.v1",
        "point": point,
        "captured_at": utc_now(),
        "workspace": str(root),
        "role": role,
        "socket_path": path,
        "unix_socket": {"entries": entries},
        "listener": {
            "pid_file": listener_pid_file,
            "pid": listener_pid,
            "process": process,
            "fd": fd,
        },
        "extra": extra or {},
        "collection": {
            "method": "procfs-and-application-witness",
            "ebpf": "not-configured",
        },
    }
    write_json(root / f"root-cause-{point}.json", snapshot)
    return snapshot


def same_process(left: dict[str, Any], right: dict[str, Any]) -> bool:
    left_identity = left.get("identity", {}) if isinstance(left, dict) else {}
    right_identity = right.get("identity", {}) if isinstance(right, dict) else {}
    fields = ("pid", "start_time_ticks", "argv_sha256")
    return bool(left_identity) and all(
        left_identity.get(field) is not None
        and left_identity.get(field) == right_identity.get(field)
        for field in fields
    )


def holds_target_listener(snapshot: dict[str, Any]) -> bool:
    listener = snapshot.get("listener", {})
    fd = listener.get("fd", {}) if isinstance(listener, dict) else {}
    process = listener.get("process", {}) if isinstance(listener, dict) else {}
    entries = snapshot.get("unix_socket", {}).get("entries", [])
    return bool(
        isinstance(process, dict)
        and process.get("alive")
        and isinstance(fd, dict)
        and fd.get("target_fd_held")
        and entries
    )
