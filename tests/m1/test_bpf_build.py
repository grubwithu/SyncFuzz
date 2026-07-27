"""Integration tests for the M1 CO-RE collector's compile-time invariants."""

from pathlib import Path
import ctypes
import hashlib
import subprocess


REPOSITORY_ROOT = Path(__file__).resolve().parents[2]
BPF_SOURCE = REPOSITORY_ROOT / "syncfuzz" / "m1" / "bpf" / "ktrace.bpf.c"
BPF_HEADER = REPOSITORY_ROOT / "syncfuzz" / "m1" / "bpf" / "ktrace.bpf.h"
LOADER_SOURCE = REPOSITORY_ROOT / "syncfuzz" / "m1" / "ktrace.c"
MANIFEST_SOURCE = REPOSITORY_ROOT / "syncfuzz" / "m1" / "manifest.c"
CJSON_DIRECTORY = REPOSITORY_ROOT / "syncfuzz" / "third_party" / "cjson"
CJSON_SOURCE = CJSON_DIRECTORY / "cJSON.c"
CJSON_HEADER = CJSON_DIRECTORY / "cJSON.h"
CJSON_LICENSE = CJSON_DIRECTORY / "LICENSE"
BTF_PATH = Path("/sys/kernel/btf/vmlinux")
BPFTool = Path("/usr/sbin/bpftool")
SYSTEM_ARCH_INCLUDE = Path("/usr/include/x86_64-linux-gnu")


def _compile_bpf_object(tmp_path: Path) -> Path:
    """Overview: build the collector against running-kernel BTF for integration checks."""
    vmlinux_header = tmp_path / "vmlinux.h"
    object_file = tmp_path / "ktrace.bpf.o"
    with vmlinux_header.open("w", encoding="utf-8") as output:
        subprocess.run(
            [str(BPFTool), "btf", "dump", "file", str(BTF_PATH), "format", "c"],
            check=True,
            stdout=output,
        )
    subprocess.run(
        [
            "clang",
            "-g",
            "-O2",
            "-target",
            "bpf",
            "-D__TARGET_ARCH_x86",
            "-I",
            str(tmp_path),
            "-I",
            str(SYSTEM_ARCH_INCLUDE),
            "-c",
            str(BPF_SOURCE),
            "-o",
            str(object_file),
        ],
        check=True,
    )
    return object_file


def test_openat_collector_compiles_against_running_kernel_btf(tmp_path) -> None:
    """Overview: verify the minimal collector is valid CO-RE BPF for this Linux kernel."""
    object_file = _compile_bpf_object(tmp_path)
    assert object_file.exists()


def test_c_loader_compiles_against_generated_libbpf_skeleton(tmp_path) -> None:
    """Overview: verify the production C CLI compiles with a skeleton generated from its BPF object."""
    object_file = _compile_bpf_object(tmp_path)
    skeleton_header = tmp_path / "ktrace.skel.h"
    binary = tmp_path / "syncfuzz-ktrace"
    with skeleton_header.open("w", encoding="utf-8") as output:
        subprocess.run(
            [str(BPFTool), "gen", "skeleton", str(object_file)],
            check=True,
            stdout=output,
        )
    subprocess.run(
        [
            "clang",
            "-std=c11",
            "-Wall",
            "-Wextra",
            "-Werror",
            "-I",
            str(tmp_path),
            "-I",
            str(CJSON_DIRECTORY),
            str(LOADER_SOURCE),
            str(MANIFEST_SOURCE),
            str(CJSON_SOURCE),
            "-lbpf",
            "-lz",
            "-o",
            str(binary),
        ],
        check=True,
    )
    assert binary.exists()
    invalid_invocation = subprocess.run(
        [str(binary)],
        check=False,
        capture_output=True,
        text=True,
    )
    assert invalid_invocation.returncode == 2
    assert "--cgroup-id <id>" in invalid_invocation.stderr
    missing_marker = subprocess.run(
        [
            str(binary),
            "--cgroup-id",
            "1",
            "--out",
            str(tmp_path / "kevents.jsonl"),
            "--duration",
            "1",
        ],
        check=False,
        capture_output=True,
        text=True,
    )
    assert missing_marker.returncode == 2
    assert "--marker-so <host-absolute-path>" in missing_marker.stderr
    relative_marker = subprocess.run(
        [
            str(binary),
            "--cgroup-id",
            "1",
            "--out",
            str(tmp_path / "kevents.jsonl"),
            "--duration",
            "1",
            "--marker-so",
            "marker.so",
        ],
        check=False,
        capture_output=True,
        text=True,
    )
    assert relative_marker.returncode == 2
    missing_manifest = subprocess.run(
        [
            str(binary),
            "--cgroup-id",
            "1",
            "--out",
            str(tmp_path / "kevents.jsonl"),
            "--duration",
            "1",
            "--marker-so",
            "/bin/sh",
        ],
        check=False,
        capture_output=True,
        text=True,
    )
    assert missing_manifest.returncode == 2
    assert "--manifest <run>/manifest.json" in missing_manifest.stderr
    relative_watchlist = subprocess.run(
        [
            str(binary),
            "--cgroup-id",
            "1",
            "--out",
            str(tmp_path / "kevents.jsonl"),
            "--duration",
            "1",
            "--watch-path",
            "relative-path",
        ],
        check=False,
        capture_output=True,
        text=True,
    )
    assert relative_watchlist.returncode == 2


def test_makefile_builds_the_cjson_m1_cli(tmp_path) -> None:
    """Overview: verify the documented Makefile links the vendored cJSON-enabled M1 CLI."""
    build_dir = tmp_path / "m1-build"
    subprocess.run(
        ["make", "--no-print-directory", "m1-build", f"BUILD_DIR={build_dir}"],
        check=True,
        cwd=REPOSITORY_ROOT,
    )
    assert (build_dir / "syncfuzz-ktrace").exists()


def test_vendored_cjson_snapshot_matches_the_accepted_adr() -> None:
    """Overview: reject a changed cJSON source snapshot before it can silently alter C JSON behavior."""
    expected_hashes = {
        CJSON_SOURCE: "607e756460fa0de37d20a7a9181f2de29c97bfb7ce5a0e6c2f548243836cd852",
        CJSON_HEADER: "25b0145150d500498e4d209cec69c18c42cf818bffcc54690be3b895a2a16dee",
        CJSON_LICENSE: "a36dda207c36db5818729c54e7ad4e8b0c6fba847491ba64f372c1a2037b6d5c",
    }
    for source_path, expected_hash in expected_hashes.items():
        actual_hash = hashlib.sha256(source_path.read_bytes()).hexdigest()
        assert actual_hash == expected_hash


def _compile_manifest_updater(tmp_path: Path) -> Path:
    """Overview: build the isolated cJSON manifest updater for lossless file-level regression tests."""
    library = tmp_path / "libsyncfuzz-manifest.so"
    subprocess.run(
        [
            "clang",
            "-std=c11",
            "-Wall",
            "-Wextra",
            "-Werror",
            "-shared",
            "-fPIC",
            "-I",
            str(REPOSITORY_ROOT / "syncfuzz" / "m1"),
            "-I",
            str(CJSON_DIRECTORY),
            str(MANIFEST_SOURCE),
            str(CJSON_SOURCE),
            "-o",
            str(library),
        ],
        check=True,
    )
    return library


def test_manifest_updater_preserves_large_numbers_and_rejects_bad_contracts(tmp_path) -> None:
    """Overview: replace only dropped_events while retaining 64-bit literals and rejecting unknown fields."""
    manifest_path = tmp_path / "manifest.json"
    original = (
        b'{"run_id":"run-1","schema_version":"1.0.0",'
        b'"started_wall_ns":1780000000123456789,"clock_name":"CLOCK_MONOTONIC",'
        b'"kernel_release":"6.0","image_digest":null,"langgraph_version":null,'
        b'"milestone":"P0","dropped_events":0,"orphan_rate":null,'
        b'"memo_hit_rate":null,"prune":null}'
    )
    manifest_path.write_bytes(original)
    library = ctypes.CDLL(str(_compile_manifest_updater(tmp_path)))
    updater = library.syncfuzz_update_manifest_dropped_events
    updater.argtypes = [ctypes.c_char_p, ctypes.c_uint64]
    updater.restype = ctypes.c_int

    assert updater(str(manifest_path).encode(), 12) == 0
    assert manifest_path.read_bytes() == original.replace(b'"dropped_events":0', b'"dropped_events":12')
    assert (tmp_path / "manifest.json.lock").exists()

    invalid = original[:-1] + b',"unexpected":true}'
    manifest_path.write_bytes(invalid)
    assert updater(str(manifest_path).encode(), 99) < 0
    assert manifest_path.read_bytes() == invalid


def test_openat_collector_preserves_failed_open_and_drop_paths() -> None:
    """Overview: verify source-level guards keep failure and data-loss evidence observable."""
    source = BPF_HEADER.read_text(encoding="utf-8") + BPF_SOURCE.read_text(encoding="utf-8")
    assert "event->errno_value = result < 0 ? (__s32)-result : 0;" in source
    assert "record_drop();" in source
    assert "bpf_ringbuf_reserve" in source
    assert 'SEC("tracepoint/syscalls/sys_enter_openat2")' in source
    assert 'SEC("tracepoint/syscalls/sys_exit_openat2")' in source
    assert "bpf_probe_read_user(&how" in source
    assert 'SEC("tracepoint/sched/sched_process_fork")' in source
    assert 'SEC("tracepoint/sched/sched_process_exec")' in source
    assert 'SEC("tracepoint/sched/sched_process_exit")' in source
    assert "ctx->child_pid" in source
    for syscall in (
        "unlinkat",
        "mkdirat",
        "renameat2",
        "linkat",
        "symlinkat",
        "fchmodat",
        "fsetxattr",
        "mount",
        "write",
        "bind",
        "listen",
    ):
        assert f'SEC("tracepoint/syscalls/sys_enter_{syscall}")' in source
        assert f'SEC("tracepoint/syscalls/sys_exit_{syscall}")' in source
    for syscall in ("newfstatat", "readlinkat", "execve", "execveat", "socket", "connect"):
        assert f'SEC("tracepoint/syscalls/sys_enter_{syscall}")' in source
        assert f'SEC("tracepoint/syscalls/sys_exit_{syscall}")' in source
    assert "event->site = args->site;" in source
    assert "socket_address_hex" in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "aux_path" in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "fill_open_identity" in source
    assert "bpf_probe_read_kernel(&file" in source
    assert "kernel_dev_to_stat_dev" in source
    assert "identity->dev = kernel_dev_to_stat_dev" in source
    assert "has_file_identity" in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "has_dirfd" in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "envp_ptr" in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "bpf_d_path" not in source
    assert "mark_cwd_unavailable" in source
    assert "cwd_len" in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "--watch-path" in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "--marker-so" in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "--manifest" in LOADER_SOURCE.read_text(encoding="utf-8")
    assert 'SEC("uprobe")' in BPF_SOURCE.read_text(encoding="utf-8")
    assert "PT_REGS_PARM1(ctx)" in source
    assert "json_payload" in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "sf_mark" in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "secondary_path" in source
    assert "secondary_dirfd" in source
    assert '#include "cJSON.h"' in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "cJSON_PrintUnformatted" in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "write_json_string" not in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "flock" in MANIFEST_SOURCE.read_text(encoding="utf-8")
    assert "cJSON_ParseWithLengthOpts" in MANIFEST_SOURCE.read_text(encoding="utf-8")
    assert "require_unified_cgroup_hierarchy" in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "CGROUP2_SUPER_MAGIC" in LOADER_SOURCE.read_text(encoding="utf-8")
    assert '.func_name = "sf_mark"' in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "bpf_program__set_autoattach" in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "bpf_program__set_type" in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "syncfuzz_attach_skeleton_except" not in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "syncfuzz_elf_symbol_file_offset" not in LOADER_SOURCE.read_text(encoding="utf-8")
    assert "libbpf_get_error" in LOADER_SOURCE.read_text(encoding="utf-8")
