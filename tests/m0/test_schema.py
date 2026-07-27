"""Regression tests for the M0 contracts explicitly frozen by PRD §4."""

import pytest
from pydantic import ValidationError

from syncfuzz.m0.errors import ContractViolation
from syncfuzz.m0.schema import (
    AEvent,
    BeliefSpan,
    HazardPair,
    KEvent,
    PruneStats,
    RunManifest,
    Sigma,
    SiteRef,
    Timeline,
    TimelineEntry,
    Verdict,
    Violation,
    export_json_schemas,
)


def test_kevent_accepts_failed_open_with_errno() -> None:
    """Overview: verify failed opens retain the errno needed for shadowing analysis."""
    event = KEvent(
        seq=1,
        ts_mono_ns=2,
        tgid=3,
        tid=4,
        starttime=5,
        ppid=6,
        syscall="openat",
        site="resolve",
        ret=-2,
        errno=2,
        cgroup_id=7,
    )
    assert event.errno == 2


def test_kevent_rejects_failed_open_without_errno() -> None:
    """Overview: verify missing failed-open evidence causes an immediate contract failure."""
    with pytest.raises(ContractViolation, match="failed open requires errno"):
        KEvent(
            seq=1,
            ts_mono_ns=2,
            tgid=3,
            tid=4,
            starttime=5,
            ppid=6,
            syscall="openat2",
            site="resolve",
            ret=-2,
            cgroup_id=7,
        )


def test_kevent_is_frozen_and_forbids_unknown_fields() -> None:
    """Overview: verify collectors cannot extend the kernel artifact schema ad hoc."""
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
    with pytest.raises(ValidationError):
        event.ret = 1
    with pytest.raises(ValidationError):
        KEvent.model_validate({**event.model_dump(), "untracked": True})


def test_aevent_preserves_checkpoint_payload() -> None:
    """Overview: verify checkpoint content remains a raw payload for M8 consumption."""
    event = AEvent(
        seq=1,
        ts_mono_ns=2,
        kind="checkpoint_written",
        turn_id="turn-1",
        checkpoint_id="checkpoint-1",
        payload={"keys": ["messages.0.content"], "values": ["socket"]},
    )
    assert event.payload["keys"] == ["messages.0.content"]


def test_sigma_identifier_is_deterministic() -> None:
    """Overview: verify replay schedules always map to stable artifact identifiers."""
    sigma = Sigma(checkpoint_id="c1", resume_index=9, nesting=2, granularity="mid_command")
    assert sigma.sigma_id() == "c1_k9_n2_mid_command"


def test_violation_rejects_consistent_verdicts() -> None:
    """Overview: verify only actual violations enter the violations artifact."""
    site = SiteRef(seq=1, syscall="openat", resource_class="file")
    hazard = HazardPair(
        c_site=site,
        w_site=site,
        u_site=site,
        resource_class="file",
        indirection_d=1,
        evidence_erased=True,
        component_id="component-1",
    )
    with pytest.raises(ValidationError):
        Violation(
            vid="v1",
            hazard=hazard,
            sigma=Sigma(checkpoint_id="c1", resume_index=2),
            cls="CONSISTENT",
            severity="low",
            poc_dir="poc/v1",
        )


def test_verdict_accepts_consistent_result() -> None:
    """Overview: verify M6 can represent a non-violation before reporting decisions."""
    verdict = Verdict(cls="CONSISTENT", severity="low", reason_code="same_identity")
    assert verdict.cls == "CONSISTENT"


def test_belief_span_preserves_declared_rebinding_status() -> None:
    """Overview: verify later timeline analysis receives the state-class declaration unchanged."""
    check = SiteRef(seq=1, syscall="newfstatat", resource_class="file")
    use = SiteRef(seq=2, syscall="openat", resource_class="file")
    span = BeliefSpan(c_site=check, u_site=use, state_class="NAME_BINDING", rebindable=True)
    assert span.rebindable is True


def test_prune_stats_calculates_rate_and_zero_total() -> None:
    """Overview: verify evaluation statistics handle ordinary and empty trace populations."""
    populated = PruneStats(
        resolve_sites_total=100,
        after_writable_prune=4,
        components=2,
        pairs_before=10,
        pairs_after=3,
        truncated_components=0,
    )
    empty = PruneStats(
        resolve_sites_total=0,
        after_writable_prune=0,
        components=0,
        pairs_before=0,
        pairs_after=0,
        truncated_components=0,
    )
    assert populated.prune_rate == 0.96
    assert empty.prune_rate == 0.0


def test_run_manifest_is_mutable_but_forbids_unknown_fields() -> None:
    """Overview: verify modules can record runtime counters without extending the schema."""
    manifest = RunManifest(
        run_id="run-1",
        started_wall_ns=1,
        clock_name="CLOCK_MONOTONIC",
        kernel_release="6.0",
        milestone="P0",
    )
    manifest.dropped_events = 2
    assert manifest.dropped_events == 2
    with pytest.raises(ValidationError):
        RunManifest.model_validate({**manifest.model_dump(), "untracked": True})


def test_timeline_entry_and_container_separate_jsonl_and_m6_shapes() -> None:
    """Overview: verify JSONL entries and complete M6 timelines have distinct frozen models."""
    event = AEvent(seq=1, ts_mono_ns=2, kind="turn_start", turn_id="turn-1")
    entry = TimelineEntry(seq=1, ts_mono_ns=2, axis="agent", aevent=event)
    timeline = Timeline(entries=(entry,))
    assert timeline.entries == (entry,)
    with pytest.raises(ValidationError):
        timeline.entries = ()


def test_export_json_schemas_writes_all_m0_contracts(tmp_path) -> None:
    """Overview: verify schema export is complete, deterministic, and readable by reviewers."""
    written = export_json_schemas(tmp_path)
    names = {path.name for path in written}
    assert names == {
        "aevent.json",
        "belief_span.json",
        "hazard_pair.json",
        "kevent.json",
        "prune_stats.json",
        "resource_id.json",
        "run_manifest.json",
        "sigma.json",
        "site_ref.json",
        "timeline.json",
        "timeline_entry.json",
        "verdict.json",
        "violation.json",
    }
    assert '"title": "KEvent"' in (tmp_path / "kevent.json").read_text(encoding="utf-8")
