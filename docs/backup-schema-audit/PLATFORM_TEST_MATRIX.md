# Windows/Linux Platform Test Matrix

**Status:** Linux `PASS` for the isolated harness; Windows `NOT_TESTED` until the native CI run completes.

## Scope

- Target platforms: **Linux desktop** and **Windows desktop**.
- Out of scope: macOS, Android, iOS, iPadOS.
- This matrix covers the isolated file-publish harness in `tests/platform/filepublish/`; it does not cover production restore, database transactions, or the web app.

## Harness

The test-only Go module `tests/platform/filepublish/` mirrors the Phase 1F file-publish safety behavior:

- `AtomicWrite`: `CreateTemp` in the target directory, chmod, write, fsync, close, `os.Link(tmp, path)` no-replace publish, remove temp, best-effort directory sync.
- `CleanupStaleTemps`: removes only `.hrt-tmp-*` regular files older than one hour.

It uses only the Go standard library. It is not linked into the product.

## Test matrix

| ID | Scenario | Linux | Windows | Notes |
|---|---|---|---|---|
| P-01 | Existing target is not overwritten | PASS | NOT_TESTED | `os.Link` returns an exists error; target content unchanged |
| P-02 | 8 concurrent writers, exactly one winner | PASS | NOT_TESTED | Target is one complete payload; no temp files remain |
| P-03 | Cleanup removes only old `.hrt-tmp-*` regular files | PASS | NOT_TESTED | Unrelated file remains; old own temp removed |
| P-04 | Missing parent directory leaves no output/temp | PASS | NOT_TESTED | `CreateTemp` fails before any publish |
| P-05 | New file permissions are 0600 on Unix | PASS | NOT_TESTED | Windows does not treat `0600` as an ACL guarantee |
| P-06 | Unicode and spaces in path | PASS | NOT_TESTED | Native path handling |
| P-07 | Directory fsync | BEST_EFFORT | NOT_TESTED | Error intentionally ignored; durability not proven |
| P-08 | Crash after temp creation before publish | NOT_TESTED | NOT_TESTED | Cleanup is best-effort, not a crash guarantee |
| P-09 | Windows ACL / sensitive-file protection | N/A | NOT_TESTED | Must be validated with Windows ACLs; Unix mode is insufficient |
| P-10 | Windows hard-link support / privilege | N/A | NOT_TESTED | NTFS and runner policy may affect `os.Link` |
| P-11 | Network filesystem / SMB semantics | NOT_TESTED | NOT_TESTED | Hard-link and atomicity semantics may differ |
| P-12 | File locking / concurrent processes | PARTIAL | NOT_TESTED | No cross-process lock; no-replace link is the only guard |

## Platform risks

1. **Unix mode vs Windows ACL**: `os.Chmod(0600)` does not grant the same protection on Windows. Sensitive private keys require Windows ACL review; do not claim Unix permissions protect Windows files.
2. **Hard links**: `os.Link` is used for no-replace publish. It is not proven on Windows runners, NTFS volumes, SMB, or restricted accounts. If unsupported, the current behavior is an error, not a fallback.
3. **Directory sync**: opening a directory and calling `Sync` is Unix-oriented; errors are ignored. Durability on Windows is `NOT_TESTED`.
4. **Crash cleanup**: only files matching `.hrt-tmp-*` and older than one hour are removed. A recent crash temp remains until the next eligible cleanup.
5. **Path semantics**: Unicode/space paths are tested on Linux; Windows path normalization is `NOT_TESTED`.
6. **Multiple processes**: no file lock. The hard-link publish prevents replacement, but concurrent cleanup and writes need native testing.
7. **No physical erasure**: deleting temp/target files cannot erase SSD blocks, snapshots, swap, or backups.

## CI

Workflow: `.github/workflows/platform-filepublish.yml`

- `ubuntu-latest` and `windows-latest`
- `go vet ./...`
- `go test -v ./...`
- Working directory: `tests/platform/filepublish`
- `setup-go` uses `go-version-file` and disables cache.
- Permissions: `contents: read`.

**Actual GitHub Actions execution: `NOT_TESTED`.** A local Linux run is not a substitute for the native Windows runner. Record the run URL and logs when executed.

## Acceptance

- Ubuntu and Windows jobs both run and pass before claiming platform coverage.
- Windows ACL behavior has separate evidence; do not infer from Unix `0600`.
- Any failure is reported with the OS, Go version, filesystem, command, and log.
- No claim of production atomicity or complete restore.

## Open decisions

1. Windows ACL strategy for private keys and backups.
2. Whether hard-link publish is acceptable on Windows or a platform-specific no-replace API is required.
3. Whether directory fsync is required or documented as best-effort.
4. Whether cross-process locking is needed for the future CLI.
5. Where native CI evidence is retained.

## Actual CI run

- **Run:** GitHub Actions `39147752217` — `Platform file-publish tests`.
- **Head:** `7a7d00ac1676e290fbefe05ab496674d37d5baa5`.
- **Result:** completed `success`; both `File publish (ubuntu-latest)` and `File publish (windows-latest)` jobs succeeded.
- **URL:** https://github.com/fantuanmtf/Oyama-s-HRT-Tracker/actions/runs/39147752217
- **Scope:** the Go harness tests for existing-target protection, concurrent publishing, cleanup, missing parent, Unix mode and Unicode/space paths passed on both runners.
- **Still not proven:** Windows ACL equivalence to Unix `0600`, hard-link privilege/edge cases, SMB/network filesystem behavior, cross-process file locking, crash cleanup, and application-level Replace/Merge atomicity. These remain `NOT_TESTED` or `BLOCKED`.

## P-13/P-14 cross-process and Windows ACL

- `P-13` runs four separate OS processes with a readiness/start barrier against one target. Linux PASS (exactly one winner, no temp files).
- `P-14` is a Windows-only `icacls` smoke test. It logs the ACL and rejects explicit `Everyone:(F/M)` and `BUILTIN\Users:(F/M)` grants. It is a smoke check, not a proof of Windows ACL equivalence or hardening.
- Windows native execution: pending the next CI run; do not treat Linux execution as Windows evidence.
