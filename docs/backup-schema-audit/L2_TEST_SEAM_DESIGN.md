# L2 Production Test Seam Design

**Status:** Task B import/merge extraction implemented; L2 import tests PASS for the tested surface; production restore remains `BLOCKED`.
**Scope:** test real production functions; do not implement L2, L3, or production restore.

## 1. Baseline

- Repo: `/home/haiyan/git/Oyama-s-HRT-Tracker`
- Branch: `agent/backup-schema-audit`
- HEAD before this work: `bbc5eee11686c3ad60e75f1d92755da08cf623bc`
- HEAD before Task B (import/merge extraction): `e17c000edc47b974176a7742043c73a4af23f221`
- Existing L1 model: `docs/backup-schema-audit/l1-harness/`
- Existing design: `RESTORE_SEMANTICS.md`, `SYNTHETIC_RESTORE_HARNESS.md`
- Test runner: Vitest 4, `environment: 'node'`, `tests/**/*.test.ts`, `tests/setup.ts`
- No `@testing-library/react`, `react-test-renderer`, jsdom, or happy-dom installed as project dependencies. Vitest may list optional peers, but they are not a test harness we can rely on without approval.

## 2. Function inventory

| Function | Source | Current access | Inputs / outputs | Side effects | Directly testable now? | Needed seam |
|---|---|---|---|---:|---|---|
| `normalizeSupply` | `src/utils/regimen.ts:643` | exported | `unknown -> Supply \| null` | none | **Yes** | none |
| `packUnitMG` | `src/utils/regimen.ts:595` | exported | `Pack -> number` | none | **Yes** | none |
| `packKindsFor` | `src/utils/regimen.ts:609` | exported | `Route -> PackKind[]` | none | **Yes** | none |
| `defaultPack` | `src/utils/regimen.ts:625` | exported | `PackKind -> Pack` | none | **Yes** | none |
| `normalizePlanItem` | `src/utils/plan.ts:141` | exported | `unknown -> PlanItem \| null` | none | **Yes** | none |
| `sanitizeImportedEvents` | `src/hooks/useAppData.ts:709` | **closure** | `unknown -> DoseEvent[]` | none | **No** | export pure helper or hook test |
| `sanitizeImportedLabResults` | `src/hooks/useAppData.ts:739` | **closure** | `unknown -> LabResult[]` | none | **No** | same |
| `sanitizeImportedTemplates` | `src/hooks/useAppData.ts:763` | **closure** | `unknown -> DoseTemplate[]` | none | **No** | same |
| `processImportedData` | `src/hooks/useAppData.ts:794` | hook return | parsed JSON, `{check}` | reads/writes `localStorage`; sets hook state | **No** | pure import engine + `StorageLike` |
| `mergeImportedData` | `src/hooks/useAppData.ts:1001` | hook return | parsed JSON | reads/writes `localStorage`; sets hook state | **No** | same |
| `buildExportPayload` | `src/hooks/useAppData.ts:1185` | hook return | none | reads `localStorage` | **No** | `buildExportPayloadFromStorage(owner, mode, storage)` |
| `applySyncedState` | `src/hooks/useAppData.ts:1230` | hook return | `SyncState` | reads/writes multiple `localStorage` keys; sets state | **No** | `applySyncedStateToStorage(state, owner, storage)` |
| `loadJSON` | `src/hooks/useAppData.ts:129` | closure | key/fallback | reads `localStorage` | **No** | storage adapter |
| `keyFor` / `sharedKey` | `src/hooks/useAppData.ts:126-127` | closures | mode/suffix | none | **No** | export key builders or test via hook |

`normalizeSupply` and `normalizePlanItem` are exported, so they can be tested immediately without modifying production code. The import/export/apply functions are defined inside the `useAppData` hook and depend on React contexts and `localStorage`; they are not importable by a plain Vitest test.

**Post-extraction note:** this table records the pre-extraction baseline. The import sanitizers and storage helpers now live in `src/utils/backupState.ts`; `processImportedDataToStorage`, `mergeImportedDataToStorage`, and `reconcileReplacementInStorage` live in `src/utils/backupImport.ts`; `buildExportPayloadFromStorage` and `applySyncedStateToStorage` live in `backupState.ts`. The hook methods are thin wrappers around these helpers.

## 3. Minimal seam proposal

### Option A — extract pure helpers with an injected storage interface (recommended)

Production refactor, implemented for export/apply in Seam A and for
import/merge/replace in Task B:

```ts
export interface StorageLike {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

export function buildExportPayloadFromStorage(owner: string, mode: Mode, storage: StorageLike): ExportPayload
export function applySyncedStateToStorage(state: SyncState, owner: string, storage: StorageLike): void
export function sanitizeImportedEvents(raw: unknown): DoseEvent[]
export function sanitizeImportedLabResults(raw: unknown): LabResult[]
export function sanitizeImportedTemplates(raw: unknown): DoseTemplate[]
export function processImportedDataToStorage(parsed: unknown, context: ImportStorageContext, options?: { check?: boolean; now?: number }): ProcessImportedDataResult
export function mergeImportedDataToStorage(parsed: unknown, context: ImportStorageContext, options?: { now?: number }): MergeImportedDataResult
export function reconcileReplacementInStorage(storage: StorageLike, owner: string, mode: ModeKey, kind: RecordKind, before: { id: string }[], after: { id: string }[], now?: number): void
```

- Move the existing logic without behavior changes.
- `useAppData` calls the same helpers with `window.localStorage`.
- Tests use an in-memory `StorageLike`, synthetic fixtures, and no React.
- This gives L2 direct coverage of the real code, not a model.
- Refactor is production code; **needs explicit approval**.

### Option B — hook-level tests

- Add `@testing-library/react` + jsdom/happy-dom and render `useAppData` with fake contexts.
- Pros: no production refactor.
- Cons: new dev dependencies, React context setup, asynchronous persistence effects, brittle; tests still interact with `localStorage`.
- Status: `REVIEW_REQUIRED`; do not add dependencies in this work item.

### Option C — browser E2E

- Real UI + real `localStorage`; this is L3, not L2.
- Status: `BLOCKED` until production restore exists.

**Recommendation:** Option A, as a behavior-preserving extraction reviewed separately. It is the smallest seam that tests the real export/import/apply functions without React or a full browser.

## 4. L2 test matrix

| ID | Target | Case | Expected | Can run now? |
|---|---|---|---|---|
| L2-01 | `normalizeSupply` | all six `Pack` branches, valid | accepted, exact branch fields preserved | Yes |
| L2-02 | `normalizeSupply` | string `pack` / unknown kind / missing field | rejected or explicit migration; no crash | Yes |
| L2-03 | `normalizeSupply` | vial `ml`/`mgPerMl` or pill/pump/sachet `mg` <= 0/NaN | rejected | Yes |
| L2-04 | `packUnitMG` / `packKindsFor` / `defaultPack` | each branch and route | correct units/allowed kinds/defaults | Yes |
| L2-05 | `normalizePlanItem` | `everyDays`/`timesPerDay`/`doseMG` boundaries | valid accepted/normalized; invalid rejected | Yes |
| L2-06 | `normalizePlanItem` | unknown route/ester, non-object extras | rejected or normalized per current contract | Yes |
| L2-07 | `sanitizeImportedEvents` | missing id/extras, bad route/time/dose, invalid ester | deterministic ids/defaults/clamping | After seam |
| L2-08 | `sanitizeImportedLabResults` | bad unit/assay, non-finite, missing fields | unit fallback; invalid records dropped | After seam |
| L2-09 | `sanitizeImportedTemplates` | bad route/ester/dose/createdAt | deterministic defaults; invalid dropped | After seam |
| L2-10 | `buildExportPayloadFromStorage` | both modes, tombs, weight, pkParams | exact v2 payload; no local-only fields | After seam |
| L2-11 | `buildExportPayloadFromStorage` | seeded `auth_token`, `enc_key`, `login_device_passes` | never present in payload | After seam |
| L2-12 | `applySyncedStateToStorage` | records/tombstones/scalars | writes expected keys only; no local-only keys | After seam |
| L2-13 | import `processImportedData` | v1/v2, missing fields, empty arrays, unknown fields | current documented behavior, no crash | After seam |
| L2-14 | import merge/replace | duplicate ids, replacement tombstones | deterministic; no duplicate ids | After seam |
| L2-15 | restore transaction | failure after some `setItem` calls | no partial user-visible state | **Blocked**: no transaction seam |
| L2-16 | credential handling | credentials in backup/state | rejected/ignored; never persisted | After seam |
| L2-17 | version compatibility | unknown critical version/fields | reject or explicit preview; no writes | After seam |

## 5. Transaction and all-or-nothing risk

`applySyncedState` writes multiple keys sequentially (`events`, `lab-results`, `dose-templates`, `weight`, `weight-at`, `pk-params`, `pk-params-at`) and then sets React state. `processImportedData` / `mergeImportedData` also write several keys directly.

`localStorage` offers no multi-key transaction. A `QuotaExceededError` or thrown exception after the first write leaves a partially applied restore. The L1 model's `apply_transaction` is only a model; it does **not** prove production atomicity.

Options for a future approved design:

1. **Staging prefix + commit marker**: write all values under a temporary namespace, then switch a single active-generation key; old generation retained until validation. Requires migration/cleanup.
2. **Snapshot + compensating rollback**: read all target keys first, apply, rollback on error; rollback itself can fail and is not crash-safe.
3. **Single JSON blob**: restore one key atomically; major schema/migration change and quota risk.
4. **IndexedDB transaction**: real transaction semantics; major storage migration and browser support work.

**Status:** production all-or-nothing restore `BLOCKED`; no option is approved. Do not claim L1 rollback proves production rollback.

## 6. Test fixtures

Use synthetic data only:

- two modes, each with events/labs/templates/deletions;
- quickDoses, supplies (all six pack branches), plan items;
- weight, pkParams, calibration;
- approved/undecided preference keys;
- credential keys seeded in storage to prove exclusion;
- malformed pack kinds, string pack, NaN/Infinity, missing ids, duplicate ids;
- v1 flat and v2 mode payloads;
- empty arrays vs absent fields.

Fixtures must be versioned and reviewed; they must not contain real health data or real keys.

## 7. Acceptance criteria

- Direct tests for all exported pure functions (`normalizeSupply`, `packUnitMG`, `packKindsFor`, `defaultPack`, `normalizePlanItem`).
- L2 harness uses production helpers, not a copy of L1 model logic.
- Import/export tests use real sanitizers and payload builders after an approved seam.
- Credential exclusion is tested from seeded storage.
- Transaction failure test proves no partial writes or is explicitly `BLOCKED`.
- No new production dependencies, no production behavior change in this design item.
- Results distinguish L1 model PASS from L2 production-code PASS.

## 8. Open decisions

1. Approve Option A extraction (`StorageLike` + pure helpers) or choose Option B/dev dependencies.
2. Exact contract for `normalizeSupply` invalid pack branches: reject vs legacy migration.
3. Whether local-only arrays gain `updatedAt`/tombstones.
4. Restore transaction mechanism.
5. Which preferences are in Option B.
6. Whether L2 may add test-only exports to production modules or must use a separate module.

## 9. Product decision supplement (Option B confirmed)

Option B is now the confirmed product direction for the backup data set. The following categories are in scope:

- `events`
- `labResults`
- `doseTemplates`
- `deletions`
- `weight`
- `pkParams`
- `quickDoses`
- `supplies`
- `plan` / `planItems`
- `calibrationMethod`
- `calibrationHistoryMode`

The product decision does **not** prove that every field is already exported, restorable, or safely cross-device. Each field still needs the production mapping and status recorded in `PRODUCTION_DATA_MODEL.md`.

### 9.1 Preference whitelist

| Product category | Production key candidate | Status |
|---|---|---|
| `theme` | `app-theme` | CONFIRMED key; cross-device restore NEEDS_DECISION |
| `language` | `hrt-lang` | CONFIRMED key; cross-device restore NEEDS_DECISION |
| `display` | candidate `app-show-prediction`, `app-hide-site-label`, `app-pixel-cats`, `app-pixel-cat-style` | `UNKNOWN` category-to-key mapping; NEEDS_DECISION |
| `reminders` | candidate `hrt-supply-lead-days` | `UNKNOWN` category-to-key mapping; NEEDS_DECISION |

**Never included:** `app-auto-backup` (could re-enable cloud upload), `app-dev-mode`, `site-notice-dismissed`, `app-onboarded`, `auth_token`, `auth_user`, `weak_password`, `login_device_passes`, `enc_key`, TOTP secrets, backup codes, passkeys, session identifiers, private OpenPGP keys, and passphrases.

The `display` and `reminders` rows are product categories, not confirmed localStorage keys. Do not silently equate them with candidates. If a preference has no confirmed key, mark it `UNKNOWN` and leave it out of the restore implementation until decided.

### 9.2 Conflict rules

| Situation | Required behavior |
|---|---|
| Same ID, reliable `updatedAt` exists | Use the timestamp only after documenting the comparison rule |
| Same ID, no reliable `updatedAt` | Preserve both records if the model can represent them; otherwise keep a pending conflict record. Do not silently choose a winner |
| Merge adds a new ID | Add and report as `added` |
| Merge updates an existing ID | Only under an approved conflict policy; report as `updated` |
| Merge skips a conflict | Report `skipped`; never silently discard |
| Invalid or duplicate ID in backup | Reject the category before writing; no partial mutation |
| Corrupt input / failed safety validation | Reject before any data change |
| Replace | Show target account, mode, categories and counts; require explicit confirmation |
| Replace failure | Restore the prior state or use a proven atomic commit; otherwise mark `BLOCKED` |

The current persistence layer has no multi-key transaction. A Replace implementation cannot claim all-or-nothing until a staging/rollback mechanism or atomic storage boundary is proven with tests.

### 9.3 L2 matrix additions

| ID | Test | Status |
|---|---|---|
| L2-18 | Option B category allowlist and exclusion list | NOT_TESTED |
| L2-19 | Preference category-to-key mapping; `UNKNOWN` categories are not restored | NOT_TESTED |
| L2-20 | Same-ID conflict without `updatedAt` preserves both or reports pending conflict | NOT_TESTED |
| L2-21 | Merge report contains added/updated/skipped/conflict/invalid counts | NOT_TESTED |
| L2-22 | Replace preview shows account, mode, categories and counts before confirmation | NOT_TESTED |
| L2-23 | Credentials seeded in storage never appear in payload or restore result | NOT_TESTED |
| L2-24 | Replace failure leaves prior state unchanged or is marked `BLOCKED` | BLOCKED |

## 10. Windows/Linux platform scope

Supported client platforms for this design are **Windows desktop** and **Linux desktop**. Android, iOS, iPadOS and macOS are out of scope; they must not be claimed as built or tested.

### 10.1 Current file-safety evidence

The isolated Phase 1F prototype uses `os.CreateTemp`, `os.Chmod`, `os.Link`, `os.Remove`, and directory `Sync`. On Linux, the no-replace publish and concurrent-writer tests passed. On Windows, the following remain `NOT_TESTED`:

- `os.Link`/hard-link availability, privileges and atomic no-replace semantics on NTFS;
- whether `os.Chmod(0600)` has meaningful confidentiality effect (Windows ACLs are the real boundary);
- directory `Sync` behavior and whether it returns an error that must be handled;
- file locking and concurrent-writer behavior;
- path length, reserved names, case-insensitivity and separator handling;
- temporary-file ACL inheritance and cleanup after abnormal termination.

The prototype must not claim Windows file safety from Linux test results.

### 10.2 Required platform tests

- GitHub Actions matrix: `ubuntu-latest` and `windows-latest`.
- Go unit tests for existing-file, concurrent-writer, temp cleanup and permission behavior.
- Permission assertions conditional on platform; Windows tests must inspect ACLs, not Unix mode bits.
- Explicit hard-link support/privilege test; if unsupported, return a clear error or a documented fallback, never a silent overwrite.
- Path, Unicode, long-path and case-collision tests.
- Crash/abnormal-exit temp-file behavior on both platforms.
- No macOS/Android/iOS jobs in this work item.

### 10.3 Status

| Item | Status |
|---|---|
| Linux no-replace prototype | PASS (Phase 1F isolated prototype) |
| Windows no-replace/ACL behavior | NOT_TESTED |
| Windows/Linux CI matrix | NOT_TESTED |
| Cross-platform transaction boundary | BLOCKED |
| macOS/Android/iOS support | OUT OF SCOPE |

## 11. Updated open decisions

1. Approve seam A (pure helpers + `StorageLike`) or choose seam B/C.
2. Resolve `display` and `reminders` to real keys or drop them.
3. Define conflict representation for same-ID/no-`updatedAt` cases.
4. Define the multi-key Replace transaction/rollback mechanism.
5. Approve or reject `normalizeSupply` strict pack-branch validation.
6. Decide whether L2 adds test-only exports or a separate test adapter.
7. Approve Windows CI and ACL-based file-protection tests.
8. Keep License Gate, Docker and production integration BLOCKED until separate approval.

## Implementation checkpoint (Seam A)

**Status:** Seam A implementation `PASS` for the tested L2 surface; production restore remains `BLOCKED`.

**Files:**

- `src/utils/backupState.ts` — pure storage helpers and sanitizers.
- `src/hooks/useAppData.ts` — delegates export/apply storage work to the helpers; keeps React state updates for the active mode.
- `tests/backupState.test.ts` — eight synthetic L2 tests.

**Behavior preserved:**

- `buildExportPayload` still returns the v2 shape, active-mode overlay, v1 flat mirrors, weight/PK stamps, and excludes credentials/local-only data.
- `applySyncedState` still sanitizes both modes before writes, writes records/tombstones and LWW weight/PK scalars, and updates active-mode React state.
- `DoseTemplate` and `QuickDose` remain re-exported from `useAppData`.

**Tests actually run:**

```text
node node_modules/vitest/vitest.mjs run
Test Files 16 passed (16)
Tests 366 passed | 4 expected fail (370)
Duration 18.25s
```

`tsc --noEmit` and `git diff --check` passed.

**Coverage added:**

- Supply pack object-union helpers and branches.
- String `Supply.pack` rejection.
- Unknown pack-kind characterization: current `normalizeSupply` accepts it; `REVIEW_REQUIRED`.
- Plan bounds/normalization.
- Import sanitizers.
- Export shape excludes credentials and local-only categories.
- Sync writes records/scalars and does not write local-only categories.
- Partial-write behavior when a later `localStorage.setItem` throws.

**Transaction finding:** a later write failure leaves earlier keys written. `applySyncedStateToStorage` is not atomic; Replace all-or-nothing remains `BLOCKED`.

**Platform:** Linux tests only. Windows `os.Link`, ACL/permission semantics, concurrent-write behavior, path handling and failure semantics are `NOT_TESTED`; native Windows CI is not yet configured.

**Not proven:** production restore, Option B export/restore, cross-key rollback, browser/UI behavior, OpenPGP, and L3 end-to-end. No production restore implementation was added.

## Implementation checkpoint (Task B: import/merge seam)

**Status:** import/merge/replace reconciliation extracted and tested; production restore remains `BLOCKED`.

**Files:**

- `src/utils/backupImport.ts` — pure import engine with an injected `StorageLike` and explicit clock.
- `src/hooks/useAppData.ts` — thin wrappers for React state, dialogs, and the weight/PK setters.
- `tests/backupImport.test.ts` — 11 synthetic L2 tests.
- `src/utils/backupState.ts` — unchanged public sanitizers/storage helpers; no existing export was removed or renamed.

**Extraction boundary:**

- Payload parsing and validation: v2 `modes`, flat v1 object, and bare v1 array.
- Replacement semantics: present arrays replace; absent arrays are silence; present empty arrays clear.
- Replacement tombstones: dropped ids are recorded; restored ids lose any tombstone that would delete them again.
- Merge semantics: union by id, keep local on duplicate, clear tombstones for incoming ids, report only newly added records.
- v1 cross-mode routing by ester (`isTestosteroneEster`) and lab unit (`isT_LabUnit`); v2 records stay in their mode block.
- `check` validation path: same sanitisation and "No valid entries" test, but no storage or tombstone write.
- The hook still owns `setEvents`/`setLabResults`/`setDoseTemplates`, `setWeight`/`setPkParams`, and `showDialog`; no UI, wording, or dialog flow changed.

**Synthetic coverage added:**

- v2 modes import, including active and other-mode replacement.
- v1 flat object and bare event-array import.
- Merge by id, including idempotence on a second run.
- Replacement tombstones for missing records and tombstone clearing for restored records.
- Cross-mode v1 events and labs; v2 records remain in their mode block.
- Malformed input, unknown routes, and the 20,000-entry ceiling.
- Duplicate ids: current behavior preserves duplicates; characterized, not silently "fixed".
- Credential fields ignored and seeded credentials left untouched.
- Empty vs missing arrays on replace and merge.
- Unknown top-level fields, unknown record fields, and unknown mode names ignored.
- Process weight clamped and merge weight only increasing, matching the previous paths.

**Checks actually run:**

```text
node node_modules/vitest/vitest.mjs run tests/backupImport.test.ts
Test Files 1 passed (1)
Tests 11 passed (11)

npx tsc --noEmit
(no output; pass)

npm test
Test Files 17 passed (17)
Tests 377 passed | 4 expected fail (381)

git diff --check
(no output; pass)
```

**Remaining limitations:**

- No multi-key transaction. Both import paths still write storage keys sequentially; a later failure can leave earlier keys written. Replace all-or-nothing remains `BLOCKED`.
- On a storage failure, the extracted helper completes active-mode tombstone reconciliation before the hook installs React state, whereas the old inline code interleaved each reconcile with its setter. The success path is identical; the failure-path interleaving is a residual difference and is not covered by a browser test.
- Duplicate ids inside a backup are preserved by the current sanitizers and import paths. Deduplication is not implemented and needs a product decision.
- Merge reports other-mode events and labs but not other-mode templates, matching the historical count.
- Merge accepts any positive numeric weight (no finite/upper bound), matching the historical behavior; process import clamps.
- Unknown fields are ignored rather than rejected; there is no strict schema or version gate.
- Credentials are excluded by construction in these paths, but this is not a security audit.
- The React hook wiring itself is not rendered in tests; L2 covers the extracted production functions, not browser/localStorage behavior.
- L2 tests do not prove production restore, Option B export/restore, rollback, OpenPGP, UI confirmation, Windows behavior, or License Gate/Docker/production readiness.
