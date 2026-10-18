# Backup Restore Semantics Design

**Status:** `NEEDS_HUMAN_APPROVAL`  
**Scope:** Option B complete-device backup restore design; no production implementation.

## 1. Scope and boundaries

This document defines restore semantics for data that the current export does **not** include:

- `quickDoses`
- `supplies`
- `plan` / `planItems`
- calibration state (`calMethod`, `calHistoryMode`)
- selected user preferences

Already-covered record data remains governed by the existing v2 export/import and tombstone behavior. This design does not redefine that behavior.

**Never included in a backup:** `auth_token`, `auth_user`, `weak_password`, `login_device_passes`, `enc_key`, TOTP secrets, passkeys, session data, or any production credential.

**Evidence base:** `PRODUCTION_DATA_MODEL.md`, `EXPORT_RESTORE_GAPS.md`, and the production code referenced there. No production export was run and no real data was used.

## 2. Restore model

### 2.1 Two explicit restore actions

| Action | Meaning | Required user confirmation |
|---|---|---|
| **Replace local data** | Clear selected local-only categories, then write backup values | Typed confirmation; show counts and categories |
| **Merge into local data** | Keep existing records and add/overwrite by explicit policy | Show conflicts and per-category policy |

There must be no silent automatic restore. The user must see:

- backup format version and creation time;
- source mode(s);
- category counts;
- whether local-only data will be replaced or merged;
- any unsupported/unknown fields.

### 2.2 Two-pass, all-or-nothing application

1. Parse and validate the complete backup into memory.
2. Build a staging map of every key/value to write.
3. Validate all counts, types, enum values and references.
4. Snapshot existing target keys.
5. Apply writes; on any failure, restore the snapshot.
6. Re-run post-write validation and report a summary.

If any step fails, no user-visible category should be partially restored. `localStorage` has no transaction; rollback is an application-level requirement and must be tested with synthetic data.

## 3. Category rules

### 3.1 Quick doses

Production type: `{id, route, ester, value, createdAt}`.
Persistence: `hrt-[u<id>-]<mode>-quick-doses`.

| Rule | Status |
|---|---|
| Include in complete-device backup | NEEDS_HUMAN_APPROVAL |
| Replace: clear current array then write backup | Design |
| Merge: add by `id`; duplicate `id` keeps local unless user chooses backup | NEEDS_DECISION |
| Missing/invalid id: reject whole category or skip record | NEEDS_DECISION |
| No `updatedAt`: cannot resolve true conflicts | NEEDS_DECISION |
| Cross-mode records: restore only matching mode key | Design |
| Empty array is meaningful; do not treat as missing | Design |

### 3.2 Supplies

Production type:

```text
{ id, route: Route, ester: Ester, pack: Pack, count: number, startedAt: epoch-ms }
```

`Pack` is a discriminated object union:

```text
{kind:'vial', ml, mgPerMl}
{kind:'pill', mg}
{kind:'pump', mg}
{kind:'sachet', mg}
{kind:'patch'}
{kind:'bulk'}
```

| Rule | Status |
|---|---|
| Include in complete-device backup | NEEDS_HUMAN_APPROVAL |
| Preserve exact pack object branch and fields | REQUIRED |
| Accept legacy bare numeric `amount` only through a documented migration path | NEEDS_DECISION |
| Reject string pack or unknown `kind` | REQUIRED |
| Validate route/ester enums and per-kind positive finite fields | REQUIRED |
| `count >= 0` finite; `startedAt` finite epoch-ms | REQUIRED |
| Merge by `id`; duplicate id keeps local unless user chooses backup | NEEDS_DECISION |
| No `updatedAt`; no authoritative conflict resolution | NEEDS_DECISION |

**Current audit:** `normalizeSupply` accepts a broad object with only string `id`, object `pack`, numeric `count`, and does not validate every pack branch. Prototype string pack is not production schema. `FAIL` / `REVIEW_REQUIRED`.

### 3.3 Plan items

Production type: `{id, route, ester, doseMG, extras, everyDays, timesPerDay}`.

| Rule | Status |
|---|---|
| Include in complete-device backup | NEEDS_HUMAN_APPROVAL |
| `route`/`ester` must be enum values | REQUIRED |
| `everyDays >= 1`; `timesPerDay >= 1` | REQUIRED |
| `doseMG` finite and `>= 0` | REQUIRED |
| `extras` object or `{}`; unknown numeric extras preserved | REQUIRED |
| When `everyDays > 1`, force `timesPerDay = 1` | Existing normalize behavior |
| Merge by `id`; duplicate id keeps local unless user chooses backup | NEEDS_DECISION |
| No `updatedAt`; cannot resolve concurrent edits | NEEDS_DECISION |
| Legacy `cpa-plan` is not part of Option B; only current `plan` | Design |

### 3.4 Calibration state

Values: `calMethod` (`off|ekf|ou_kalman|mipd`) and `calHistoryMode` (`forward|retrospective`).

| Rule | Status |
|---|---|
| Include in complete-device backup | NEEDS_HUMAN_APPROVAL |
| Restore only if user confirms; default to keep current | NEEDS_DECISION |
| Invalid value: reject or fall back to current/default | NEEDS_DECISION |
| Calibration is not a per-record array; replace is simpler than merge | Design |
| Calibration affects PK interpretation; show warning before overwrite | REQUIRED |

### 3.5 Preferences

| Key | Category | Include? | Reason |
|---|---|---|---|
| `hrt-lang` | UI language | NEEDS_HUMAN_APPROVAL | User preference, low risk |
| `app-theme` | UI theme | NEEDS_HUMAN_APPROVAL | User preference |
| `hrt-supply-lead-days` | supply warning | NEEDS_HUMAN_APPROVAL | Affects reminders, not health records |
| `app-show-prediction` | UI behavior | NEEDS_HUMAN_APPROVAL | UI preference |
| `app-hide-site-label` | UI preference | NEEDS_HUMAN_APPROVAL | UI preference |
| `hrt-mode` | active mode | NEEDS_HUMAN_APPROVAL | Mode selection; must not silently switch |
| `app-pixel-cats` | cosmetic | NEEDS_HUMAN_APPROVAL | Low value, optional |
| `app-pixel-cat-style` | cosmetic | NEEDS_HUMAN_APPROVAL | Low value, optional |
| `app-auto-backup` | security/behavior | **EXCLUDE** | Could re-enable cloud upload unexpectedly |
| `app-dev-mode` | developer behavior | **EXCLUDE** | Security/debug surface |
| `site-notice-dismissed` | device-specific | **EXCLUDE** | Not user data |
| `app-onboarded` | device-specific | **EXCLUDE** | Not user data |
| `auth_*`, `weak_password`, `login_device_passes`, `enc_key` | credentials | **EXCLUDE** | Never backup |

**Recommendation:** include only language, theme, supply lead days, show-prediction and hide-site-label; keep cosmetic and behavioral/security preferences out of Option B until explicitly approved.

## 4. Conflict policy table

| Situation | Default proposal | Status |
|---|---|---|
| Same record `id` exists locally and in backup | Keep local; show conflict | NEEDS_DECISION |
| No `updatedAt` on local-only record | Cannot order; require user choice | NEEDS_DECISION |
| Backup contains record absent locally | Add | Design |
| Local record absent from backup | Keep; backup is not a deletion source | Design |
| Duplicate IDs inside backup | Reject whole category | Design |
| Invalid record in backup | Reject whole category or skip with report | NEEDS_DECISION |
| Backup mode differs from current mode | Restore matching mode key; do not cross-copy | Design |
| Existing record deleted locally | No tombstone for local-only arrays; cannot distinguish never-existed vs deleted | NEEDS_DECISION |
| Preferences conflict | Keep current; apply only selected keys | NEEDS_DECISION |
| Calibration conflict | Keep current unless user confirms replace | NEEDS_DECISION |
| Older format version | Read-only preview; no auto-migrate | Design |
| Unknown critical field | Reject restore | Design |
| Unknown non-critical field | Preserve or ignore with report | NEEDS_DECISION |

**Important:** local-only arrays have no tombstones and no `updatedAt`. Merge conflict semantics cannot be made authoritative without a schema change. This is a `NEEDS_HUMAN_APPROVAL` product decision, not something to guess.

## 5. Account and mode boundaries

- Backup currently has `mode` but no account identifier. Restoring into a different account must show a warning; do not infer identity from filename.
- Local-only data is stored under `hrt-[u<id>-]<mode>-...`. A restore must target the **current account namespace** and matching mode; never write another account's namespace.
- Option A remains a record backup only. Option B is a separate format/scope; do not label Option A “complete backup.”
- Restoring a complete-device backup must not overwrite server-side cloud backups, shares, sessions or credentials.

## 6. Credentials and security

Explicitly excluded and must never be accepted from a backup file:

- `auth_token`, `auth_user`
- `weak_password`
- `login_device_passes`
- `enc_key`
- TOTP secret / backup codes
- passkeys
- session IDs
- private OpenPGP keys or passphrases

If a backup contains such fields, reject the restore or ignore them with a visible warning; do not write them to `localStorage`.

## 7. Synthetic test specification

All tests use fictional IDs, dates and values.

| ID | Test | Expected |
|---|---|---|
| T1 | Round-trip both modes with local-only arrays | Exact category counts and IDs |
| T2 | Merge adds new IDs, keeps local on conflict | No duplicate IDs |
| T3 | Replace clears only selected categories | Credentials/preferences excluded |
| T4 | Duplicate ID inside backup | Reject category, no writes |
| T5 | Invalid Supply pack kind/fields | Reject or explicit skip; no partial write |
| T6 | String Supply.pack (Phase1D) | Rejected; no production schema fallback |
| T7 | Plan `everyDays`/`timesPerDay` bounds | Invalid rejected; valid normalized |
| T8 | Calibration invalid/legacy value | No silent overwrite; status reported |
| T9 | Preferences acceptance/exclusion matrix | Only approved keys applied |
| T10 | Unknown format version / critical field | Read-only preview; no writes |
| T11 | Mid-write failure rollback | Previous keys restored |
| T12 | Cross-account / cross-mode restore warning | No namespace crossover |
| T13 | Credentials present in backup | Rejected/ignored, never persisted |
| T14 | Empty arrays vs missing fields | Empty preserved; missing reported |
| T15 | Repeated restore idempotence | No duplicates, stable counts |

## 8. Acceptance criteria

- No production implementation in this work item.
- All local-only categories have explicit include/exclude decisions.
- `Supply.pack` is defined by its object union, never a string.
- Conflict policy is explicit; unresolved cases are marked `NEEDS_DECISION`.
- Credentials are excluded by construction and tested negatively.
- Synthetic tests cover replace, merge, duplicates, invalid data, rollback, versions and cross-mode.
- Restore is all-or-nothing at the application level.
- No claim of production readiness, security audit, or License Gate clearance.

## 9. Open decisions

1. Option B: include quickDoses, supplies, plan, calibration? `NEEDS_HUMAN_APPROVAL`
2. Default conflict policy for local-only records with no `updatedAt`: keep local or prefer backup?
3. Should local-only arrays gain tombstones/`updatedAt`? If yes, schema change and audit required.
4. Which preferences are in scope?
5. Should restore be replace-only for Option B, or support merge?
6. Should calibration be restored only with an explicit warning?
7. Account identity warning and cross-account restore behavior.
8. Backup format version and migration policy.
