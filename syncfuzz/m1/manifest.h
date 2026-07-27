#ifndef SYNCFUZZ_M1_MANIFEST_H
#define SYNCFUZZ_M1_MANIFEST_H

#include <stdint.h>

/* Overview: atomically replace only manifest dropped_events after strict validation under the shared lock. */
int syncfuzz_update_manifest_dropped_events(const char *manifest_path, uint64_t dropped_events);

#endif /* SYNCFUZZ_M1_MANIFEST_H */
