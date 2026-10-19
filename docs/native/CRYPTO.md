# Native crypto backend decision

**Status:** candidate analysis for the planned C/C++ core. No backend is linked
in Milestone 0.

## 1. Requirements

The native core needs a real OpenPGP implementation, not just cipher
primitives:

- public-key encryption to X25519/EdDSA keys (the key type the current Go CLI
  generates), with room for RSA/ECDH keys if the contract expands;
- passphrase-locked private keys (S2K/KDF) and key unlocking;
- integrity-protected encrypted data (SEIPD/MDC; AEAD support is desirable);
- detached/inline signatures and verification with a pinned public key;
- signature over the plaintext bundle, inside the encryption;
- deterministic failure on wrong key, wrong passphrase, tamper, truncation,
  unknown version, and trailing packets;
- no home-grown cryptography and no custom KDF;
- native Linux and Windows builds;
- a licence compatible with this repository's MIT distribution and the
  project's License Gate.

The website never receives keys or passphrases; this backend lives only in the
offline Tools.

## 2. Candidates

| Candidate | Type | Licence | Windows | Fit |
| --- | --- | --- | --- | --- |
| **RNP** (`rnpgp/rnp`) | C++ OpenPGP library with C API | BSD-2-Clause | Builds with CMake; used by Thunderbird | **Primary candidate**: complete OpenPGP message/signature layer |
| Botan | C++ crypto primitives | BSD-2-Clause | Yes | Primitives only; would require a separately reviewed OpenPGP layer |
| OpenSSL | C crypto primitives | Apache-2.0 | Yes | Primitives only; same OpenPGP-layer burden |
| libgcrypt | C crypto primitives | LGPL-2.1+ | Yes | Primitives only; LGPL obligations and OpenPGP-layer burden |
| GPGME / GnuPG | C wrapper around GnuPG | LGPL-2.1+ / GPL-3.0 | Possible | External keyring/agent, subprocess dependence, licence and packaging concerns |
| Sequoia | Rust OpenPGP | LGPL-2.0+ | Yes | Rust, deferred by the C/C++-first decision |

### Why RNP is the leading candidate

- It is an OpenPGP implementation, not a bag of primitives, so the native core
  does not have to invent packet parsing, signature handling, or key unlocking.
- It has a C API suitable for a stable C ABI and a C++ implementation that fits
  the chosen direction.
- BSD-2-Clause is compatible with the repository's MIT distribution subject to
  notice retention and the License Gate.
- It is used by a large mail client, which gives it a meaningful interoperability
  and fuzzing history.

Before adoption, verify against the exact pinned version:

- EdDSA Curve25519 and X25519 support matching the current key generator;
- passphrase-locked key unlock and S2K parameters;
- SEIPD/MDC and (if used) AEAD behaviour;
- signature verification and issuer/key-ID handling;
- rejection of trailing packets and malformed lengths;
- deterministic error mapping to `hrtcold_status`;
- Windows build and runtime dependencies (no unexpected GPL/LGPL closure);
- security-response process and release cadence.

If any of those fail, do not paper over them in our code. Either choose another
OpenPGP implementation or keep the Go CLI as the authority.

## 3. Not recommended as the first path

- **Primitives-only libraries** (Botan, OpenSSL, libgcrypt) require us to build
  an OpenPGP layer, including packet parsing, signature framing, key handling,
  and downgrade defences. That is exactly the kind of security-critical code we
  should not write from scratch for a backup tool.
- **GPGME/GnuPG** introduces an external keyring/agent, process-boundary
  behaviour, GPL/LGPL obligations, and platform packaging differences. It may be
  a compatibility option later, not the default native backend.
- **Rust/Sequoia** is deferred by the C/C++-first decision, not rejected on
  security grounds.

## 4. Decision process

1. Build an RNP spike outside the release build: unlock a generated key,
   encrypt/decrypt, sign/verify, and reject the existing negative cases.
2. Run the Go CLI's synthetic vectors through both implementations.
3. Record licence notices and Windows build evidence.
4. Only then add the dependency to `CMakeLists.txt` behind
   `HRTCOLD_CRYPTO_BACKEND=rnp`.

Until step 4, `--with-crypto` only records intent; Milestone 0 does not fetch or
link anything.

## 5. Security rules for the backend

- Never log or include passphrases, private keys, session keys, or plaintext in
  errors.
- Do not fall back to unencrypted or unsigned operation silently.
- Do not treat a PKESK key ID/fingerprint as proof of recipient; only a
  successful decrypt proves participation.
- Keep signature verification mandatory when a pinned key is supplied.
- Zeroize temporary key/passphrase buffers on a best-effort basis and document
  that this is not physical erasure.
- Treat the backend as untrusted input surface: fuzz the parser and run
  sanitizers.
- Pin the dependency version; update through a reviewed change with notices and
  test evidence.

## 6. Licence gate

No native crypto dependency is added until its licence, notices, static/dynamic
linking implications, and Windows redistribution are reviewed. This document is
an engineering analysis, not a legal opinion. The License Gate remains a
blocker for release, not for the M0 scaffold.
