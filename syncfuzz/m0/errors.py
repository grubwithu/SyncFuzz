"""M0 error types and fail-fast contract assertions."""


class SyncFuzzError(Exception):
    """Overview: provide the common base class for framework failures."""


class ContractViolation(SyncFuzzError):
    """Overview: represent an invalid frozen contract without recovery semantics."""


class DataLossError(SyncFuzzError):
    """Overview: represent trace loss that makes downstream analysis invalid."""


class PairingError(SyncFuzzError):
    """Overview: represent a missing atomic checkpoint-to-snapshot pairing."""


def sf_assert(condition: bool, message: str) -> None:
    """Overview: raise ContractViolation immediately when a contract is false.

    This deliberately avoids Python's ``assert`` statement so optimization flags
    cannot remove an invariant check.
    """
    if not condition:
        raise ContractViolation(message)
