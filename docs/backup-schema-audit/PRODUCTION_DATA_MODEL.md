# Backup Schema Audit — Production Data Model

**Baseline:** `Privacy/Add-GPG` @ `5c5d9fe39945c3f03c740bbaf76f313be20f7dbe`  
**Audit branch:** `agent/backup-schema-audit`  
**Date:** 2026-10-19  
**Status:** read-only audit; no production code changed

## Scope and evidence rules

This document maps the data that a full-device backup would need. It is based on
production TypeScript sources, persistence code, sanitizers, and export/import
paths. It does not use real user data and does not run production exports.

Labels:

- **VERIFIED** — directly supported by production code.
- **INFERRED** — reasonable conclusion from code, not directly tested.
- **UNKNOWN** — not established from available evidence.
- **NEEDS_HUMAN_APPROVAL** — product/security decision, not an audit result.

## Storage namespace

`src/hooks/useAppData.ts` uses:

- Signed-out owner: `local`, keys `hrt-<suffix>`.
- Signed-in owner: `u<userId>`, keys `hrt-u<id>-<suffix>`.
- Transmasc mode adds `masc-` after the owner prefix.
- Mode suffixes: `events`, `lab-results`, `dose-templates`, `quick-doses`,
  `deletions`.
- Shared suffixes: `weight`, `pk-params`, `cal-method`, `cal-history-mode`,
  `weight-at`, `pk-params-at`.

Supplies and plan use additional mode keys (`...supplies`, `...plan`) in the
same namespace; their normalizers are in `src/utils/regimen.ts` and
`src/utils/plan.ts`.

## Field matrix

| Entity / field group | Production type and fields | Optional / default | Persistence | Current export | Current restore | Status / risk |
|---|---|---|---|---|---|---|
| `DoseEvent` | `logic.ts:108`: `id`, `route: Route`, `timeH`, `doseMG`, `ester: Ester`, `extras`, `updatedAt?` | `updatedAt` optional; import generates missing id | `hrt-[u<id>-][masc-]events` | Yes (v2 and v1 mirror) | Sanitized import; sync applies | **VERIFIED**. `timeH` is hours since 1970; dose bounded by import sanitizer; unknown ester strings preserved |
| `LabResult` | `logic.ts:137`: `id`, `timeH`, `concValue`, `unit`, `assay?`, `updatedAt?` | `assay` optional; invalid unit defaults to `pmol/l` on import | `...lab-results` | Yes | Sanitized import; sync applies | **VERIFIED**. Units limited to `pg/ml`, `pmol/l`, `ng/dl`, `nmol/l`; assay to `lcms`/`immuno` |
| `DoseTemplate` | `src/hooks/useAppData.ts:87`: `id`, `name`, `route`, `ester`, `doseMG`, `extras`, `createdAt`, `updatedAt?` | `createdAt` defaults to now if missing | `...dose-templates` | Yes | Sanitized import; sync applies | **VERIFIED**. `extras` is `any`; route/ester validation |
| `deletions` / tombstones | Per-mode deletion records | Pruned by age; max 5000 per kind (`syncMerge.ts`) | `...deletions` | Yes | Replacement import records dropped records as deletions; sync merges | **VERIFIED**. Required for multi-device delete semantics |
| `QuickDose` | `useAppData.ts:99`: `id`, `route`, `ester`, `value`, `createdAt` | No export metadata | `...quick-doses` | **No** | **No** | **VERIFIED omission**. Local-only by design in current code |
| `Supply` | `src/utils/regimen.ts:583`: `id`, `route`, `ester`, `pack`, `count`, `startedAt` | `pack` is an object union; `normalizeSupply` supplies defaults for legacy bare `amount` | `...masc-supplies` | **No** | **No** | **VERIFIED omission**. `normalizeSupply` does not validate all pack branches; see below |
| `PlanItem` | `src/utils/plan.ts:9`: `id`, `route`, `ester`, `doseMG`, `extras`, `everyDays`, `timesPerDay` | No defaults in exported data; normalizer fills/validates | `...masc-plan` | **No** | **No** | **VERIFIED omission**. `normalizePlanItem` has no upper bounds |
| `weight` | Number (kg) | Default `70` when absent on load | shared `...weight` | Yes | LWW via `applySyncedState` | **VERIFIED**. Default can hide missing data |
| `weightUpdatedAt` | Epoch ms | `undefined` if absent | shared `...weight-at` | Yes | LWW | **VERIFIED**. Used for sync conflict resolution |
| `pkParams` | `PKCustomParams` (`logic.ts:1410`), or null | Defaults from `DEFAULT_PK_PARAMS`; null means cleared | shared `...pk-params` | Yes (nullable) | LWW | **VERIFIED**. Many numeric PK fields; see below |
| `pkParamsUpdatedAt` | Epoch ms | `undefined` if absent | shared `...pk-params-at` | Yes | LWW | **VERIFIED** |
| `calibrationMethod` | `off`, `ekf`, `ou_kalman`, `mipd` | `normalizeCalibrationMethod` maps legacy `average`→`mipd`, `adaptive`→`ekf`, default `mipd` | shared `...cal-method` | **No** | **No** | **VERIFIED omission**. Local-only in `applySyncedState` |
| `calibrationHistoryMode` | `forward`, `retrospective` | Defaults to `retrospective` in load path | shared `...cal-history-mode` | **No** | **No** | **VERIFIED omission** |
| Preferences | `hrt-lang`, `app-theme`, `app-show-prediction`, `app-auto-backup`, `app-dev-mode`, `app-hide-site-label`, `hrt-supply-lead-days`, `hrt-mode`, `app-pixel-cats`, `app-pixel-cat-style`, `site-notice-dismissed`, `app-onboarded` | Varies | `localStorage` | **No** | **No** | **VERIFIED omission**; product decision required for complete-device backup |
| Credentials | `auth_token`, `auth_user`, `weak_password`, `login_device_passes`, `enc_key` | n/a | `localStorage` | **No** | **No** | **VERIFIED**; must never be included in backup |
| Export metadata | `meta.version=2`, `meta.exportedAt`, `mode` | v1 flat mirrors also emitted | Export payload only | Yes | Import supports v1/v2 | **VERIFIED**. No format/scope identifier beyond `meta.version` |

## `Supply.pack` union (VERIFIED)

`src/utils/regimen.ts:583` defines a discriminated object union:

```ts
type Pack =
  | { kind: 'vial'; ml: number; mgPerMl: number }
  | { kind: 'pill'; mg: number }
  | { kind: 'pump'; mg: number }
  | { kind: 'sachet'; mg: number }
  | { kind: 'patch' }
  | { kind: 'bulk' };
```

`packUnitMG`:

- `vial`: `ml * mgPerMl`
- `pill`, `pump`, `sachet`: `mg`
- `patch`, `bulk`: `1`

`packKindsFor`:

- injection: `vial`, `bulk`
- oral/sublingual: `pill`, `bulk`
- gel: `pump`, `sachet`, `bulk`
- patchApply: `patch`
- default: `bulk`

`normalizeSupply` at `regimen.ts:643` is permissive: it accepts any object with a
string `id`, an object `pack`, and numeric `count` without validating the pack
branch or route/ester compatibility. Legacy bare `amount` maps to patch or bulk.
This is a **format risk**, not evidence that production data is invalid.

## `PlanItem` validation (VERIFIED)

`normalizePlanItem` (`plan.ts:141`) requires:

- object with string `id`;
- valid `Route` and `Ester` enums;
- `everyDays >= 1`;
- `timesPerDay` rounded and `>= 1`;
- finite `doseMG >= 0`;
- `extras` is an object or `{}`.

It forces `timesPerDay = 1` when `everyDays > 1`. It does **not** impose upper
bounds on `everyDays`, `timesPerDay`, `doseMG`, or extras. `doseMG` is `0` for
patch plans; patch release rate is in `extras`.

## `PKCustomParams` (VERIFIED)

`logic.ts:1410` contains numeric defaults for:

- E2 clearance (general/injection), E2 formation fractions for EB/EV/EC/EN/EU;
- oral, sublingual (quick/casual/standard/strict), gel (arm/thigh/scrotal)
  bioavailability;
- T clearance (general/injection), T formation fractions for TC/TE/TU, and
  gel (arm/thigh/scrotal) bioavailability.

The exact constants are in `DEFAULT_PK_PARAMS`; a future backup schema should
carry the object as a versioned numeric map, not duplicate the defaults in the
format spec. Missing fields fall back to defaults; whether that is desirable for
restore is a product decision.

## Export payload (VERIFIED)

`buildExportPayload()` (`useAppData.ts:1185-1218`) returns:

```text
meta: { version: 2, exportedAt }
mode
weight
weightUpdatedAt
modes.transfem/transmasc:
  events, labResults, doseTemplates, deletions
flat v1 mirrors: events, labResults, doseTemplates (active mode)
pkParams (nullable)
pkParamsUpdatedAt
```

It does **not** include quick doses, supplies, plan, calibration settings, or
preferences. `applySyncedState()` (`1230-1290`) writes sanitized records,
tombstones, and LWW `weight`/`pkParams`; it does not write those local-only
groups.

## Unknowns

- Whether the product intends preferences to be device-local only or portable.
- Whether `weight` absence vs `70` should be distinguishable in a backup.
- Whether `pkParams = null` means “use defaults” or “explicitly cleared” in all
  restore paths.
- Exact import/restore behavior for supplies/plan/quick doses does not exist
  today; any proposed behavior is **UNKNOWN** until implemented and tested.
- No production backup/restore tests exist for these fields (see test plan).

## Status

| Area | Status |
|---|---|
| Current record export model | **PASS_WITH_CAVEATS** |
| Complete device backup model | **FAIL** (fields missing; restore semantics absent) |
| `Supply.pack` production object model | **PASS** (documented) |
| Phase 1D synthetic `Supply.pack` string model | **FAIL** (does not match production) |
| Import/restore coverage for local-only fields | **FAIL / NOT_IMPLEMENTED** |
| Product scope decision | **NEEDS_HUMAN_APPROVAL** |
