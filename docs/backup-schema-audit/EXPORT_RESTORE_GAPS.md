# Export / Import / Restore Gap Analysis

**Baseline:** `Privacy/Add-GPG` @ `5c5d9fe39945c3f03c740bbaf76f313be20f7dbe`  
**Audit branch:** `agent/backup-schema-audit`  
**Status:** read-only audit

## 1. Current export path

`buildExportPayload()` (`src/hooks/useAppData.ts:1185-1218`) returns:

- `meta.version = 2`, `meta.exportedAt`
- `mode`
- `weight`, `weightUpdatedAt`
- `modes.transfem` and `modes.transmasc`, each with:
  - `events`
  - `labResults`
  - `doseTemplates`
  - `deletions`
- flat v1 mirrors `events`, `labResults`, `doseTemplates` for the active mode
- `pkParams` (nullable) and `pkParamsUpdatedAt`

**Not exported:** quick doses, supplies, plan items, calibration method/history
mode, preferences, auth token, device passes, cloud key.

## 2. Current import path

`processImportedData()` (`useAppData.ts:794`) and `mergeImportedData()`
(`1001`) support:

- v2 payloads with `modes`; v1 flat payloads;
- moving cross-mode records to the other mode;
- merge by record id;
- replacement import records missing records as tombstones;
- confirmation if either mode has data.

Sanitizers (`706-780`):

| Record   | Validation / default                                                                                                                                                                                        |
| -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Event    | Valid `Route`; finite `timeH` in range; dose clamped to `0..DOSE_MG_MAX`; ester regex `^[A-Za-z0-9_]{1,24}$`, unknown codes preserved; extras object; id generated if absent; `updatedAt` retained if valid |
| Lab      | Finite time/value; unit allowlist (default `pmol/l`); assay allowlist                                                                                                                                       |
| Template | Valid route; drug-code ester; finite non-negative dose; `createdAt` defaults to now                                                                                                                         |

`applySyncedState()` (`1230-1290`) sanitizes both modes before writing. It writes
records, tombstones, and LWW `weight`/`pkParams`; it does **not** write quick
doses, supplies, plan, calibration, or preferences.

## 3. Gap matrix

| Data group     | Export | Import/restore | Persistence                             | Gap / risk                                                                                  | Status                              |
| -------------- | ------ | -------------- | --------------------------------------- | ------------------------------------------------------------------------------------------- | ----------------------------------- |
| Events         | Yes    | Yes            | mode `events`                           | Dose clamp, route/ester/time validation; unknown ester preserved                            | **PASS_WITH_CAVEATS**               |
| Lab results    | Yes    | Yes            | mode `lab-results`                      | Invalid unit defaults to `pmol/l`; assay allowlist                                          | **PASS_WITH_CAVEATS**               |
| Dose templates | Yes    | Yes            | mode `dose-templates`                   | `extras: any`; `createdAt` defaults to now                                                  | **PASS_WITH_CAVEATS**               |
| Tombstones     | Yes    | Yes            | mode `deletions`                        | Pruning and LWW; replacement import records dropped records                                 | **PASS_WITH_CAVEATS**               |
| Weight         | Yes    | Yes            | shared `weight`                         | Missing weight loads as `70`; absence vs default not encoded                                | **REVIEW_REQUIRED**                 |
| PK params      | Yes    | Yes            | shared `pk-params`                      | Null semantics; defaults; many numeric fields                                               | **REVIEW_REQUIRED**                 |
| Quick doses    | **No** | **No**         | mode `quick-doses`                      | Local-only; no export/import path                                                           | **FAIL** for complete device backup |
| Supplies       | **No** | **No**         | mode `supplies`                         | Local-only; `pack` object union not represented in current export; normalizer is permissive | **FAIL**                            |
| Plan           | **No** | **No**         | mode `plan`                             | Local-only; no restore path; no upper bounds in normalizer                                  | **FAIL**                            |
| Calibration    | **No** | **No**         | shared `cal-method`, `cal-history-mode` | Local-only; default `mipd`/`retrospective`                                                  | **FAIL**                            |
| Preferences    | **No** | **No**         | multiple keys                           | Portable vs device-local undecided                                                          | **NEEDS_HUMAN_APPROVAL**            |
| Credentials    | **No** | **No**         | auth/device/enc keys                    | Must never be backed up                                                                     | **PASS** (excluded)                 |

## 4. Version and compatibility risks

- Export has `meta.version = 2` but no explicit backup scope (`records` vs
  `device`). A future complete-device backup must not be mistaken for a current
  record backup.
- v1 flat mirrors duplicate active-mode records. Import must not double-count
  them when `modes` is present.
- Unknown fields are generally accepted by `extras`; strict schema validation is
  not part of the current export format.
- Missing fields may silently become defaults (`weight=70`, calibration
  `mipd/retrospective`, lab unit `pmol/l`). This can change clinical meaning.
- No migration/version map exists for local-only data because it has never been
  exported.
- `Supply.pack` is a discriminated object union. A string `pack` (Phase 1D
  prototype) is not production-compatible.
- The current export is not a complete device backup; UI wording must say
  “records backup” unless Option B is implemented.

## 5. Cloud sync vs export

Cloud sync uses the same record/tombstone/LWW scalar model, not supplies, plan,
quick doses, or calibration. `applySyncedState` is the write-back boundary.
Therefore adding fields to cold backup does not automatically add them to cloud
sync, and vice versa. A future complete-device backup needs its own restore
semantics; reusing cloud sync would silently drop local-only data.

## 6. Test coverage gap

No direct tests were found for:

- `sanitizeImportedEvents`, `sanitizeImportedLabResults`,
  `sanitizeImportedTemplates`;
- `processImportedData`, `mergeImportedData`;
- `buildExportPayload`;
- `applySyncedState`;
- `normalizeSupply`, `normalizePlanItem`;
- `saveSupply`, quick-dose persistence.

Existing tests cover crypto compatibility, shares, calibration/PK, routes, and
worker hardening, not backup completeness.

## 7. Status

| Item                             | Status                                     |
| -------------------------------- | ------------------------------------------ |
| Current record backup            | **PASS_WITH_CAVEATS**                      |
| Complete device backup           | **FAIL**                                   |
| Local-only restore semantics     | **NOT_IMPLEMENTED / NEEDS_HUMAN_APPROVAL** |
| Version/scope discrimination     | **REVIEW_REQUIRED**                        |
| Credentials exclusion            | **PASS**                                   |
| Test coverage for export/restore | **FAIL**                                   |
