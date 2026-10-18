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

## Next work item

1. Decide whether to ship Option A only or commit to Option B.
2. If Option B is approved, define restore semantics for local-only data and
   add synthetic tests T1–T13 before implementation.
3. Align the experimental HRTB schema with the production `Supply.pack` object
   union before any format stabilization.

## Push status

Pending. Push only this branch to `origin`; never to `upstream`. Do not merge,
release, deploy, or change protection rules.
