# L2 Production Test Seam Design

**Status:** design only; `NEEDS_HUMAN_APPROVAL` for any production refactor.
**Scope:** test real production functions; do not implement L2, L3, or production restore.

## 1. Baseline

- Repo: `/home/haiyan/git/Oyama-s-HRT-Tracker`
- Branch: `agent/backup-schema-audit`
- HEAD before this work: `bbc5eee11686c3ad60e75f1d92755da08cf623bc`
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

## 3. Minimal seam proposal

### Option A — extract pure helpers with an injected storage interface (recommended)

Proposed production refactor, **not implemented in this work item**:

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
