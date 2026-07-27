// SPDX-License-Identifier: GPL-2.0
/* Deterministic syscall workload used only by M1's privileged acceptance test. */

#define _GNU_SOURCE

#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <linux/openat2.h>
#include <sched.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mount.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <sys/un.h>
#include <sys/wait.h>
#include <sys/xattr.h>
#include <unistd.h>

extern char **environ;
extern void sf_mark(const char *json_payload);

/* Overview: convert one syscall failure into a stable negative errno result for the acceptance caller. */
static int negative_errno(void)
{
    return errno == 0 ? -EIO : -errno;
}

/* Overview: close one descriptor while preserving the first operation failure seen by the workload. */
static int close_preserving_result(int descriptor, int result)
{
    if (close(descriptor) != 0 && result == 0)
        return negative_errno();
    return result;
}

/* Overview: write a complete small fixture payload while handling short writes without retrying other operations. */
static int write_payload(int descriptor, const char *payload)
{
    size_t offset = 0;
    size_t length = strlen(payload);

    while (offset < length) {
        ssize_t written = write(descriptor, payload + offset, length - offset);

        if (written <= 0)
            return negative_errno();
        offset += (size_t)written;
    }
    return 0;
}

/* Overview: require a deliberately absent openat target to fail with ENOENT for the frozen shadowing case. */
static int expect_missing_openat(int directory_fd, const char *name)
{
    int descriptor = openat(directory_fd, name, O_RDONLY | O_CLOEXEC);

    if (descriptor >= 0) {
        close(descriptor);
        return -EEXIST;
    }
    return errno == ENOENT ? 0 : negative_errno();
}

/* Overview: require a deliberately absent openat2 target to fail with ENOENT using the raw syscall ABI. */
static int expect_missing_openat2(int directory_fd, const char *name)
{
    struct open_how how = {.flags = O_RDONLY | O_CLOEXEC};
    int descriptor = (int)syscall(SYS_openat2, directory_fd, name, &how, sizeof(how));

    if (descriptor >= 0) {
        close(descriptor);
        return -EEXIST;
    }
    return errno == ENOENT ? 0 : negative_errno();
}

/* Overview: create one fixture through openat O_CREAT and write it to exercise bind and resolve evidence. */
static int create_fixture(int directory_fd)
{
    int descriptor = openat(directory_fd, "created", O_CREAT | O_RDWR | O_TRUNC | O_CLOEXEC, 0600);
    int result;

    if (descriptor < 0)
        return negative_errno();
    result = write_payload(descriptor, "created");
    return close_preserving_result(descriptor, result);
}

/* Overview: open the created fixture through openat2 so its success path carries a file identity. */
static int open_fixture_openat2(int directory_fd)
{
    struct open_how how = {.flags = O_RDONLY | O_CLOEXEC};
    int descriptor = (int)syscall(SYS_openat2, directory_fd, "created", &how, sizeof(how));

    if (descriptor < 0)
        return negative_errno();
    return close_preserving_result(descriptor, 0);
}

/* Overview: write a pre-created watch target so M1's inode watchlist filter emits a raw write event. */
static int write_watch_target(int directory_fd)
{
    int descriptor = openat(directory_fd, "watch-target", O_WRONLY | O_TRUNC | O_CLOEXEC);
    int result;

    if (descriptor < 0)
        return negative_errno();
    result = write_payload(descriptor, "watched");
    return close_preserving_result(descriptor, result);
}

/* Overview: perform directory and name-rebinding operations while preserving each syscall's raw trace evidence. */
static int mutate_fixture_names(int directory_fd)
{
    if (mkdirat(directory_fd, "dir", 0700) != 0)
        return negative_errno();
    if (syscall(SYS_renameat2, directory_fd, "created", directory_fd, "renamed", 0) != 0)
        return negative_errno();
    if (linkat(directory_fd, "renamed", directory_fd, "hard-link", 0) != 0)
        return negative_errno();
    return symlinkat("renamed", directory_fd, "symbolic-link") == 0 ? 0 : negative_errno();
}

/* Overview: resolve symlink and inode metadata through their explicit at-style syscalls without path reconstruction. */
static int resolve_fixture_names(int directory_fd)
{
    char target[32];
    struct stat metadata;

    if (readlinkat(directory_fd, "symbolic-link", target, sizeof(target)) < 0)
        return negative_errno();
    return syscall(SYS_newfstatat, directory_fd, "renamed", &metadata, 0) == 0 ? 0 : negative_errno();
}

/* Overview: apply mode and xattr changes through their fd-specific syscalls and retain failures as traceable effects. */
static int change_fixture_metadata(int directory_fd)
{
    int descriptor;
    int result = 0;

    if (syscall(SYS_fchmodat, directory_fd, "renamed", 0600) != 0)
        return negative_errno();
    descriptor = openat(directory_fd, "renamed", O_RDONLY | O_CLOEXEC);
    if (descriptor < 0)
        return negative_errno();
    if (fsetxattr(descriptor, "user.syncfuzz", "1", 1, 0) != 0)
        result = negative_errno();
    return close_preserving_result(descriptor, result);
}

/* Overview: bind, listen on, and connect to a Unix socket so all socket lifecycle hooks receive one event. */
static int exercise_unix_socket(const char *root)
{
    struct sockaddr_un address = {.sun_family = AF_UNIX};
    int listener;
    int client;
    int result = 0;

    if (snprintf(address.sun_path, sizeof(address.sun_path), "%s/socket", root) >=
        (int)sizeof(address.sun_path))
        return -ENAMETOOLONG;
    listener = socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
    if (listener < 0)
        return negative_errno();
    if (bind(listener, (const struct sockaddr *)&address, sizeof(address)) != 0)
        result = negative_errno();
    if (result == 0 && listen(listener, 1) != 0)
        result = negative_errno();
    client = result == 0 ? socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0) : -1;
    if (client < 0 && result == 0)
        result = negative_errno();
    if (result == 0 && connect(client, (const struct sockaddr *)&address, sizeof(address)) != 0)
        result = negative_errno();
    if (client >= 0)
        result = close_preserving_result(client, result);
    result = close_preserving_result(listener, result);
    unlink(address.sun_path);
    return result;
}

/* Overview: invoke a mount syscall expected to fail in the container while retaining its raw bind-site trace. */
static int attempt_unprivileged_mount(const char *root)
{
    char target[PATH_MAX];

    if (snprintf(target, sizeof(target), "%s/mount-target", root) >= (int)sizeof(target))
        return -ENAMETOOLONG;
    if (mkdir(target, 0700) != 0 && errno != EEXIST)
        return negative_errno();
    if (mount("none", target, "tmpfs", 0, "size=4096") == 0) {
        umount(target);
        return 0;
    }
    return (errno == EPERM || errno == EACCES) ? 0 : negative_errno();
}

/* Overview: wait for one child and surface abnormal termination without conflating it with trace collection. */
static int wait_for_child(pid_t child)
{
    int status;

    if (waitpid(child, &status, 0) < 0)
        return negative_errno();
    return WIFEXITED(status) && WEXITSTATUS(status) == 0 ? 0 : -ECHILD;
}

/* Overview: fork and execve a fixed binary to exercise process-fork, exec, and exit telemetry. */
static int exercise_execve(void)
{
    char *const argv[] = {"/bin/true", NULL};
    pid_t child = fork();

    if (child < 0)
        return negative_errno();
    if (child == 0) {
        execve(argv[0], argv, environ);
        _exit(127);
    }
    return wait_for_child(child);
}

/* Overview: fork and invoke execveat with a fixed binary to cover the separate raw syscall hook. */
static int exercise_execveat(void)
{
    char *const argv[] = {"/bin/true", NULL};
    pid_t child = fork();

    if (child < 0)
        return negative_errno();
    if (child == 0) {
        syscall(SYS_execveat, AT_FDCWD, argv[0], argv, environ, 0);
        _exit(127);
    }
    return wait_for_child(child);
}

/* Overview: remove all fixture names through unlinkat so M1 records the cleanup bind operations explicitly. */
static int remove_fixture_names(int directory_fd)
{
    const char *const files[] = {"hard-link", "symbolic-link", "renamed", "watch-target"};
    size_t index;

    for (index = 0; index < sizeof(files) / sizeof(files[0]); index += 1) {
        if (unlinkat(directory_fd, files[index], 0) != 0)
            return negative_errno();
    }
    return unlinkat(directory_fd, "dir", AT_REMOVEDIR) == 0 ? 0 : negative_errno();
}

/* Overview: execute the fixed twenty-plus-operation workload inside the target cgroup and stop at the first fault. */
int main(int argc, char **argv)
{
    char marker_payload[] = "{\"phase\":\"acceptance\"}";
    int directory_fd;
    int result;

    if (argc != 2)
        return 2;
    sf_mark(marker_payload);
    directory_fd = open(argv[1], O_RDONLY | O_DIRECTORY | O_CLOEXEC);
    if (directory_fd < 0)
        return 1;
    result = expect_missing_openat(directory_fd, "missing-openat-one");
    if (result == 0)
        result = expect_missing_openat2(directory_fd, "missing-openat2-two");
    if (result == 0)
        result = expect_missing_openat(directory_fd, "missing-openat-three");
    if (result == 0)
        result = create_fixture(directory_fd);
    if (result == 0)
        result = open_fixture_openat2(directory_fd);
    if (result == 0)
        result = write_watch_target(directory_fd);
    if (result == 0)
        result = mutate_fixture_names(directory_fd);
    if (result == 0)
        result = resolve_fixture_names(directory_fd);
    if (result == 0)
        result = change_fixture_metadata(directory_fd);
    if (result == 0)
        result = exercise_unix_socket(argv[1]);
    if (result == 0)
        result = attempt_unprivileged_mount(argv[1]);
    if (result == 0)
        result = exercise_execve();
    if (result == 0)
        result = exercise_execveat();
    if (result == 0)
        result = remove_fixture_names(directory_fd);
    result = close_preserving_result(directory_fd, result);
    return result == 0 ? 0 : 1;
}
