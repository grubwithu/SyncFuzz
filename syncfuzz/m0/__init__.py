"""M0 public foundation: contracts, clocks, identities, and artifact IO."""

from .clock import CLOCK_NAME, assert_same_domain, mono_ns, wall_ns
from .artifact import RunDir, open_run
from .errors import ContractViolation, DataLossError, PairingError, SyncFuzzError, sf_assert
from .ids import ResourceClass, ResourceId
from .schema import (
    AEvent,
    BeliefSpan,
    Event,
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

__all__ = [
    "CLOCK_NAME",
    "ContractViolation",
    "DataLossError",
    "AEvent",
    "BeliefSpan",
    "Event",
    "HazardPair",
    "KEvent",
    "PairingError",
    "ResourceClass",
    "ResourceId",
    "PruneStats",
    "RunManifest",
    "RunDir",
    "SyncFuzzError",
    "Sigma",
    "SiteRef",
    "Violation",
    "Verdict",
    "Timeline",
    "TimelineEntry",
    "assert_same_domain",
    "mono_ns",
    "sf_assert",
    "open_run",
    "wall_ns",
    "export_json_schemas",
]
