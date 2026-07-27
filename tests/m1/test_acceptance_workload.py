"""Compile-time coverage checks for the privileged M1 acceptance workload."""

from pathlib import Path
import subprocess


REPOSITORY_ROOT = Path(__file__).resolve().parents[2]
WORKLOAD_SOURCE = REPOSITORY_ROOT / "tests" / "m1" / "ktrace_acceptance_workload.c"


def test_acceptance_workload_compiles_without_runtime_dependencies() -> None:
    """Overview: reject C syntax or warning regressions before the privileged VM acceptance run."""
    subprocess.run(
        [
            "clang",
            "-std=c11",
            "-Wall",
            "-Wextra",
            "-Werror",
            "-fsyntax-only",
            str(WORKLOAD_SOURCE),
        ],
        check=True,
    )


def test_acceptance_workload_covers_the_frozen_m1_syscall_surface() -> None:
    """Overview: retain every P0 workload operation needed to exercise M1's frozen hook set."""
    source = WORKLOAD_SOURCE.read_text(encoding="utf-8")
    for syscall in (
        "openat",
        "openat2",
        "mkdirat",
        "renameat2",
        "linkat",
        "symlinkat",
        "readlinkat",
        "newfstatat",
        "fchmodat",
        "fsetxattr",
        "mount",
        "write",
        "socket",
        "bind",
        "listen",
        "connect",
        "execve",
        "execveat",
        "unlinkat",
    ):
        assert syscall in source
    assert source.count("missing-") >= 3
    assert "sf_mark" in source
