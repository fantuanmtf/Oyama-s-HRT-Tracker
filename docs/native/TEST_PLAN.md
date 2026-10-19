# Native core test plan

**Status:** Milestone 0 defines the plan and runs header checks. The native
implementation tests below are future work.

## 1. Principles

- Synthetic data only. No real health records, private keys, passphrases, or
  production backups.
- Test failure paths as carefully as success paths: no plaintext output on
  failure is a security property.
- Differential tests compare native behaviour with the Go CLI and the website
  importer; one implementation passing alone is not parity.
- Sanitizers and fuzzers are required before claiming a native parser is safe to
  ship for testing.
- No test claims production readiness or a cryptographic audit.

## 2. Milestone 0 checks (present)

| Check | Evidence |
| --- | --- |
| CMake configure on Linux and Windows | native CI job |
| C11 header compile/link/run | `hrtcold_header_c` |
| C++17 header compile/link/run | `hrtcold_header_cpp` |
| Shared limit defaults and status values | header tests |
| Install metadata generation | local `cmake --install` smoke test; CI install step planned |
| Sanitizer build on GCC/Clang | Linux CI with `--enable-sanitizers` |
| Warnings as errors | Linux/Windows CI with `--enable-werror` |

## 3. Container parser (M1)

- Valid minimal container; exact payload length.
- Bad magic, unknown version, non-zero flags.
- Zero length, declared length too large, truncation, trailing bytes.
- 16 MiB boundary and one-byte overflow.
- Payload SHA-256 matches the Go implementation.
- Fuzz with arbitrary byte strings; assert no crash, no out-of-bounds read, and
  bounded allocation.

## 4. Bundle envelope validator (M1)

- Valid synthetic bundle and both modes.
- Duplicate JSON keys, depth 64/65, 1 MiB string boundary.
- Missing/unknown format, wrong version, invalid RFC 3339 `createdAt`, invalid
  mode.
- Every category array shape; wrong types; tombstones; calibration enums;
  preference allowlist; scalar types.
- Unknown fields accepted.
- Invalid record values do not become valid through coercion.
- Differential: the same inputs through the Go validator and the website
  parser where their responsibilities overlap.

## 5. Crypto (M2)

- Round-trip encrypt/decrypt with fresh synthetic keys.
- Exact plaintext bytes.
- Wrong private key, wrong passphrase, tampered ciphertext, truncation.
- Literal, compressed-plaintext, signed-only, recipient-less, empty encrypted
  packet with trailing packets, second encrypted packet, marker packets.
- Multi-recipient and hidden-recipient messages.
- Known-answer vectors from the backend and interoperability vectors generated
  by the Go CLI.
- No output file/buffer on failure.

## 6. Signing and replay (M3)

- Sign-then-encrypt with a separate signer key.
- Verify with the pinned public key; reject wrong key, missing signature,
  invalid signature, and unsigned input with `--verify-key`.
- Report signer primary fingerprint and content SHA-256 exactly.
- Replay states: new, repeat, newer, older refused, older allowed, same-time
  different content refused/allowed; override never lowers the mark.
- Corrupt replay state, unknown version, wrong permissions, missing directory.
- Replay state never contains plaintext or secrets.
- Differential: native and Go produce the same replay decisions for the same
  state and inputs.

## 7. I/O and atomic publication (M3)

- Existing output is never replaced.
- Temp file cleanup after failures.
- Hard-link no-replace behaviour on Linux and Windows.
- Passphrase-file mode rejection on Unix.
- Missing parent directory, permission failure, read-only destination.
- Crash simulation: leave a temp/journal file and verify the documented
  recovery/cleanup behaviour. Directory fsync durability remains best-effort.

## 8. Fuzzing and sanitizers

- libFuzzer or AFL++ targets for container parsing and bundle JSON parsing.
- ASan/UBSan on Linux; MSan where supported for parser-only targets.
- Corpus seeded from the synthetic fixture and malformed variants.
- Run fuzzers for a bounded CI smoke time on every parser change, with longer
  runs before a release candidate.
- Never commit crash artifacts containing real data; use synthetic seeds.

## 9. Platform matrix

| Platform | M0 | M1-M3 |
| --- | --- | --- |
| Linux x86_64 (GCC/Clang) | configure/build/header checks | full native tests, ASan/UBSan, fuzz smoke |
| Windows x86_64 (MSVC) | configure/build/header checks | full native tests, native hard-link behaviour |
| macOS | not tested | build/test when a maintainer can run it |
| Other Unix | not tested | community reports, no claim |

## 10. Go/native integration (M4)

- cgo round-trip and error mapping, or subprocess protocol tests if that path is
  chosen.
- Crash isolation: a native abort must not corrupt the Go process or leave a
  claimed-success output.
- Feature parity checklist for `keygen`, `encrypt`, `decrypt`, `inspect`, and
  replay.
- Performance measurements on the same synthetic inputs; report real numbers,
  not assumptions.

## 11. Evidence format

Record:

- exact commit and dependency versions;
- commands run and their output/exit status;
- platform and compiler versions;
- sanitizer/fuzzer configuration and duration;
- known failures or untested paths.

A green test run is evidence for the tested surface, not a security audit or a
production approval.
