"""M0 frozen Pydantic contracts defined by PRD §4."""

from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Literal, TypeAlias

from pydantic import BaseModel, ConfigDict, Field, model_validator

from .errors import sf_assert
from .ids import ResourceId

SCHEMA_VERSION = "1.0.0"

_FROZEN_CONFIG = ConfigDict(extra="forbid", frozen=True)

SiteKind = Literal["bind", "resolve", "proc", "mark"]
AEventKind = Literal[
    "turn_start",
    "turn_end",
    "tool_call_start",
    "tool_call_end",
    "checkpoint_written",
    "llm_call",
    "assertion_candidate",
]
ViolationClass = Literal[
    "REBOUND",
    "RESIDUE",
    "MISSING",
    "DUPLICATE",
    "ORPHAN",
    "ESCAPED",
    "BELIEF_DIVERGENCE",
]
VerdictClass = Literal["CONSISTENT", *ViolationClass.__args__]
Severity = Literal["critical", "high", "medium", "low"]


class KEvent(BaseModel):
    """Overview: represent one raw kernel-axis event without analysis-derived fields."""

    model_config = _FROZEN_CONFIG

    seq: int
    ts_mono_ns: int
    tgid: int
    tid: int
    starttime: int
    ppid: int
    syscall: str
    site: SiteKind
    args_raw: dict = Field(default_factory=dict)
    ret: int
    errno: int | None = None
    dev: int | None = None
    ino: int | None = None
    content_hash: str | None = None
    cgroup_id: int

    @model_validator(mode="after")
    def require_errno_for_failed_open(self) -> "KEvent":
        """Overview: reject failed open events that would erase shadowing evidence."""
        is_failed_open = self.syscall in {"openat", "openat2"} and self.ret < 0
        sf_assert(not is_failed_open or self.errno is not None, "failed open requires errno")
        return self


class AEvent(BaseModel):
    """Overview: represent one raw agent-axis event in the shared monotonic clock domain."""

    model_config = _FROZEN_CONFIG

    seq: int
    ts_mono_ns: int
    kind: AEventKind
    turn_id: str
    tool_call_id: str | None = None
    checkpoint_id: str | None = None
    ctx_hash: str | None = None
    payload: dict = Field(default_factory=dict)


Event: TypeAlias = KEvent | AEvent


class TimelineEntry(BaseModel):
    """Overview: represent one aligned kernel or agent event in the co-axial timeline."""

    model_config = _FROZEN_CONFIG

    seq: int
    ts_mono_ns: int
    axis: Literal["kernel", "agent"]
    kevent: KEvent | None = None
    aevent: AEvent | None = None
    attributed_to: str | None = None
    orphan: bool = False
    late_effect: bool = False
    via_proxy: str | None = None


class Timeline(BaseModel):
    """Overview: provide the immutable full-timeline input shape required by M6."""

    model_config = _FROZEN_CONFIG

    entries: tuple[TimelineEntry, ...]


class SiteRef(BaseModel):
    """Overview: identify a trace site referenced by a hazard or belief-span contract."""

    model_config = _FROZEN_CONFIG

    seq: int
    syscall: str
    abs_path: str | None = None
    resource_class: str
    tool_call_id: str | None = None


class HazardPair(BaseModel):
    """Overview: bind one check-write-use candidate with computed provenance metadata."""

    model_config = _FROZEN_CONFIG

    c_site: SiteRef
    w_site: SiteRef
    u_site: SiteRef
    resource_class: str
    indirection_d: int
    evidence_erased: bool
    component_id: str


class Sigma(BaseModel):
    """Overview: describe one frozen rollback schedule supplied to replay."""

    model_config = _FROZEN_CONFIG

    checkpoint_id: str
    resume_index: int
    nesting: int = 1
    granularity: Literal["tool_call", "mid_command"] = "tool_call"

    def sigma_id(self) -> str:
        """Overview: produce the deterministic artifact-directory identifier for this schedule."""
        return f"{self.checkpoint_id}_k{self.resume_index}_n{self.nesting}_{self.granularity}"


class Violation(BaseModel):
    """Overview: record one non-consistent PRD violation and its reproducibility metadata."""

    model_config = _FROZEN_CONFIG

    vid: str
    hazard: HazardPair
    sigma: Sigma
    delta_env: str | None = None
    delta_inject: str | None = None
    cls: ViolationClass
    severity: Severity
    id_t: ResourceId | None = None
    id_b: ResourceId | None = None
    min_rollback_distance: int | None = None
    poc_dir: str


class Verdict(BaseModel):
    """Overview: represent the pure M6 comparison result before violation reporting."""

    model_config = _FROZEN_CONFIG

    cls: VerdictClass
    severity: Severity
    id_t: ResourceId | None = None
    id_b: ResourceId | None = None
    reason_code: str


class BeliefSpan(BaseModel):
    """Overview: record a check-to-use span and its declared rebinding status."""

    model_config = _FROZEN_CONFIG

    c_site: SiteRef
    u_site: SiteRef
    state_class: str
    rebindable: bool


class PruneStats(BaseModel):
    """Overview: retain provenance-pruning counts for reproducible evaluation statistics."""

    model_config = _FROZEN_CONFIG

    resolve_sites_total: int
    after_writable_prune: int
    components: int
    pairs_before: int
    pairs_after: int
    truncated_components: int

    @property
    def prune_rate(self) -> float:
        """Overview: calculate the fraction of resolve sites removed by writable pruning."""
        if self.resolve_sites_total == 0:
            return 0.0
        return 1.0 - self.after_writable_prune / self.resolve_sites_total


class RunManifest(BaseModel):
    """Overview: hold mutable run-level metadata and hard-failure accounting fields."""

    model_config = ConfigDict(extra="forbid")

    run_id: str
    schema_version: str = SCHEMA_VERSION
    started_wall_ns: int
    clock_name: str
    kernel_release: str
    image_digest: str | None = None
    langgraph_version: str | None = None
    milestone: Literal["P0", "P1", "P2", "P3", "P4", "P5", "P6"]
    dropped_events: int = 0
    orphan_rate: float | None = None
    memo_hit_rate: float | None = None
    prune: PruneStats | None = None


_SCHEMA_MODELS: tuple[tuple[str, type[BaseModel]], ...] = (
    ("aevent", AEvent),
    ("belief_span", BeliefSpan),
    ("hazard_pair", HazardPair),
    ("kevent", KEvent),
    ("prune_stats", PruneStats),
    ("resource_id", ResourceId),
    ("run_manifest", RunManifest),
    ("sigma", Sigma),
    ("site_ref", SiteRef),
    ("timeline", Timeline),
    ("timeline_entry", TimelineEntry),
    ("verdict", Verdict),
    ("violation", Violation),
)


def export_json_schemas(directory: str | Path) -> tuple[Path, ...]:
    """Overview: atomically export deterministic JSON Schema files for every M0 model."""
    root = Path(directory)
    root.mkdir(parents=True, exist_ok=True)
    written: list[Path] = []
    for stem, model in _SCHEMA_MODELS:
        target = root / f"{stem}.json"
        temporary = target.with_suffix(".json.tmp")
        temporary.write_text(
            json.dumps(model.model_json_schema(), indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )
        os.replace(temporary, target)
        written.append(target)
    return tuple(written)
