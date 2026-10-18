# Tools Checkpoint — hrt-cold

**Status:** Tools branch checkpoint complete locally. The `.hrtb` format remains
experimental and is **not stable**. This checkpoint does **not** unblock the
License Gate, Docker, production website integration, L3 browser tests, merge,
release, or a pull request.

## Branch and baseline

- **Branch:** `Recovery/Tools`
- **Base:** `agent/backup-schema-audit` @ `9874c40afb89b0e3c9201280ebfe8b1415548eb4`
- **History:** branched from the audit work; no website UI commits were included.
- **Interface contract:** `docs/backup-schema-audit/TOOLS_INTERFACE.md`, copied
  from `agent/add-gpg-website` at `32edc7068423916f8ab7cfa416ae7bf31d2f7609`.

## Commits

| Commit | Purpose |
| --- | --- |
| `73dd4f5` | `docs: add website-Tools complete-backup interface` |
| `36ccf29` | `feat(tools): add offline OpenPGP cold-backup CLI` |
| `07cc714` | `ci(tools): test hrt-cold on Linux and Windows` |
| this commit | `docs(tools): record Tools checkpoint` |

## Files

- `tools/hrt-cold/go.mod`, `go.sum` — standalone module pinned to gopenpgp
  `v3.5.2`.
- `tools/hrt-cold/main.go` — CLI entry point and command flags.
- `tools/hrt-cold/pgp.go` — key generation, encryption, decryption, recipient
  inspection.
- `tools/hrt-cold/container.go` — experimental `HRTB` container parser/builder.
- `tools/hrt-cold/bundle.go` — complete-backup envelope validation.
- `tools/hrt-cold/atomicfile.go` — temp + fsync + hard-link no-replace publish.
- `tools/hrt-cold/passfile_unix.go`, `passfile_windows.go` — passphrase-file
  checks.
- `tools/hrt-cold/*_test.go` — synthetic tests.
- `tools/hrt-cold/testdata/synthetic-bundle.json` — fictional bundle only.
- `tools/hrt-cold/README.md` — usage, format, safety, limits.
- `tools/hrt-cold/THIRD_PARTY_NOTICES.md` — runtime license notices.
- `.github/workflows/tools-hrt-cold.yml` — Ubuntu and Windows native tests.
- `docs/backup-schema-audit/TOOLS_INTERFACE.md` — website/Tools boundary.

## Implemented commands

- `keygen`: armored public/private keys, passphrase-locked private key, full
  fingerprint output, no overwrite.
- `encrypt`: validates the website `oyama-hrt-complete-backup` envelope and
  version, encrypts to the recipient public key, wraps in `.hrtb`, no
  compression, no overwrite.
- `decrypt`: validates container, decrypts and verifies OpenPGP integrity,
  validates the bundle envelope, writes atomically, no overwrite.
- `inspect`: reads container metadata and unencrypted recipient key IDs without
  decrypting; `--recipient` maps a key ID to the full fingerprint.

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
| `go test -count=1 -v ./...` | PASS — 23 top-level tests, 45 subtests, `ok ... 9.832s` |
| `go test -race -count=1 ./...` | PASS — `ok ... 16.804s` |
| `go build -trimpath ./...` | PASS |
| `go mod verify` | PASS — `all modules verified` |
| `git diff --check` | PASS |
| `GOOS=windows GOARCH=amd64 go vet ./...` | PASS (cross-compile) |
| `GOOS=windows GOARCH=amd64 go test -c` | PASS (test binary compiles) |
| `GOOS=windows GOARCH=amd64 go build` | PASS (cross-compile) |

Staticcheck, govulncheck, gofumpt, and golangci-lint are not installed in this
environment; they were not run. This is not a cryptographic audit.

### Manual Linux round-trip

Commands: `keygen` → `encrypt` → `inspect` → `decrypt` on the synthetic
fixture.

- `cmp bundle.json decrypted.json` — exit 0, exact match.
- Bundle SHA-256: `f9bfe87717ec02e8274ac765e088a506b8d4ed57c280da22b608fef4a2aac305`.
- Decrypted SHA-256: `f9bfe87717ec02e8274ac765e088a506b8d4ed57c280da22b608fef4a2aac305`.
- `.hrtb` SHA-256: `04838da09353f0fad0cc47ed62ece256b53bc2e66e0003785da8c78be5571df5`.
- Inspect reported `magic: HRTB`, `containerVersion: 1`, `containerFlags: 0`,
  `recipientCount: 1`, recipient key ID, recipient subkey fingerprint, primary
  fingerprint, and `decrypted: false`.
- Modes: passphrase `0600`, private key `0600`, public key `0644`, `.hrtb`
  `0600`, decrypted JSON `0600`.
- No `.hrt-cold-*` temp files remained.
- Keys, passphrase, bundle, and outputs were under
  `/tmp/opencode/hrt-cold-manual-20261019` and were not committed.

## Failure coverage

Tests use fresh keys generated at test time and synthetic data only. Covered:

- round-trip and exact byte equality;
- wrong private key;
- wrong passphrase;
- tampered ciphertext (MDC failure);
- truncated container;
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
- `inspect` fingerprint matching and wrong-recipient rejection;
- passphrase-file mode rejection on Unix.

## Platform status

- **Linux:** native `go vet`, `go test`, race tests, build, and manual CLI
  round-trip PASS.
- **Windows:** native GitHub Actions workflow added for `windows-latest`
  (`go vet`, `go test -count=1 -v`, `go build`). Windows test binary
  cross-compiles locally. **Native Windows execution is pending the first CI
  run**; do not claim Windows results until the run URL and logs are recorded.
- **Windows semantics:** `os.Link` hard-link publish, NTFS error mapping,
  directory fsync behavior, and ACL equivalence to Unix `0600` are not proven
  by Linux tests. Passphrase-file ACLs are not verified by the tool.
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
- No signature or sender authentication; recipient fingerprint display is not
  trust establishment.
- No anti-rollback, key rotation, revocation workflow, recovery kit, or recovery
  drill.
- Bundle record semantics are not re-implemented; the website importer remains
  authoritative.
- `inspect` cannot detect ciphertext tampering without decrypting.
- Crash cleanup is not automatic; a crash can leave a `.hrt-cold-*` temp file.
- Directory fsync/crash durability is best-effort.
- No physical erasure claim.
- Native Windows CI result is pending.
- License Gate, Docker, production website integration, L3 browser tests, merge,
  release, and PR remain **BLOCKED / not started**.

## Next step

After the branch is pushed to `origin/Recovery/Tools`, record the remote SHA and
the GitHub Actions run URL. If the Windows job fails, fix the Windows-specific
failure and rerun before treating the Tools checkpoint as complete. Then stop:
do not start website integration, L3, merge, release, or PR work.
