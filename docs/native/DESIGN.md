# Native core design

## 1. Context

`tools/hrt-cold` currently implements the offline cold-backup workflow in Go:
container validation, bundle-envelope validation, OpenPGP encrypt/decrypt,
pinned signature verification, a local replay high-water mark, and atomic
no-replace writes. It is the working reference implementation.

The next Tools line moves the performance- and library-sensitive parts to a
C/C++ core. This document freezes the design before code is written.

## 2. Why C/C++ first

- C/C++ toolchains and libraries are available on more systems, including
  embedded and older Linux distributions, with fewer build-tool assumptions.
- The user chose C/C++ first; Rust is deferred to avoid maintaining a second
  toolchain and packaging story.
- The Go CLI can remain the orchestration/CLI layer while the native library is
  introduced behind a stable C ABI.

This is a direction, not a claim that the migration is complete. The Go CLI
remains the only executable implementation until the milestones below land.

## 3. Goals

- One C ABI usable from C11, C++17, Go (cgo), and other runtimes.
- Behaviour compatible with the Go CLI and the website importer for the
  `.hrtb` container and complete-backup envelope.
- No custom cryptography. Use a reviewed OpenPGP implementation.
- Fail closed: no plaintext output on validation, integrity, signature, or
  replay failure.
- Bounded memory and parsing: explicit limits, no unbounded recursion.
- Deterministic, testable error reporting without leaking secrets.
- CMake package metadata and pkg-config for consumers; no generated build
  artefacts committed.

## 4. Non-goals

- Changing the website envelope, `.hrtb` format, or `formatVersion`.
- Adding OpenPGP to the website or moving keys into the browser.
- Replacing the website importer's record-level validation or merge/Replace
  semantics.
- Key management, revocation, recovery kits, or a GUI.
- Production approval, an independent cryptographic audit, or a claim that the
  format is stable.
- A C/C++ implementation of the website application.

## 5. Architecture

```text
                 +-----------------------------+
                 |  caller: CLI / Go / library |
                 +--------------+--------------+
                                |
                    stable C ABI (include/hrtcold)
                                |
   +----------------------------+----------------------------+
   |            |               |             |              |
 container     bundle          crypto        replay         io
 parser        validator       (OpenPGP)     state          atomic file
   |            |               |             |              |
 HRTB header   strict JSON     RNP/other     high-water     temp + fsync +
 + payload     envelope        backend       mark file      no-replace link
```

Planned modules:

1. **container** — parse/build the 16-byte `HRTB` header, enforce version,
   flags, payload length, total-size, truncation, and trailing-byte rules.
2. **bundle** — strict JSON envelope validation: duplicate keys, depth, string
   size, `format`, `formatVersion`, RFC 3339 `createdAt`, mode, both `modes`
   blocks, category shapes, tombstones, calibration, preferences, scalar types.
   Unknown fields are accepted; record semantics stay with the website.
3. **crypto** — OpenPGP encrypt/decrypt, sign/verify, passphrase-locked keys,
   integrity protection. No home-grown primitives or KDF.
4. **replay** — local high-water mark keyed by signer primary fingerprint,
   with 0600 state, corruption detection, and explicit override semantics.
5. **io** — read limits, passphrase-file permission checks on Unix, temp file +
   fsync + hard-link no-replace publication, best-effort directory fsync.
6. **cli (optional)** — a thin C++ CLI can follow the library; the Go CLI may
   call the library through cgo first.

## 6. Data flow

### Encrypt

1. Read bundle with `max_bundle_bytes`.
2. Validate the envelope.
3. Unlock signer key only when signing is requested.
4. Sign plaintext, then encrypt to the recipient (sign-then-encrypt).
5. Wrap the binary OpenPGP message in `HRTB`.
6. Publish atomically without replacing an existing output.
7. Report recipient fingerprint, signer fingerprint, and plaintext SHA-256.

### Decrypt / verify

1. Read container with `max_container_bytes`.
2. Parse the outer `HRTB` header and payload bounds.
3. Validate the outer OpenPGP packet sequence (PKESK+, one integrity-protected
   encrypted packet, EOF; only legal markers before it).
4. Decrypt and verify integrity; read the body to EOF.
5. If a pinned verification key is supplied, require a valid signature from it
   or one of its subkeys; otherwise report unverified provenance.
6. Re-validate the bundle envelope.
7. Check the replay high-water mark for verified backups.
8. Publish atomically without replacing an existing output; update replay state
   only after the output lands.

## 7. Security invariants

- Literal, compressed-plaintext, signed-only, recipient-less, truncated, and
  trailing-packet OpenPGP payloads are rejected.
- A failed integrity, signature, bundle, replay, or I/O check writes no
  plaintext output.
- Pinned verification fails closed on unsigned input or a different signer.
- Replay state is local and best-effort; a fresh device or deleted state file
  cannot be protected. Overrides never lower the high-water mark.
- Secrets are not logged, included in error messages, or stored in the replay
  file. Best-effort zeroization is documented as best-effort, not a guarantee.
- All parsers use explicit size/depth limits and reject trailing data.
- The library never invents its own cryptography.

## 8. Milestones

| Milestone | Scope | Exit criteria |
| --- | --- | --- |
| **M0 (this change)** | configure/CMake, C ABI vocabulary, docs, C/C++ header checks | clean configure/build/test on Linux and Windows CI |
| M1 | container parser + bundle validator + limits | unit tests and differential tests against Go/website vectors |
| M2 | crypto backend integration: encrypt/decrypt and integrity | known-answer and negative tests; no plaintext on failure |
| M3 | sign/verify, passphrase handling, replay state, atomic writes | cross-implementation round-trip and replay tests |
| M4 | Go integration (cgo or subprocess) and/or native CLI | feature parity with the Go CLI for tested paths |
| M5 | fuzzing, sanitizer matrix, Windows durability review | documented evidence; still not an audit |

M0 deliberately ships no native implementation. The interface and build
contract can be reviewed before cryptographic code exists.

## 9. Open decisions

- **Crypto backend:** RNP is the leading candidate; see
  [`CRYPTO.md`](CRYPTO.md). The dependency is not added in M0.
- **JSON parser:** a small in-tree strict parser or a vetted library with
  duplicate-key and depth controls. Must not silently normalise duplicate keys.
- **Go integration:** cgo in-process versus a small native helper process.
  Decide after M2 with performance and crash-isolation evidence.
- **ABI stability:** v1 is additive; struct layout changes require new entry
  points, not silent reinterpretation.
- **License Gate:** dependency licences must be reviewed before M2 ships.
