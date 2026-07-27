// SPDX-License-Identifier: GPL-2.0
/* M1 user-space collector: attach CO-RE programs and serialize raw events only. */

#define _GNU_SOURCE

#include <errno.h>
#include <inttypes.h>
#include <signal.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/vfs.h>
#include <time.h>

#include <bpf/bpf.h>
#include <bpf/libbpf.h>
#include <linux/magic.h>

#include "cJSON.h"
#include "ktrace.skel.h"
#include "manifest.h"

#define MAX_PATH 512
#define MAX_SOCKET_ADDRESS 128
#define MAX_MARKER_PAYLOAD 512
#define MAX_WATCH_PATHS 64

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

struct file_identity {
    __u64 dev;
    __u64 ino;
};

struct collector {
    FILE *output;
    uint64_t next_seq;
    int write_error;
};

struct options {
    uint64_t cgroup_id;
    const char *out_path;
    const char *marker_so_path;
    const char *manifest_path;
    int duration_s;
    const char **watch_paths;
    size_t watch_path_count;
};

static volatile sig_atomic_t keep_running = 1;

/* Overview: print the approved M1 CLI shape including the explicit host marker shared-object path. */
static void print_usage(const char *program)
{
    fprintf(stderr,
            "Usage: %s --cgroup-id <id> --out <run>/kevents.jsonl --duration <s> "
            "--marker-so <host-absolute-path> --manifest <run>/manifest.json "
            "[--watch-path <host-absolute-path>]...\n",
            program);
}

/* Overview: stop the ring-buffer polling loop promptly when a normal termination signal arrives. */
static void request_stop(int signal_number)
{
    (void)signal_number;
    keep_running = 0;
}

/* Overview: parse a strictly positive unsigned cgroup identifier without accepting partial values. */
static int parse_cgroup_id(const char *text, uint64_t *value)
{
    char *end = NULL;
    unsigned long long parsed;

    errno = 0;
    parsed = strtoull(text, &end, 10);
    if (errno != 0 || end == text || *end != '\0' || parsed == 0)
        return -EINVAL;
    *value = parsed;
    return 0;
}

/* Overview: parse a positive collection duration in whole seconds for the bounded CLI invocation. */
static int parse_duration(const char *text, int *value)
{
    char *end = NULL;
    long parsed;

    errno = 0;
    parsed = strtol(text, &end, 10);
    if (errno != 0 || end == text || *end != '\0' || parsed < 1 || parsed > INT32_MAX)
        return -EINVAL;
    *value = (int)parsed;
    return 0;
}

/* Overview: append one absolute host watch path while bounding loader map population to its fixed capacity. */
static int append_watch_path(struct options *options, const char *path)
{
    const char **paths;

    if (path[0] != '/' || options->watch_path_count >= MAX_WATCH_PATHS)
        return -EINVAL;
    paths = realloc(options->watch_paths, (options->watch_path_count + 1) * sizeof(*paths));
    if (!paths)
        return -ENOMEM;
    options->watch_paths = paths;
    options->watch_paths[options->watch_path_count] = path;
    options->watch_path_count += 1;
    return 0;
}

/* Overview: release only loader-owned watchlist allocation after parse failures or collection completion. */
static void free_options(struct options *options)
{
    free(options->watch_paths);
    options->watch_paths = NULL;
    options->watch_path_count = 0;
}

/* Overview: parse approved M1 arguments and reject malformed watchlist input before loading BPF. */
static int parse_options(int argc, char **argv, struct options *options)
{
    int index;

    memset(options, 0, sizeof(*options));
    for (index = 1; index < argc; index += 2) {
        if (index + 1 >= argc)
            return -EINVAL;
        if (strcmp(argv[index], "--cgroup-id") == 0) {
            if (parse_cgroup_id(argv[index + 1], &options->cgroup_id) != 0)
                return -EINVAL;
        } else if (strcmp(argv[index], "--out") == 0) {
            options->out_path = argv[index + 1];
        } else if (strcmp(argv[index], "--duration") == 0) {
            if (parse_duration(argv[index + 1], &options->duration_s) != 0)
                return -EINVAL;
        } else if (strcmp(argv[index], "--watch-path") == 0) {
            if (append_watch_path(options, argv[index + 1]) != 0)
                return -EINVAL;
        } else if (strcmp(argv[index], "--marker-so") == 0) {
            if (options->marker_so_path != NULL || argv[index + 1][0] != '/')
                return -EINVAL;
            options->marker_so_path = argv[index + 1];
        } else if (strcmp(argv[index], "--manifest") == 0) {
            if (options->manifest_path != NULL)
                return -EINVAL;
            options->manifest_path = argv[index + 1];
        } else {
            return -EINVAL;
        }
    }
    return options->cgroup_id != 0 && options->out_path != NULL && options->marker_so_path != NULL &&
                   options->manifest_path != NULL && options->duration_s > 0
               ? 0
               : -EINVAL;
}

/* Overview: map the raw BPF site discriminator to the frozen KEvent site string. */
static const char *site_name(__u32 site)
{
    switch (site) {
    case KTRACE_SITE_BIND:
        return "bind";
    case KTRACE_SITE_RESOLVE:
        return "resolve";
    case KTRACE_SITE_PROC:
        return "proc";
    default:
        return NULL;
    }
}

/* Overview: map low-numbered network, process, and mount syscalls without a high-complexity switch. */
static const char *low_syscall_name(__u32 syscall_number)
{
    switch (syscall_number) {
    case 1:
        return "write";
    case 41:
        return "socket";
    case 42:
        return "connect";
    case 49:
        return "bind";
    case 50:
        return "listen";
    case 59:
        return "execve";
    case 165:
        return "mount";
    case 190:
        return "fsetxattr";
    default:
        return NULL;
    }
}

/* Overview: map filesystem syscall numbers while keeping the frozen event-name mapping reviewable. */
static const char *filesystem_syscall_name(__u32 syscall_number)
{
    switch (syscall_number) {
    case 257:
        return "openat";
    case 437:
        return "openat2";
    case 258:
        return "mkdirat";
    case 262:
        return "newfstatat";
    case 263:
        return "unlinkat";
    case 265:
        return "linkat";
    case 266:
        return "symlinkat";
    case 267:
        return "readlinkat";
    case 268:
        return "fchmodat";
    case 316:
        return "renameat2";
    case 322:
        return "execveat";
    default:
        return NULL;
    }
}

/* Overview: resolve every emitted raw syscall number through bounded category-specific lookup tables. */
static const char *syscall_name(__u32 syscall_number)
{
    const char *name = low_syscall_name(syscall_number);

    return name ? name : filesystem_syscall_name(syscall_number);
}

/* Overview: map lifecycle discriminator values to stable scheduler-event names for KEvent records. */
static const char *process_syscall_name(__u32 action)
{
    switch (action) {
    case KTRACE_PROC_FORK:
        return "sched_process_fork";
    case KTRACE_PROC_EXEC:
        return "sched_process_exec";
    case KTRACE_PROC_EXIT:
        return "sched_process_exit";
    default:
        return NULL;
    }
}

/* Overview: bound a BPF user-string field by both its helper return value and fixed event capacity. */
static size_t recorded_string_length(const char value[MAX_PATH], __s32 recorded_length)
{
    size_t bound;

    if (recorded_length <= 0)
        return 0;
    bound = (size_t)recorded_length;
    if (bound > MAX_PATH)
        bound = MAX_PATH;
    return strnlen(value, bound);
}

/* Overview: bound a fixed raw sockaddr field by its BPF-recorded byte count without text decoding. */
static size_t recorded_socket_address_length(__s32 recorded_length)
{
    if (recorded_length <= 0)
        return 0;
    if (recorded_length > MAX_SOCKET_ADDRESS)
        return MAX_SOCKET_ADDRESS;
    return (size_t)recorded_length;
}

/* Overview: preserve an exact unsigned 64-bit JSON integer without cJSON's lossy double conversion. */
static cJSON *json_unsigned(uint64_t value)
{
    char text[32];
    int length = snprintf(text, sizeof(text), "%" PRIu64, value);

    if (length < 0 || (size_t)length >= sizeof(text))
        return NULL;
    return cJSON_CreateRaw(text);
}

/* Overview: preserve an exact signed integer as a validated decimal token owned by cJSON. */
static cJSON *json_signed(int64_t value)
{
    char text[32];
    int length = snprintf(text, sizeof(text), "%" PRId64, value);

    if (length < 0 || (size_t)length >= sizeof(text))
        return NULL;
    return cJSON_CreateRaw(text);
}

/* Overview: transfer one created cJSON value into an object and clean up it on insertion failure. */
static int json_add_item(cJSON *object, const char *name, cJSON *item)
{
    if (!item)
        return -ENOMEM;
    if (!cJSON_AddItemToObject(object, name, item)) {
        cJSON_Delete(item);
        return -ENOMEM;
    }
    return 0;
}

/* Overview: add an exact unsigned kernel value without silently rounding 64-bit timestamps or identities. */
static int json_add_unsigned(cJSON *object, const char *name, uint64_t value)
{
    return json_add_item(object, name, json_unsigned(value));
}

/* Overview: add an exact signed syscall result or raw argument as an unrounded JSON integer. */
static int json_add_signed(cJSON *object, const char *name, int64_t value)
{
    return json_add_item(object, name, json_signed(value));
}

/* Overview: add a boolean raw-capture flag through cJSON rather than formatting JSON text manually. */
static int json_add_boolean(cJSON *object, const char *name, bool value)
{
    return json_add_item(object, name, cJSON_CreateBool(value));
}

/* Overview: add an explicitly unknown frozen-schema field as a cJSON null value. */
static int json_add_null(cJSON *object, const char *name)
{
    return json_add_item(object, name, cJSON_CreateNull());
}

/* Overview: add a trusted NUL-terminated literal through cJSON's string escaping implementation. */
static int json_add_string(cJSON *object, const char *name, const char *value)
{
    return json_add_item(object, name, cJSON_CreateString(value));
}

/* Overview: copy one bounded BPF string span before cJSON serialization so no unterminated buffer is read. */
static int json_add_string_span(cJSON *object, const char *name, const char *value, size_t length)
{
    char *copy;
    cJSON *item;

    if (length == SIZE_MAX)
        return -EOVERFLOW;
    copy = malloc(length + 1);
    if (!copy)
        return -ENOMEM;
    memcpy(copy, value, length);
    copy[length] = '\0';
    item = cJSON_CreateString(copy);
    free(copy);
    return json_add_item(object, name, item);
}

/* Overview: hex-encode raw sockaddr bytes before passing the resulting JSON string to cJSON. */
static int json_add_hex_string(cJSON *object, const char *name, const char *value, size_t length)
{
    static const char hexadecimal[] = "0123456789abcdef";
    char *encoded;
    cJSON *item;
    size_t index;

    if (length > (SIZE_MAX - 1) / 2)
        return -EOVERFLOW;
    encoded = malloc(length * 2 + 1);
    if (!encoded)
        return -ENOMEM;
    for (index = 0; index < length; index += 1) {
        unsigned char byte = (unsigned char)value[index];

        encoded[index * 2] = hexadecimal[byte >> 4];
        encoded[index * 2 + 1] = hexadecimal[byte & 0x0f];
    }
    encoded[length * 2] = '\0';
    item = cJSON_CreateString(encoded);
    free(encoded);
    return json_add_item(object, name, item);
}

/* Overview: create the common strict KEvent envelope and attach an empty raw-argument object. */
static cJSON *json_new_event(
    struct collector *collector,
    uint64_t ts_mono_ns,
    uint64_t starttime,
    uint32_t tgid,
    uint32_t tid,
    uint32_t ppid,
    const char *syscall,
    const char *site,
    cJSON **args)
{
    cJSON *event = cJSON_CreateObject();
    cJSON *raw_args = cJSON_CreateObject();
    int result;

    if (!event || !raw_args)
        goto failed;
    result = json_add_unsigned(event, "seq", collector->next_seq++);
    if (result != 0)
        goto failed;
    result = json_add_unsigned(event, "ts_mono_ns", ts_mono_ns);
    if (result != 0)
        goto failed;
    result = json_add_unsigned(event, "tgid", tgid);
    if (result != 0)
        goto failed;
    result = json_add_unsigned(event, "tid", tid);
    if (result != 0)
        goto failed;
    result = json_add_unsigned(event, "starttime", starttime);
    if (result != 0)
        goto failed;
    result = json_add_unsigned(event, "ppid", ppid);
    if (result != 0)
        goto failed;
    result = json_add_string(event, "syscall", syscall);
    if (result != 0)
        goto failed;
    result = json_add_string(event, "site", site);
    if (result != 0)
        goto failed;
    if (!cJSON_AddItemToObject(event, "args_raw", raw_args))
        goto failed;
    *args = raw_args;
    return event;

failed:
    cJSON_Delete(raw_args);
    cJSON_Delete(event);
    return NULL;
}

/* Overview: add result and optional identity fields while retaining null for raw values that were unavailable. */
static int json_add_event_result(
    cJSON *event,
    __s32 ret,
    __s32 errno_value,
    __u32 has_file_identity,
    __u64 dev,
    __u64 ino,
    uint64_t cgroup_id)
{
    int result = json_add_signed(event, "ret", ret);

    if (result != 0)
        return result;
    result = ret < 0 ? json_add_signed(event, "errno", errno_value) : json_add_null(event, "errno");
    if (result != 0)
        return result;
    result = has_file_identity ? json_add_unsigned(event, "dev", dev) : json_add_null(event, "dev");
    if (result != 0)
        return result;
    result = has_file_identity ? json_add_unsigned(event, "ino", ino) : json_add_null(event, "ino");
    if (result != 0)
        return result;
    result = json_add_null(event, "content_hash");
    if (result != 0)
        return result;
    return json_add_unsigned(event, "cgroup_id", cgroup_id);
}

/* Overview: render one complete cJSON event as an atomic JSONL line and release its owned tree. */
static int write_json_event(struct collector *collector, cJSON *event)
{
    char *line = cJSON_PrintUnformatted(event);
    int result = 0;

    if (!line) {
        result = -ENOMEM;
    } else if (fputs(line, collector->output) == EOF || fputc('\n', collector->output) == EOF) {
        result = -EIO;
    }
    cJSON_free(line);
    cJSON_Delete(event);
    return result;
}

/* Overview: add file-descriptor and open-flag fields to a raw syscall argument object. */
static int json_add_file_arguments(cJSON *args, const struct ktrace_event *event)
{
    int result = json_add_signed(args, "dirfd", event->dirfd);

    if (result != 0)
        return result;
    result = json_add_signed(args, "secondary_dirfd", event->secondary_dirfd);
    if (result != 0)
        return result;
    result = json_add_boolean(args, "has_dirfd", event->has_dirfd != 0);
    if (result != 0)
        return result;
    result = json_add_signed(args, "fd", event->fd);
    if (result != 0)
        return result;
    result = json_add_signed(args, "backlog", event->backlog);
    if (result != 0)
        return result;
    result = json_add_unsigned(args, "flags", event->flags);
    if (result != 0)
        return result;
    return json_add_unsigned(args, "mode", event->mode);
}

/* Overview: add socket metadata as raw values without decoding or classifying network addresses. */
static int json_add_socket_arguments(cJSON *args, const struct ktrace_event *event)
{
    size_t length = recorded_socket_address_length(event->socket_address_len);
    int result = json_add_unsigned(args, "socket_domain", event->socket_domain);

    if (result != 0)
        return result;
    result = json_add_unsigned(args, "socket_type", event->socket_type);
    if (result != 0)
        return result;
    result = json_add_unsigned(args, "socket_protocol", event->socket_protocol);
    if (result != 0)
        return result;
    result = json_add_hex_string(args, "socket_address_hex", event->socket_address, length);
    if (result != 0)
        return result;
    return json_add_signed(args, "socket_address_len", event->socket_address_len);
}

/* Overview: add write and exec pointer metadata without dereferencing user arguments in user space. */
static int json_add_execution_arguments(cJSON *args, const struct ktrace_event *event)
{
    int result = json_add_unsigned(args, "value_size", event->value_size);

    if (result != 0)
        return result;
    result = json_add_unsigned(args, "argv_ptr", event->argv_ptr);
    if (result != 0)
        return result;
    return json_add_unsigned(args, "envp_ptr", event->envp_ptr);
}

/* Overview: add raw path and cwd strings with recorded lengths and without any path resolution. */
static int json_add_path_arguments(cJSON *args, const struct ktrace_event *event)
{
    size_t primary_length = recorded_string_length(event->user_path, event->user_path_len);
    size_t secondary_length = recorded_string_length(event->secondary_path, event->secondary_path_len);
    size_t aux_length = recorded_string_length(event->aux_path, event->aux_path_len);
    size_t cwd_length = recorded_string_length(event->cwd, event->cwd_len);
    int result = json_add_boolean(args, "has_secondary_path", event->has_secondary_path != 0);

    if (result != 0)
        return result;
    result = json_add_string_span(args, "user_path", event->user_path, primary_length);
    if (result != 0)
        return result;
    result = json_add_string_span(args, "secondary_path", event->secondary_path, secondary_length);
    if (result != 0)
        return result;
    result = json_add_string_span(args, "aux_path", event->aux_path, aux_length);
    if (result != 0)
        return result;
    result = json_add_string_span(args, "cwd", event->cwd, cwd_length);
    if (result != 0)
        return result;
    result = json_add_signed(args, "user_path_len", event->user_path_len);
    if (result != 0)
        return result;
    result = json_add_signed(args, "secondary_path_len", event->secondary_path_len);
    if (result != 0)
        return result;
    result = json_add_signed(args, "aux_path_len", event->aux_path_len);
    if (result != 0)
        return result;
    return json_add_signed(args, "cwd_len", event->cwd_len);
}

/* Overview: serialize one syscall event as strict KEvent JSON without resolving or classifying paths. */
static int write_syscall_event(struct collector *collector, const struct ktrace_event *event)
{
    const char *syscall = syscall_name(event->syscall_nr);
    const char *site = site_name(event->site);
    cJSON *args;
    cJSON *json_event;
    int result;

    if (!syscall || !site)
        return -EPROTO;
    json_event = json_new_event(collector, event->ts_mono_ns, event->starttime, event->tgid,
                                event->tid, event->ppid, syscall, site, &args);
    if (!json_event)
        return -ENOMEM;
    result = json_add_file_arguments(args, event);
    if (result == 0)
        result = json_add_socket_arguments(args, event);
    if (result == 0)
        result = json_add_execution_arguments(args, event);
    if (result == 0)
        result = json_add_path_arguments(args, event);
    if (result == 0)
        result = json_add_event_result(json_event, event->ret, event->errno_value,
                                       event->has_file_identity, event->dev, event->ino,
                                       event->cgroup_id);
    if (result != 0) {
        cJSON_Delete(json_event);
        return result;
    }
    return write_json_event(collector, json_event);
}

/* Overview: serialize one scheduler lifecycle event as strict KEvent JSON for later causal attribution. */
static int write_process_event(struct collector *collector, const struct ktrace_proc_event *event)
{
    const char *syscall = process_syscall_name(event->proc_action);
    cJSON *args;
    cJSON *json_event;
    int result;

    if (!syscall)
        return -EPROTO;
    json_event = json_new_event(collector, event->ts_mono_ns, event->starttime, event->tgid,
                                event->tid, event->ppid, syscall, "proc", &args);
    if (!json_event)
        return -ENOMEM;
    result = json_add_unsigned(args, "proc_action", event->proc_action);
    if (result == 0)
        result = json_add_unsigned(args, "peer_tgid", event->peer_tgid);
    if (result == 0)
        result = json_add_event_result(json_event, 0, 0, 0, 0, 0, event->cgroup_id);
    if (result != 0) {
        cJSON_Delete(json_event);
        return result;
    }
    return write_json_event(collector, json_event);
}

/* Overview: serialize one opaque sf_mark JSON payload without parsing its future M5-only semantics. */
static int write_marker_event(struct collector *collector, const struct ktrace_mark_event *event)
{
    size_t payload_length = recorded_string_length(event->json_payload, event->json_payload_len);
    cJSON *args;
    cJSON *json_event;
    int result;

    json_event = json_new_event(collector, event->ts_mono_ns, event->starttime, event->tgid,
                                event->tid, event->ppid, "sf_mark", "mark", &args);
    if (!json_event)
        return -ENOMEM;
    result = json_add_string_span(args, "json_payload", event->json_payload, payload_length);
    if (result == 0)
        result = json_add_signed(args, "json_payload_len", event->json_payload_len);
    if (result == 0)
        result = json_add_event_result(json_event, 0, 0, 0, 0, 0, event->cgroup_id);
    if (result != 0) {
        cJSON_Delete(json_event);
        return result;
    }
    return write_json_event(collector, json_event);
}

/* Overview: dispatch fixed-size raw ring-buffer records and stop collection if serialization fails. */
static int handle_ring_event(void *context, void *data, size_t size)
{
    struct collector *collector = context;
    int result;

    if (size == sizeof(struct ktrace_event))
        result = write_syscall_event(collector, data);
    else if (size == sizeof(struct ktrace_proc_event))
        result = write_process_event(collector, data);
    else if (size == sizeof(struct ktrace_mark_event))
        result = write_marker_event(collector, data);
    else
        result = -EPROTO;
    if (result != 0)
        collector->write_error = result;
    return result;
}

/* Overview: poll the loaded ring buffer for the requested monotonic duration or an interrupt signal. */
static int poll_events(struct ring_buffer *ring_buffer, struct collector *collector, int duration_s)
{
    struct timespec started;

    if (clock_gettime(CLOCK_MONOTONIC, &started) != 0)
        return -errno;
    while (keep_running) {
        struct timespec now;
        int result;

        if (clock_gettime(CLOCK_MONOTONIC, &now) != 0)
            return -errno;
        if (now.tv_sec - started.tv_sec >= duration_s)
            return 0;
        result = ring_buffer__poll(ring_buffer, 100);
        if (result == -EINTR)
            continue;
        if (result < 0)
            return result;
        if (collector->write_error != 0)
            return collector->write_error;
    }
    return 0;
}

/* Overview: read the BPF drop counter after polling so loss remains observable to the CLI. */
static int read_dropped_events(struct ktrace_bpf *skeleton, uint64_t *dropped)
{
    uint32_t key = 0;
    int map_fd = bpf_map__fd(skeleton->maps.dropped_events);

    if (map_fd < 0)
        return -EINVAL;
    if (bpf_map_lookup_elem(map_fd, &key, dropped) != 0)
        return -errno;
    return 0;
}

/* Overview: resolve approved host watch paths to inode keys and populate the BPF write filter map. */
static int populate_write_watchlist(struct ktrace_bpf *skeleton, const struct options *options)
{
    int map_fd = bpf_map__fd(skeleton->maps.write_watchlist);
    uint8_t enabled = 1;
    size_t index;

    if (map_fd < 0)
        return -EINVAL;
    for (index = 0; index < options->watch_path_count; index += 1) {
        struct stat metadata;
        struct file_identity identity;

        if (stat(options->watch_paths[index], &metadata) != 0)
            return -errno;
        identity.dev = (__u64)metadata.st_dev;
        identity.ino = (__u64)metadata.st_ino;
        if (bpf_map_update_elem(map_fd, &identity, &enabled, BPF_ANY) != 0)
            return -errno;
    }
    return 0;
}

/* Overview: reject a missing or non-regular marker shared object before attempting a host uprobe attach. */
static int validate_marker_shared_object(const char *marker_so_path)
{
    struct stat metadata;

    if (stat(marker_so_path, &metadata) != 0)
        return -errno;
    return S_ISREG(metadata.st_mode) ? 0 : -EINVAL;
}

/* Overview: reject non-unified cgroup mounts because M1's target ID is a cgroup v2 identity. */
static int require_unified_cgroup_hierarchy(void)
{
    struct statfs filesystem;

    if (statfs("/sys/fs/cgroup", &filesystem) != 0)
        return -errno;
    return (unsigned long)filesystem.f_type == (unsigned long)CGROUP2_SUPER_MAGIC ? 0 : -EOPNOTSUPP;
}

/* Overview: attach the loaded marker program to sf_mark in the explicit host shared object. */
static int attach_marker(struct ktrace_bpf *skeleton, const struct options *options,
                         struct bpf_link **marker_link)
{
    const struct bpf_uprobe_opts attach_options = {
        .sz = sizeof(attach_options),
        .func_name = "sf_mark",
    };
    long attach_error;

    *marker_link = bpf_program__attach_uprobe_opts(skeleton->progs.trace_marker, -1,
                                                   options->marker_so_path, 0, &attach_options);
    attach_error = libbpf_get_error(*marker_link);
    if (attach_error != 0) {
        *marker_link = NULL;
        return (int)attach_error;
    }
    return 0;
}

/* Overview: build a temporary artifact path in the output directory for atomic JSONL publication. */
static char *temporary_path_for(const char *out_path)
{
    size_t length = strlen(out_path);
    char *temporary = malloc(length + 5);

    if (!temporary)
        return NULL;
    memcpy(temporary, out_path, length);
    memcpy(temporary + length, ".tmp", 5);
    return temporary;
}

/* Overview: load, attach, poll, and detach the CO-RE skeleton while preserving any collection failure. */
static int collect(const struct options *options, struct collector *collector, uint64_t *dropped)
{
    struct ktrace_bpf *skeleton = NULL;
    struct ring_buffer *ring_buffer = NULL;
    struct bpf_link *marker_link = NULL;
    int result = 0;

    result = require_unified_cgroup_hierarchy();
    if (result != 0)
        goto done;
    result = validate_marker_shared_object(options->marker_so_path);
    if (result != 0)
        goto done;
    skeleton = ktrace_bpf__open();
    if (!skeleton) {
        result = -errno;
        goto done;
    }
    result = bpf_program__set_type(skeleton->progs.trace_marker, BPF_PROG_TYPE_KPROBE);
    if (result != 0)
        goto done;
    bpf_program__set_autoattach(skeleton->progs.trace_marker, false);
    skeleton->rodata->target_cgroup_id = options->cgroup_id;
    result = ktrace_bpf__load(skeleton);
    if (result != 0)
        goto done;
    result = populate_write_watchlist(skeleton, options);
    if (result != 0)
        goto done;
    ring_buffer = ring_buffer__new(bpf_map__fd(skeleton->maps.events), handle_ring_event, collector, NULL);
    if (!ring_buffer) {
        result = -errno;
        goto done;
    }
    result = ktrace_bpf__attach(skeleton);
    if (result != 0)
        goto done;
    result = attach_marker(skeleton, options, &marker_link);
    if (result != 0)
        goto done;
    result = poll_events(ring_buffer, collector, options->duration_s);
    if (result == 0)
        result = read_dropped_events(skeleton, dropped);

done:
    bpf_link__destroy(marker_link);
    ring_buffer__free(ring_buffer);
    ktrace_bpf__destroy(skeleton);
    return result;
}

/* Overview: provide the standalone C libbpf CLI required to collect one cgroup-scoped KEvent artifact. */
int main(int argc, char **argv)
{
    struct collector collector = {.next_seq = 1};
    struct options options;
    char *temporary_path;
    uint64_t dropped = 0;
    int result;

    if (parse_options(argc, argv, &options) != 0) {
        free_options(&options);
        print_usage(argv[0]);
        return 2;
    }
    temporary_path = temporary_path_for(options.out_path);
    if (!temporary_path) {
        fprintf(stderr, "syncfuzz-ktrace: cannot allocate temporary path\n");
        free_options(&options);
        return 1;
    }
    collector.output = fopen(temporary_path, "w");
    if (!collector.output) {
        fprintf(stderr, "syncfuzz-ktrace: cannot open %s: %s\n", temporary_path, strerror(errno));
        free(temporary_path);
        free_options(&options);
        return 1;
    }
    signal(SIGINT, request_stop);
    signal(SIGTERM, request_stop);
    result = collect(&options, &collector, &dropped);
    if (fclose(collector.output) != 0 && result == 0)
        result = -errno;
    if (result == 0)
        result = syncfuzz_update_manifest_dropped_events(options.manifest_path, dropped);
    if (result == 0 && dropped != 0)
        result = -EOVERFLOW;
    if (result == 0 && rename(temporary_path, options.out_path) != 0)
        result = -errno;
    if (result != 0) {
        remove(temporary_path);
        if (dropped != 0)
            fprintf(stderr, "syncfuzz-ktrace: dropped_events=%" PRIu64 "\n", dropped);
        if (result == -EOPNOTSUPP)
            fprintf(stderr, "syncfuzz-ktrace: cgroup v2 unified hierarchy is required\n");
        fprintf(stderr, "syncfuzz-ktrace: collection failed: %s\n", strerror(-result));
    } else {
        fprintf(stderr, "syncfuzz-ktrace: dropped_events=%" PRIu64 "\n", dropped);
    }
    free(temporary_path);
    free_options(&options);
    return result == 0 ? 0 : 1;
}
