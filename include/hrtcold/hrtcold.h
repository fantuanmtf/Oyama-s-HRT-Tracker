/*
 * hrtcold public C ABI — Milestone 0 contract.
 *
 * This header fixes the version, status vocabulary, limits, and error shape for
 * the planned native core. It declares no implemented symbols yet: the working
 * implementation remains the Go CLI in tools/hrt-cold.
 *
 * The function entry points, ownership rules, and threading model are specified
 * in docs/native/ABI.md. They will be added here with the implementation, under
 * the same header and ABI version.
 *
 * Header is C11 and C++17 compatible. No allocation, I/O, or global state is
 * performed by the inline helpers below.
 */
#ifndef HRTCOLD_HRTCOLD_H
#define HRTCOLD_HRTCOLD_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define HRTCOLD_VERSION_MAJOR 0
#define HRTCOLD_VERSION_MINOR 1
#define HRTCOLD_VERSION_PATCH 0
#define HRTCOLD_VERSION_STRING "0.1.0"
#define HRTCOLD_ABI_VERSION 1

/* OpenPGP v4 fingerprints are 20 bytes (40 hex); v6 are 32 bytes (64 hex). */
#define HRTCOLD_FINGERPRINT_HEX_LEN 64
#define HRTCOLD_FINGERPRINT_BUF_LEN (HRTCOLD_FINGERPRINT_HEX_LEN + 1)

/* Lowercase SHA-256 hex, NUL-terminated. */
#define HRTCOLD_SHA256_HEX_LEN 64
#define HRTCOLD_SHA256_BUF_LEN (HRTCOLD_SHA256_HEX_LEN + 1)

/* RFC 3339 timestamp with room for fractional seconds and a terminating NUL. */
#define HRTCOLD_TIMESTAMP_BUF_LEN 40

/*
 * Failure codes are stable for ABI version 1. New codes may be appended;
 * existing values do not change.
 */
typedef enum hrtcold_status {
    HRTCOLD_OK = 0,
    HRTCOLD_ERR_INVALID_ARGUMENT = 1,
    HRTCOLD_ERR_BUFFER_TOO_SMALL = 2,
    HRTCOLD_ERR_INVALID_CONTAINER = 3,
    HRTCOLD_ERR_INVALID_BUNDLE = 4,
    HRTCOLD_ERR_CRYPTO = 5,
    HRTCOLD_ERR_SIGNATURE = 6,
    HRTCOLD_ERR_REPLAY = 7,
    HRTCOLD_ERR_IO = 8,
    HRTCOLD_ERR_UNSUPPORTED = 9,
    HRTCOLD_ERR_INTERNAL = 10
} hrtcold_status;

/*
 * Fixed-size error detail. The library never returns a pointer into its own
 * memory. `message` is always NUL-terminated after a call that reports a
 * non-OK status; it is empty on success.
 */
typedef struct hrtcold_error {
    hrtcold_status code;
    char message[256];
} hrtcold_error;

/*
 * Parser limits. The defaults match the Go CLI:
 * 16 MiB container, 15 MiB bundle, JSON depth 64, 1 MiB per JSON string.
 * Callers may lower them; raising them requires a future format decision.
 */
typedef struct hrtcold_limits {
    uint64_t max_container_bytes;
    uint64_t max_bundle_bytes;
    uint32_t max_json_depth;
    uint32_t max_json_string_bytes;
} hrtcold_limits;

static inline const char *hrtcold_version_string(void) {
    return HRTCOLD_VERSION_STRING;
}

static inline void hrtcold_limits_default(hrtcold_limits *out) {
    if (out == NULL) {
        return;
    }
    out->max_container_bytes = 16u * 1024u * 1024u;
    out->max_bundle_bytes = 15u * 1024u * 1024u;
    out->max_json_depth = 64u;
    out->max_json_string_bytes = 1024u * 1024u;
}

static inline void hrtcold_error_clear(hrtcold_error *err) {
    if (err == NULL) {
        return;
    }
    err->code = HRTCOLD_OK;
    err->message[0] = '\0';
}

#ifdef __cplusplus
} /* extern "C" */
#endif

#endif /* HRTCOLD_HRTCOLD_H */
