// SPDX-License-Identifier: GPL-2.0
//go:build ignore

/*
 * The resource collector observes successful, cgroup-scoped syscalls. It
 * intentionally captures only a bounded pathname and FD identity; pathname
 * resolution and dependency closure remain user-space probe responsibilities.
 */

typedef unsigned char __u8;
typedef unsigned int __u32;
typedef unsigned long long __u64;
typedef long long __s64;
typedef int __s32;

#define BPF_MAP_TYPE_HASH 1
#define BPF_MAP_TYPE_ARRAY 2
#define BPF_MAP_TYPE_RINGBUF 27

#define __uint(name, value) int (*name)[value]
#define __type(name, value) typeof(value) *name
#define SEC(name) __attribute__((section(name), used))
#define __always_inline inline __attribute__((always_inline))

#define TASK_COMM_LEN 16
#define SYNCFUZZ_PATH_LEN 128

#define BPF_ANY 0

static void *(*bpf_map_lookup_elem)(void *map, const void *key) = (void *)1;
static long (*bpf_map_update_elem)(void *map, const void *key, const void *value, __u64 flags) = (void *)2;
static long (*bpf_map_delete_elem)(void *map, const void *key) = (void *)3;
static __u64 (*bpf_ktime_get_ns)(void) = (void *)5;
static long (*bpf_get_current_comm)(void *buf, __u32 size_of_buf) = (void *)16;
static __u64 (*bpf_get_current_pid_tgid)(void) = (void *)14;
static __u64 (*bpf_get_current_cgroup_id)(void) = (void *)80;
static long (*bpf_probe_read_user_str)(void *dst, __u32 size, const void *unsafe_ptr) = (void *)114;
static void *(*bpf_ringbuf_reserve)(void *ringbuf, __u64 size, __u64 flags) = (void *)131;
static void (*bpf_ringbuf_submit)(void *data, __u64 flags) = (void *)132;

enum syncfuzz_resource_event_kind {
	SYNCFUZZ_OPENAT = 1,
	SYNCFUZZ_CLOSE = 2,
	SYNCFUZZ_DUP = 3,
	SYNCFUZZ_UNLINKAT = 4,
	SYNCFUZZ_RENAMEAT2 = 5,
	SYNCFUZZ_LINKAT = 6,
	SYNCFUZZ_SYMLINKAT = 7,
	SYNCFUZZ_MKDIRAT = 8,
	SYNCFUZZ_SOCKET = 9,
	SYNCFUZZ_BIND = 10,
	SYNCFUZZ_LISTEN = 11,
	SYNCFUZZ_CONNECT = 12,
	SYNCFUZZ_ACCEPT = 13,
	SYNCFUZZ_CHDIR = 14,
	SYNCFUZZ_CHMOD = 15,
	SYNCFUZZ_CHOWN = 16,
	SYNCFUZZ_SETXATTR = 17,
};

/* Linux x86_64 syscall numbers. The Go loader rejects other architectures. */
#define SYSCALL_CLOSE 3
#define SYSCALL_DUP 32
#define SYSCALL_DUP2 33
#define SYSCALL_SOCKET 41
#define SYSCALL_CONNECT 42
#define SYSCALL_ACCEPT 43
#define SYSCALL_BIND 49
#define SYSCALL_LISTEN 50
#define SYSCALL_CHDIR 80
#define SYSCALL_CHMOD 90
#define SYSCALL_CHOWN 92
#define SYSCALL_SETXATTR 188
#define SYSCALL_LSETXATTR 189
#define SYSCALL_FSETXATTR 190
#define SYSCALL_OPENAT 257
#define SYSCALL_MKDIRAT 258
#define SYSCALL_FCHOWNAT 260
#define SYSCALL_UNLINKAT 263
#define SYSCALL_LINKAT 265
#define SYSCALL_SYMLINKAT 266
#define SYSCALL_FCHMODAT 268
#define SYSCALL_ACCEPT4 288
#define SYSCALL_DUP3 292
#define SYSCALL_RENAMEAT2 316

struct trace_event_raw_sys_enter {
	char common[8];
	__s64 id;
	__u64 args[6];
};

struct trace_event_raw_sys_exit {
	char common[8];
	__s64 id;
	__s64 ret;
};

struct pending_resource_syscall {
	__u64 cgroup_id;
	__u64 arg0;
	__u32 kind;
	char path[SYNCFUZZ_PATH_LEN];
};

struct syncfuzz_resource_event {
	__u64 monotonic_ns;
	__u64 cgroup_id;
	__s64 result;
	__u32 pid;
	__u32 kind;
	__s32 fd;
	char comm[TASK_COMM_LEN];
	char path[SYNCFUZZ_PATH_LEN];
	__u32 reserved;
};

struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, __u64);
} target_cgroup SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 16384);
	__type(key, __u64);
	__type(value, struct pending_resource_syscall);
} pending_syscalls SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 24);
} events SEC(".maps");

static __always_inline int scope_matches(void) {
	__u32 key = 0;
	__u64 *wanted = bpf_map_lookup_elem(&target_cgroup, &key);
	if (!wanted || *wanted == 0) {
		return 0;
	}
	return bpf_get_current_cgroup_id() == *wanted;
}

static __always_inline __u32 syscall_kind(__s64 syscall_id) {
	switch (syscall_id) {
	case SYSCALL_OPENAT:
		return SYNCFUZZ_OPENAT;
	case SYSCALL_CLOSE:
		return SYNCFUZZ_CLOSE;
	case SYSCALL_DUP:
	case SYSCALL_DUP2:
	case SYSCALL_DUP3:
		return SYNCFUZZ_DUP;
	case SYSCALL_UNLINKAT:
		return SYNCFUZZ_UNLINKAT;
	case SYSCALL_RENAMEAT2:
		return SYNCFUZZ_RENAMEAT2;
	case SYSCALL_LINKAT:
		return SYNCFUZZ_LINKAT;
	case SYSCALL_SYMLINKAT:
		return SYNCFUZZ_SYMLINKAT;
	case SYSCALL_MKDIRAT:
		return SYNCFUZZ_MKDIRAT;
	case SYSCALL_SOCKET:
		return SYNCFUZZ_SOCKET;
	case SYSCALL_BIND:
		return SYNCFUZZ_BIND;
	case SYSCALL_LISTEN:
		return SYNCFUZZ_LISTEN;
	case SYSCALL_CONNECT:
		return SYNCFUZZ_CONNECT;
	case SYSCALL_ACCEPT:
	case SYSCALL_ACCEPT4:
		return SYNCFUZZ_ACCEPT;
	case SYSCALL_CHDIR:
		return SYNCFUZZ_CHDIR;
	case SYSCALL_CHMOD:
	case SYSCALL_FCHMODAT:
		return SYNCFUZZ_CHMOD;
	case SYSCALL_CHOWN:
	case SYSCALL_FCHOWNAT:
		return SYNCFUZZ_CHOWN;
	case SYSCALL_SETXATTR:
	case SYSCALL_LSETXATTR:
	case SYSCALL_FSETXATTR:
		return SYNCFUZZ_SETXATTR;
	default:
		return 0;
	}
}

static __always_inline void capture_path(__u32 kind, __u64 arg0, __u64 arg1, __u64 arg3, char *destination) {
	if (kind == SYNCFUZZ_OPENAT || kind == SYNCFUZZ_UNLINKAT || kind == SYNCFUZZ_MKDIRAT || kind == SYNCFUZZ_CHMOD || kind == SYNCFUZZ_CHOWN) {
		bpf_probe_read_user_str(destination, SYNCFUZZ_PATH_LEN, (const void *)arg1);
		return;
	}
	if (kind == SYNCFUZZ_RENAMEAT2 || kind == SYNCFUZZ_LINKAT || kind == SYNCFUZZ_SYMLINKAT) {
		bpf_probe_read_user_str(destination, SYNCFUZZ_PATH_LEN, (const void *)arg3);
		return;
	}
	if (kind == SYNCFUZZ_CHDIR || kind == SYNCFUZZ_SETXATTR) {
		bpf_probe_read_user_str(destination, SYNCFUZZ_PATH_LEN, (const void *)arg0);
	}
}

static __always_inline __s32 event_fd(__u32 kind, struct pending_resource_syscall *pending, __s64 result) {
	if (kind == SYNCFUZZ_OPENAT || kind == SYNCFUZZ_SOCKET || kind == SYNCFUZZ_ACCEPT || kind == SYNCFUZZ_DUP) {
		return (__s32)result;
	}
	if (kind == SYNCFUZZ_CLOSE || kind == SYNCFUZZ_BIND || kind == SYNCFUZZ_LISTEN || kind == SYNCFUZZ_CONNECT) {
		return (__s32)pending->arg0;
	}
	return -1;
}

SEC("tracepoint/raw_syscalls/sys_enter")
int syncfuzz_resource_enter(struct trace_event_raw_sys_enter *ctx) {
	__u32 kind = syscall_kind(ctx->id);
	__u64 pid_tgid;
	struct pending_resource_syscall pending = {};
	if (kind == 0 || !scope_matches()) {
		return 0;
	}
	pid_tgid = bpf_get_current_pid_tgid();
	pending.kind = kind;
	pending.cgroup_id = bpf_get_current_cgroup_id();
	pending.arg0 = ctx->args[0];
	capture_path(kind, pending.arg0, ctx->args[1], ctx->args[3], pending.path);
	bpf_map_update_elem(&pending_syscalls, &pid_tgid, &pending, BPF_ANY);
	return 0;
}

SEC("tracepoint/raw_syscalls/sys_exit")
int syncfuzz_resource_exit(struct trace_event_raw_sys_exit *ctx) {
	__u64 pid_tgid = bpf_get_current_pid_tgid();
	struct pending_resource_syscall *pending = bpf_map_lookup_elem(&pending_syscalls, &pid_tgid);
	struct syncfuzz_resource_event *event;
	if (!pending) {
		return 0;
	}
	if (ctx->ret < 0) {
		bpf_map_delete_elem(&pending_syscalls, &pid_tgid);
		return 0;
	}
	event = bpf_ringbuf_reserve(&events, sizeof(*event), 0);
	if (!event) {
		bpf_map_delete_elem(&pending_syscalls, &pid_tgid);
		return 0;
	}
	event->monotonic_ns = bpf_ktime_get_ns();
	event->cgroup_id = pending->cgroup_id;
	event->result = ctx->ret;
	event->pid = pid_tgid >> 32;
	event->kind = pending->kind;
	event->fd = event_fd(pending->kind, pending, ctx->ret);
	bpf_get_current_comm(event->comm, sizeof(event->comm));
	__builtin_memcpy(event->path, pending->path, sizeof(event->path));
	bpf_ringbuf_submit(event, 0);
	bpf_map_delete_elem(&pending_syscalls, &pid_tgid);
	return 0;
}

char LICENSE[] SEC("license") = "Dual MIT/GPL";
