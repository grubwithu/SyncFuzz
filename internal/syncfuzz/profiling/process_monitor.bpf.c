// SPDX-License-Identifier: GPL-2.0
//go:build ignore

/*
 * The first live collector intentionally limits itself to process lifecycle
 * events. It establishes cgroup-scoped, host-side eBPF attribution before the
 * later syscall/resource collectors add pathname, FD, and IPC details.
 */

typedef unsigned char __u8;
typedef unsigned int __u32;
typedef unsigned long long __u64;
typedef int __s32;

#define BPF_MAP_TYPE_ARRAY 2
#define BPF_MAP_TYPE_RINGBUF 27

#define __uint(name, value) int (*name)[value]
#define __type(name, value) typeof(value) *name
#define SEC(name) __attribute__((section(name), used))
#define __always_inline inline __attribute__((always_inline))

#define TASK_COMM_LEN 16

static void *(*bpf_map_lookup_elem)(void *map, const void *key) = (void *)1;
static __u64 (*bpf_ktime_get_ns)(void) = (void *)5;
static long (*bpf_get_current_comm)(void *buf, __u32 size_of_buf) = (void *)16;
static __u64 (*bpf_get_current_pid_tgid)(void) = (void *)14;
static __u64 (*bpf_get_current_cgroup_id)(void) = (void *)80;
static void *(*bpf_ringbuf_reserve)(void *ringbuf, __u64 size, __u64 flags) = (void *)131;
static void (*bpf_ringbuf_submit)(void *data, __u64 flags) = (void *)132;

enum syncfuzz_process_event_kind {
	SYNCFUZZ_PROCESS_FORK = 1,
	SYNCFUZZ_PROCESS_EXEC = 2,
	SYNCFUZZ_PROCESS_EXIT = 3,
};

struct syncfuzz_process_event {
	__u64 monotonic_ns;
	__u64 cgroup_id;
	__u32 pid;
	__u32 parent_pid;
	__u32 child_pid;
	__u32 kind;
	char comm[TASK_COMM_LEN];
};

struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, __u64);
} target_cgroup SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 24);
} events SEC(".maps");

/* Matches tracepoint:sched:sched_process_fork after the common 8-byte header. */
struct trace_event_raw_sched_process_fork {
	char common[8];
	char parent_comm[TASK_COMM_LEN];
	__s32 parent_pid;
	char child_comm[TASK_COMM_LEN];
	__s32 child_pid;
};

static __always_inline int scope_matches(void) {
	__u32 key = 0;
	__u64 *wanted = bpf_map_lookup_elem(&target_cgroup, &key);
	if (!wanted || *wanted == 0) {
		return 0;
	}
	return bpf_get_current_cgroup_id() == *wanted;
}

static __always_inline int emit_process_event(__u32 kind, __u32 pid, __u32 parent_pid, __u32 child_pid) {
	struct syncfuzz_process_event *event;
	if (!scope_matches()) {
		return 0;
	}
	event = bpf_ringbuf_reserve(&events, sizeof(*event), 0);
	if (!event) {
		return 0;
	}
	event->monotonic_ns = bpf_ktime_get_ns();
	event->cgroup_id = bpf_get_current_cgroup_id();
	event->pid = pid;
	event->parent_pid = parent_pid;
	event->child_pid = child_pid;
	event->kind = kind;
	/*
	 * Do not pass a pointer derived from the tracepoint context here. The BPF
	 * verifier forbids dereferencing a modified ctx pointer in this helper.
	 * For fork events this is the parent comm; PID fields retain the precise
	 * parent/child relationship needed by the Go-side normalizer.
	 */
	bpf_get_current_comm(event->comm, sizeof(event->comm));
	bpf_ringbuf_submit(event, 0);
	return 0;
}

SEC("tracepoint/sched/sched_process_fork")
int syncfuzz_process_fork(struct trace_event_raw_sched_process_fork *ctx) {
	return emit_process_event(SYNCFUZZ_PROCESS_FORK, ctx->child_pid, ctx->parent_pid, ctx->child_pid);
}

SEC("tracepoint/sched/sched_process_exec")
int syncfuzz_process_exec(void *ctx) {
	__u64 pid_tgid = bpf_get_current_pid_tgid();
	return emit_process_event(SYNCFUZZ_PROCESS_EXEC, pid_tgid >> 32, 0, 0);
}

SEC("tracepoint/sched/sched_process_exit")
int syncfuzz_process_exit(void *ctx) {
	__u64 pid_tgid = bpf_get_current_pid_tgid();
	return emit_process_event(SYNCFUZZ_PROCESS_EXIT, pid_tgid >> 32, 0, 0);
}

char LICENSE[] SEC("license") = "Dual MIT/GPL";
