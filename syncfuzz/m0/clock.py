"""M0's sole event-clock domain: Linux CLOCK_MONOTONIC."""

from __future__ import annotations

import time

from .errors import sf_assert

CLOCK_NAME = "CLOCK_MONOTONIC"


def mono_ns() -> int:
    """Overview: return an event timestamp in the eBPF-compatible clock domain."""
    return time.clock_gettime_ns(time.CLOCK_MONOTONIC)


def wall_ns() -> int:
    """Overview: return a wall-clock timestamp only for human-readable manifests.

    The returned value must never order kernel or agent events.
    """
    return time.clock_gettime_ns(time.CLOCK_REALTIME)


def assert_same_domain(user_ns: int, kernel_ns: int, tolerance_ns: int = 50_000_000) -> None:
    """Overview: fail when user and kernel clock samples exceed scheduling tolerance.

    The tolerance is a startup probe allowance, not an event-ordering heuristic.
    """
    sf_assert(tolerance_ns >= 0, "clock tolerance must be non-negative")
    sf_assert(
        abs(user_ns - kernel_ns) < tolerance_ns,
        f"clock domain mismatch: user={user_ns} kernel={kernel_ns}",
    )
