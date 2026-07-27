"""Regression tests for raw-JSONL artifact communication and fail-fast gates."""

import pytest

from syncfuzz.m0.artifact import open_run
from syncfuzz.m0.errors import ContractViolation, DataLossError
from syncfuzz.m0.schema import AEvent, KEvent, RunManifest


def test_jsonl_round_trip_uses_uncompressed_contract_path(tmp_path) -> None:
    """Overview: verify producers and consumers use the approved raw kevents JSONL path."""
    run = open_run(tmp_path / "run")
    event = KEvent(
        seq=1,
        ts_mono_ns=2,
        tgid=3,
        tid=4,
        starttime=5,
        ppid=6,
        syscall="connect",
        site="resolve",
        ret=0,
        cgroup_id=7,
    )
    assert run.write("kevents", [event]) == 1
    assert run.path("kevents").name == "kevents.jsonl"
    assert list(run.read("kevents", KEvent)) == [event]


def test_write_replaces_previous_artifact_only_after_success(tmp_path) -> None:
    """Overview: verify a failed producer cannot corrupt the last complete artifact."""
    run = open_run(tmp_path / "run")
    first = AEvent(seq=1, ts_mono_ns=2, kind="turn_start", turn_id="turn-1")
    run.write("aevents", [first])
    with pytest.raises(AttributeError):
        run.write("aevents", [object()])
    assert list(run.read("aevents", AEvent)) == [first]
    assert not run.path("aevents").with_suffix(".jsonl.tmp").exists()


def test_read_rejects_malformed_jsonl_record(tmp_path) -> None:
    """Overview: verify consumers fail instead of skipping malformed artifact records."""
    run = open_run(tmp_path / "run")
    run.path("aevents").write_text("not-json\n", encoding="utf-8")
    with pytest.raises(ContractViolation, match="bad record"):
        list(run.read("aevents", AEvent))


def test_single_object_round_trip_and_kind_validation(tmp_path) -> None:
    """Overview: verify manifest IO is distinct from JSONL artifact IO."""
    run = open_run(tmp_path / "run")
    manifest = RunManifest(
        run_id="run-1",
        started_wall_ns=1,
        clock_name="CLOCK_MONOTONIC",
        kernel_release="6.0",
        milestone="P0",
    )
    run.write_one("manifest", manifest)
    assert run.read_one("manifest", RunManifest) == manifest
    with pytest.raises(ContractViolation, match="not a JSONL artifact"):
        run.write("manifest", [])


def test_require_lossless_rejects_dropped_events(tmp_path) -> None:
    """Overview: verify M5 and M8 cannot analyze traces that lost kernel events."""
    run = open_run(tmp_path / "run")
    run.write_one(
        "manifest",
        RunManifest(
            run_id="run-1",
            started_wall_ns=1,
            clock_name="CLOCK_MONOTONIC",
            kernel_release="6.0",
            milestone="P0",
            dropped_events=1,
        ),
    )
    with pytest.raises(DataLossError, match="dropped_events=1"):
        run.require_lossless()


def test_unknown_artifact_name_fails_immediately(tmp_path) -> None:
    """Overview: verify no module can create an undeclared cross-module artifact."""
    run = open_run(tmp_path / "run")
    with pytest.raises(ContractViolation, match="unknown artifact name"):
        run.path("ad_hoc")
