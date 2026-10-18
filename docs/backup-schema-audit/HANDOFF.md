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

| Commit | Purpose |
|---|---|
| `4f36e1b` | Map production data model, persistence, export, and sync behavior |
| `cb4cd10` | Document export/restore coverage gaps and test blind spots |
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
