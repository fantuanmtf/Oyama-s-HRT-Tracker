# Synthetic Restore Test Harness Design

**Status:** `NEEDS_HUMAN_APPROVAL` for product semantics; harness specification `PASS` (design only); execution `NOT_TESTED`.  
**Scope:** Option B complete-device restore; pure synthetic data only. No production implementation.

## 1. Why a layered harness

A test model is not the production restore path. Three layers must stay distinct:

| Layer                              | What it proves                                               | Can run now?                                | Evidence                                    |
| ---------------------------------- | ------------------------------------------------------------ | ------------------------------------------- | ------------------------------------------- |
| **L1 — Pure restore model**        | The specified merge/replace/conflict rules behave as written | Yes, independent of app code                | Model tests + fixture diffs                 |
| **L2 — Production-function tests** | The real sanitizers/export/apply functions match the model   | No; restore functions do not exist          | Requires exported functions or a test seam  |
| **L3 — End-to-end restore**        | Browser/localStorage behavior, rollback, UI confirmation     | No; production restore is `NOT_IMPLEMENTED` | Requires approved production implementation |

A passing L1 run must never be reported as “restore works.” L2 and L3 remain `BLOCKED` until production code exists.

## 2. Fixture design

All fixtures are fictional. No tokens, keys, passwords, real dates or health records.

```text
docs/backup-schema-audit/harness/fixtures/
  local-before.json          # current device state
  backup-device-v1.json      # Option B synthetic backup
  expected-replace.json      # exact expected state after replace
  expected-merge-keep-local.json
  expected-merge-prefer-backup.json
  invalid-supply-string-pack.json
  invalid-supply-branch.json
  invalid-plan-bounds.json
  invalid-calibration.json
  invalid-credentials.json
  unknown-critical-field.json
  unknown-minor-field.json
  duplicate-id.json
  empty-vs-missing.json
```

Fixture envelope:

```json
{
  "fixtureVersion": 1,
  "description": "synthetic only",
  "account": "synthetic-user-a",
  "mode": "transfem",
  "data": {
    "quickDoses": [],
    "supplies": [],
    "plan": [],
    "calibration": {},
    "preferences": {}
  }
}
```

Provide both modes (`transfem`, `transmasc`) and at least one cross-mode fixture. IDs are fixed strings such as `evt-001`, `sup-001`, `plan-001`; timestamps are fictional (`2030-01-01T00:00:00Z` or epoch-ms constants).

## 3. Category fixtures

### Quick doses

```json
{ "id": "qd-001", "route": "injection", "ester": "EV", "value": 2.0, "createdAt": 1893456000000 }
```

Cover: valid, empty array, duplicate id, same id different value, invalid route/ester, non-finite value.

### Supplies

Use the production object union, never a string:

```json
{
  "id": "sup-001",
  "route": "injection",
  "ester": "EV",
  "pack": { "kind": "vial", "ml": 5, "mgPerMl": 40 },
  "count": 2,
  "startedAt": 1893456000000
}
```

Cover every branch:

```text
vial    {ml, mgPerMl}
pill    {mg}
pump    {mg}
sachet  {mg}
patch   {}
bulk    {}
```

Negative fixtures: string `pack`, unknown `kind`, missing branch field, zero/negative/NaN field, negative count, invalid route/ester.

### Plan items

```json
{
  "id": "plan-001",
  "route": "injection",
  "ester": "EV",
  "doseMG": 2,
  "extras": { "releaseRate": 0 },
  "everyDays": 3.5,
  "timesPerDay": 1
}
```

Cover: daily (`everyDays=1`, `timesPerDay=2`), non-daily (forces `timesPerDay=1`), zero/negative dose, `everyDays<1`, invalid route/ester, extras object/null, unknown extras.

### Calibration

```json
{ "method": "ekf", "historyMode": "forward" }
```

Cover valid values, legacy aliases (`average`, `adaptive`), invalid method/history mode, missing object.

### Preferences

Fixture keys are allowlisted:

```json
{
  "hrt-lang": "zh",
  "app-theme": "dark",
  "hrt-supply-lead-days": 14,
  "app-show-prediction": true,
  "app-hide-site-label": false
}
```

Excluded keys must be rejected/ignored and tested:

```text
app-auto-backup, app-dev-mode, site-notice-dismissed, app-onboarded,
auth_token, auth_user, weak_password, login_device_passes, enc_key
```

## 4. Test matrix T1–T15

| ID  | Purpose                                 | Setup                                 | Operation     | Expected                                     | Layer                  |
| --- | --------------------------------------- | ------------------------------------- | ------------- | -------------------------------------------- | ---------------------- |
| T1  | Both-mode round trip                    | records + local-only in both modes    | replace       | exact counts/IDs per mode                    | L1                     |
| T2  | Merge adds new IDs                      | local has A/B; backup has B/C         | merge         | A/B/C; B policy applied                      | L1                     |
| T3  | Replace clears only selected categories | local has extra quick/supply/plan     | replace       | selected cleared; credentials untouched      | L1                     |
| T4  | Duplicate ID inside backup              | two `sup-001` entries                 | validate      | reject category, zero writes                 | L1                     |
| T5  | Invalid pack branch                     | string/missing/negative               | validate      | reject; no partial state                     | L1                     |
| T6  | Phase1D string pack                     | `"pack":"vial"`                       | validate      | reject with schema error                     | L1                     |
| T7  | Plan bounds                             | `everyDays=0`, `timesPerDay=0`        | validate      | reject; valid values normalized              | L1                     |
| T8  | Calibration conflict                    | local `ekf`, backup `mipd`            | merge/replace | no silent overwrite; decision recorded       | L1                     |
| T9  | Preference matrix                       | include + excluded keys               | merge         | only allowlisted applied; credentials absent | L1                     |
| T10 | Unknown version/field                   | `formatVersion:999`, unknown critical | validate      | read-only preview; no writes                 | L1                     |
| T11 | Mid-write rollback                      | valid backup; fault at key 3          | apply         | prior snapshot restored; no partial          | L1 simulated / L3 real |
| T12 | Cross-account/mode                      | different account/mode                | restore       | warning; no namespace crossover              | L1                     |
| T13 | Credentials in backup                   | auth/enc_key fields                   | validate      | reject/ignore; never persisted               | L1                     |
| T14 | Empty vs missing                        | `[]` vs absent                        | merge         | empty preserved; missing reported            | L1                     |
| T15 | Idempotence                             | run same restore twice                | merge         | no duplicates; stable counts                 | L1                     |

### Conflict-strategy variants

For T2/T4/T8/T9, run each supported policy:

1. **Keep local** — existing record wins; backup record reported as skipped.
2. **Prefer backup** — backup wins; local value reported as replaced.
3. **Skip conflicting ID** — add only new IDs.
4. **Require user choice** — harness records an unresolved conflict; no write until decision.

No policy may be assumed final. The product choice is `NEEDS_HUMAN_APPROVAL`.

## 5. Execution model

### L1 — pure model

Implement a small, language-neutral fixture runner (TypeScript or Python) that:

1. loads `local-before.json` and a backup fixture;
2. validates the backup against the schema rules;
3. applies the selected policy in memory;
4. writes a canonical JSON result;
5. compares it byte-for-byte with the expected fixture.

The runner must not import production code or browser APIs. It is a specification oracle, not a restore implementation.

### L2 — production-function tests

Blocked until production exports or test seams exist for:

- local-only load/normalize functions;
- restore merge/replace;
- key-level write/rollback;
- preference allowlist.

Do not duplicate production logic in L1 and call it an integration test.

### L3 — end-to-end

Requires approved production restore UI/API, a synthetic browser profile, and fault injection. Must verify:

- no credential write;
- no partial category on failure;
- no cross-account/cross-mode write;
- exact localStorage keys after restore;
- reload and re-export read-back equality.

## 6. Atomicity boundaries

`localStorage` has no transaction primitive. The harness must test the application-level strategy:

1. snapshot every target key;
2. validate all values;
3. write staging keys with a unique prefix;
4. commit by replacing target keys;
5. on any failure, restore snapshot and remove staging keys.

Failure injection points: JSON parse, schema validation, quota error, exception during write, reload mismatch. T11 must verify the snapshot restore, not just the absence of an exception.

If the product cannot guarantee this without an architecture change, record `BLOCKED`; do not claim atomic restore.

## 7. Acceptance criteria

- [ ] All fixtures are synthetic and contain no secrets.
- [ ] T1–T15 have expected-state files and explicit pass/fail rules.
- [ ] Supply fixtures use the production object union, never strings.
- [ ] Excluded preference/credential keys are tested negatively.
- [ ] Conflict variants are explicit; unresolved policy is `NEEDS_DECISION`.
- [ ] L1 runner does not import production code.
- [ ] L2/L3 are marked `BLOCKED` until production restore exists.
- [ ] Rollback test proves snapshot restoration, not just error return.
- [ ] No claim of production restore, security audit, or License Gate clearance.

## 8. Open decisions

1. Option B category inclusion (quickDoses/supplies/plan/calibration).
2. Default conflict policy for local-only records without `updatedAt`.
3. Whether local-only records gain `updatedAt`/tombstones.
4. Preference allowlist.
5. Replace-only vs merge for Option B.
6. Cross-account restore policy.
7. Backup format version/migration policy.

**Current status:** design `PASS`; L1 implementation `NOT_TESTED`; L2/L3 `BLOCKED`; product semantics `NEEDS_HUMAN_APPROVAL`.
