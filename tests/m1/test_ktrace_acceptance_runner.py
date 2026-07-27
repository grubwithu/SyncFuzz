"""Unit tests for strict artifact validation used by the privileged M1 acceptance runner."""

import copy

import pytest

from tests.m1 import ktrace_acceptance_runner
from tests.m1.ktrace_acceptance_runner import _start_container, _validate_event


def _event() -> dict:
    """Overview: provide one complete minimal KEvent envelope for validation failure tests."""
    return {
        "seq": 1,
        "ts_mono_ns": 1,
        "tgid": 1,
        "tid": 1,
        "starttime": 1,
        "ppid": 1,
        "syscall": "openat",
        "site": "resolve",
        "args_raw": {},
        "ret": -2,
        "errno": 2,
        "dev": None,
        "ino": None,
        "content_hash": None,
        "cgroup_id": 9,
    }


def test_event_validator_accepts_the_closed_kevent_shape() -> None:
    """Overview: accept a complete failed-open KEvent with the target cgroup and required errno evidence."""
    assert _validate_event(_event(), 9, 0) == 1


def test_event_validator_rejects_missing_failed_open_errno() -> None:
    """Overview: fail fast when a failed open would otherwise erase required ENOENT evidence."""
    event = copy.deepcopy(_event())
    event["errno"] = None

    with pytest.raises(RuntimeError, match="lacks errno"):
        _validate_event(event, 9, 0)


def test_event_validator_rejects_unknown_schema_field() -> None:
    """Overview: reject schema drift rather than accepting extra artifact fields during M1 acceptance."""
    event = copy.deepcopy(_event())
    event["unexpected"] = True

    with pytest.raises(RuntimeError, match="fields differ"):
        _validate_event(event, 9, 0)


def test_acceptance_container_disables_default_seccomp_before_mount_coverage(monkeypatch, tmp_path) -> None:
    """Overview: require the test container to reach the mount tracepoint instead of Docker rejecting it in seccomp."""
    commands: list[list[str]] = []

    def record_command(command: list[str], cwd=None):
        """Overview: record the Docker launch command without creating a real container in this unit test."""
        commands.append(command)
        return None

    monkeypatch.setattr(ktrace_acceptance_runner, "_run", record_command)
    monkeypatch.setattr(ktrace_acceptance_runner, "_cgroup_id_for_container", lambda _: 42)

    assert _start_container(tmp_path, "m1-test-container") == 42
    assert "--security-opt" in commands[0]
    assert "seccomp=unconfined" in commands[0]
