# L1 Synthetic Restore Harness — Implementation Results

**Status:** L1 model `PASS`; production restore `BLOCKED`; product semantics `NEEDS_HUMAN_APPROVAL`.

This directory contains a **pure synthetic model**, not production restore code. It has no imports from `src/`, no network calls, no production account access, and no real health data.

## Files

- `l1_restore_model.py` — pure Python validation/merge/replace/transaction model.
- `test_l1_restore_model.py` — T1–T15 synthetic tests.

## Command and result

```text
python3 -m unittest -v test_l1_restore_model.py
Ran 15 tests in 0.005s
OK
```

Initial run had 14 passes and one test-fixture error: T8 placed calibration under `modes.transfem`, while the model uses top-level `calibration`. The test fixture was corrected; the model was not changed to accommodate the wrong path.

## Test mapping

| ID | Test | Result | What it proves |
|---|---|---|---|
| T1 | Round-trip both modes with local-only arrays | PASS | Category counts and IDs survive model round-trip |
| T2 | Merge adds IDs and keeps local on conflict | PASS | `keep-local` policy is deterministic |
| T3 | Replace only selected categories | PASS | Unselected categories are preserved |
| T4 | Duplicate IDs inside backup | PASS | Whole category is rejected |
| T5 | Valid/invalid `Supply.pack` object union | PASS | Branch-specific fields are validated |
| T6 | String `Supply.pack` (Phase 1D) | PASS | Experimental string schema is rejected |
| T7 | Plan bounds | PASS | `everyDays`, `timesPerDay`, `doseMG` bounds enforced |
| T8 | Invalid calibration | PASS | Invalid method/history mode rejected |
| T9 | Preferences and credentials | PASS | Approved prefs only; credentials rejected |
| T10 | Unknown version | PASS | Unsupported version rejected |
| T11 | Transaction rollback | PASS | Simulated commit failure restores prior state |
| T12 | Cross-mode/account warnings | PASS | Warnings emitted; no silent crossover |
| T13 | Credentials never persist | PASS | Credential fields do not reach state |
| T14 | Empty vs missing | PASS | Empty arrays preserved; missing reported |
| T15 | Repeated merge idempotence | PASS | No duplicate IDs after repeat |

## What this does not prove

- It does not test production `buildExportPayload`, `processImportedData`, `applySyncedState`, `localStorage`, or UI.
- It does not prove production restore atomicity; `localStorage` has no transaction.
- It does not prove OpenPGP integrity, container parsing, key handling, or security.
- It does not settle conflict policy without `updatedAt`/tombstones.
- It does not prove complete-device backup is ready; Option B remains `NEEDS_HUMAN_APPROVAL`.

## Open decisions carried forward

1. Include quickDoses, supplies, plan, calibration and which preferences in Option B.
2. Default conflict policy for records without `updatedAt`: keep local, prefer backup, skip, or ask user.
3. Whether to add `updatedAt`/tombstones to local-only data (schema change).
4. Replace-only vs merge for Option B.
5. Account identity warning and cross-account restore behavior.
6. Version/migration policy.

## Status summary

| Area | Status |
|---|---|
| L1 pure model design | PASS |
| L1 tests T1–T15 | PASS |
| Production-function tests (L2) | BLOCKED |
| End-to-end restore (L3) | BLOCKED |
| Production restore implementation | NOT_IMPLEMENTED |
| Option B product scope | NEEDS_HUMAN_APPROVAL |
| Conflict policy without timestamps | NEEDS_HUMAN_APPROVAL |
| License Gate / Docker / production integration | BLOCKED |
