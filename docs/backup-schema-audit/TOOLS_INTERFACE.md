# Website ↔ Tools Complete-Backup Interface

**Status:** website export/import data layer implemented; Replace is `BLOCKED`.
**Scope:** hand-off format between the website and the separate offline Tools
project. The website does **not** encrypt or decrypt this file.

## Boundary

- The website produces and consumes an **unencrypted UTF-8 JSON file**.
- OpenPGP encryption/decryption, private keys, passphrases, and any claim of
  cold-backup encryption belong to the offline **Tools** workflow, not the
  website.
- The website does not add OpenPGP.js/LGPL or any runtime dependency for this
  format, and never stores or asks for a private key or passphrase.

## Tools encryption requirements

- A Tools decryptor must require a genuinely encrypted OpenPGP message, not
  merely a parseable OpenPGP payload. Literal, compressed-plaintext, and
  signed-only payloads must be rejected without producing a bundle.
- OpenPGP public-key encrypted session-key (PKESK) recipient key IDs and
  fingerprints are unauthenticated metadata. Tools may display them as
  advertised values, but must not present them as recipient verification.
  Only successful decryption shows that a private key participated.
- A failed decryption, wrong key, failed integrity check, or unencrypted
  payload must not write a plaintext bundle output.

## Envelope

```json
{
  "format": "oyama-hrt-complete-backup",
  "formatVersion": 1,
  "createdAt": "2030-01-01T00:00:00.000Z",
  "mode": "transfem",
  "modes": {
    "transfem": {
      "events": [],
      "labResults": [],
      "doseTemplates": [],
      "deletions": { "events": {}, "labResults": {}, "doseTemplates": {} },
      "quickDoses": [],
      "supplies": [],
      "plan": []
    },
    "transmasc": {
      "events": [],
      "labResults": [],
      "doseTemplates": [],
      "deletions": { "events": {}, "labResults": {}, "doseTemplates": {} },
      "quickDoses": [],
      "supplies": [],
      "plan": []
    }
  },
  "weight": 70,
  "weightUpdatedAt": 1893456000000,
  "pkParams": null,
  "pkParamsUpdatedAt": 1893456000000,
  "calibration": { "method": "mipd", "historyMode": "retrospective" },
  "preferences": {
    "app-theme": "dark",
    "hrt-lang": "en",
    "app-show-prediction": true,
    "app-hide-site-label": false,
    "hrt-supply-lead-days": 14
  }
}
```

`weight`, `pkParams`, and their timestamps are omitted when the bundle says
nothing about them. `pkParams: null` means "explicitly cleared", not "missing".
Missing category arrays mean "not mentioned"; `[]` means "explicitly empty".

## Supply.pack

The only accepted form is the production discriminated object union:

```text
{ "kind": "vial", "ml": 5, "mgPerMl": 40 }
{ "kind": "pill", "mg": 2 }
{ "kind": "pump", "mg": 1.25 }
{ "kind": "sachet", "mg": 1.5 }
{ "kind": "patch" }
{ "kind": "bulk" }
```

A string `pack`, an unknown `kind`, or a missing branch field rejects the whole
supplies category for that mode. It is never silently converted.

## Preference allowlist

Only these keys are exported:

- `app-theme`
- `hrt-lang`
- `app-show-prediction`
- `app-hide-site-label`
- `hrt-supply-lead-days`

Excluded by construction: `app-auto-backup`, `app-dev-mode`,
`app-onboarded`, `site-notice-dismissed`, `hrt-mode`, cosmetic cat settings,
and every credential (`auth_token`, `auth_user`, `weak_password`,
`login_device_passes`, `enc_key`, TOTP/passkey/session data).

Unknown preference keys in an imported bundle are reported and ignored.

## Import semantics

1. Parse and validate the envelope and every category in memory.
2. Show creation time, source mode, per-mode counts, conflicts, invalid records,
   blocked categories, and ignored preferences.
3. On confirmation, merge only categories that passed validation.
   - New id: add.
   - Same id with reliable `updatedAt` on both sides: newer wins; older is
     skipped.
   - Same id without a reliable `updatedAt`, equal timestamps with different
     contents, or any same id in `quickDoses`/`supplies`/`plan`: **abort that
     category** and report it.
   - Duplicate id inside the bundle or the local category: abort that category.
4. Tombstones are unioned; a restored record clears its own tombstone.
5. Preferences are returned to the website's React state; the data layer does
   not write credential keys.

**Replace all** is not implemented and remains `BLOCKED`: `localStorage` has no
proven multi-key transaction, so no all-or-nothing claim is made.

## Tools workflow

A Tools implementation can treat the file as opaque JSON:

1. Read the website file.
2. Encrypt it with the user's OpenPGP public key (or decrypt an armored file)
   entirely offline.
3. Write the plaintext bundle back for the website import path only after a
   successful encrypted-message decrypt and integrity check.

The website does not call Tools, does not launch it, and does not exchange keys
or passphrases with it.
