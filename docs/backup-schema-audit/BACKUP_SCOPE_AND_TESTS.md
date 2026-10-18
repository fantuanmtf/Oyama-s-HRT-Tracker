# Backup Scope and Test Plan

**Baseline:** `Privacy/Add-GPG` @ `5c5d9fe39945c3f03c740bbaf76f313be20f7dbe`  
**Status:** proposed audit scope; product decision **NEEDS_HUMAN_APPROVAL**

## 1. Two explicit scopes

### Option A — Current record backup

Include:

- per-mode events;
- per-mode lab results;
- per-mode dose templates;
- per-mode deletions/tombstones;
- weight and `weightUpdatedAt`;
- PK params and `pkParamsUpdatedAt`;
- `meta.version`, `meta.exportedAt`, `mode`;
- v1 flat mirrors as required for compatibility.

Explicitly exclude:

- quick doses;
- supplies;
- plan/plan items;
- calibration method/history mode;
- preferences;
- auth token, device passes, cloud key, passwords.

**Status:** **PASS_WITH_CAVEATS** — matches current `buildExportPayload` except
for UI wording and documented defaults.

### Option B — Complete device backup

Include everything in Option A plus:

- quick doses;
- supplies with the production `Pack` object union;
- plan/plan items;
- calibration method and history mode;
- selected portable preferences (decision required);
- a scope/version field that distinguishes this from Option A.

Restore requirements:

- preserve `Supply.pack` branch and numeric fields exactly;
- preserve Plan `everyDays`/`timesPerDay` and patch `extras`;
- restore calibration settings before computing derived values;
- decide whether preferences are portable or device-local;
- do not restore credentials;
- preserve tombstones and LWW timestamps;
- validate every field before writing;
- never claim complete restore until tested with synthetic data.

**Status:** **FAIL / NEEDS_HUMAN_APPROVAL** — no export/import/restore path for
local-only data exists today.

## 2. Recommendation

Ship **Option A** as a “records backup” with explicit exclusions and a clear UI
label. Do not call it a full device backup.

Treat **Option B** as a separate product capability with its own format scope and
restore semantics. Do not silently add local-only fields to the existing v2
payload; that would change the meaning of `meta.version = 2`.

## 3. Risk register

| Risk | Impact | Mitigation | Status |
|---|---|---|---|
| Missing quick doses/supplies/plan/calibration | Restore appears successful but loses data | Explicit scope label; Option B separate | **OPEN** |
| `weight=70` default masks missing weight | Clinical/PK display changes | Encode presence explicitly; test absent vs 70 | **REVIEW_REQUIRED** |
| Calibration defaults `mipd/retrospective` | Restored calculations differ | Persist calibration in Option B; test | **REVIEW_REQUIRED** |
| `Supply.pack` string/object mismatch | Restore rejects or corrupts supplies | Use production union in schema; branch tests | **FAIL** |
| Unknown ester codes | Rejected or changed | Preserve strings; document allowlist | **REVIEW_REQUIRED** |
| Tombstone pruning | Deleted record reappears after restore | Preserve tombstones; test merge/replace | **REVIEW_REQUIRED** |
| Duplicate active-mode mirrors | Duplicate records on import | Test v2 with v1 mirrors | **REVIEW_REQUIRED** |
| Preferences portability | Surprising UI/behavior | Human decision | **NEEDS_HUMAN_APPROVAL** |
| Credentials accidentally exported | Account compromise | Never include auth/device/enc keys; test | **PASS** (current) |

## 4. Synthetic test plan

All tests must use fictional IDs, dates, doses, labs, supplies, and plans. Do not
use real health data.

| ID | Test | Expected |
|---|---|---|
| T1 | Current export shape | v2 payload contains only documented fields; no credentials |
| T2 | v2 import replace | Missing records become tombstones; no duplicates from v1 mirrors |
| T3 | v2 import merge | Records merge by id; LWW weight/PK preserved |
| T4 | Cross-mode records | Moved to correct mode; no loss |
| T5 | Option B round-trip | Quick doses, supplies, plan, calibration all restored exactly |
| T6 | Supply pack branches | vial/pill/pump/sachet/patch/bulk round-trip; invalid branch rejected |
| T7 | Plan edge cases | `everyDays=1`, `3.5`, `timesPerDay`, patch `doseMG=0`, extras |
| T8 | Calibration defaults | Missing vs explicit `off`/`ekf`/`ou_kalman`/`mipd` and history modes |
| T9 | Weight presence | Missing weight vs explicit `70` distinguished |
| T10 | Unknown/legacy fields | Unknown critical version rejected; optional unknown fields preserved or warned |
| T11 | Credentials exclusion | Auth/device/enc keys absent from export and restore |
| T12 | Preferences policy | Portable vs device-local decision enforced; no silent import |
| T13 | Failure safety | Invalid record writes nothing; existing data unchanged |

## 5. Existing coverage

- No production tests directly cover export/import completeness, sanitizers,
  supplies, plan, quick doses, or calibration persistence.
- Existing tests cover crypto compatibility, shares, calibration/PK math, routes,
  and worker hardening.
- The Phase 1D/1F isolated CLI tests are separate from production tests.

## 6. Status

| Item | Status |
|---|---|
| Option A defined | **PASS_WITH_CAVEATS** |
| Option B defined | **PASS** (design only) |
| Option B implemented/restored | **FAIL / NOT_IMPLEMENTED** |
| Test plan | **PASS** (plan only) |
| Production test coverage | **FAIL** |
| Product scope | **NEEDS_HUMAN_APPROVAL** |
