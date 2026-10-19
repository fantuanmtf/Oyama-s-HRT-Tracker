# Backup Schema Audit — Handoff

## Goal and scope

Audit the production data model and export/restore paths to define what a
complete-device backup (Option B) would require. This work item is documentation
and test planning only; it does not change production code, schema, dependencies,
Docker, or releases.

## Baseline

- Repository: `Oyama-s-HRT-Tracker`
- Base branch: `Privacy/Add-GPG`
- Base commit: `5c5d9fe39945c3f03c740bbaf76f313be20f7dbe`
- Work branch: `agent/backup-schema-audit`
- Origin: `git@github.com:fantuanmtf/Oyama-s-HRT-Tracker.git` (user fork)
- Upstream: `https://github.com/xunxunProjects/Oyama-s-HRT-Tracker.git`

## Commits

| Commit    | Purpose                                                                       |
| --------- | ----------------------------------------------------------------------------- |
| `4f36e1b` | Map production data model, persistence, export, and sync behavior             |
| `cb4cd10` | Document export/restore coverage gaps and test blind spots                    |
| `f218d99` | Define current-record vs complete-device backup scope and synthetic test plan |

## Evidence

- `PRODUCTION_DATA_MODEL.md`: field matrix, production type definitions,
  persistence keys, `Supply.pack` union, `PKCustomParams` defaults, export payload,
  and `applySyncedState` behavior.
- `EXPORT_RESTORE_GAPS.md`: current export/import flow, sanitizer behavior,
  v1/v2 compatibility, omissions, and test coverage gaps.
- `BACKUP_SCOPE_AND_TESTS.md`: Option A current-record backup, Option B
  complete-device backup, exclusions, risk register, and synthetic test plan
  T1–T13.

## Checks

- `git diff --check`: passed before each commit (whitespace/conflict markers).
- No production tests were run: this work item changes documentation only.
- No real health data, keys, tokens, passphrases, or backups were used or committed.
- Working tree was clean after the three documentation commits.

## Findings and open issues

- **PASS_WITH_CAVEATS** — current-record backup covers events, lab results,
  dose templates, tombstones, weight, PK parameters, mode, and metadata.
- **FAIL** — complete-device backup is not implemented: quick doses, supplies,
  plan items, calibration settings, and user preferences are local-only.
- **FAIL** — the Phase 1D prototype represents `Supply.pack` as a string, while
  production uses a discriminated object union with branch-specific fields.
- **REVIEW_REQUIRED** — `normalizeSupply` accepts any object with a string `id`,
  an object `pack`, and numeric `count` without validating route/ester or the
  pack branch; `normalizePlanItem` has no upper bounds on `everyDays`,
  `timesPerDay`, `doseMG`, or `extras`.
- **NOT_IMPLEMENTED** — restore paths for local-only data. `applySyncedState`
  writes records, tombstones, weight, and PK parameters, but not quick doses,
  supplies, plan, calibration, or preferences.
- **NEEDS_HUMAN_APPROVAL** — whether Option B is the product direction, and
  which preferences belong in a backup.
- **NOT_TESTED** — there are no direct tests for
  `sanitizeImportedEvents`, `sanitizeImportedLabResults`,
  `sanitizeImportedDoseTemplates`, `processImportedData`, `mergeImportedData`,
  `buildExportPayload`, `applySyncedState`, `normalizeSupply`,
  `normalizePlanItem`, `saveSupply`, or quick-dose persistence.

## Restore semantics work item

- **Commit:** `473414f` — `docs: design complete-device restore semantics`
- **Document:** `docs/backup-schema-audit/RESTORE_SEMANTICS.md`
- **Status:** `NEEDS_HUMAN_APPROVAL`
- Covers Option B restore for quick doses, supplies, plan items, calibration
  state, and selected preferences.
- Defines replace vs merge actions, all-or-nothing application, category rules,
  conflict policy, credential exclusions, account/mode boundaries, and
  synthetic tests T1–T15.
- **REVIEW_REQUIRED:** local-only arrays have no tombstones or `updatedAt`;
  conflict resolution cannot be authoritative without a product/schema decision.
- **FAIL:** current export remains record-only; local-only restore is not
  implemented.
- No production code was changed; no real data was used.

## Next work item

1. Decide whether to ship Option A only or commit to Option B.
2. If Option B is approved, approve the restore conflict policy and decide
   whether local-only arrays need tombstones/`updatedAt`.
3. Approve the preference inclusion/exclusion matrix.
4. Align the experimental HRTB schema with the production `Supply.pack` object
   union before any format stabilization.

## Push status

`473414f` is committed locally. Push only this branch to `origin` after the
final safety check; never to `upstream`. Do not merge, release, deploy, or
change protection rules.

## Synthetic Restore Test Harness (2026-10-19)

- **Document:** `SYNTHETIC_RESTORE_HARNESS.md`
- **Commit:** `196d7cf` `docs: design synthetic restore test harness`
- **Scope:** test design for Option B restore; no production implementation.
- **Layers:** L1 pure model (can run without production restore), L2 production-function tests (blocked until test seams exist), L3 end-to-end restore (blocked until production restore exists).
- **Coverage:** T1-T15, replace/merge, duplicate IDs, ID conflicts without `updatedAt`, invalid `Supply.pack`, plan bounds, calibration, credentials, unknown fields, empty-vs-missing, rollback/atomicity.
- **Status:** design PASS; L1 NOT_TESTED; L2/L3 BLOCKED; product conflict policy and preference scope NEEDS_HUMAN_APPROVAL.
- **Push:** this handoff is committed after `196d7cf`; push status is recorded at the final checkpoint.

### Open decisions

1. Option B include/exclude list for quickDoses, supplies, plan, calibration and preferences.
2. Default conflict policy where local-only records lack `updatedAt`/tombstones.
3. Whether to add timestamps/tombstones to local-only schemas (schema change; needs audit).
4. Which preferences are in Option B.
5. Replace-only vs merge support for Option B.

### L1 synthetic restore harness (implemented)

- **Goal:** execute T1-T15 against a pure, isolated model without production imports or network access.
- **Code:** `l1-harness/l1_restore_model.py`, `l1-harness/test_l1_restore_model.py`.
- **Results:** `L1_HARNESS_RESULTS.md`.
- **Command:** `python3 -m unittest -v test_l1_restore_model.py`.
- **Result:** 15/15 tests PASS; 0.005s.
- **Commit:** `eaa0ad6` (`test: add isolated L1 restore model harness`); follow-up commit removes accidentally committed `__pycache__` and adds `.gitignore`.
- **Status:** L1 PASS; L2 production-function tests BLOCKED; L3 end-to-end restore BLOCKED; production restore NOT_IMPLEMENTED; conflict/preference policy NEEDS_HUMAN_APPROVAL.
- **Limits:** L1 validates only the synthetic model and test specification. It does not prove production export/import, transaction, OpenPGP, UI, or storage behavior.
- **Push:** branch is pushed through `76777fe`; the L1 follow-up commit is recorded at the final checkpoint.

### Open decisions (L1)

1. Which Option B categories are in product scope.
2. Conflict policy where local-only records lack `updatedAt`/tombstones.
3. Whether to add timestamps/tombstones to local-only schemas.
4. Which preferences are in Option B.
5. Replace-only vs merge support for Option B.
6. Whether L2 tests may import production TypeScript modules directly or require extracted fixtures.

## L2 production test seam design

- **Document:** `L2_TEST_SEAM_DESIGN.md`.
- **Status:** design PASS; direct pure-function tests can start now; hook-closure tests BLOCKED until a seam is approved; L3 BLOCKED until production restore exists.
- **Directly testable:** `normalizeSupply`, `packUnitMG`, `packKindsFor`, `defaultPack`, `normalizePlanItem`.
- **Not directly testable:** `sanitizeImportedEvents`, `sanitizeImportedLabResults`, `sanitizeImportedTemplates`, `processImportedData`, `mergeImportedData`, `buildExportPayload`, `applySyncedState` (closures inside `useAppData`).
- **Recommended seam:** extract pure export/apply helpers with an injected `StorageLike` interface, then test them with an in-memory storage. This is a production refactor and requires separate approval.
- **Transaction risk:** restore spans multiple `localStorage` keys; there is no transaction or proven rollback. All-or-nothing restore remains BLOCKED.
- **Coverage matrix:** L2-01 to L2-17; see the document.
- **Push:** pending until this work item is committed.

## L2 product/platform supplement

- **Confirmed Option B scope:** events, labResults, doseTemplates, deletions, weight, pkParams, quickDoses, supplies, plan/planItems, calibrationMethod, calibrationHistoryMode.
- **Preference whitelist:** `theme` = `app-theme` and `language` = `hrt-lang` confirmed; `display` and `reminders` are UNKNOWN category-to-key mappings pending human decision. Do not invent keys.
- **Never included:** auth token/session/password, cloud key, private keys/passphrases, device passes, TOTP, passkeys, device identity.
- **Conflict rules:** same ID without reliable `updatedAt` must preserve both or remain pending; no silent overwrite or discard. Merge must report added/updated/skipped/conflict/invalid. Corrupt/unsafe input is rejected before writes.
- **Replace:** preview account/mode/categories/counts, explicit confirmation, and rollback or verified atomic commit; otherwise BLOCKED.
- **Platform scope:** Windows and Linux only. Linux no-replace link behavior is PASS from Phase 1F; Windows `os.Link`, ACL/`Chmod`, directory sync, path and concurrency behavior are NOT_TESTED. Apple platforms are out of scope.
- **L2 matrix:** L2-01..L2-24, including product-scope, preference mapping, conflict reporting, replace preview, and Windows/Linux tests.
- **Status:** L2 design PASS; L2 implementation BLOCKED; L3 BLOCKED; Windows NOT_TESTED; License Gate/Docker/production BLOCKED.
- **Push:** this supplement is committed after `1e528c2`; final push status is recorded at the checkpoint.

## L2 Seam A implementation

- **Files:** `src/utils/backupState.ts`, `src/hooks/useAppData.ts`, `tests/backupState.test.ts`.
- **Behavior:** export/apply storage logic extracted to pure helpers with `StorageLike`; active-mode React state updates remain in the hook.
- **Tests:** 8 new synthetic tests; full suite 16 files / 366 passed / 4 expected fail; `tsc --noEmit` and `git diff --check` pass.
- **Transaction:** partial-write test confirms earlier keys remain when a later `setItem` throws. Replace all-or-nothing remains BLOCKED.
- **Platform:** Linux only. Windows `os.Link`, ACL/permissions, path and concurrency behavior NOT_TESTED.
- **Status:** L2 Seam A implementation PASS for tested surface; production restore/export/rollback NOT implemented; Option B still NEEDS_HUMAN_APPROVAL; License Gate/Docker/production BLOCKED.
- **Push:** implementation commit and this handoff update are recorded at the checkpoint.

## Platform file-publish CI (Linux/Windows)

- **Workflow:** `.github/workflows/platform-filepublish.yml` runs on pull requests, pushes to `main` and `agent/**`, and manual dispatch. Matrix: `ubuntu-latest` and `windows-latest`; read-only permissions; `go vet` and `go test` in `tests/platform/filepublish`.
- **Harness:** test-only Go module using only the standard library. `AtomicWrite` uses `CreateTemp`, fsync, close, and `os.Link` no-replace publish; `CleanupStaleTemps` only removes regular `.hrt-tmp-*` files older than one hour.
- **Matrix:** `P-01`..`P-12` in `PLATFORM_TEST_MATRIX.md`: existing target unchanged, eight concurrent writers with one winner, cleanup ownership, missing parent, Unix `0600`, Unicode/space path, directory fsync best-effort, crash cleanup, Windows ACL/hard-link/SMB/file-lock behavior.
- **Local Linux:** `go test -v ./...` 6/6 PASS in 0.056s.
- **Windows:** native CI is configured; actual Windows execution and ACL/hard-link/file-lock behavior remain `NOT_TESTED`.
- **Status:** platform harness PASS on Linux; Windows `NOT_TESTED`; crash cleanup `REVIEW_REQUIRED`; production Replace/Merge, atomic restore, License Gate, Docker and production integration remain `BLOCKED`.
- **Push:** pending at the time of this handoff; final checkpoint records the remote SHA.

### Platform CI actual result

- GitHub Actions run `39147752217` for head `7a7d00ac1676e290fbefe05ab496674d37d5baa5` completed `success`.
- Both `File publish (ubuntu-latest)` and `File publish (windows-latest)` jobs passed.
- URL: https://github.com/fantuanmtf/Oyama-s-HRT-Tracker/actions/runs/39147752217
- Windows ACL equivalence, SMB/network filesystem behavior, cross-process locking, crash cleanup and application-level atomic restore remain `NOT_TESTED`/`BLOCKED`.

### P-13/P-14 platform additions

- Added `publish_crossprocess_test.go`: four-process no-replace race with a start barrier, plus a Windows-only `icacls` ACL smoke test.
- Linux: all Go tests PASS, including cross-process; Windows ACL test SKIPs on Linux.
- Windows native run remains NOT_TESTED until CI completes; Windows ACL result is smoke evidence only, not a security guarantee.

## Native CI: cross-process and Windows ACL

- **Run:** [39148724074](https://github.com/fantuanmtf/Oyama-s-HRT-Tracker/actions/runs/39148724074), head `41836e1688b4c3ae383c245a5109953141af3200`, `completed / success`.
- **Jobs:** `File publish (ubuntu-latest)` success; `File publish (windows-latest)` success.
- **Coverage:** cross-process publish winner and Windows `icacls` ACL smoke check executed on native Windows.
- **Status:** Windows ACL smoke `PASS`; SMB/network filesystem, hard-link privilege edge cases, crash cleanup and application-level Replace/Merge atomicity remain `NOT_TESTED` or `BLOCKED`.
- **Push:** pending at the time of this handoff; final checkpoint records the remote SHA.

## L2 import/merge seam implementation (Task B)

- **Code + tests commit:** `fa4c416` — `refactor: extract backup import reconciliation seam`.
- **Files:** `src/utils/backupImport.ts`, `src/hooks/useAppData.ts`, `tests/backupImport.test.ts`.
- **What changed:** `processImportedData` / `mergeImportedData` payload parsing, replacement and merge decisions, v1 cross-mode routing, replacement tombstones, and merge counts moved into pure functions with an injected `StorageLike` and clock. The hook still owns React state, dialogs, and `setWeight` / `setPkParams`.
- **API compatibility:** existing `backupState.ts` exports are unchanged; `DoseTemplate` / `QuickDose` re-exports remain; the hook's returned object is unchanged.
- **Synthetic L2 coverage:** v2 modes, v1 flat object and bare array, merge by id, replacement tombstones, tombstone clearing, cross-mode records, malformed/oversized input, duplicate ids (characterized as preserved), credentials ignored, empty vs missing arrays, unknown fields/modes, process weight clamping, merge weight behavior, and second-run idempotence.
- **Checks actually run:**
  - `npx tsc --noEmit` — pass.
  - `npm test` — 17 files passed; 377 tests passed, 4 expected fail (381 total); 16.39s.
  - `node node_modules/vitest/vitest.mjs run tests/backupImport.test.ts` — 11/11 pass.
  - `git diff --check` — pass.
  - `npx eslint src/utils/backupImport.ts tests/backupImport.test.ts src/hooks/useAppData.ts` — no errors; warnings only (existing hook effects plus `any` warnings).
- **Status:** L2 import/merge surface PASS for the tested functions. Production restore, Option B export/restore, and L3 end-to-end remain `BLOCKED` / `NOT_IMPLEMENTED`.
- **Remaining limitations:** no multi-key transaction or rollback; failure-path tombstone/state ordering differs from the old inline code (success path identical); duplicate ids are preserved; unknown fields are ignored; merge counts other-mode templates differently from other-mode events/labs; hook wiring is not rendered in tests; Windows/browser/OpenPGP behavior remains unverified.
- **Safety:** tests use fictional ids, dates, doses, labs, and synthetic credential strings only. No real health data, credentials, keys, or backups were used.
- **Push:** this handoff update is committed after `fa4c416`; the final checkpoint records the remote SHA.

### Open decisions (unchanged)

1. Option B include/exclude and preference scope.
2. Conflict representation for same-id records without reliable `updatedAt`.
3. Replace transaction/rollback mechanism.
4. Whether duplicate ids in a backup should reject the category or be deduplicated.
5. Whether unknown critical fields/versions should reject the import rather than be ignored.
6. Whether the React hook import wrappers need a separate hook/browser test harness.

## Website complete-backup bundle checkpoint (2026-10-19)

- **Branch:** `agent/add-gpg-website`, created from the approved audit tip
  `9874c40` (a descendant of `Privacy/Add-GPG` @ `5c5d9fe`). The audit tip
  already carries the L2 seams `0918bee` and `fa4c416`, so no cherry-picks or
  history rewrites were needed.
- **Implementation commits:**
  - `0e80021` — `feat: add complete backup bundle data layer`
  - `b70ac2d` — `feat: wire unencrypted complete backup export and merge import`
  - `1c45860` — `fix: reject non-positive supply lead days`
  - `f05d92c` — `fix: preserve bundle tombstones exactly`
- **Files:**
  - `src/utils/backupBundle.ts` — versioned envelope, strict validators,
    export builder, preview/conflict inspector, merge-only import.
  - `tests/backupBundle.test.ts` — 10 synthetic tests.
  - `src/hooks/useAppData.ts` — export/preview/apply wrappers; active-mode state
    install after merge.
  - `src/App.tsx` — bundle detection in the existing import path, preview
    dialog, preference application, export download.
  - `src/components/ExportSection.tsx`, `src/pages/ExportSettings.tsx`,
    `src/pages/ImportSettings.tsx` — unencrypted labels and Tools guidance.
  - `src/i18n/translations.ts` — Chinese and English strings; other packs fall
    back to English.
  - `docs/backup-schema-audit/TOOLS_INTERFACE.md` — hand-off format and
    offline Tools boundary.
- **Envelope:** `format: "oyama-hrt-complete-backup"`, `formatVersion: 1`,
  `createdAt`, `mode`, `modes`, `weight`/`weightUpdatedAt`,
  `pkParams`/`pkParamsUpdatedAt`, `calibration`, `preferences`.
- **Covered data:** per-mode events, labs, templates, deletions, quick doses,
  supplies, plan; shared weight and PK params; calibration method/history
  mode; portable preferences.
- **Supply.pack:** production object union only
  (`vial|pill|pump|sachet|patch|bulk`). String packs, unknown kinds, and missing
  branch fields reject the supplies category; no legacy string coercion.
- **Preference allowlist:** `app-theme`, `hrt-lang`, `app-show-prediction`,
  `app-hide-site-label`, `hrt-supply-lead-days` only. `app-auto-backup`,
  `app-dev-mode`, onboarding/site-notice flags, cosmetic settings, and all
  credentials are excluded by construction.
- **Import:** parse and validate the entire envelope, show creation time,
  source mode, per-mode counts, conflicts, invalid records, blocked categories,
  and ignored preferences, then merge only safe categories. Same id with a
  reliable newer `updatedAt` updates; older is skipped; same id without a
  reliable timestamp, equal timestamps with different content, or any same id
  in quick doses/supplies/plan aborts that category (`C1-A`). Duplicate ids
  abort the category. Tombstones are unioned and written back exactly (no age
  pruning at restore); restored records clear their own tombstones.
- **Replace:** not implemented; explicitly marked `BLOCKED`. No all-or-nothing
  or rollback claim is made.
- **Encryption:** none added. No OpenPGP.js/LGPL, no keys, no passphrases, no
  cold-backup encryption claim. The website file is plaintext JSON; encryption
  belongs to the offline Tools workflow (`TOOLS_INTERFACE.md`).
- **Checks actually run:**
  - `npx tsc --noEmit` — pass.
  - `node node_modules/vitest/vitest.mjs run` — 18 files passed; 387 tests
    passed, 4 expected fail (391 total); 26.84s.
  - `node node_modules/vitest/vitest.mjs run tests/backupBundle.test.ts` — 10/10
    pass.
  - `npx prettier --check` on changed source/test files — pass.
  - `npx eslint src/utils/backupBundle.ts tests/backupBundle.test.ts` — no
    errors.
  - `npm run build` — Vite production build pass (chunk-size warning only).
  - `git diff --check` — pass.
- **Synthetic coverage:** both-mode round-trip, all six pack branches, string
  and malformed pack rejection, duplicate ids, invalid records, unknown
  format/version, empty-vs-missing arrays, same-id conflict abort, newer/older
  timestamp behavior, preference whitelist, and credential exclusion from both
  export and import.
- **Status:** website complete-backup data layer and UI checkpoint `PASS` for
  the tested surface. This is not production readiness, not a security audit,
  not OpenPGP/cold-backup encryption, and not a proven restore rollback.
- **Not proven / remaining:** no browser/L3 end-to-end test for the new rows or
  dialog; no multi-key transaction; no Replace; no Tools-side OpenPGP command;
  Windows/Linux browser behavior and platform file handling remain outside this
  checkpoint.
- **Push:** pending at the time of this handoff; the final checkpoint records
  the remote SHA for `origin/agent/add-gpg-website`.

### Open decisions after the website checkpoint

1. Replace/rollback transaction design remains `BLOCKED`.
2. Whether local-only records should gain `updatedAt`/tombstones.
3. Whether a future UI should let the user resolve same-id conflicts per
   category instead of aborting the category.
4. Tools-side OpenPGP CLI/file behavior and its own test matrix.
5. Browser/L3 test harness for export, preview, merge, and preference effects.

## Website complete-backup security fixes (2026-10-19)

- **Branch/worktree:** `agent/add-gpg-website` in
  `/tmp/opencode/hrt-fix-website-p0`; base `32edc7068423916f8ab7cfa416ae7bf31d2f7609`.
- **Fix commit:** `d4b47a9` — `fix: harden complete-backup merge against unsafe
tombstones and timestamps`.
- **Files:** `src/utils/backupBundle.ts`, `src/utils/backupState.ts`,
  `src/App.tsx`, `src/pages/ImportSettings.tsx`, `src/i18n/translations.ts`,
  `tests/backupBundle.test.ts`.

### Fixes

- Incoming tombstones are bounded before use: timestamps beyond
  `now + 5 minutes` are rejected, entries older than the existing 180-day sync
  TTL are dropped, and each record kind is capped at
  `TOMBSTONE_MAX_PER_KIND` (5000). Every rejected/expired/capped entry is
  reported in the preview (`issues`, per-kind `deletionImpact.droppedIds`).
- An incoming tombstone no longer deletes a local record whose `updatedAt` is
  newer; the tombstone is skipped and the per-kind conflict is reported.
  Tombstones for a record category blocked by validation are not applied.
- A local tombstone newer than (or equal to) an incoming record prevents the
  record from being restored and is not cleared. A strictly newer incoming
  record clears the tombstone and both actions are reported.
- Future `updatedAt`, `weightUpdatedAt`, and `pkParamsUpdatedAt` beyond the
  5-minute skew allowance are rejected (category/scalar blocked); legitimate
  older timestamps are preserved.
- Complete-backup merge now builds all writes first, snapshots every target
  key, and restores the snapshot if a write throws. If rollback itself fails,
  the thrown message and UI copy say partial writes may have occurred. The
  data layer makes no local-storage transaction claim.
- Entry arrays reuse `MAX_IMPORT_ENTRIES` (20000). `timesPerDay` is finite and
  bounded to 1..24; `everyDays` is finite and bounded to 1..3650; NaN and
  Infinity are rejected.
- Preview text now shows weight, PK parameter keys/values, calibration, and
  per-kind deletion impact (counts plus ids where available). Calibration
  overwrite has a separate explicit confirmation dialog. Equal-time scalar
  values that differ are reported as conflicts and blocked instead of being
  silently skipped.
- Import settings copy now distinguishes regular record import (replaces) from
  complete-backup import (merge only).
- Credential and excluded-preference handling is unchanged: the allowlist still
  excludes `auth_token`, `auth_user`, `weak_password`, `login_device_passes`,
  `enc_key`, `app-auto-backup`, `app-dev-mode`, onboarding/site-notice flags,
  and cosmetic settings.

### Tests actually run

- `npx tsc --noEmit` — pass.
- `node node_modules/vitest/vitest.mjs run tests/backupBundle.test.ts` —
  20/20 pass.
- `node node_modules/vitest/vitest.mjs run` — 18 files pass; 397 passed,
  4 expected fail (401 total).
- `npm run build` — Vite production build pass (existing chunk-size warning
  only).
- `npx prettier --check` on changed source/test files — pass.
- `npx eslint` on changed files — no errors; existing `any` warnings only.
- `git diff --check` — pass.

New synthetic cases cover: tombstone clock-skew/TTL/cap bounds, newer-local
record protection, no resurrection from an older bundle, tombstone clearing by
a newer record, future record/scalar timestamps, mid-write rollback, failed
rollback partial-state reporting, plan numeric bounds, the 20000-entry cap,
equal-time weight/PK conflicts, calibration overwrite preview, and preview
deletion counts/ids.

### Remaining limits

- Rollback is an application-level snapshot/restore, not a `localStorage`
  transaction. It is proven only for the in-memory `StorageLike` failure cases
  in tests; quota, browser crash, or a storage that fails every write can still
  leave partial state, which is why the UI says partial writes may have
  occurred.
- The bounds above apply to complete-backup import. The cloud sync merge path
  (`src/utils/syncMerge.ts`) was not changed in this work item.
- No browser/L3 end-to-end test, no Replace implementation, no OpenPGP/key/
  passphrase work, and no schema, database, Worker, Docker, or License changes.
- This checkpoint does not clear L3, production readiness, the License Gate, or
  Docker, and it does not claim all-or-nothing restore beyond the tested
  snapshot rollback.
- Push status: this handoff update is committed after `d4b47a9`; the final
  remote SHA is recorded at the checkpoint.

## Website complete-backup confirmation supplements (2026-10-19)

- **Branch/worktree:** `agent/add-gpg-website` in
  `/tmp/opencode/hrt-fix-website-p0`; base `82381f9b1f7db3fd866ee21d563ca6e3b64a274d`.
- **Code + tests commit:** `23637e4` — account and plaintext confirmations.
- **Files:**
  - `src/utils/backupWarnings.ts` — pure account-target and plaintext-warning
    copy helpers; no bundle fields.
  - `src/App.tsx` — preview names the current target account, warns that the
    format has no source-account identifier, makes the merge confirmation a
    danger confirm, and gates the complete-backup download behind an explicit
    plaintext-health-data confirmation.
  - `src/i18n/translations.ts` — Chinese and English copy for both
    confirmations; other packs fall back as before.
  - `tests/backupWarnings.test.ts` — five synthetic copy/account-target tests
    using the shipped English and Chinese strings.
  - `tests/backupBundle.test.ts` — one invariant test: no source-account field
    is added and `formatVersion` stays `1`.
  - `docs/backup-schema-audit/BACKUP_TEST_VECTORS.md` — website/Tools acceptance
    boundary vectors and authority/status.
- **Account warning:** uses the existing `useAuth()` user; shows the username
  when signed in or `this device (not signed in)` otherwise. It states that the
  bundle format records no source account, so a cross-account merge cannot be
  detected. No account ID was added to the envelope and `formatVersion` was not
  changed.
- **Plaintext export:** the download now requires a confirm whose copy says the
  bundle is unencrypted plaintext health data, recommends the offline Tools CLI
  (`hrt-cold encrypt`), and states that the website does not encrypt the file.
  Encrypted JSON, regular JSON, clipboard, CSV and PDF export paths are
  unchanged.
- **Parity document:** BTV-1 calibration `average`/`adaptive` (website
  authoritative), BTV-2 `createdAt` date-only vs RFC 3339 (Tools authoritative),
  BTV-3 `formatVersion: 1.0` (UNRESOLVED), BTV-4 duplicate JSON keys (Tools
  authoritative), BTV-5 `weight: null` and BTV-6 `weightUpdatedAt: 0` (website
  authoritative), BTV-7 unknown per-record fields (UNRESOLVED), and BTV-8
  tombstone TTL/skew/cap (website authoritative).
- **Checks actually run:**
  - `npx tsc --noEmit` — pass (no output).
  - Focused backup tests (`backupWarnings`, `backupBundle`) — 2 files passed;
    26 tests passed.
  - Full Vitest run — 19 files passed; 403 passed, 4 expected fail (407 total);
    14.33s.
  - `npx prettier --check` on all changed source/test/doc files — pass.
  - `npx eslint` on changed TS/TSX files — 0 errors; 1 pre-existing
    `@typescript-eslint/no-explicit-any` warning in `src/App.tsx`.
  - `npm run build` — Vite production build pass; existing large-chunk warning
    only.
  - `git diff --check` — pass.
- **Not changed / not claimed:** no Tools/CLI change, no OpenPGP/key/passphrase
  work, no new runtime dependency, no schema or `formatVersion` change, no
  Replace, signing, key management, L3, production restore rollback, browser
  end-to-end test, License Gate, Docker, or production approval.
- **Unresolved:** BTV-3 (`formatVersion: 1.0`) and BTV-7 (unknown per-record
  fields) remain product/interface decisions. A future source-account field is
  out of scope. Cross-account detection is impossible with the current format.
- **Push:** pending at the time of this handoff; the final checkpoint records
  the remote SHA for `origin/agent/add-gpg-website`.

### Final push checkpoint

- Code + docs commit `6aa8f3c098e0ba3808d049f46ed63fb4e0d4ba8f` was pushed
  non-force to `origin/agent/add-gpg-website` (`82381f9..6aa8f3c` fast-forward).
- `git ls-remote origin refs/heads/agent/add-gpg-website` returned the same SHA,
  and the worktree was clean before this docs-only note.
- This note is a follow-up commit on the same branch; no other ref, main,
  upstream, Tools checkout, PR, merge, or release was changed.

## Website complete-backup review fixes (2026-10-19)

- **Branch/worktree:** `agent/add-gpg-website` in
  `/tmp/opencode/hrt-fix-website-p1`; base `48ae09f9b49db7848916b1b33de303a36006971e`.
- **Tools reference:** `Recovery/Tools` @ `38c80fc` was read only through git
  objects; no Tools checkout, branch, or file was changed.
- **Code + tests commit:** `250eeefb4b03aa8705be29244f98c3d81623d186` —
  `fix: harden complete-backup export and merge semantics`.
- **Files:** `src/utils/backupBundle.ts`, `src/App.tsx`,
  `src/hooks/useAppData.ts`, `src/i18n/translations.ts`,
  `tests/backupBundle.test.ts`.

### Fixes

- **P1-1 expired local tombstone:** `reconcileBundle` prunes the local tombstone
  map with `pruneTombstones(..., now)` before `planCategoryMerge`. An expired
  local deletion no longer hides an older incoming record. Regression test:
  a 181-day-old local tombstone plus a 182-day-old incoming record restores the
  record.
- **P1-2 prototype ids:** record validation rejects `__proto__`, `constructor`,
  and `prototype`; tombstone validation already did. Local tombstone maps are
  re-copied into null-prototype objects, accepted/candidate maps use
  null-prototype storage, and every lookup uses `Object.hasOwn` (record
  indexes remain `Map`). Regression tests cover rejected record/tombstone ids
  and an unrelated `toString` record not being hidden by a local `constructor`
  key.
- **P1-3 export/importer parity:** export now shares `MAX_IMPORT_ENTRIES`
  (refuses above the limit), collapses duplicate ids to the newest copy (last
  copy without stamps) with a report, drops invalid records with a report, and
  reports dropped tombstone/stamp entries. A new
  `CompleteBackupExportReport`/`CompleteBackupExportError` surface carries the
  issues to the UI. One `isDrugCode` rule now governs events, templates, quick
  doses, supplies, and plan, so a cross-version `FUTURE1` ester round-trips in
  every category. Tests cover duplicates, >20,000 entries, invalid records, and
  the cross-version ester.
- **P1-4 clock-skew asymmetry:** export validators receive the same clock and
  future-stamp bound as import. Future records are dropped/reported, and future
  weight/PK stamps are omitted/reported. Tests cover each stamp and future
  tombstones.
- **P2-1 stale confirmation closure:** `AppContent` keeps a latest
  preview/apply/build snapshot in `completeBackupRef`. Every confirm path
  re-previews, compares a stable `completeBackupPreviewFingerprint`, re-opens
  the preview if counts/conflicts/scalars changed, and applies through the
  latest hook closure. The calibration gate and plaintext export confirmation
  use the same latest snapshot; export builds at confirm time.
- **P2-2 preferences-only:** a new pure `classifyCompleteBackupOutcome` returns
  `applied`, `settings-only`, or `nothing`. The UI applies returned preferences
  even when `applied` is empty and shows a settings-only result; tests cover the
  data-layer result.
- **P2-3 `applied`/`writeNeeded`:** `planDeletions` sets `writeNeeded` only when
  an accepted incoming tombstone survives, and a category blocked during
  reconciliation removes its accepted tombstones and recomputes the flag.
  `classifyCompleteBackupOutcome` reports `nothing` when every category is
  blocked, so the UI no longer claims success. Tests assert no `applied`
  entries, `nothing`, and zero storage writes for an all-blocked bundle.
- **P3 UI/report polish:** deletion-impact id lists use `formatIds` (five ids
  plus `+N more`); duplicate-id issues use the same helper. Same-bundle
  record+tombstone contradictions are reported with the winning side. The
  `let parsed: any` warning in `App.tsx` is now `unknown`; ESLint reports no
  `no-explicit-any` warning for the changed files.

### Tests actually run

- `npx tsc --noEmit` — pass.
- Focused backup suite — 1 file, 33/33 tests pass.
- Full Vitest — 19 files pass; 415 passed, 4 expected fail (419 total).
- `npm run build` — Vite production build pass (existing chunk-size warning
  only).
- `npx prettier --check` on changed source/test/doc files — pass.
- `npx eslint` on `src/App.tsx`, `src/utils/backupBundle.ts`,
  `tests/backupBundle.test.ts` — no errors or warnings. A broader run including
  `src/hooks/useAppData.ts` still shows the pre-existing hook warnings and two
  pre-existing `any` warnings outside this change.
- `git diff --check` — pass.

### P2-1 verification limit

The dialog-closure behavior has no browser/React test in this work item.
Exact blocker: `vitest.config.ts` uses `environment: 'node'` and includes only
`tests/**/*.test.ts`; there is no jsdom/happy-dom or React Testing Library
dependency; `AppContent` is not exported and requires the Dialog, Auth,
Language, HRT-mode, and PixelCat providers plus `localStorage`. The data-layer
mechanism used by the fix (re-preview against changed active arrays and compare
`completeBackupPreviewFingerprint`) is tested. The wiring inside `AppContent`
is reviewed but not rendered in a test, so this handoff does not claim a
browser-level regression test for P2-1.

### Remaining limits / not changed

- No L3/browser E2E, Replace, rollback transaction, signing, key management,
  DB/Worker/Docker/License work, Tools branch change, PR, merge, or release.
- Website import still uses `JSON.parse`, so duplicate JSON keys remain
  last-wins (BTV-4); case-variant keys are not normalized (BTV-16).
- Export and import are symmetric per device, but clocks more than five minutes
  apart can still reject an otherwise valid bundle (BTV-13).
- BTV-3 (`formatVersion: 1.0`) and BTV-7 (unknown per-record fields) remain
  unresolved product/interface decisions.
- The multi-key snapshot rollback remains application-level, not a
  `localStorage` transaction.
- Push status: pending at the time of this handoff; the final checkpoint records
  the remote SHA for `origin/agent/add-gpg-website`.
