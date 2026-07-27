#ifndef SYNCFUZZ_M1_KTRACE_BPF_H
#define SYNCFUZZ_M1_KTRACE_BPF_H

#include "vmlinux.h"
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

#define MAX_PATH 512
#define MAX_SOCKET_ADDRESS 128
#define MAX_MARKER_PAYLOAD 512
#define SYSCALL_WRITE_X86_64 1
#define SYSCALL_SOCKET_X86_64 41
#define SYSCALL_CONNECT_X86_64 42
#define SYSCALL_BIND_X86_64 49
#define SYSCALL_LISTEN_X86_64 50
#define SYSCALL_EXECVE_X86_64 59
#define SYSCALL_MOUNT_X86_64 165
#define SYSCALL_FSETXATTR_X86_64 190
#define SYSCALL_OPENAT_X86_64 257
#define SYSCALL_OPENAT2_X86_64 437
#define SYSCALL_MKDIRAT_X86_64 258
#define SYSCALL_NEWFSTATAT_X86_64 262
#define SYSCALL_UNLINKAT_X86_64 263
#define SYSCALL_LINKAT_X86_64 265
#define SYSCALL_SYMLINKAT_X86_64 266
#define SYSCALL_READLINKAT_X86_64 267
#define SYSCALL_FCHMODAT_X86_64 268
#define SYSCALL_RENAMEAT2_X86_64 316
#define SYSCALL_EXECVEAT_X86_64 322
#define OPEN_FLAG_CREATE 0100

enum ktrace_site {
    KTRACE_SITE_BIND = 0,
    KTRACE_SITE_RESOLVE = 1,
    KTRACE_SITE_PROC = 2,
};

enum ktrace_proc_action {
    KTRACE_PROC_FORK = 1,
    KTRACE_PROC_EXEC = 2,
    KTRACE_PROC_EXIT = 3,
};

struct open_enter_args {
    __u64 ts_mono_ns;
    __u32 syscall_nr;
    __s32 dirfd;
    __s32 secondary_dirfd;
    __u32 flags;
    __u32 mode;
    __s32 user_path_len;
    __s32 secondary_path_len;
    char user_path[MAX_PATH];
    char secondary_path[MAX_PATH];
};

struct open_how_user {
    __u64 flags;
    __u64 mode;
    __u64 resolve;
};

struct bind_enter_args {
    __u64 ts_mono_ns;
    __u32 syscall_nr;
    __u32 site;
    __u32 has_dirfd;
    __s32 dirfd;
    __s32 secondary_dirfd;
    __s32 fd;
    __s32 backlog;
    __u32 flags;
    __u32 mode;
    __u32 has_secondary_path;
    __u32 socket_domain;
    __u32 socket_type;
    __u32 socket_protocol;
    __u64 value_size;
    __u64 argv_ptr;
    __u64 envp_ptr;
    __s32 user_path_len;
    __s32 secondary_path_len;
    __s32 aux_path_len;
    __s32 socket_address_len;
    char user_path[MAX_PATH];
    char secondary_path[MAX_PATH];
    char aux_path[MAX_PATH];
    char socket_address[MAX_SOCKET_ADDRESS];
};

struct ktrace_event {
    __u64 ts_mono_ns;
    __u64 cgroup_id;
    __u64 starttime;
    __u32 tgid;
    __u32 tid;
    __u32 ppid;
    __s32 ret;
    __s32 errno_value;
    __u64 dev;
    __u64 ino;
    __u32 syscall_nr;
    __u32 site;
    __u32 proc_action;
    __u32 peer_tgid;
    __u32 has_file_identity;
    __u32 has_secondary_path;
    __u32 has_dirfd;
    __s32 dirfd;
    __s32 secondary_dirfd;
    __s32 fd;
    __s32 backlog;
    __u32 flags;
    __u32 mode;
    __u32 socket_domain;
    __u32 socket_type;
    __u32 socket_protocol;
    __u64 value_size;
    __u64 argv_ptr;
    __u64 envp_ptr;
    __s32 user_path_len;
    __s32 secondary_path_len;
    __s32 aux_path_len;
    __s32 socket_address_len;
    __s32 cwd_len;
    char user_path[MAX_PATH];
    char secondary_path[MAX_PATH];
    char aux_path[MAX_PATH];
    char socket_address[MAX_SOCKET_ADDRESS];
    char cwd[MAX_PATH];
};

struct file_identity {
    __u64 dev;
    __u64 ino;
};

struct ktrace_proc_event {
    __u64 ts_mono_ns;
    __u64 cgroup_id;
    __u64 starttime;
    __u32 tgid;
    __u32 tid;
    __u32 ppid;
    __u32 site;
    __u32 proc_action;
    __u32 peer_tgid;
};

struct ktrace_mark_event {
    __u64 ts_mono_ns;
    __u64 cgroup_id;
    __u64 starttime;
    __u32 tgid;
    __u32 tid;
    __u32 ppid;
    __s32 json_payload_len;
    char json_payload[MAX_MARKER_PAYLOAD];
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 16384);
    __type(key, __u64);
    __type(value, struct open_enter_args);
} open_enter_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct open_enter_args);
} open_enter_scratch SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 16384);
    __type(key, __u64);
    __type(value, struct bind_enter_args);
} bind_enter_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct bind_enter_args);
} bind_enter_scratch SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 64 << 20);
} events SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u64);
} dropped_events SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 64);
    __type(key, struct file_identity);
    __type(value, __u8);
} write_watchlist SEC(".maps");

const volatile __u64 target_cgroup_id = 0;

/* Overview: decide whether the current task belongs to the explicitly traced cgroup. */
static __always_inline bool is_in_scope(void)
{
    return target_cgroup_id == 0 ||
           bpf_get_current_cgroup_id() == target_cgroup_id;
}

/* Overview: count any event that cannot enter the ring buffer for manifest reporting. */
static __always_inline void record_drop(void)
{
    __u32 index = 0;
    __u64 *count = bpf_map_lookup_elem(&dropped_events, &index);

    if (count)
        __sync_fetch_and_add(count, 1);
}

/* Overview: attach anti-PID-reuse process identity to one raw kernel event. */
static __always_inline void fill_process_identity(__u64 *starttime, __u32 *ppid)
{
    struct task_struct *task = (struct task_struct *)bpf_get_current_task_btf();

    *starttime = BPF_CORE_READ(task, start_boottime);
    *ppid = BPF_CORE_READ(task, real_parent, tgid);
}

/* Overview: mark cwd unavailable on P0 tracepoints rather than fabricating a post-syscall path. */
static __always_inline void mark_cwd_unavailable(struct ktrace_event *event)
{
    event->cwd_len = 0;
}

/* Overview: convert Linux's internal dev_t layout into the stat(2) encoding used by host-side watch paths. */
static __always_inline __u64 kernel_dev_to_stat_dev(__u64 kernel_dev)
{
    __u64 major = kernel_dev >> 20;
    __u64 minor = kernel_dev & ((1U << 20) - 1);

    return (minor & 0xff) | (major << 8) | ((minor & ~0xff) << 12);
}

/* Overview: read one current fd's inode identity in the stat(2) device encoding used by the host watchlist. */
static __always_inline bool lookup_fd_identity(__s32 fd, struct file_identity *identity)
{
    struct task_struct *task;
    struct files_struct *files;
    struct fdtable *fdtable;
    struct file **file_table;
    struct file *file;
    struct inode *inode;
    __u32 index;

    if (fd < 0 || fd > 1048576)
        return false;
    index = (__u32)fd;
    task = (struct task_struct *)bpf_get_current_task_btf();
    files = BPF_CORE_READ(task, files);
    if (!files)
        return false;
    fdtable = BPF_CORE_READ(files, fdt);
    if (!fdtable)
        return false;
    file_table = BPF_CORE_READ(fdtable, fd);
    if (!file_table)
        return false;
    if (bpf_probe_read_kernel(&file, sizeof(file), &file_table[index]) != 0)
        return false;
    if (!file)
        return false;
    inode = BPF_CORE_READ(file, f_inode);
    if (!inode)
        return false;
    identity->dev = kernel_dev_to_stat_dev(BPF_CORE_READ(inode, i_sb, s_dev));
    identity->ino = BPF_CORE_READ(inode, i_ino);
    return true;
}

/* Overview: attach an fd identity to an event only when kernel lookup proves the identity exists. */
static __always_inline void fill_fd_identity(struct ktrace_event *event, __s32 fd)
{
    struct file_identity identity;

    event->has_file_identity = 0;
    event->dev = 0;
    event->ino = 0;
    if (!lookup_fd_identity(fd, &identity))
        return;
    event->dev = identity.dev;
    event->ino = identity.ino;
    event->has_file_identity = 1;
}

/* Overview: attach the returned open fd's identity only for successful opens. */
static __always_inline void fill_open_identity(struct ktrace_event *event, long result)
{
    if (result < 0)
        fill_fd_identity(event, -1);
    else
        fill_fd_identity(event, (__s32)result);
}

/* Overview: test one write fd against loader-populated inode keys without matching path strings in BPF. */
static __always_inline bool is_watchlisted_fd(__s32 fd)
{
    struct file_identity identity;

    if (!lookup_fd_identity(fd, &identity))
        return false;
    return bpf_map_lookup_elem(&write_watchlist, &identity) != NULL;
}

/* Overview: initialize map-backed raw open arguments without using BPF stack space. */
static __always_inline struct open_enter_args *lookup_open_scratch(void)
{
    __u32 scratch_index = 0;
    struct open_enter_args *args = bpf_map_lookup_elem(&open_enter_scratch, &scratch_index);

    if (args) {
        args->secondary_dirfd = 0;
        args->user_path_len = 0;
        args->secondary_path_len = 0;
    }
    return args;
}

/* Overview: attach a raw open argument record to the calling thread for its exit event. */
static __always_inline int store_open_enter(__u64 pid_tgid, struct open_enter_args *args)
{
    return bpf_map_update_elem(&open_enter_map, &pid_tgid, args, BPF_ANY);
}

/* Overview: emit a paired open result while retaining failed resolves and raw arguments. */
static __always_inline int emit_open_exit(struct trace_event_raw_sys_exit *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    struct open_enter_args *args = bpf_map_lookup_elem(&open_enter_map, &pid_tgid);
    struct ktrace_event *event;
    long result;

    if (!args)
        return 0;

    result = ctx->ret;
    event = bpf_ringbuf_reserve(&events, sizeof(*event), 0);
    if (!event) {
        record_drop();
        bpf_map_delete_elem(&open_enter_map, &pid_tgid);
        return 0;
    }

    event->ts_mono_ns = args->ts_mono_ns;
    event->cgroup_id = bpf_get_current_cgroup_id();
    event->tgid = pid_tgid >> 32;
    event->tid = (__u32)pid_tgid;
    event->ret = (__s32)result;
    event->errno_value = result < 0 ? (__s32)-result : 0;
    fill_open_identity(event, result);
    event->syscall_nr = args->syscall_nr;
    event->site = args->flags & OPEN_FLAG_CREATE ? KTRACE_SITE_BIND : KTRACE_SITE_RESOLVE;
    event->proc_action = 0;
    event->peer_tgid = 0;
    event->has_secondary_path = 0;
    event->has_dirfd = 1;
    event->dirfd = args->dirfd;
    event->secondary_dirfd = args->secondary_dirfd;
    event->fd = -1;
    event->backlog = 0;
    event->flags = args->flags;
    event->mode = args->mode;
    event->socket_domain = 0;
    event->socket_type = 0;
    event->socket_protocol = 0;
    event->value_size = 0;
    event->argv_ptr = 0;
    event->envp_ptr = 0;
    event->user_path_len = args->user_path_len;
    event->secondary_path_len = args->secondary_path_len;
    event->aux_path_len = 0;
    event->socket_address_len = 0;
    __builtin_memcpy(event->user_path, args->user_path, sizeof(event->user_path));
    __builtin_memcpy(event->secondary_path, args->secondary_path, sizeof(event->secondary_path));
    fill_process_identity(&event->starttime, &event->ppid);
    mark_cwd_unavailable(event);
    bpf_ringbuf_submit(event, 0);
    bpf_map_delete_elem(&open_enter_map, &pid_tgid);
    return 0;
}

/* Overview: emit one raw process-lifecycle fact without assigning it to an agent tool call. */
static __always_inline int emit_process_event(__u32 action, __u32 peer_tgid)
{
    __u64 pid_tgid;
    struct ktrace_proc_event *event;

    if (!is_in_scope())
        return 0;

    pid_tgid = bpf_get_current_pid_tgid();
    event = bpf_ringbuf_reserve(&events, sizeof(*event), 0);
    if (!event) {
        record_drop();
        return 0;
    }

    event->ts_mono_ns = bpf_ktime_get_ns();
    event->cgroup_id = bpf_get_current_cgroup_id();
    event->tgid = pid_tgid >> 32;
    event->tid = (__u32)pid_tgid;
    event->site = KTRACE_SITE_PROC;
    event->proc_action = action;
    event->peer_tgid = peer_tgid;
    fill_process_identity(&event->starttime, &event->ppid);
    bpf_ringbuf_submit(event, 0);
    return 0;
}

/* Overview: emit one marker payload unchanged so later modules can parse its declared JSON ABI. */
static __always_inline int emit_marker_event(const char *json_payload)
{
    __u64 pid_tgid;
    struct ktrace_mark_event *event;

    if (!is_in_scope())
        return 0;
    pid_tgid = bpf_get_current_pid_tgid();
    event = bpf_ringbuf_reserve(&events, sizeof(*event), 0);
    if (!event) {
        record_drop();
        return 0;
    }
    event->ts_mono_ns = bpf_ktime_get_ns();
    event->cgroup_id = bpf_get_current_cgroup_id();
    event->tgid = pid_tgid >> 32;
    event->tid = (__u32)pid_tgid;
    event->json_payload_len = bpf_probe_read_user_str(event->json_payload,
                                                       sizeof(event->json_payload),
                                                       json_payload);
    fill_process_identity(&event->starttime, &event->ppid);
    bpf_ringbuf_submit(event, 0);
    return 0;
}

/* Overview: initialize map-backed path arguments for bind or resolve syscalls without BPF stack use. */
static __always_inline struct bind_enter_args *lookup_bind_scratch(void)
{
    __u32 scratch_index = 0;
    struct bind_enter_args *args = bpf_map_lookup_elem(&bind_enter_scratch, &scratch_index);

    if (args) {
        args->site = KTRACE_SITE_BIND;
        args->has_dirfd = 0;
        args->dirfd = 0;
        args->secondary_dirfd = 0;
        args->fd = -1;
        args->backlog = 0;
        args->flags = 0;
        args->mode = 0;
        args->has_secondary_path = 0;
        args->socket_domain = 0;
        args->socket_type = 0;
        args->socket_protocol = 0;
        args->value_size = 0;
        args->argv_ptr = 0;
        args->envp_ptr = 0;
        args->user_path_len = 0;
        args->secondary_path_len = 0;
        args->aux_path_len = 0;
        args->socket_address_len = 0;
    }
    return args;
}

/* Overview: associate raw path syscall arguments with the calling thread until its exit event. */
static __always_inline int store_bind_enter(__u64 pid_tgid, struct bind_enter_args *args)
{
    return bpf_map_update_elem(&bind_enter_map, &pid_tgid, args, BPF_ANY);
}

/* Overview: emit one raw bind or resolve syscall result with one or two uninterpreted user paths. */
static __always_inline int emit_bind_exit(struct trace_event_raw_sys_exit *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    struct bind_enter_args *args = bpf_map_lookup_elem(&bind_enter_map, &pid_tgid);
    struct ktrace_event *event;

    if (!args)
        return 0;

    event = bpf_ringbuf_reserve(&events, sizeof(*event), 0);
    if (!event) {
        record_drop();
        bpf_map_delete_elem(&bind_enter_map, &pid_tgid);
        return 0;
    }

    event->ts_mono_ns = args->ts_mono_ns;
    event->cgroup_id = bpf_get_current_cgroup_id();
    event->tgid = pid_tgid >> 32;
    event->tid = (__u32)pid_tgid;
    event->ret = (__s32)ctx->ret;
    event->errno_value = ctx->ret < 0 ? (__s32)-ctx->ret : 0;
    fill_fd_identity(event, ctx->ret >= 0 ? args->fd : -1);
    event->syscall_nr = args->syscall_nr;
    event->site = args->site;
    event->proc_action = 0;
    event->peer_tgid = 0;
    event->has_secondary_path = args->has_secondary_path;
    event->has_dirfd = args->has_dirfd;
    event->dirfd = args->dirfd;
    event->secondary_dirfd = args->secondary_dirfd;
    event->fd = args->fd;
    event->backlog = args->backlog;
    event->flags = args->flags;
    event->mode = args->mode;
    event->socket_domain = args->socket_domain;
    event->socket_type = args->socket_type;
    event->socket_protocol = args->socket_protocol;
    event->value_size = args->value_size;
    event->argv_ptr = args->argv_ptr;
    event->envp_ptr = args->envp_ptr;
    event->user_path_len = args->user_path_len;
    event->secondary_path_len = args->secondary_path_len;
    event->aux_path_len = args->aux_path_len;
    event->socket_address_len = args->socket_address_len;
    __builtin_memcpy(event->user_path, args->user_path, sizeof(event->user_path));
    __builtin_memcpy(event->secondary_path, args->secondary_path, sizeof(event->secondary_path));
    __builtin_memcpy(event->aux_path, args->aux_path, sizeof(event->aux_path));
    __builtin_memcpy(event->socket_address, args->socket_address, sizeof(event->socket_address));
    fill_process_identity(&event->starttime, &event->ppid);
    mark_cwd_unavailable(event);
    bpf_ringbuf_submit(event, 0);
    bpf_map_delete_elem(&bind_enter_map, &pid_tgid);
    return 0;
}

/* Overview: copy a bounded raw sockaddr at entry so later consumers can decode it outside eBPF. */
static __always_inline void capture_socket_address(
    struct bind_enter_args *args,
    const void *address,
    __u64 supplied_length)
{
    __u64 length = supplied_length;

    if (length > MAX_SOCKET_ADDRESS)
        length = MAX_SOCKET_ADDRESS;
    args->socket_address_len = (__s32)length;
    if (length > 0 &&
        bpf_probe_read_user(args->socket_address, length, address) != 0)
        args->socket_address_len = 0;
}

#endif /* SYNCFUZZ_M1_KTRACE_BPF_H */
