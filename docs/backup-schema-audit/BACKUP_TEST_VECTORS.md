# Website ↔ Tools Acceptance Vectors

**Status:** documentation only. No schema, validator, `formatVersion`, or Tools
code was changed in this work item.

**Compared revisions**

| Side    | Revision                                 | Evidence                                                   |
| ------- | ---------------------------------------- | ---------------------------------------------------------- |
| Website | `agent/add-gpg-website` @ `82381f9`      | `src/utils/backupBundle.ts`, `src/utils/backupWarnings.ts` |
| Tools   | `Recovery/Tools` @ `25674fd` (read-only) | `tools/hrt-cold/bundle.go`, `tools/hrt-cold/README.md`     |
| Format  | —                                        | `docs/backup-schema-audit/TOOLS_INTERFACE.md`              |

The Tools `hrt-cold` validator checks the hand-off envelope and category shapes;
it does **not** validate clinical record semantics or apply restore rules. The
website importer remains authoritative for record validation, merge behavior,
and conflicts. The vectors below were derived by reading both implementations;
they are not a cross-language conformance run and no Tools tests were executed
for this document.

**Authority legend**

- **WEBSITE AUTHORITATIVE** — the website's importer/merge behavior decides what
  happens after a bundle is accepted.
- **TOOLS AUTHORITATIVE** — the Tools envelope rule defines the hand-off
  boundary; the website's more permissive parser does not make the input valid.
- **UNRESOLVED** — the two sides disagree or the product decision is missing;
  do not claim parity until it is decided in a separate approved work item.

## Known boundary differences

| ID    | Boundary                         | Synthetic input / vector                                                                            | Website behavior                                                                                                          | Tools `hrt-cold` behavior                                                                                                    | Authority / status        | Interoperability consequence                                                                                                             |
| ----- | -------------------------------- | --------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- | ------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| BTV-1 | Calibration legacy aliases       | `"calibration":{"method":"average","historyMode":"forward"}` or `method:"adaptive"`                 | Accepts and normalizes: `average` → `mipd`, `adaptive` → `ekf`.                                                           | Rejects anything outside `off`, `ekf`, `ou_kalman`, `mipd`.                                                                  | **WEBSITE AUTHORITATIVE** | A website-accepted legacy bundle cannot be encrypted by Tools unless the alias is rewritten first. Website exports never emit aliases.   |
| BTV-2 | `createdAt` format               | `"createdAt":"2030-01-01"` (date only)                                                              | Accepts any non-empty string `Date.parse` can read, including date-only.                                                  | Requires full RFC 3339; date-only is rejected.                                                                               | **TOOLS AUTHORITATIVE**   | Hand-edited date-only bundles pass website import but fail Tools encryption. The website writer emits RFC 3339 with milliseconds.        |
| BTV-3 | `formatVersion` numeric spelling | `"formatVersion":1.0`                                                                               | `1.0 !== 1` is false in JavaScript, so it is accepted.                                                                    | Decodes the raw number and compares its textual form with `"1"`; `"1.0"` is rejected.                                        | **UNRESOLVED**            | JSON numbers `1` and `1.0` are numerically equal. Decide whether the interface requires the exact integer token or only numeric value 1. |
| BTV-4 | Duplicate JSON object keys       | `{"format":"oyama-hrt-complete-backup","format":"other",...}` in the same object                    | `JSON.parse` keeps the last key and does not report the duplicate.                                                        | Rejects duplicate keys before typed decoding.                                                                                | **TOOLS AUTHORITATIVE**   | A malicious or hand-edited bundle can mean different things to the two sides. The website parser is not hardened against duplicates.     |
| BTV-5 | `weight: null`                   | `"weight":null`                                                                                     | Rejects/blocks the `weight` scalar: null is not a plausible finite body weight. Missing means "no weight claim".          | Accepts `null` for `weight` (`allowNull = true`) as an envelope value.                                                       | **WEBSITE AUTHORITATIVE** | Tools accepting the envelope does not mean the bundle can be merged; the website will not restore a null weight.                         |
| BTV-6 | `weightUpdatedAt: 0`             | `"weightUpdatedAt":0`                                                                               | Rejects/blocks the `weight` scalar: stamps must be positive finite numbers.                                               | Accepts `0` (minimum is 0).                                                                                                  | **WEBSITE AUTHORITATIVE** | `0` is not a valid website timestamp; omit the field when the bundle says nothing about weight.                                          |
| BTV-7 | Unknown per-record fields        | Event object containing `"unknownField":"synthetic"` outside the known record schema                | Validators rebuild known fields only; unknown per-record fields are silently dropped on import.                           | Does not inspect record semantics; treats records as opaque objects and would preserve the bytes when encrypting/decrypting. | **UNRESOLVED**            | Decide whether unknown record fields must round-trip. Today a file that passes through website import loses them without a report.       |
| BTV-8 | Tombstone TTL / skew / cap       | Deletion timestamp `now + 10 min`; deletion older than 180 days; more than 5000 tombstones per kind | Bounds entries before use: 5-minute future skew rejected, 180-day TTL dropped, 5000-per-kind cap applied, drops reported. | Only checks that `deletions` is an object; no TTL, clock-skew, cap, or record-level semantics.                               | **WEBSITE AUTHORITATIVE** | Tools accepting an old or oversized tombstone set does not mean every entry will be applied. The website merge is the safety boundary.   |

## Related invariant (no new field)

The bundle has **no source-account identifier** and this work item does not add
one. The website import preview names the current target account and warns that
the format cannot detect a cross-account bundle; the user must confirm the merge
explicitly. Tools remains opaque to account identity. Whether a future schema
should carry an account identifier is **UNRESOLVED** and out of scope here.

## Acceptance status

- These vectors document current code, not approved parity.
- BTV-1, BTV-2, BTV-4, BTV-5, BTV-6, and BTV-8 have a designated authoritative
  side above.
- BTV-3 and BTV-7 remain **UNRESOLVED** product/interface decisions.
- No claim is made about production readiness, L3 end-to-end restore, OpenPGP
  key handling, License Gate approval, or Docker.
