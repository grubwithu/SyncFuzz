// SPDX-License-Identifier: GPL-2.0
/* M1 manifest updater: lossless, cJSON-validated replacement of the drop counter. */

#define _GNU_SOURCE

#include <ctype.h>
#include <errno.h>
#include <fcntl.h>
#include <inttypes.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/file.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>

#include "cJSON.h"
#include "manifest.h"

enum manifest_field {
    MANIFEST_RUN_ID,
    MANIFEST_SCHEMA_VERSION,
    MANIFEST_STARTED_WALL_NS,
    MANIFEST_CLOCK_NAME,
    MANIFEST_KERNEL_RELEASE,
    MANIFEST_IMAGE_DIGEST,
    MANIFEST_LANGGRAPH_VERSION,
    MANIFEST_MILESTONE,
    MANIFEST_DROPPED_EVENTS,
    MANIFEST_ORPHAN_RATE,
    MANIFEST_MEMO_HIT_RATE,
    MANIFEST_PRUNE,
    MANIFEST_FIELD_COUNT,
};

enum prune_field {
    PRUNE_RESOLVE_SITES_TOTAL,
    PRUNE_AFTER_WRITABLE_PRUNE,
    PRUNE_COMPONENTS,
    PRUNE_PAIRS_BEFORE,
    PRUNE_PAIRS_AFTER,
    PRUNE_TRUNCATED_COMPONENTS,
    PRUNE_FIELD_COUNT,
};

/* Overview: map one exact RunManifest key to its closed schema index without accepting unknown fields. */
static int manifest_field_index(const char *name)
{
    static const char *const names[MANIFEST_FIELD_COUNT] = {
        "run_id", "schema_version", "started_wall_ns", "clock_name", "kernel_release",
        "image_digest", "langgraph_version", "milestone", "dropped_events", "orphan_rate",
        "memo_hit_rate", "prune",
    };
    size_t index;

    if (!name)
        return -1;
    for (index = 0; index < MANIFEST_FIELD_COUNT; index += 1) {
        if (strcmp(name, names[index]) == 0)
            return (int)index;
    }
    return -1;
}

/* Overview: map one exact PruneStats key to its closed schema index without accepting unknown fields. */
static int prune_field_index(const char *name)
{
    static const char *const names[PRUNE_FIELD_COUNT] = {
        "resolve_sites_total", "after_writable_prune", "components", "pairs_before",
        "pairs_after", "truncated_components",
    };
    size_t index;

    if (!name)
        return -1;
    for (index = 0; index < PRUNE_FIELD_COUNT; index += 1) {
        if (strcmp(name, names[index]) == 0)
            return (int)index;
    }
    return -1;
}

/* Overview: verify the frozen P0-to-P6 milestone enum rather than accepting arbitrary strings. */
static bool is_valid_milestone(const cJSON *item)
{
    static const char *const milestones[] = {"P0", "P1", "P2", "P3", "P4", "P5", "P6"};
    const char *value = cJSON_GetStringValue(item);
    size_t index;

    if (!value)
        return false;
    for (index = 0; index < sizeof(milestones) / sizeof(milestones[0]); index += 1) {
        if (strcmp(value, milestones[index]) == 0)
            return true;
    }
    return false;
}

/* Overview: accept exactly the nullable string shape used by optional manifest metadata. */
static bool is_string_or_null(const cJSON *item)
{
    return cJSON_IsString(item) || cJSON_IsNull(item);
}

/* Overview: validate the complete optional PruneStats object when a manifest includes it. */
static int validate_prune(const cJSON *prune)
{
    bool seen[PRUNE_FIELD_COUNT] = {};
    const cJSON *child;
    size_t index;

    if (!cJSON_IsObject(prune))
        return -EPROTO;
    cJSON_ArrayForEach(child, prune) {
        int field = prune_field_index(child->string);

        if (field < 0 || seen[field] || !cJSON_IsNumber(child))
            return -EPROTO;
        seen[field] = true;
    }
    for (index = 0; index < PRUNE_FIELD_COUNT; index += 1) {
        if (!seen[index])
            return -EPROTO;
    }
    return 0;
}

/* Overview: validate one closed-schema RunManifest field without coercing JSON values or parsing large numbers. */
static bool is_valid_manifest_item(enum manifest_field field, const cJSON *item)
{
    switch (field) {
    case MANIFEST_RUN_ID:
    case MANIFEST_SCHEMA_VERSION:
    case MANIFEST_CLOCK_NAME:
    case MANIFEST_KERNEL_RELEASE:
        return cJSON_IsString(item);
    case MANIFEST_STARTED_WALL_NS:
    case MANIFEST_DROPPED_EVENTS:
        return cJSON_IsNumber(item);
    case MANIFEST_IMAGE_DIGEST:
    case MANIFEST_LANGGRAPH_VERSION:
        return is_string_or_null(item);
    case MANIFEST_MILESTONE:
        return is_valid_milestone(item);
    case MANIFEST_ORPHAN_RATE:
    case MANIFEST_MEMO_HIT_RATE:
        return cJSON_IsNumber(item) || cJSON_IsNull(item);
    case MANIFEST_PRUNE:
        if (cJSON_IsNull(item))
            return true;
        return validate_prune(item) == 0;
    default:
        return false;
    }
}

/* Overview: strictly validate the complete frozen RunManifest object before M1 mutates its sole field. */
static int validate_manifest_fields(const cJSON *manifest)
{
    bool seen[MANIFEST_FIELD_COUNT] = {};
    const cJSON *child;
    size_t index;

    if (!cJSON_IsObject(manifest))
        return -EPROTO;
    cJSON_ArrayForEach(child, manifest) {
        int field = manifest_field_index(child->string);

        if (field < 0)
            return -EPROTO;
        if (seen[field])
            return -EPROTO;
        if (!is_valid_manifest_item((enum manifest_field)field, child))
            return -EPROTO;
        seen[field] = true;
    }
    for (index = 0; index < MANIFEST_FIELD_COUNT; index += 1) {
        if (!seen[index])
            return -EPROTO;
    }
    return 0;
}

/* Overview: parse a whole JSON byte sequence with cJSON and reject trailing bytes or embedded NULs. */
static int validate_manifest_document(const char *contents, size_t length)
{
    const char *parse_end = NULL;
    const char *cursor;
    cJSON *manifest;
    int result;

    if (memchr(contents, '\0', length) != NULL)
        return -EPROTO;
    manifest = cJSON_ParseWithLengthOpts(contents, length, &parse_end, 0);
    if (!manifest || !parse_end) {
        cJSON_Delete(manifest);
        return -EPROTO;
    }
    cursor = parse_end;
    while (cursor < contents + length && isspace((unsigned char)*cursor))
        cursor += 1;
    if (cursor != contents + length) {
        cJSON_Delete(manifest);
        return -EPROTO;
    }
    result = validate_manifest_fields(manifest);
    cJSON_Delete(manifest);
    return result;
}

/* Overview: read one regular manifest file completely while preserving its bytes and permission mode. */
static int read_manifest(const char *path, char **contents, size_t *length, mode_t *mode)
{
    struct stat metadata;
    char *buffer = NULL;
    size_t offset = 0;
    int descriptor;
    int result = 0;

    descriptor = open(path, O_RDONLY | O_CLOEXEC);
    if (descriptor < 0)
        return -errno;
    if (fstat(descriptor, &metadata) != 0) {
        result = -errno;
        goto done;
    }
    if (!S_ISREG(metadata.st_mode) || metadata.st_size < 0) {
        result = -EPROTO;
        goto done;
    }
    if ((uintmax_t)metadata.st_size > SIZE_MAX - 1) {
        result = -EFBIG;
        goto done;
    }
    buffer = malloc((size_t)metadata.st_size + 1);
    if (!buffer) {
        result = -ENOMEM;
        goto done;
    }
    while (offset < (size_t)metadata.st_size) {
        ssize_t read_count = read(descriptor, buffer + offset, (size_t)metadata.st_size - offset);

        if (read_count < 0 && errno == EINTR)
            continue;
        if (read_count <= 0) {
            result = read_count == 0 ? -EIO : -errno;
            goto done;
        }
        offset += (size_t)read_count;
    }
    buffer[offset] = '\0';
    *contents = buffer;
    *length = offset;
    *mode = metadata.st_mode;
    buffer = NULL;

done:
    free(buffer);
    if (close(descriptor) != 0 && result == 0)
        result = -errno;
    return result;
}

/* Overview: allocate one sibling pathname suffix without modifying the caller-owned artifact path. */
static char *path_with_suffix(const char *path, const char *suffix)
{
    size_t path_length = strlen(path);
    size_t suffix_length = strlen(suffix);
    char *result;

    if (path_length > SIZE_MAX - suffix_length - 1)
        return NULL;
    result = malloc(path_length + suffix_length + 1);
    if (!result)
        return NULL;
    memcpy(result, path, path_length);
    memcpy(result + path_length, suffix, suffix_length + 1);
    return result;
}

/* Overview: advance over JSON whitespace while preserving the original byte offsets for token replacement. */
static const char *skip_whitespace(const char *cursor, const char *end)
{
    while (cursor < end && isspace((unsigned char)*cursor))
        cursor += 1;
    return cursor;
}

/* Overview: advance over one validated JSON string without interpreting its escape payload. */
static int skip_json_string(const char **cursor, const char *end)
{
    const char *position = *cursor;

    if (position >= end || *position != '"')
        return -EPROTO;
    position += 1;
    while (position < end) {
        if (*position == '"') {
            *cursor = position + 1;
            return 0;
        }
        if (*position == '\\') {
            position += 1;
            if (position >= end)
                return -EPROTO;
        } else if ((unsigned char)*position < 0x20) {
            return -EPROTO;
        }
        position += 1;
    }
    return -EPROTO;
}

/* Overview: skip a primitive cJSON-validated token until its containing JSON delimiter. */
static int skip_json_primitive(const char **cursor, const char *end)
{
    const char *position = *cursor;
    const char *start = position;

    while (position < end && *position != ',' && *position != '}' && *position != ']' &&
           !isspace((unsigned char)*position))
        position += 1;
    if (position == start)
        return -EPROTO;
    *cursor = position;
    return 0;
}

/* Overview: skip a nested cJSON-validated object or array while ignoring delimiters embedded in strings. */
static int skip_json_container(const char **cursor, const char *end)
{
    const char *position = *cursor;
    int depth = 0;

    while (position < end) {
        if (*position == '"') {
            int result;

            result = skip_json_string(&position, end);
            if (result != 0)
                return result;
            continue;
        }
        if (*position == '{' || *position == '[')
            depth += 1;
        if (*position == '}' || *position == ']')
            depth -= 1;
        position += 1;
        if (depth == 0) {
            *cursor = position;
            return 0;
        }
    }
    return -EPROTO;
}

/* Overview: skip one cJSON-validated value only to find the next top-level manifest key boundary. */
static int skip_json_value(const char **cursor, const char *end)
{
    const char *position = skip_whitespace(*cursor, end);

    if (position >= end)
        return -EPROTO;
    if (*position == '"') {
        *cursor = position;
        return skip_json_string(cursor, end);
    }
    *cursor = position;
    if (*position == '{' || *position == '[')
        return skip_json_container(cursor, end);
    return skip_json_primitive(cursor, end);
}

/* Overview: compare one raw JSON member key to the canonical unescaped dropped_events spelling. */
static bool is_dropped_events_key(const char *start, const char *end)
{
    static const char key[] = "\"dropped_events\"";

    return (size_t)(end - start) == sizeof(key) - 1 && memcmp(start, key, sizeof(key) - 1) == 0;
}

/* Overview: consume one non-negative JSON decimal integer token without accepting signs, fractions, or exponents. */
static int consume_decimal_token(const char **cursor, const char *end)
{
    const char *position = *cursor;

    if (position >= end || *position < '0' || *position > '9')
        return -EPROTO;
    if (*position == '0') {
        *cursor = position + 1;
        return 0;
    }
    while (position < end && *position >= '0' && *position <= '9')
        position += 1;
    *cursor = position;
    return 0;
}

/* Overview: consume one top-level object delimiter and report whether the validated object has ended. */
static int advance_member_delimiter(const char **cursor, const char *end, bool *finished)
{
    const char *position = skip_whitespace(*cursor, end);

    if (position >= end)
        return -EPROTO;
    if (*position == ',') {
        *cursor = position + 1;
        *finished = false;
        return 0;
    }
    if (*position != '}')
        return -EPROTO;
    position += 1;
    if (skip_whitespace(position, end) != end)
        return -EPROTO;
    *cursor = position;
    *finished = true;
    return 0;
}

/* Overview: read one top-level JSON key and position the cursor at its following value without decoding either. */
static int read_member_prefix(const char **cursor, const char *end,
                              const char **key_start, const char **key_end)
{
    const char *position = skip_whitespace(*cursor, end);
    int result;

    if (position >= end || *position != '"')
        return -EPROTO;
    *key_start = position;
    result = skip_json_string(&position, end);
    if (result != 0)
        return result;
    *key_end = position;
    position = skip_whitespace(position, end);
    if (position >= end || *position != ':')
        return -EPROTO;
    *cursor = skip_whitespace(position + 1, end);
    return *cursor < end ? 0 : -EPROTO;
}

/* Overview: record the sole decimal dropped_events token or skip a different cJSON-validated member value. */
static int locate_member_value(const char **cursor, const char *end,
                               const char *key_start, const char *key_end, bool *found,
                               const char **token_start, const char **token_end)
{
    int result;

    if (!is_dropped_events_key(key_start, key_end))
        return skip_json_value(cursor, end);
    if (*found)
        return -EPROTO;
    *token_start = *cursor;
    result = consume_decimal_token(cursor, end);
    if (result != 0)
        return result;
    *token_end = *cursor;
    *found = true;
    return 0;
}

/* Overview: locate the only legal non-negative decimal dropped_events token in the original JSON bytes. */
static int locate_dropped_events(const char *contents, size_t length,
                                 const char **token_start, const char **token_end)
{
    const char *cursor = skip_whitespace(contents, contents + length);
    const char *end = contents + length;
    bool found = false;

    if (cursor >= end)
        return -EPROTO;
    if (*cursor != '{')
        return -EPROTO;
    cursor += 1;
    while (true) {
        bool finished;
        const char *key_start;
        const char *key_end;
        int result;

        cursor = skip_whitespace(cursor, end);
        if (cursor >= end)
            return -EPROTO;
        if (*cursor == '}')
            return found ? 0 : -EPROTO;
        result = read_member_prefix(&cursor, end, &key_start, &key_end);
        if (result != 0)
            return result;
        result = locate_member_value(&cursor, end, key_start, key_end, &found, token_start, token_end);
        if (result != 0)
            return result;
        result = advance_member_delimiter(&cursor, end, &finished);
        if (result != 0)
            return result;
        if (!finished)
            continue;
        return found ? 0 : -EPROTO;
    }
}

/* Overview: replace only the located decimal counter token while preserving every other manifest byte. */
static int replace_dropped_events(const char *contents, size_t length,
                                  const char *token_start, const char *token_end,
                                  uint64_t dropped_events, char **updated, size_t *updated_length)
{
    char value[32];
    size_t prefix_length = (size_t)(token_start - contents);
    size_t suffix_length = length - (size_t)(token_end - contents);
    int value_length = snprintf(value, sizeof(value), "%" PRIu64, dropped_events);
    char *replacement;

    if (value_length < 0 || (size_t)value_length >= sizeof(value) ||
        prefix_length > SIZE_MAX - suffix_length - (size_t)value_length - 1)
        return -EOVERFLOW;
    replacement = malloc(prefix_length + (size_t)value_length + suffix_length + 1);
    if (!replacement)
        return -ENOMEM;
    memcpy(replacement, contents, prefix_length);
    memcpy(replacement + prefix_length, value, (size_t)value_length);
    memcpy(replacement + prefix_length + (size_t)value_length, token_end, suffix_length);
    replacement[prefix_length + (size_t)value_length + suffix_length] = '\0';
    *updated = replacement;
    *updated_length = prefix_length + (size_t)value_length + suffix_length;
    return 0;
}

/* Overview: fsync the containing directory so the already-renamed manifest name is durable after a crash. */
static int fsync_parent_directory(const char *path)
{
    char *directory = strdup(path);
    char *slash;
    int descriptor;
    int result;

    if (!directory)
        return -ENOMEM;
    slash = strrchr(directory, '/');
    if (!slash) {
        free(directory);
        directory = strdup(".");
        if (!directory)
            return -ENOMEM;
    } else if (slash == directory) {
        slash[1] = '\0';
    } else {
        *slash = '\0';
    }
    descriptor = open(directory, O_RDONLY | O_DIRECTORY | O_CLOEXEC);
    free(directory);
    if (descriptor < 0)
        return -errno;
    result = fsync(descriptor) == 0 ? 0 : -errno;
    close(descriptor);
    return result;
}

/* Overview: atomically publish validated replacement bytes through a same-directory temporary file and rename. */
static int write_manifest_atomically(const char *path, const char *contents, size_t length, mode_t mode)
{
    char *temporary_path = path_with_suffix(path, ".tmp.XXXXXX");
    size_t offset = 0;
    int descriptor;
    int result = 0;

    if (!temporary_path)
        return -ENOMEM;
    descriptor = mkstemp(temporary_path);
    if (descriptor < 0) {
        free(temporary_path);
        return -errno;
    }
    if (fchmod(descriptor, mode & 0777) != 0) {
        result = -errno;
        goto close_temporary;
    }
    while (offset < length) {
        ssize_t written = write(descriptor, contents + offset, length - offset);

        if (written < 0) {
            if (errno == EINTR)
                continue;
            result = -errno;
            goto close_temporary;
        }
        if (written == 0) {
            result = -EIO;
            goto close_temporary;
        }
        offset += (size_t)written;
    }
    if (fsync(descriptor) != 0) {
        result = -errno;
        goto close_temporary;
    }

close_temporary:
    if (close(descriptor) != 0 && result == 0)
        result = -errno;
    if (result != 0)
        goto discard_temporary;
    if (rename(temporary_path, path) != 0) {
        result = -errno;
        goto discard_temporary;
    }
    if (result == 0)
        result = fsync_parent_directory(path);

discard_temporary:
    if (result != 0)
        unlink(temporary_path);
    free(temporary_path);
    return result;
}

/* Overview: atomically update only dropped_events after locked strict validation and lossless token replacement. */
int syncfuzz_update_manifest_dropped_events(const char *manifest_path, uint64_t dropped_events)
{
    char *lock_path;
    char *contents = NULL;
    char *updated = NULL;
    const char *token_start;
    const char *token_end;
    size_t length;
    size_t updated_length;
    mode_t mode;
    int lock_descriptor;
    int result;

    if (!manifest_path || manifest_path[0] == '\0')
        return -EINVAL;
    lock_path = path_with_suffix(manifest_path, ".lock");
    if (!lock_path)
        return -ENOMEM;
    lock_descriptor = open(lock_path, O_CREAT | O_RDWR | O_CLOEXEC, 0600);
    free(lock_path);
    if (lock_descriptor < 0)
        return -errno;
    do {
        result = flock(lock_descriptor, LOCK_EX) == 0 ? 0 : -errno;
    } while (result == -EINTR);
    if (result != 0)
        goto done;
    result = read_manifest(manifest_path, &contents, &length, &mode);
    if (result != 0)
        goto done;
    result = validate_manifest_document(contents, length);
    if (result != 0)
        goto done;
    result = locate_dropped_events(contents, length, &token_start, &token_end);
    if (result != 0)
        goto done;
    result = replace_dropped_events(contents, length, token_start, token_end, dropped_events,
                                    &updated, &updated_length);
    if (result != 0)
        goto done;
    result = validate_manifest_document(updated, updated_length);
    if (result == 0)
        result = write_manifest_atomically(manifest_path, updated, updated_length, mode);

done:
    free(updated);
    free(contents);
    close(lock_descriptor);
    return result;
}
