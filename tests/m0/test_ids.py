"""Regression tests for PRD §4.4 resource identities."""

import pytest
from pydantic import ValidationError

from syncfuzz.m0.errors import ContractViolation
from syncfuzz.m0.ids import ResourceId


def test_file_identity_ignores_human_readable_path() -> None:
    """Overview: verify a file identity is based on device, inode, and content hash."""
    left = ResourceId(cls="file", dev=1, ino=2, content_hash="abc", abs_path="/one")
    right = ResourceId(cls="file", dev=1, ino=2, content_hash="abc", abs_path="/two")
    assert left == right
    assert hash(left) == hash(right)


def test_file_identity_detects_same_inode_content_change() -> None:
    """Overview: verify content changes on one inode produce a different file identity."""
    before = ResourceId(cls="file", dev=1, ino=2, content_hash="before")
    after = ResourceId(cls="file", dev=1, ino=2, content_hash="after")
    assert before != after


def test_executable_identity_requires_resolved_absolute_path() -> None:
    """Overview: verify executable identity includes its resolved absolute path."""
    left = ResourceId(cls="executable", abs_path="/bin/tool", dev=1, ino=2, content_hash="abc")
    right = ResourceId(cls="executable", abs_path="/usr/bin/tool", dev=1, ino=2, content_hash="abc")
    assert left != right


def test_socket_identity_uses_peer_process_identity() -> None:
    """Overview: verify Unix sockets compare their peer process rather than pathname."""
    left = ResourceId(cls="unix_socket", pid=10, starttime=20, exe_hash="abc")
    right = ResourceId(cls="unix_socket", pid=10, starttime=20, exe_hash="abc")
    assert left == right


def test_key_rejects_missing_required_identity_field() -> None:
    """Overview: verify incomplete identity data cannot silently enter a comparison."""
    resource = ResourceId(cls="file", dev=1, ino=2)
    with pytest.raises(ContractViolation, match="incomplete ResourceId"):
        resource.key()


def test_identity_models_are_frozen_and_reject_unknown_fields() -> None:
    """Overview: verify schema callers cannot mutate identities or add private extensions."""
    resource = ResourceId(cls="process", pid=10, starttime=20)
    with pytest.raises(ValidationError):
        resource.pid = 11
    with pytest.raises(ValidationError):
        ResourceId(cls="process", pid=10, starttime=20, untracked="value")
