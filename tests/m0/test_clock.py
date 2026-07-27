"""Regression tests for M0's shared event-clock primitives."""

import time
from unittest.mock import patch

import pytest

from syncfuzz.m0.clock import CLOCK_NAME, assert_same_domain, mono_ns, wall_ns
from syncfuzz.m0.errors import ContractViolation


def test_mono_ns_uses_clock_monotonic() -> None:
    """Overview: verify event timestamps use the kernel-compatible monotonic clock."""
    with patch("syncfuzz.m0.clock.time.clock_gettime_ns", return_value=123) as gettime:
        assert mono_ns() == 123
    assert gettime.call_args.args[0] == time.CLOCK_MONOTONIC
    assert CLOCK_NAME == "CLOCK_MONOTONIC"


def test_wall_ns_uses_realtime_only_for_manifest_metadata() -> None:
    """Overview: verify the separate wall clock is available for manifest metadata."""
    with patch("syncfuzz.m0.clock.time.clock_gettime_ns", return_value=456) as gettime:
        assert wall_ns() == 456
    assert gettime.call_args.args[0] == time.CLOCK_REALTIME


def test_assert_same_domain_accepts_samples_inside_tolerance() -> None:
    """Overview: verify small scheduling skew does not reject the shared clock domain."""
    assert_same_domain(100, 104, tolerance_ns=5)


def test_assert_same_domain_rejects_tolerance_boundary() -> None:
    """Overview: verify boundary-or-larger skew is treated as a clock-domain failure."""
    with pytest.raises(ContractViolation, match="clock domain mismatch"):
        assert_same_domain(100, 105, tolerance_ns=5)


def test_assert_same_domain_rejects_negative_tolerance() -> None:
    """Overview: verify invalid startup-probe configuration fails before comparison."""
    with pytest.raises(ContractViolation, match="non-negative"):
        assert_same_domain(1, 1, tolerance_ns=-1)
