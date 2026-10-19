# Changelog

## 0.1.0 — 2026-10-19 (pre-release)

Initial independent release of Oyama HRT Tracker Tools.

- `hrt-cold` offline OpenPGP cold-backup CLI for the
  `oyama-hrt-complete-backup` envelope.
- `keygen`, `encrypt`, `decrypt`, `inspect`, and `version` commands.
- Sign-then-encrypt with optional signer key; pinned `--verify-key` mode fails
  closed on unsigned input, bad signatures, or a different signer.
- Verified-decrypt local anti-replay high-water mark, with `--allow-older` and
  `--no-replay-check` overrides.
- Strict `.hrtb` container and bundle-envelope validation, bounded parsing,
  atomic no-replace writes, and no plaintext output on failure.
- Static Linux/Windows amd64 and 386 binaries built with `CGO_ENABLED=0`.
- Per-binary SHA-512/MD5 sums and detached GPG signatures in the release.
- Documentation-only C/C++ native core Milestone 0: `configure`, CMake, C ABI
  header, and design/ABI/build/crypto/test documents. No native implementation
  or crypto backend is linked yet.

This is an experimental pre-release. The `.hrtb` format is not stable and the
project has not had an independent cryptographic audit.
