# C ABI contract (planned v1)

**Status:** design only. The Milestone 0 header defines the shared vocabulary
(version, status, limits, error detail) but exports no functions. This document
is the contract the implementation will follow.

## 1. Language and visibility

- Public header: `include/hrtcold/hrtcold.h`, C11 and C++17 compatible.
- `extern "C"` around every exported declaration.
- C ABI version: `HRTCOLD_ABI_VERSION` (currently `1`).
- Symbols will be exported with an `HRTCOLD_API` visibility macro. The M0
  interface target exports no symbols.
- Consumers must not depend on struct padding beyond the declared fields.

## 2. Calling convention and ownership

- All functions are synchronous and return `hrtcold_status`.
- `HRTCOLD_OK` is zero; every other value is a failure.
- Input buffers are borrowed for the duration of the call. The library never
  retains a caller pointer after returning.
- Output buffers are owned by the caller and freed with the matching function.
- Strings written by the library are NUL-terminated and truncated rather than
  overflowing a fixed buffer; truncation is reported as `BUFFER_TOO_SMALL`
  where the API requires the full value.
- On failure, outputs are left empty or unchanged. No plaintext output is
  produced after a failed validation, integrity, signature, or replay check.
- `hrtcold_error` is caller-owned. `message` is always NUL-terminated after a
  call. Error messages never contain passphrases, private-key material, or
  decrypted records.

## 3. Limits and defaults

`hrtcold_limits_default()` sets:

| Limit | Default |
| --- | ---: |
| container bytes | 16 MiB |
| bundle bytes | 15 MiB |
| JSON depth | 64 |
| JSON string bytes | 1 MiB |

Callers may lower limits. Raising them changes the accepted format and needs a
future interface decision.

## 4. Planned types

```c
typedef enum hrtcold_replay_status {
    HRTCOLD_REPLAY_DISABLED = 0,
    HRTCOLD_REPLAY_NEW = 1,
    HRTCOLD_REPLAY_REPEAT = 2,
    HRTCOLD_REPLAY_NEWER = 3,
    HRTCOLD_REPLAY_OLDER_ALLOWED = 4,
    HRTCOLD_REPLAY_CONFLICT_ALLOWED = 5
} hrtcold_replay_status;

typedef struct hrtcold_buffer {
    uint8_t *data;
    size_t len;
} hrtcold_buffer;

typedef struct hrtcold_container_info {
    uint16_t version;
    uint16_t flags;
    uint64_t payload_length;
    uint8_t payload_sha256[32];
} hrtcold_container_info;

typedef struct hrtcold_encrypt_options {
    const char *recipient_public_armor;
    const char *signer_private_armor;      /* NULL = unsigned */
    const uint8_t *signer_passphrase;      /* may be NULL */
    size_t signer_passphrase_len;
    const hrtcold_limits *limits;          /* NULL = defaults */
} hrtcold_encrypt_options;

typedef struct hrtcold_decrypt_options {
    const char *private_armor;
    const uint8_t *passphrase;
    size_t passphrase_len;
    const char *verify_public_armor;       /* NULL = no provenance claim */
    int allow_unsigned;                    /* explicit opt-in */
    int allow_older;                       /* replay override */
    int no_replay_check;                   /* deliberate restore */
    const char *replay_state_path;         /* NULL = platform default */
    const hrtcold_limits *limits;          /* NULL = defaults */
} hrtcold_decrypt_options;

typedef struct hrtcold_verify_result {
    hrtcold_buffer plaintext;
    int signature_present;
    int signature_verified;
    char signer_fingerprint[HRTCOLD_FINGERPRINT_BUF_LEN];
    char signer_primary_fingerprint[HRTCOLD_FINGERPRINT_BUF_LEN];
    char content_sha256[HRTCOLD_SHA256_BUF_LEN];
    char bundle_created_at[HRTCOLD_TIMESTAMP_BUF_LEN];
    hrtcold_replay_status replay_status;
} hrtcold_verify_result;
```

`plaintext` is allocated by the library and released with
`hrtcold_verify_result_free()`. Freeing best-effort zeroizes the buffer before
returning it to the allocator; this is not a claim of physical erasure.

## 5. Planned entry points

These are the planned v1 signatures. They are not in the M0 header yet.

```c
hrtcold_status hrtcold_container_inspect(
    const uint8_t *data, size_t len,
    hrtcold_container_info *out,
    hrtcold_error *err);

hrtcold_status hrtcold_bundle_validate(
    const uint8_t *json, size_t len,
    const hrtcold_limits *limits,
    hrtcold_error *err);

hrtcold_status hrtcold_encrypt(
    const hrtcold_encrypt_options *options,
    const uint8_t *bundle, size_t bundle_len,
    hrtcold_buffer *out,
    hrtcold_error *err);

hrtcold_status hrtcold_decrypt_verify(
    const hrtcold_decrypt_options *options,
    const uint8_t *container, size_t container_len,
    hrtcold_verify_result *out,
    hrtcold_error *err);

void hrtcold_buffer_free(hrtcold_buffer *buffer);
void hrtcold_verify_result_free(hrtcold_verify_result *result);
```

### Behaviour notes

- `container_inspect` validates the 16-byte header and bounds, then the outer
  OpenPGP sequence. It never decrypts and never claims a verified recipient.
- `bundle_validate` validates only the envelope. Record semantics remain the
  website importer's responsibility.
- `encrypt` signs the plaintext first when `signer_private_armor` is non-NULL,
  then encrypts. Omitted signer fields produce an unsigned backup; the caller
  is responsible for surfacing that downgrade.
- `decrypt_verify` requires a genuinely encrypted message. With
  `verify_public_armor`, an unsigned message or a different signer is an error.
  Without it, the caller must set `allow_unsigned` to proceed and must not claim
  provenance.
- Replay checking is active only for verified backups. `allow_older` overrides
  an older or same-time-different result without lowering the high-water mark.
- Atomic output publication is not part of these in-memory entry points; a
  separate file API will own temp/fsync/no-replace semantics.

## 6. Threading

- No mutable global state. Calls are independent unless they share an output
  object.
- A single result/options object must not be used concurrently.
- Crypto-backend one-time initialisation, if any, must be thread-safe and
  documented by the backend.
- The library does not install signal handlers, atexit hooks, or global
  locale changes.

## 7. Error model

- The status enum is stable for ABI v1. New codes may be appended.
- `BUFFER_TOO_SMALL` means a fixed output buffer could not hold the complete
  value; no partial secret is returned.
- `INVALID_CONTAINER` and `INVALID_BUNDLE` are distinct so callers can produce
  the right user-facing language.
- `SIGNATURE` and `REPLAY` are distinct; replay is not a cryptographic failure.
- `UNSUPPORTED` covers a version/algorithm the build cannot handle.
- `INTERNAL` is a bug or allocation failure; callers must not assume the
  message is stable across versions.

## 8. Versioning

- Additive functions keep `HRTCOLD_ABI_VERSION`.
- Changing a public struct layout or the meaning of a status requires a new ABI
  version and new entry points. The old symbols remain for one major cycle.
- The package version and ABI version are independent.
- `hrtcold_version_string()` is informational; feature detection uses
  `HRTCOLD_ABI_VERSION` and the presence of entry points.
