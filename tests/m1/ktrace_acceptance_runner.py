"""Privileged Docker/cgroup-v2 acceptance runner for the M1 collector."""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
from typing import Any


REPOSITORY_ROOT = Path(__file__).resolve().parents[2]
WORKLOAD_SOURCE = REPOSITORY_ROOT / "tests" / "m1" / "ktrace_acceptance_workload.c"
MARKER_SOURCE = REPOSITORY_ROOT / "tests" / "m1" / "ktrace_acceptance_marker.c"
K_EVENT_FIELDS = frozenset(
    {
        "seq",
        "ts_mono_ns",
        "tgid",
        "tid",
        "starttime",
        "ppid",
        "syscall",
        "site",
        "args_raw",
        "ret",
        "errno",
        "dev",
        "ino",
        "content_hash",
        "cgroup_id",
    }
)
MANIFEST_FIELDS = frozenset(
    {
        "run_id",
        "schema_version",
        "started_wall_ns",
        "clock_name",
        "kernel_release",
        "image_digest",
        "langgraph_version",
        "milestone",
        "dropped_events",
        "orphan_rate",
        "memo_hit_rate",
        "prune",
    }
)
REQUIRED_SYSCALLS = frozenset(
    {
        "openat",
        "openat2",
        "mkdirat",
        "renameat2",
        "linkat",
        "symlinkat",
        "readlinkat",
        "newfstatat",
        "fchmodat",
        "fsetxattr",
        "mount",
        "write",
        "socket",
        "bind",
        "listen",
        "connect",
        "execve",
        "execveat",
        "unlinkat",
        "sched_process_fork",
        "sched_process_exec",
        "sched_process_exit",
        "sf_mark",
    }
)


def _fail(message: str) -> None:
    """Overview: terminate the acceptance run with an explicit contract failure instead of a skipped result."""
    raise RuntimeError(message)


def _run(command: list[str], cwd: Path | None = None) -> subprocess.CompletedProcess[str]:
    """Overview: execute one required external command and include its output when the host prerequisite fails."""
    completed = subprocess.run(command, cwd=cwd, capture_output=True, text=True, check=False)
    if completed.returncode != 0:
        _fail(
            f"command failed ({completed.returncode}): {' '.join(command)}\n"
            f"stdout:\n{completed.stdout}\nstderr:\n{completed.stderr}"
        )
    return completed


def _compile_workload(root: Path) -> tuple[Path, Path]:
    """Overview: compile a marker library and its fixed syscall workload into the host directory bind-mounted by Docker."""
    marker_library = root / "libsyncfuzz_marker.so"
    workload = root / "ktrace-acceptance-workload"

    _run(["clang", "-shared", "-fPIC", str(MARKER_SOURCE), "-o", str(marker_library)])
    _run(
        [
            "clang",
            "-std=c11",
            "-Wall",
            "-Wextra",
            "-Werror",
            str(WORKLOAD_SOURCE),
            "-L",
            str(root),
            "-lsyncfuzz_marker",
            "-Wl,-rpath,/work",
            "-o",
            str(workload),
        ]
    )
    return marker_library, workload


def _cgroup_id_for_container(container_name: str) -> int:
    """Overview: resolve the target container's cgroup-v2 inode identity from its init process without PID reuse heuristics."""
    process_id = _run(["docker", "inspect", "--format", "{{.State.Pid}}", container_name]).stdout.strip()
    if not process_id.isdecimal() or int(process_id) <= 0:
        _fail(f"docker returned invalid container pid: {process_id!r}")
    cgroup_file = Path("/proc") / process_id / "cgroup"
    for line in cgroup_file.read_text(encoding="utf-8").splitlines():
        if line.startswith("0::"):
            relative_path = line.removeprefix("0::").lstrip("/")
            return (Path("/sys/fs/cgroup") / relative_path).stat().st_ino
    _fail(f"container {container_name} has no unified cgroup-v2 path")


def _start_container(root: Path, container_name: str) -> int:
    """Overview: start an isolated Docker workload container and return its cgroup-v2 identity for M1 filtering."""
    _run(
        [
            "docker",
            "run",
            "--rm",
            "-d",
            "--name",
            container_name,
            "--security-opt",
            "seccomp=unconfined",
            "-v",
            f"{root}:/work",
            "ubuntu:22.04",
            "sh",
            "-c",
            "sleep 30",
        ]
    )
    return _cgroup_id_for_container(container_name)


def _write_manifest(path: Path) -> None:
    """Overview: write the complete frozen RunManifest shape required before M1 may update dropped_events."""
    manifest = {
        "run_id": "m1-acceptance",
        "schema_version": "1.0.0",
        "started_wall_ns": time.time_ns(),
        "clock_name": "CLOCK_MONOTONIC",
        "kernel_release": os.uname().release,
        "image_digest": None,
        "langgraph_version": None,
        "milestone": "P0",
        "dropped_events": 0,
        "orphan_rate": None,
        "memo_hit_rate": None,
        "prune": None,
    }
    path.write_text(json.dumps(manifest, separators=(",", ":")), encoding="utf-8")


def _collect_artifacts(
    collector: Path, marker_library: Path, root: Path, cgroup_id: int, container_name: str
) -> tuple[Path, Path]:
    """Overview: run M1 against one cgroup-scoped workload and retain artifacts only after the collector exits successfully."""
    run_directory = root / "run"
    run_directory.mkdir()
    events_path = run_directory / "kevents.jsonl"
    manifest_path = run_directory / "manifest.json"
    _write_manifest(manifest_path)
    command = [
        "sudo",
        "-n",
        str(collector),
        "--cgroup-id",
        str(cgroup_id),
        "--out",
        str(events_path),
        "--duration",
        "8",
        "--watch-path",
        str(root / "watch-target"),
        "--marker-so",
        str(marker_library),
        "--manifest",
        str(manifest_path),
    ]
    process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        time.sleep(1)
        _run(["docker", "exec", container_name, "/work/ktrace-acceptance-workload", "/work"])
    finally:
        stdout, stderr = process.communicate(timeout=12)
    if process.returncode != 0:
        _fail(f"collector failed ({process.returncode})\nstdout:\n{stdout}\nstderr:\n{stderr}")
    return events_path, manifest_path


def _require_int(value: Any, field: str) -> None:
    """Overview: reject JSON booleans, strings, and nulls where the frozen KEvent contract requires an integer."""
    if type(value) is not int:
        _fail(f"KEvent.{field} must be an integer, got {value!r}")


def _validate_event(event: dict[str, Any], cgroup_id: int, previous_sequence: int) -> int:
    """Overview: enforce the closed KEvent envelope and its failed-open invariant without analysis-level interpretation."""
    if set(event) != K_EVENT_FIELDS:
        _fail(f"KEvent fields differ from frozen schema: {sorted(event)}")
    for field in ("seq", "ts_mono_ns", "tgid", "tid", "starttime", "ppid", "ret", "cgroup_id"):
        _require_int(event[field], field)
    if event["seq"] <= previous_sequence:
        _fail(f"KEvent sequence is not strictly increasing: {event['seq']} after {previous_sequence}")
    if event["cgroup_id"] != cgroup_id:
        _fail(f"KEvent escaped target cgroup: {event['cgroup_id']} != {cgroup_id}")
    if not isinstance(event["syscall"], str) or event["site"] not in {"bind", "resolve", "proc", "mark"}:
        _fail(f"KEvent has invalid syscall/site: {event['syscall']!r}/{event['site']!r}")
    if not isinstance(event["args_raw"], dict):
        _fail("KEvent.args_raw must be an object")
    for field in ("errno", "dev", "ino"):
        if event[field] is not None and type(event[field]) is not int:
            _fail(f"KEvent.{field} must be an integer or null")
    if event["content_hash"] is not None and not isinstance(event["content_hash"], str):
        _fail("KEvent.content_hash must be a string or null")
    if event["syscall"] in {"openat", "openat2"} and event["ret"] < 0 and event["errno"] is None:
        _fail("failed open event lacks errno")
    return event["seq"]


def _read_events(path: Path, cgroup_id: int) -> list[dict[str, Any]]:
    """Overview: parse every JSONL record and validate it before asserting workload-specific coverage evidence."""
    events: list[dict[str, Any]] = []
    previous_sequence = 0

    for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        try:
            event = json.loads(line)
        except json.JSONDecodeError as error:
            _fail(f"invalid JSONL at line {line_number}: {error}")
        if not isinstance(event, dict):
            _fail(f"KEvent line {line_number} is not an object")
        previous_sequence = _validate_event(event, cgroup_id, previous_sequence)
        events.append(event)
    if not events:
        _fail("M1 emitted no KEvents")
    return events


def _assert_workload_coverage(events: list[dict[str, Any]]) -> None:
    """Overview: require raw evidence for every frozen workload operation without inferring names, paths, or provenance."""
    observed = {event["syscall"] for event in events}
    missing = REQUIRED_SYSCALLS - observed
    if missing:
        _fail(f"M1 missed workload syscalls: {sorted(missing)}")
    missing_opens = {"missing-openat-one", "missing-openat2-two", "missing-openat-three"}
    failed_paths = {
        event["args_raw"].get("user_path")
        for event in events
        if event["syscall"] in {"openat", "openat2"} and event["ret"] < 0 and event["errno"] == 2
    }
    if not missing_opens.issubset(failed_paths):
        _fail(f"M1 did not preserve all three ENOENT opens: {sorted(failed_paths)}")
    if not any(
        event["syscall"] in {"openat", "openat2"}
        and event["ret"] >= 0
        and event["dev"] is not None
        and event["ino"] is not None
        for event in events
    ):
        _fail("M1 emitted no successful open with dev/ino")
    if not any(
        event["syscall"] == "sf_mark"
        and event["args_raw"].get("json_payload") == '{"phase":"acceptance"}'
        for event in events
    ):
        _fail("M1 did not preserve the sf_mark payload")


def _validate_manifest(path: Path) -> None:
    """Overview: require M1's manifest update to retain the closed contract and report zero ring-buffer loss."""
    manifest = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(manifest, dict) or set(manifest) != MANIFEST_FIELDS:
        _fail("RunManifest fields differ from the frozen schema")
    if manifest["dropped_events"] != 0:
        _fail(f"M1 reported dropped events: {manifest['dropped_events']!r}")


def run_acceptance(collector: Path) -> None:
    """Overview: coordinate the complete privileged M1 acceptance run and always remove its Docker container."""
    if not collector.is_file():
        _fail(f"collector binary does not exist: {collector}")
    if shutil.which("docker") is None or shutil.which("sudo") is None:
        _fail("M1 acceptance requires docker and sudo on the host")
    container_name = f"syncfuzz-m1-acceptance-{os.getpid()}"
    with tempfile.TemporaryDirectory(prefix="syncfuzz-m1-acceptance-") as temporary:
        root = Path(temporary)
        (root / "watch-target").write_text("initial", encoding="utf-8")
        (root / "mount-target").mkdir()
        marker_library, _ = _compile_workload(root)
        cgroup_id = _start_container(root, container_name)
        try:
            events_path, manifest_path = _collect_artifacts(
                collector, marker_library, root, cgroup_id, container_name
            )
            events = _read_events(events_path, cgroup_id)
            _assert_workload_coverage(events)
            _validate_manifest(manifest_path)
        finally:
            subprocess.run(["docker", "rm", "-f", container_name], capture_output=True, text=True, check=False)


def main() -> int:
    """Overview: expose the privileged acceptance runner as a file-input CLI with an explicit collector binary."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--collector", required=True, type=Path)
    arguments = parser.parse_args()
    try:
        run_acceptance(arguments.collector)
    except RuntimeError as error:
        print(f"m1 acceptance failed: {error}", flush=True)
        return 1
    print("m1 acceptance passed", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
