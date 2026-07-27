// SPDX-License-Identifier: GPL-2.0

/* Overview: provide the stable M2 marker ABI for the M1 acceptance workload without parsing payload data. */
__attribute__((noinline, visibility("default"))) void sf_mark(const char *json_payload)
{
    (void)json_payload;
}
