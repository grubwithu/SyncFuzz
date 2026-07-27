// SPDX-License-Identifier: GPL-2.0
/* M1 kernel collector: only capture raw syscall evidence for later M5 analysis. */

#include "ktrace.bpf.h"

SEC("tracepoint/syscalls/sys_enter_openat")
/* Overview: store raw openat arguments at syscall entry without path analysis. */
int trace_enter_openat(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct open_enter_args *args;

    if (!is_in_scope())
        return 0;

    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_open_scratch();
    if (!args)
        return 0;

    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_OPENAT_X86_64;
    args->dirfd = (__s32)ctx->args[0];
    args->flags = (__u32)ctx->args[2];
    args->mode = (__u32)ctx->args[3];
    args->user_path_len = bpf_probe_read_user_str(args->user_path, sizeof(args->user_path),
                                                   (const char *)ctx->args[1]);
    store_open_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_openat2")
/* Overview: store raw openat2 arguments, including the user-space open_how flags and mode. */
int trace_enter_openat2(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct open_enter_args *args;
    struct open_how_user how = {};

    if (!is_in_scope())
        return 0;

    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_open_scratch();
    if (!args)
        return 0;

    bpf_probe_read_user(&how, sizeof(how), (const void *)ctx->args[2]);
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_OPENAT2_X86_64;
    args->dirfd = (__s32)ctx->args[0];
    args->flags = (__u32)how.flags;
    args->mode = (__u32)how.mode;
    args->user_path_len = bpf_probe_read_user_str(args->user_path, sizeof(args->user_path),
                                                   (const char *)ctx->args[1]);
    store_open_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_openat")
/* Overview: emit both successful and failed openat results with their original arguments. */
int trace_exit_openat(struct trace_event_raw_sys_exit *ctx)
{
    return emit_open_exit(ctx);
}

SEC("tracepoint/syscalls/sys_exit_openat2")
/* Overview: emit both successful and failed openat2 results with their original arguments. */
int trace_exit_openat2(struct trace_event_raw_sys_exit *ctx)
{
    return emit_open_exit(ctx);
}

SEC("tracepoint/sched/sched_process_fork")
/* Overview: record a parent-to-child PID edge for later M5 process-tree construction. */
int trace_process_fork(struct trace_event_raw_sched_process_fork *ctx)
{
    return emit_process_event(KTRACE_PROC_FORK, (__u32)ctx->child_pid);
}

SEC("tracepoint/sched/sched_process_exec")
/* Overview: record a process exec lifecycle fact without parsing the executable path. */
int trace_process_exec(struct trace_event_raw_sched_process_exec *ctx)
{
    return emit_process_event(KTRACE_PROC_EXEC, 0);
}

SEC("tracepoint/sched/sched_process_exit")
/* Overview: record a process exit lifecycle fact for later causal attribution. */
int trace_process_exit(struct trace_event_raw_sched_process_template *ctx)
{
    return emit_process_event(KTRACE_PROC_EXIT, 0);
}

SEC("tracepoint/syscalls/sys_enter_unlinkat")
/* Overview: store raw unlinkat path arguments before their namespace-binding result. */
int trace_enter_unlinkat(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_UNLINKAT_X86_64;
    args->has_dirfd = 1;
    args->dirfd = (__s32)ctx->args[0];
    args->flags = (__u32)ctx->args[2];
    args->user_path_len = bpf_probe_read_user_str(args->user_path, sizeof(args->user_path),
                                                   (const char *)ctx->args[1]);
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_unlinkat")
/* Overview: emit raw unlinkat completion evidence for later provenance construction. */
int trace_exit_unlinkat(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("tracepoint/syscalls/sys_enter_mkdirat")
/* Overview: store raw mkdirat path and mode arguments before namespace binding. */
int trace_enter_mkdirat(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_MKDIRAT_X86_64;
    args->has_dirfd = 1;
    args->dirfd = (__s32)ctx->args[0];
    args->mode = (__u32)ctx->args[2];
    args->user_path_len = bpf_probe_read_user_str(args->user_path, sizeof(args->user_path),
                                                   (const char *)ctx->args[1]);
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_mkdirat")
/* Overview: emit raw mkdirat completion evidence for later provenance construction. */
int trace_exit_mkdirat(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("tracepoint/syscalls/sys_enter_renameat2")
/* Overview: store both renameat2 paths and dirfds without resolving either pathname. */
int trace_enter_renameat2(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_RENAMEAT2_X86_64;
    args->has_dirfd = 1;
    args->dirfd = (__s32)ctx->args[0];
    args->secondary_dirfd = (__s32)ctx->args[2];
    args->flags = (__u32)ctx->args[4];
    args->has_secondary_path = 1;
    args->user_path_len = bpf_probe_read_user_str(args->user_path, sizeof(args->user_path),
                                                   (const char *)ctx->args[1]);
    args->secondary_path_len = bpf_probe_read_user_str(args->secondary_path,
                                                        sizeof(args->secondary_path),
                                                        (const char *)ctx->args[3]);
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_renameat2")
/* Overview: emit raw renameat2 completion evidence for later provenance construction. */
int trace_exit_renameat2(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("tracepoint/syscalls/sys_enter_linkat")
/* Overview: store both linkat paths and dirfds without resolving either pathname. */
int trace_enter_linkat(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_LINKAT_X86_64;
    args->has_dirfd = 1;
    args->dirfd = (__s32)ctx->args[0];
    args->secondary_dirfd = (__s32)ctx->args[2];
    args->flags = (__u32)ctx->args[4];
    args->has_secondary_path = 1;
    args->user_path_len = bpf_probe_read_user_str(args->user_path, sizeof(args->user_path),
                                                   (const char *)ctx->args[1]);
    args->secondary_path_len = bpf_probe_read_user_str(args->secondary_path,
                                                        sizeof(args->secondary_path),
                                                        (const char *)ctx->args[3]);
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_linkat")
/* Overview: emit raw linkat completion evidence for later provenance construction. */
int trace_exit_linkat(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("tracepoint/syscalls/sys_enter_symlinkat")
/* Overview: store symlink target and link-path raw strings without path interpretation. */
int trace_enter_symlinkat(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_SYMLINKAT_X86_64;
    args->secondary_dirfd = (__s32)ctx->args[1];
    args->has_secondary_path = 1;
    args->user_path_len = bpf_probe_read_user_str(args->user_path, sizeof(args->user_path),
                                                   (const char *)ctx->args[0]);
    args->secondary_path_len = bpf_probe_read_user_str(args->secondary_path,
                                                        sizeof(args->secondary_path),
                                                        (const char *)ctx->args[2]);
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_symlinkat")
/* Overview: emit raw symlinkat completion evidence for later provenance construction. */
int trace_exit_symlinkat(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("tracepoint/syscalls/sys_enter_newfstatat")
/* Overview: store raw newfstatat arguments as a resolve-site check without resolving its pathname. */
int trace_enter_newfstatat(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_NEWFSTATAT_X86_64;
    args->site = KTRACE_SITE_RESOLVE;
    args->has_dirfd = 1;
    args->dirfd = (__s32)ctx->args[0];
    args->flags = (__u32)ctx->args[3];
    args->user_path_len = bpf_probe_read_user_str(args->user_path, sizeof(args->user_path),
                                                   (const char *)ctx->args[1]);
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_newfstatat")
/* Overview: emit raw newfstatat completion evidence for later belief-span construction. */
int trace_exit_newfstatat(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("tracepoint/syscalls/sys_enter_readlinkat")
/* Overview: store raw readlinkat arguments as a resolve-site lookup without following the link in BPF. */
int trace_enter_readlinkat(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_READLINKAT_X86_64;
    args->site = KTRACE_SITE_RESOLVE;
    args->has_dirfd = 1;
    args->dirfd = (__s32)ctx->args[0];
    args->user_path_len = bpf_probe_read_user_str(args->user_path, sizeof(args->user_path),
                                                   (const char *)ctx->args[1]);
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_readlinkat")
/* Overview: emit raw readlinkat completion evidence for later provenance construction. */
int trace_exit_readlinkat(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("tracepoint/syscalls/sys_enter_fchmodat")
/* Overview: store raw fchmodat path and mode arguments before the namespace-affecting permission change. */
int trace_enter_fchmodat(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_FCHMODAT_X86_64;
    args->has_dirfd = 1;
    args->dirfd = (__s32)ctx->args[0];
    args->mode = (__u32)ctx->args[2];
    args->user_path_len = bpf_probe_read_user_str(args->user_path, sizeof(args->user_path),
                                                   (const char *)ctx->args[1]);
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_fchmodat")
/* Overview: emit raw fchmodat completion evidence without deciding whether the permission change matters. */
int trace_exit_fchmodat(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("tracepoint/syscalls/sys_enter_execve")
/* Overview: store raw execve pathname and vector addresses without interpreting command arguments or environment. */
int trace_enter_execve(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_EXECVE_X86_64;
    args->site = KTRACE_SITE_RESOLVE;
    args->argv_ptr = ctx->args[1];
    args->envp_ptr = ctx->args[2];
    args->user_path_len = bpf_probe_read_user_str(args->user_path, sizeof(args->user_path),
                                                   (const char *)ctx->args[0]);
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_execve")
/* Overview: emit raw execve completion evidence even when the attempted execution fails. */
int trace_exit_execve(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("tracepoint/syscalls/sys_enter_execveat")
/* Overview: store raw execveat pathname, dirfd, flags, and vector addresses without resolving the executable. */
int trace_enter_execveat(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_EXECVEAT_X86_64;
    args->site = KTRACE_SITE_RESOLVE;
    args->has_dirfd = 1;
    args->dirfd = (__s32)ctx->args[0];
    args->flags = (__u32)ctx->args[4];
    args->argv_ptr = ctx->args[2];
    args->envp_ptr = ctx->args[3];
    args->user_path_len = bpf_probe_read_user_str(args->user_path, sizeof(args->user_path),
                                                   (const char *)ctx->args[1]);
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_execveat")
/* Overview: emit raw execveat completion evidence while retaining failed path-resolution attempts. */
int trace_exit_execveat(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("tracepoint/syscalls/sys_enter_fsetxattr")
/* Overview: store an fd-based xattr name, value length, and flags before its binding-side effect. */
int trace_enter_fsetxattr(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_FSETXATTR_X86_64;
    args->fd = (__s32)ctx->args[0];
    args->flags = (__u32)ctx->args[4];
    args->value_size = ctx->args[3];
    args->user_path_len = bpf_probe_read_user_str(args->user_path, sizeof(args->user_path),
                                                   (const char *)ctx->args[1]);
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_fsetxattr")
/* Overview: emit raw fsetxattr completion evidence without deciding whether the metadata affects a resource. */
int trace_exit_fsetxattr(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("tracepoint/syscalls/sys_enter_mount")
/* Overview: store raw mount source, target, filesystem type, and flags before the binding result. */
int trace_enter_mount(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_MOUNT_X86_64;
    args->flags = (__u32)ctx->args[3];
    args->has_secondary_path = 1;
    args->user_path_len = bpf_probe_read_user_str(args->user_path, sizeof(args->user_path),
                                                   (const char *)ctx->args[0]);
    args->secondary_path_len = bpf_probe_read_user_str(args->secondary_path,
                                                        sizeof(args->secondary_path),
                                                        (const char *)ctx->args[1]);
    args->aux_path_len = bpf_probe_read_user_str(args->aux_path, sizeof(args->aux_path),
                                                  (const char *)ctx->args[2]);
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_mount")
/* Overview: emit raw mount completion evidence without reconstructing any path inside eBPF. */
int trace_exit_mount(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("tracepoint/syscalls/sys_enter_write")
/* Overview: retain a write only when its current fd maps to a loader-provided watchlist inode. */
int trace_enter_write(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    __s32 fd;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    fd = (__s32)ctx->args[0];
    if (!is_watchlisted_fd(fd))
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_WRITE_X86_64;
    args->fd = fd;
    args->value_size = ctx->args[2];
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_write")
/* Overview: emit the retained write completion without inspecting the bytes written. */
int trace_exit_write(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("tracepoint/syscalls/sys_enter_socket")
/* Overview: store the raw socket protocol triple before its resolve-site creation result. */
int trace_enter_socket(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_SOCKET_X86_64;
    args->site = KTRACE_SITE_RESOLVE;
    args->socket_domain = (__u32)ctx->args[0];
    args->socket_type = (__u32)ctx->args[1];
    args->socket_protocol = (__u32)ctx->args[2];
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_socket")
/* Overview: emit raw socket creation completion evidence without interpreting the endpoint. */
int trace_exit_socket(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("tracepoint/syscalls/sys_enter_connect")
/* Overview: store one raw connect fd and sockaddr before its resolve-site completion result. */
int trace_enter_connect(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_CONNECT_X86_64;
    args->site = KTRACE_SITE_RESOLVE;
    args->fd = (__s32)ctx->args[0];
    capture_socket_address(args, (const void *)ctx->args[1], ctx->args[2]);
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_connect")
/* Overview: emit raw connect completion evidence without deriving peer identity in the collector. */
int trace_exit_connect(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("tracepoint/syscalls/sys_enter_bind")
/* Overview: store one raw bind fd and sockaddr before its namespace-binding completion result. */
int trace_enter_bind(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_BIND_X86_64;
    args->fd = (__s32)ctx->args[0];
    capture_socket_address(args, (const void *)ctx->args[1], ctx->args[2]);
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_bind")
/* Overview: emit raw bind completion evidence without classifying socket address families. */
int trace_exit_bind(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("tracepoint/syscalls/sys_enter_listen")
/* Overview: store one listener fd and backlog before the namespace-binding listen result. */
int trace_enter_listen(struct trace_event_raw_sys_enter *ctx)
{
    __u64 pid_tgid;
    struct bind_enter_args *args;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    args = lookup_bind_scratch();
    if (!args)
        return 0;
    args->ts_mono_ns = bpf_ktime_get_ns();
    args->syscall_nr = SYSCALL_LISTEN_X86_64;
    args->fd = (__s32)ctx->args[0];
    args->backlog = (__s32)ctx->args[1];
    store_bind_enter(pid_tgid, args);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_listen")
/* Overview: emit raw listen completion evidence without inferring process ownership. */
int trace_exit_listen(struct trace_event_raw_sys_exit *ctx)
{
    return emit_bind_exit(ctx);
}

SEC("uprobe")
/* Overview: capture the sole sf_mark JSON argument without interpreting agent-level marker fields. */
int trace_marker(struct pt_regs *ctx)
{
    return emit_marker_event((const char *)PT_REGS_PARM1(ctx));
}

char LICENSE[] SEC("license") = "GPL";
