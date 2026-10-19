# Tools Checkpoint — hrt-cold

**Status:** independent Tools project; `v0.1.0` pre-release published from
`xunxunProjects/Oyama-s-HRT-Tracker-Tools` (`main`). The `.hrtb` format remains
experimental and is **not stable**. Signing, pinned-key verification, and local
anti-replay are implemented; the website's direct plaintext import path remains
unauthenticated. This checkpoint does **not** unblock the License Gate, Docker
production readiness, an independent audit, or a production-support claim.

## Repository and baseline

- **Repository:** `xunxunProjects/Oyama-s-HRT-Tracker-Tools`
- **Branch:** `main`
- **History:** imported from `fantuanmtf/Oyama-s-HRT-Tracker:Recovery/Tools` @
  `8424d6e`; that branch was derived from `agent/backup-schema-audit` @
  `9874c40afb89b0e3c9201280ebfe8b1415548eb4`.
- **History cleanup:** the repository is tools-only: website source, assets,
  author/credits content, audit/demo docs, and non-tools CI/tests are absent
  from the tree. Their history remains in Git.
- **Interface contract:** `docs/backup-schema-audit/TOOLS_INTERFACE.md`, synced
  from the `agent/add-gpg-website` review in `6e27748` and kept as the one
  cross-project reference in this repository.

## Commits

| Commit | Purpose |
| --- | --- |
| `73dd4f5` | `docs: add website-Tools complete-backup interface` |
| `36ccf29` | `feat(tools): add offline OpenPGP cold-backup CLI` |
| `07cc714` | `ci(tools): test hrt-cold on Linux and Windows` |
| `adb564f` | `docs(tools): record Tools checkpoint` |
| `f73f0c9` | `fix(tools): reject packets after encrypted OpenPGP payload` |
| `6e27748` | `docs(tools): sync audit, recovery demo, and platform CI` |
| `d2ef418` | `feat(tools): sign, verify, and reject replayed cold backups` |
| `497fe11` | `chore(tools): make Recovery/Tools a tools-only branch` |
| `801793c` | `feat(tools): add docs-first C/C++ native scaffold` |
| `8424d6e` | `feat(tools): add version command and multiarch release packaging` |
| `0511d05` | `chore(tools): target the dedicated tools repository`; tagged `v0.1.0` |

## Native C/C++ core (Milestone 0)

The native line is docs-first, code-later:

- `configure` — hand-written CMake wrapper with prefix/build-dir/build-type,
  sanitizer, warnings-as-errors, and crypto-backend options.
- `CMakeLists.txt`, `cmake/` — interface-only `hrtcold::native` target, install
  metadata, pkg-config, and C11/C++17 header checks.
- `include/hrtcold/hrtcold.h` — v1 ABI vocabulary: version, status, limits,
  fixed-size error detail; no implemented symbols.
- `tests/native/` — header compile/link/run checks.
- `docs/native/` — design, ABI, build, crypto-backend analysis, and test plan.

No native parser, OpenPGP backend, replay store, or file writer is implemented
yet. The Go CLI remains the authority. RNP is the leading crypto-backend
candidate; it is not linked in Milestone 0. The next milestone is the container
parser and bundle validator with differential tests against the Go CLI.

## Files

- `tools/hrt-cold/go.mod`, `go.sum` — standalone module pinned to gopenpgp
  `v3.5.2`.
- `tools/hrt-cold/main.go` — CLI entry point and command flags.
- `tools/hrt-cold/pgp.go` — key generation, signing, encryption, decryption,
  signature verification, recipient inspection.
- `tools/hrt-cold/replay.go` — local verified-backup high-water mark.
- `tools/hrt-cold/container.go` — experimental `HRTB` container parser/builder.
- `tools/hrt-cold/bundle.go` — complete-backup envelope validation.
- `tools/hrt-cold/atomicfile.go` — temp + fsync + hard-link no-replace publish.
- `tools/hrt-cold/passfile_unix.go`, `passfile_windows.go` — passphrase-file
  checks.
- `tools/hrt-cold/*_test.go` — synthetic tests.
- `tools/hrt-cold/testdata/synthetic-bundle.json` — fictional bundle only.
- `tools/hrt-cold/README.md` — usage, format, safety, limits.
- `tools/hrt-cold/THIRD_PARTY_NOTICES.md` — runtime license notices.
- `.github/workflows/tools-hrt-cold.yml` — Ubuntu and Windows Go/native-scaffold
  tests.
- `docs/backup-schema-audit/TOOLS_INTERFACE.md` — website/Tools boundary.
- `configure`, `CMakeLists.txt`, `cmake/` — native Milestone 0 build contract.
- `include/hrtcold/hrtcold.h` — native v1 ABI vocabulary.
- `tests/native/` — C and C++ header checks.
- `docs/native/` — native design, ABI, build, crypto-backend analysis, and test
  plan.

## Implemented commands

- `keygen`: armored public/private keys, passphrase-locked private key, full
  fingerprint output, no overwrite.
- `encrypt`: validates the website `oyama-hrt-complete-backup` envelope and
  version; optionally signs the plaintext with `--signer-private` and
  `--signer-passphrase-file`; encrypts to the recipient public key, wraps in
  `.hrtb`, no compression, no overwrite. Prints signer fingerprint and
  plaintext SHA-256.
- `decrypt`: validates container, requires a genuinely encrypted OpenPGP
  message, verifies OpenPGP integrity; with `--verify-key`, requires a valid
  signature from the pinned key; for verified backups, enforces a local replay
  high-water mark; validates the bundle envelope, writes atomically, no
  overwrite.
- `inspect`: reads container metadata and unauthenticated PKESK recipient
  identifiers without decrypting; `--recipient` compares the advertised key ID
  to the supplied key for information only. It never verifies the recipient.
- `version`: prints the CLI build version, Go runtime, and `goos/goarch`.

## Security remediation after checkpoint

Three review findings were fixed on top of the checkpoint:

- **P0 — unencrypted OpenPGP payload accepted.** `decrypt` now checks
  go-crypto message metadata (`IsEncrypted` and `DecryptedWith`) and consumes
  the literal body to EOF so the MDC/integrity check runs. Literal,
  compressed-plaintext, and signed-only payloads fail without creating an
  output file.
- **P1 — unauthenticated PKESK shown as recipient proof.** `inspect` now labels
  recipient metadata as advertised/unverified, always emits
  `recipientVerified: false`, and treats `--recipient` matching as
  informational. It rejects containers without a public-key recipient and
  encrypted data packet.
- **P0 — trailing plaintext after an empty encrypted packet accepted.**
  `decrypt` and `inspect` now share `validateOuterPacketSequence`. It skips only
  legal marker packets, collects PKESK packets, requires exactly one
  integrity-protected encrypted data packet, and requires EOF immediately after
  it. Trailing literal, compressed, and signed packets, a second encrypted
  packet, missing PKESK, or missing encrypted data are rejected before any
  output is written.

Regression tests cover literal, compressed-plaintext, signed-only, missing
recipient, missing encrypted payload, forged PKESK key ID, empty encrypted
packet with trailing literal/compressed/signed packets, multi-recipient and
hidden-recipient messages, legal marker packets, wrong key, tamper, and
truncation, and assert that no output file is created on failure. This is an
engineering fix with regression tests, not a cryptographic audit.

## Signing, provenance, and anti-replay

`encrypt` signs the plaintext bundle when `--signer-private` and
`--signer-passphrase-file` are supplied. Sign-then-encrypt keeps the signer
identity inside the ciphertext, and the signature covers `createdAt`, every
record, and future envelope fields without changing the `.hrtb` container or
the website format. The signing key may be the recipient key or a separate key
pair; separate keys keep signing and decryption authority apart.

`decrypt --verify-key` pins the signer public key. The message must be signed,
the signature must verify, and it must come from that key or one of its
subkeys; otherwise decryption fails without writing output. Output reports
`signatureVerified`, `signerFingerprint`, `signerPrimaryFingerprint`, and
`contentSha256`. Without `--verify-key`, legacy unsigned backups still decrypt
with a warning and `signatureVerified: false`; no provenance is claimed.

Verified decrypts use a local high-water mark keyed by signer primary
fingerprint. It defaults to `<user-config>/hrt-cold/replay.json` (mode `0600`;
group/world-writable state is refused), overridable with `--replay-state`.
Newer bundles are accepted and raise the mark; exact repeats are accepted;
older `createdAt` or same-time-different content is refused unless
`--allow-older`, and an override never lowers the mark. `--no-replay-check`
performs a deliberate restore without reading or updating state.

This is local rollback detection, not global freshness. A fresh device, a
deleted state file, a compromised signer key, or a malicious binary defeats it.
It also cannot authenticate the website's direct plaintext JSON import; that
path has no keys or signature. Website-level provenance would require changing
the website's no-keys boundary and is not implemented.

## Container and limits

Experimental Phase 1F-aligned container:

```text
"HRTB" | u16 version=1 | u16 flags=0 | u64 payload length | OpenPGP payload
```

Rejects short headers, bad magic, unknown version, non-zero flags, zero length,
oversized declarations, truncation, and trailing bytes. Total container limit
16 MiB; bundle limit 15 MiB; key file 1 MiB; passphrase 4 KiB; JSON depth 64;
JSON string 1 MiB. No custom cryptography or KDF.

## Local checks actually run

Working directory: `tools/hrt-cold` on Linux/amd64 with Go 1.27.1.

| Check | Result |
| --- | --- |
| `gofmt -l .` | clean |
| `go vet ./...` | PASS |
| `go test -count=1 -v ./...` | PASS — 101 tests, `ok ... 15.969s` |
| `go test -race -count=1 ./...` | PASS — `ok ... 36.040s` |
| `go build -trimpath ./...` | PASS |
| `go mod verify` | PASS — `all modules verified` |
| `git diff --check` | PASS |
| `GOOS=windows GOARCH=amd64 go vet ./...` | PASS (cross-compile) |
| `GOOS=windows GOARCH=amd64 go test -c` | PASS (test binary compiles) |
| `GOOS=windows GOARCH=amd64 go build` | PASS (cross-compile) |
| GitHub Actions `39228438599` | PASS — Ubuntu and Windows native jobs for `a4e1edd` |

Staticcheck, govulncheck, gofumpt, and golangci-lint are not installed in this
environment; they were not run. This is not a cryptographic audit.

### Manual Linux round-trip

Commands: two `keygen` runs (recipient and signer) → signed `encrypt` →
`decrypt --verify-key` → repeat `decrypt` on the synthetic fixture.

- `cmp bundle.json restored.json` — exit 0, exact match.
- Bundle/decrypted SHA-256:
  `f9bfe87717ec02e8274ac765e088a506b8d4ed57c280da22b608fef4a2aac305`.
- Encrypt reported `signature: signed`, the signer fingerprint, and that
  content SHA-256.
- Decrypt reported `signatureVerified: true`, `signerPrimaryFingerprint` equal
  to the signer key, `contentSha256` equal to the bundle hash, and
  `replayStatus: new`.
- The repeat decrypt reported `replayStatus: repeat` and also matched byte for
  byte.
- Replay state was written with mode `0600` and stored only the signer
  fingerprint, `createdAt`, content hash, and last-seen time.
- Modes: passphrase `0600`, private keys `0600`, public keys `0644`, `.hrtb`
  `0600`, decrypted JSON `0600`.
- No `.hrt-cold-*` temp files remained.
- Keys, passphrase, bundle, and outputs were under a mode-`0700` temporary
  directory and were not committed.

## Failure coverage

Tests use fresh keys generated at test time and synthetic data only. Covered:

- round-trip and exact byte equality;
- wrong private key;
- wrong passphrase;
- tampered ciphertext (MDC failure);
- truncated container;
- unencrypted literal payload;
- compressed-plaintext payload;
- signed-only payload;
- forged PKESK recipient key ID;
- empty integrity-protected encrypted packet followed by a trailing literal,
  compressed, or signed packet;
- second encrypted data packet or marker packet after the encrypted data packet;
- multi-recipient and hidden-recipient messages, plus legal marker packets;
- container with no public-key recipient;
- container with no encrypted data packet;
- unknown container version;
- reserved flags;
- bad magic;
- trailing bytes;
- oversized bundle and oversized container;
- existing encrypt/decrypt output unchanged;
- malformed, duplicate-key, wrong-format, and wrong-version plaintext bundles;
- missing output parent (write failure after successful decryption);
- atomic no-replace publication and eight concurrent writers with exactly one
  winner;
- no `.hrt-cold-*` temp leaks after tested failures;
- `inspect` rejects invalid payloads and reports advertised-recipient matches
  as unverified information;
- passphrase-file mode rejection on Unix;
- signed round-trip with a separate signer key and recipient key;
- wrong pinned verification key and unsigned backup with `--verify-key`;
- verified replay states: new, repeat, older refused, allowed older override,
  same-time-different-content conflict, and high-water mark not lowered by an
  override;
- replay state round-trip, `0600` mode, corrupt version, and group/world-writable
  state rejection.

## Platform status

- **Linux:** native `go vet`, `go test`, race tests, build, and a signed manual
  CLI round-trip PASS.
- **Windows:** native GitHub Actions workflow run
  [39228438599](https://github.com/fantuanmtf/Oyama-s-HRT-Tracker/actions/runs/39228438599)
  passed both `Go tests (ubuntu-latest)` and `Go tests (windows-latest)` for
  `a4e1edd`, including the signing and replay tests. Windows ran native
  `go vet`, `go test -count=1 -v`, and `go build`; the formatting step is
  Linux-only.
- **Windows semantics:** native tests exercised `os.Link` no-replace publish,
  Windows error mapping, atomic-write failure paths, and passphrase-file
  handling. Directory fsync durability and ACL equivalence to Unix `0600`
  remain unproven; passphrase-file ACLs are not verified by the tool.
- **Other filesystems:** FAT/exFAT and SMB/network filesystems are `NOT_TESTED`;
  unsupported hard links fail closed with no rename fallback.
- **macOS/Android/iOS:** out of scope.

## License inventory

Runtime closure from `go list -deps`:

| Module | Version | License |
| --- | --- | --- |
| `github.com/ProtonMail/gopenpgp/v3` | v3.5.2 | MIT |
| `github.com/ProtonMail/go-crypto` | v1.5.2 | BSD-3-Clause |
| `github.com/cloudflare/circl` | v1.6.3 | BSD-3-Clause |
| `golang.org/x/crypto` | v0.41.0 | BSD-3-Clause |
| `golang.org/x/sys` | v0.35.0 | BSD-3-Clause |

No GPL/LGPL runtime dependency is in that closure. Full texts are in
`tools/hrt-cold/THIRD_PARTY_NOTICES.md`. This is an engineering inventory, not
a legal opinion. **License Gate remains BLOCKED.**

## Known limitations / blockers

- The `.hrtb` format is experimental and not stable.
- Signing and pinned-key verification exist but are opt-in. Without
  `--signer-private` / `--verify-key`, no provenance is claimed.
- Recipient fingerprint display is not trust establishment; compare it through
  an independent channel.
- PKESK recipient key IDs/fingerprints are unauthenticated and `inspect` only
  reports advertised values; a match is not recipient verification.
- Anti-replay is a local high-water mark only; a fresh device, deleted state,
  compromised signer key, or malicious binary defeats it. It does not cover the
  website's direct plaintext import.
- No key rotation, revocation workflow, recovery kit, or recovery drill.
- Bundle record semantics are not re-implemented; the website importer remains
  authoritative.
- `inspect` cannot detect ciphertext tampering without decrypting.
- Crash cleanup is not automatic; a crash can leave a `.hrt-cold-*` temp file.
- Directory fsync/crash durability is best-effort.
- No physical erasure claim.
- License Gate, Docker, production website integration, release, and PR remain
  **BLOCKED / not started**. Website L3 browser coverage lives in the separate
  website PR, not in this Tools branch.

## Push and next step

- **Push:** `Recovery/Tools` pushed to `origin` as `7b0995a..adb564f`.
- **Remote SHA:** `adb564f30b84376e0d3abd4d1406f6441e868cf0`.
- **Native CI:** [run 39152774184](https://github.com/fantuanmtf/Oyama-s-HRT-Tracker/actions/runs/39152774184),
  `completed / success`; Ubuntu and Windows jobs both passed.
- **Next step:** stop at the Tools checkpoint. Do not start production website
  integration, L3 browser tests, merge, release, or a PR. Keep License Gate,
  Docker, and production integration blocked pending separate approval.
- The final documentation commit after `adb564f` only records CI evidence; the
  branch may be one commit ahead of the SHA that produced run `39152774184`.
  Its own CI run is the authoritative check for the final commit.
