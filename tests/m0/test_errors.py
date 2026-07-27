"""Regression tests for M0 fail-fast error primitives."""

import pytest

from syncfuzz.m0.errors import ContractViolation, DataLossError, PairingError, SyncFuzzError, sf_assert


def test_sf_assert_allows_true_conditions() -> None:
    """Overview: verify a satisfied contract does not raise an exception."""
    sf_assert(True, "unused")


def test_sf_assert_raises_contract_violation() -> None:
    """Overview: verify a violated contract fails with its original message."""
    with pytest.raises(ContractViolation, match="missing artifact"):
        sf_assert(False, "missing artifact")


def test_specialized_errors_share_framework_base() -> None:
    """Overview: verify downstream callers can catch all framework failures uniformly."""
    assert issubclass(ContractViolation, SyncFuzzError)
    assert issubclass(DataLossError, SyncFuzzError)
    assert issubclass(PairingError, SyncFuzzError)
