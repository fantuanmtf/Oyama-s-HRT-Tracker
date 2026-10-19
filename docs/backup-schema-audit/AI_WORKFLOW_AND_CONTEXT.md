# AI Workflow and Context Handoff

**Snapshot:** 2026-10-19. **Scope:** durable working context for agents on the
complete-device backup, offline Tools, backend-smoke, and integration tracks.

This is a documentation handoff, not a production approval. It does not claim
L3/browser coverage, OpenPGP encryption in the website, all-or-nothing restore,
Windows durability, License Gate clearance, or the absence of vulnerabilities.

## 1. Branch and PR map

| Ref                                | SHA                                        | Meaning                                                  |
| ---------------------------------- | ------------------------------------------ | -------------------------------------------------------- |
| `origin/agent/add-gpg-website`     | `8f2650b74e62d3c9177b723c9ef079fc820f8a31` | Website complete-backup data/UI; PR #129 head            |
| `origin/Recovery/Tools`            | `7ee628b6455dee31ca044e453dff31e6d17a7e56` | Offline `hrt-cold` OpenPGP CLI; experimental, not merged |
| `origin/Privacy/Add-GPG`           | `9874c40afb89b0e3c9201280ebfe8b1415548eb4` | Product branch for the GPG/backup line                   |
| `origin/agent/backup-schema-audit` | `9874c40afb89b0e3c9201280ebfe8b1415548eb4` | Audit/L2 test-seam tip; same commit as `Privacy/Add-GPG` |
| `upstream/main`                    | `5c5d9fe39945c3f03c740bbaf76f313be20f7dbe` | Base of PR #129                                          |

The website and Tools branches both descend from the audit tip `9874c40`.
`Privacy/Add-GPG` is absent from `upstream`; it exists on `origin` (the fork).
For that reason PR #129 targets `upstream/main` and includes the audit/L2
commits plus the website commits. Do not rebase the website branch onto
`5c5d9fe`; that would drop dependencies.

| Item                                                                            | State at snapshot                                                                                                           |
| ------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| PR #129, "feat: complete device backup export/import with verification preview" | Open, base `main`, **draft status now ready** (`draft=false`); head `8f2650b`; 41 commits, 36 files; not approved or merged |
| Issue #127, "Complete device backup: remaining hardening and integration work"  | Open                                                                                                                        |
| Issue #128, "Create Recovery/Tools branch for offline cold-backup CLI"          | Open; the branch exists at `7ee628b`, so reconcile and close only with evidence                                             |

Use GitHub for current CI/review state. The SHAs above are the durable anchors
for this handoff.

## 2. What is implemented, and what is not

Status words here mean: `PASS` = tested surface only; `NOT_TESTED` = no direct
evidence; `BLOCKED` = intentionally not started or not safe to claim.

### Website complete-backup bundle

Implemented at `8f2650b`:

- Versioned plaintext envelope `format: "oyama-hrt-complete-backup"`,
  `formatVersion: 1`.
- Per-mode events, labs, templates, tombstones, quick doses, supplies, and plan;
  shared weight/PK parameters; calibration; five allowlisted preferences.
- Export/preview/merge UI with plaintext and target-account warnings.
- Merge-only import. Same-id conflict rules abort the affected category rather
  than silently overwriting. Replace is not implemented.
- Tombstone TTL, future-clock bound, per-kind cap, duplicate-id checks, entry
  caps, prototype-safe IDs, and scalar conflict reporting.
- Synthetic tests for both modes, all supply pack branches, credentials
  exclusion, tombstone bounds, clock skew, and rollback failure paths.

Not done or not proven:

- **BTV-3** (`formatVersion: 1.0`) and **BTV-7** (unknown per-record fields)
  remain unresolved interface decisions.
- No signing, source authentication, or anti-replay.
- No **Replace all-or-nothing** transaction. Rollback is an application-level
  snapshot, not a `localStorage` transaction.
- No **L3 browser E2E** for export, preview, confirmation, merge, or preference
  effects. The Vitest environment is Node-only.
- No production-readiness claim; no encryption is performed by the website.

### Tools offline OpenPGP CLI

Implemented at `7ee628b` in `tools/hrt-cold`:

- Standalone Go CLI: `keygen`, `encrypt`, `decrypt`, and `inspect`.
- Experimental `HRTB` container, bundle-envelope validation, and outer OpenPGP
  packet-sequence validation.
- Encrypted-payload checks: literal, compressed-plaintext, signed-only,
  recipient-less, truncated, and trailing-packet inputs are rejected.
- No plaintext output on validation/decryption failure; atomic no-replace file
  publication.
- Synthetic tests on Linux and native Windows CI.

Not done or not proven:

- Experimental `.hrtb` format; not stable and not wired into the website.
- **Tools OpenPGP v6 support, independent MDC verification, and cryptographic
  recipient verification.** MDC is delegated to gopenpgp/go-crypto and covered
  by tamper tests, but there is no independent cryptographic audit. `inspect`
  reports unauthenticated PKESK advertisements; only a successful `decrypt`
  shows that the supplied private key participated.
- Signing, source authentication, anti-replay, key rotation, revocation, and
  recovery drills.
- **Windows durability** and ACL equivalence; directory fsync, SMB/network
  filesystems, crash cleanup, and lock behavior are not proven.
- Production website integration, License Gate, Docker, and production approval.

## 3. AI context-budget rules

Follow these rules even when the full history is available.

1. **Read in this order:** `docs/backup-schema-audit/HANDOFF.md` (current-state
   block), this file, one relevant work-item document, then the specific source
   or test files for the track.
2. **Do not re-read** full audit reports, old chat transcripts, or the entire
   `HANDOFF.md` chronology for a new task. The older handoff sections are a
   historical log and may name superseded SHAs.
3. **Keep outputs narrow.** Use targeted search (`rg -n`), a specific
   `git show <sha>:<path>`, short log ranges, and bounded test output. Do not
   dump whole files or full CI logs into context.
4. **Pin the revision.** Check `git rev-parse HEAD` and `git status` before
   editing. Never assume a branch name still means the SHA in an old report.
5. **Use one subagent per independent track.** Give each subagent a base SHA,
   an explicit write scope, and the exact checks to run. Do not let two tracks
   edit the same contract or UI file concurrently.
6. **Checkpoint after each logical unit.** Record branch, HEAD, tests actually
   run, blockers, and the next command. Commit or leave a clean, explicit
   handoff; do not carry unexplained partial work.
7. **Stop on unexplained risk.** Stop and report for data loss, partial writes,
   credential/secret exposure, a new license or runtime dependency, production
   data access, a remote binding, or a security claim without evidence.
8. **Do not overclaim.** Never claim L3, production readiness, "no
   vulnerabilities", website OpenPGP encryption, or all-or-nothing restore.
9. **Use synthetic data only.** No real health records, production bundles,
   credentials, private keys, passphrases, or tokens in files, commands, tests,
   or handoffs.

### Source map

| Track                  | Start with these files                                                                                                                                                                                                   |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Website bundle/import  | `src/utils/backupBundle.ts`, `backupState.ts`, `backupImport.ts`, `backupWarnings.ts`; `src/hooks/useAppData.ts`, `src/App.tsx`; `tests/backupBundle.test.ts`, `tests/backupState.test.ts`, `tests/backupImport.test.ts` |
| Website UI             | `src/pages/ImportSettings.tsx`, `src/pages/ExportSettings.tsx`, `src/components/ExportSection.tsx`, `src/i18n/translations.ts`                                                                                           |
| Tools CLI              | `tools/hrt-cold/README.md`, `bundle.go`, `container.go`, `pgp.go`, `atomicfile.go`, matching `*_test.go`                                                                                                                 |
| Backend smoke          | `worker.ts`, `wrangler.toml`, `tests/workerHardening.test.ts`                                                                                                                                                            |
| Contract decisions     | `TOOLS_INTERFACE.md`, `BACKUP_TEST_VECTORS.md`                                                                                                                                                                           |
| Restore/replace design | `RESTORE_SEMANTICS.md`, `L2_TEST_SEAM_DESIGN.md`, `SYNTHETIC_RESTORE_HARNESS.md`                                                                                                                                         |

## 4. Scheduling and parallel tracks

| Track                             | Work                                                              | Can run                                                      | Dependencies and exit gate                                                                                                                             |
| --------------------------------- | ----------------------------------------------------------------- | ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **A. Website contract/UI**        | Bundle validation, preview/merge, confirmation, UI copy           | Now, from `8f2650b` or a reviewed successor                  | Depends on BTV-3/7 and `TOOLS_INTERFACE.md`. Gate: website checks pass, review comments resolved, contract decisions recorded.                         |
| **B. Tools CLI/API**              | `hrt-cold` behavior, `.hrtb`, envelope/container checks           | Now, from `7ee628b`                                          | Depends on the same envelope contract. Gate: Go checks pass; no `.hrtb` stability or production-integration claim until decisions and review.          |
| **C. Security hardening/signing** | Threat model, signing, source auth, anti-replay, key lifecycle    | C1 input hardening now; C2 signing after A/B contract freeze | Depends on A/B interface and an approved threat model. Gate: test vectors, key lifecycle, and review; no "authenticated" claim without them.           |
| **D. Backend smoke/security**     | Local Worker smoke, auth boundaries, headers/CORS, error handling | Now, in parallel                                             | Local D1 only. Gate: evidence recorded, `NOT_TESTED` list explicit, no production binding or deployment.                                               |
| **E. L3 integration**             | Browser E2E for export, preview, confirm, merge, preferences      | After A and D baseline; B if encrypted handoff is in scope   | Needs a browser harness (not present in the Node-only Vitest config). Gate: pinned SHAs and synthetic browser evidence; still not production approval. |

Suggested waves:

1. **Wave 1:** A, B, and D in parallel. Freeze any contract change before it is
   merged; only one track edits `TOOLS_INTERFACE.md` or shared bundle files.
2. **Wave 2:** C1 hardening and A follow-ups. Resolve BTV-3/7 before declaring
   the bundle contract stable.
3. **Wave 3:** C2 signing/anti-replay and E browser integration. Both depend on
   the frozen contract and reviewed key/test design.

PR #129 is the convergence point for track A only. Do not mix Tools, backend,
or unrelated security work into the website PR. Issue #127 tracks the remaining
hardening/integration work; issue #128 tracks the Tools branch and should not be
closed until its actual branch/CI state is reconciled.

## 5. Test matrix and commands

Run only the checks for the surface changed. A docs-only change does not require
a build or test suite; it does require `git diff --check`.

### Website

```sh
npx tsc --noEmit
npx vitest run
npm run build
npx prettier --check <changed files>
npx eslint <changed TS/TSX files>
git diff --check
```

Use focused Vitest files while iterating, then the full run before a checkpoint.
The current Vitest config is Node-only and does not exercise React/browser
behavior; that is why L3 remains `NOT_TESTED`.

### Tools

```sh
cd tools/hrt-cold
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build -trimpath ./...
go mod verify
gofmt -l .
git diff --check
```

Native Windows CI is required for Windows claims. Cross-compilation is not a
substitute for Windows durability/ACL evidence.

### Backend local smoke

```sh
npx wrangler d1 migrations apply hrt-tracker-prod --local
npx wrangler dev --local --port 8787
curl -sS -D /tmp/hrt-transparency.headers -o /tmp/hrt-transparency.json \
  http://127.0.0.1:8787/api/transparency
curl -sS -i -X OPTIONS http://127.0.0.1:8787/api/transparency
```

Provide a disposable local `JWT_SECRET` through an untracked local config such
as `.dev.vars`; never reuse a production secret. Use **only** `--local` and a
local D1 database. Do not run `wrangler deploy`, `--remote`, remote migrations,
or any production D1/R2/Worker command.

### Check selection

| Change            | Minimum evidence                                                                          |
| ----------------- | ----------------------------------------------------------------------------------------- |
| Docs only         | `git diff --check`; state that no build/tests were run                                    |
| Website bundle/UI | `tsc`, focused tests, full Vitest, build, Prettier/ESLint on changed files                |
| Tools             | `go test`, race, vet, build, `go mod verify`, plus native-platform CI if claiming Windows |
| Backend           | Local smoke evidence plus `NOT_TESTED` limits; no production claim                        |
| L3                | Browser E2E with synthetic data and pinned SHAs; otherwise `NOT_TESTED`/`BLOCKED`         |

## 6. Backend testing plan and limits

### Plan

Use a fresh local D1 database and synthetic accounts/data. Keep the local server
bound to loopback and record status, headers, and the narrow response body for
each check.

| ID   | Check                             | Expected evidence                                                                                                                                                                                                                |
| ---- | --------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| B-01 | Local migrations and Worker start | Local D1 migration succeeds; `wrangler dev --local` starts without touching remote bindings                                                                                                                                      |
| B-02 | `GET /api/transparency`           | 200 JSON aggregate; documented fields; no PII, tokens, or per-user records                                                                                                                                                       |
| B-03 | Response headers                  | `Content-Type: application/json`, `Cache-Control: public, max-age=15` (aggregate + anonymized data; intentional public cache — decision 2026-10-19), security headers present, CORS origin/method/header values match the Worker |
| B-04 | CORS preflight                    | `OPTIONS` succeeds with the allowed methods/headers; record the exact response                                                                                                                                                   |
| B-05 | Authentication boundary           | Missing, malformed, expired, and revoked tokens fail closed; session-level 401 includes `X-Session-Invalid: 1` where expected                                                                                                    |
| B-06 | Authorization boundary            | A normal synthetic user cannot use admin routes or read another user's content/share; record status only, never data                                                                                                             |
| B-07 | Error handling                    | Malformed JSON, bad methods, unknown routes, and invalid IDs return expected 4xx; no stack traces, SQL, secrets, or cross-user data                                                                                              |
| B-08 | Input limits                      | Truncated/oversized bodies fail without 500 or partial writes; limits are recorded as observed                                                                                                                                   |
| B-09 | Rate limiting                     | A small local burst returns 429 with `Retry-After` when the local binding is active; do not run a load/DoS test                                                                                                                  |
| B-10 | Cleanup                           | Stop the local server and confirm no remote resources, committed secrets, or production data were created                                                                                                                        |

`NOT_TESTED` must be written explicitly for anything not exercised, including
passkey/WebAuthn browser flows, R2, scheduled jobs, production rate limits,
TLS/WAF, and deployed configuration.

### Limits

- Local Miniflare/Wrangler behavior is not the production Cloudflare runtime.
- Local D1 is not production D1; local R2/queue behavior may differ or be absent.
- No load, fuzzing, penetration, or denial-of-service testing is implied.
- This plan checks selected boundaries; it cannot establish "no vulnerabilities".
- Do not use real accounts, health data, tokens, or production URLs. If a check
  unexpectedly exposes another user's data or a secret, stop, preserve only the
  minimal evidence, and report it without retrying against production.

## 7. Handoff template

Copy this block into the relevant work-item handoff. Do not include secrets,
tokens, private keys, passphrases, real bundles, or production identifiers.

```markdown
## Checkpoint — <date> — <track>

- Branch:
- Base / HEAD:
- Commits:
- Scope changed:
- Files:
- Tests actually run (command -> result):
- Not run / NOT_TESTED:
- Blockers:
- Security / data-loss / license notes:
- Next command:
- Push status / remote SHA:
```

For a docs-only checkpoint, say plainly that no build or tests were run and
record `git diff --check` separately from test evidence.

## 8. Open decisions and next priorities

| Open decision                                                   | Blocks                    | Current state                                                             |
| --------------------------------------------------------------- | ------------------------- | ------------------------------------------------------------------------- |
| BTV-3: accept JSON `1.0` or require the exact integer token `1` | Stable bundle/interface   | Unresolved                                                                |
| BTV-7: preserve, reject, or report unknown per-record fields    | Stable bundle/interface   | Unresolved                                                                |
| Source-account identity and cross-account detection             | Import safety UX/contract | No account field; target-account warning only                             |
| Replace transaction/rollback design                             | Replace feature           | `BLOCKED`; application snapshot is not a transaction                      |
| Signing, source authentication, anti-replay                     | Trust model               | Not implemented                                                           |
| Tools OpenPGP v6, MDC/recipient verification, `.hrtb` stability | Tools release/interface   | Unproven; MDC relies on the library, `inspect` does not verify recipients |
| Windows durability/ACL/SMB behavior                             | Platform support claim    | `NOT_TESTED`; native CI is evidence, not proof                            |
| Production/License Gate/Docker approval                         | Release/deployment        | `BLOCKED`                                                                 |

Next priorities:

1. **Decide BTV-3 and BTV-7** and record the result in
   `BACKUP_TEST_VECTORS.md` and `TOOLS_INTERFACE.md` before calling the contract
   stable.
2. **Review PR #129 and its current CI**, but do not merge on the basis of this
   handoff. The website PR is ready for review, not production-ready.
3. **Build the L3 browser harness** for export, preview, confirmation, merge,
   preference effects, and the account/plaintext warnings.
4. **Run the local backend smoke matrix** and record both evidence and
   `NOT_TESTED` limits; keep production bindings out of scope.
5. **Close Tools verification gaps** (v6/MDC/recipient verification and Windows
   durability) before any Tools release claim.
6. **Design signing/source authentication/anti-replay and Replace transaction
   semantics** only after the contract is frozen and the threat model is
   approved.

No production, L3, encryption-at-rest, all-or-nothing, or vulnerability-free
claim is made by this document.
