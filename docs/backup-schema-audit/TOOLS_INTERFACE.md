# Website ↔ Tools Complete-Backup Interface

**Status:** website export, merge, and Replace data layers implemented. Replace
is a separate danger action backed by a crash-recoverable localStorage journal;
it changes no envelope or `.hrtb` bytes.
**Tools reference:** `Recovery/Tools` @ `d2ef418` (read-only),
`tools/hrt-cold/`. The Tools CLI is an experimental checkpoint; the `.hrtb`
container is not stable and is not wired into the website.
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
- The outer OpenPGP packet sequence must contain one or more public-key
  encrypted session-key (PKESK) packets and exactly one integrity-protected
  encrypted data packet, with no packet after it. Only legal marker packets
  may be skipped before the encrypted data packet. A trailing literal,
  compressed, or signed packet must be rejected even when the encrypted data
  packet itself is empty.
- PKESK recipient key IDs and fingerprints are unauthenticated metadata. Tools
  may display them as advertised values, but must not present them as
  recipient verification. Only successful decryption shows that a private key
  participated.
- A failed decryption, wrong key, wrong passphrase, failed integrity check,
  truncation, or unencrypted payload must not write a plaintext bundle output.
- The Tools `inspect` command may read only the outer container and the
  unauthenticated PKESK advertisements. It does not decrypt, verify the
  message integrity check (MDC), or detect ciphertext tampering; its
  `recipientVerified` result remains false.

## Tools signing, provenance, and anti-replay

The `.hrtb` container and the website envelope are unchanged. When
`encrypt --signer-private --signer-passphrase-file` is supplied, Tools signs
the **plaintext bundle** with the signer's OpenPGP signing key and then encrypts
the signed message to the recipient. The signature therefore covers the
complete-backup envelope, its `createdAt`, every record, and any future fields
without a second format. The signer identity is inside the encryption and is
visible only to someone who can decrypt.

`decrypt --verify-key FILE` pins the expected signer public key:

- the message must be signed, the signature must verify, and it must come from
  the supplied key or one of its subkeys;
- a signature from another key, a bad signature, an unsigned message, or a
  detached/ambiguous signer fails closed and writes no output;
- output includes `signatureVerified`, `signerFingerprint`,
  `signerPrimaryFingerprint`, and `contentSha256` (SHA-256 of the exact
  plaintext bundle);
- without `--verify-key`, Tools prints a warning and does not claim
  provenance. This keeps legacy unsigned backups readable, but such a decrypt
  is unauthenticated.

A verified decrypt also consults a local anti-replay high-water mark, keyed by
the signer's primary fingerprint. The state file defaults to
`<user-config>/hrt-cold/replay.json` (mode `0600`, group/world-writable files
are refused) and can be changed with `--replay-state FILE`:

- a newer `createdAt` is accepted and raises the mark;
- the exact same bundle (same `createdAt` and content hash) is accepted as a
  repeat without changing the mark;
- an older `createdAt`, or the same `createdAt` with different content, is
  refused unless `--allow-older` is passed; an override never lowers the mark;
- `--no-replay-check` skips the local state for a deliberate restore.

This is local, best-effort rollback detection. It is not a global freshness
service: a different device, a deleted state file, a compromised signer key, or
a malicious binary can defeat it. It does not authenticate backups imported
directly into the website as plaintext JSON; that path has no keys and no
signature. Website-level provenance would require changing the website's
no-keys boundary and is not implemented.

## Website Replace (internal)

The website has a separate "replace this device" action, not part of this
hand-off format. It accepts only a complete bundle: both modes, every category,
both tombstone maps, and calibration must be present, and no duplicate id or
dropped record is allowed. It writes a single localStorage journal containing
the snapshot and the target values before changing any record key; a failed
write restores the snapshot, and an interrupted process is rolled back (or
finished, if the journal had reached `committed`) before React mounts. The
journal needs free localStorage headroom for the target data; if it cannot be
written, Replace aborts before touching records. Scalars omitted by the bundle
are left alone. This is an application-level transaction
over localStorage, not a storage primitive, and it does not change Tools input
or output.

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
4. Tombstones are unioned; a restored record clears its own tombstone. Expired
   local tombstones are pruned and the pruned map is written back during the
   same merge, so a later sync cannot re-delete a restored record.
5. Preferences are returned to the website's React state; the data layer does
   not write credential keys.
6. Record and tombstone ids that collide with `Object.prototype` names are
   rejected at bundle validation. A syntactically valid drug code this build
   does not know is preserved, treated as unmodelled, and never rewritten to a
   known ester.

**Replace all** is not implemented and remains `BLOCKED`: `localStorage` has no
proven multi-key transaction, so no all-or-nothing claim is made.

## Tools validation and `.hrtb` container

Tools validates the hand-off **envelope**, not clinical record semantics. The
website importer remains authoritative for per-record validation, merge,
conflict, tombstone, and preference behavior.

- Bundle input is limited to 15 MiB. The JSON reader rejects duplicate object
  keys (exact spelling), excessive nesting, oversized strings, and trailing
  values before typed decoding.
- `format` must be `oyama-hrt-complete-backup`; `formatVersion` must be the
  raw numeric token `1`; `createdAt` must be RFC 3339; `mode` must be
  `transfem` or `transmasc`; both `modes` blocks must be present.
- Category arrays must contain objects, `deletions` and `preferences` must be
  objects, calibration enums are checked, and `weight` / `weightUpdatedAt` /
  `pkParamsUpdatedAt` must be numeric when present.
- Unknown fields are accepted for forward compatibility. Record contents are
  opaque to Tools.
- Envelope struct fields are matched case-insensitively by Go's JSON decoder;
  a lowercase key and a case variant can both resolve to the same field, with
  the later member winning. Category keys remain exact lowercase. See BTV-16.

The `.hrtb` container (experimental) is big-endian:

```text
offset 0   magic "HRTB" (4 bytes)
offset 4   version 1 (2 bytes)
offset 6   flags 0, all bits reserved (2 bytes)
offset 8   payload length, unsigned 64-bit (8 bytes)
offset 16  one binary OpenPGP message (exactly the declared length)
```

The container is at most 16 MiB; the payload length must be non-zero and match
the remaining file exactly. Unknown versions and any non-zero flags are
rejected. No plaintext fallback exists: `decrypt` and `inspect` require an
encrypted OpenPGP message with the packet sequence described above.

## Tools workflow

The current Tools CLI (`hrt-cold`) supports:

1. `keygen` — write an armored passphrase-locked private key (mode `0600` on
   Unix) and a public key (mode `0644` on Unix), never overwriting an existing
   output, and print the primary-key fingerprint.
2. `encrypt` — validate the website bundle envelope; when signer flags are
   supplied, sign the plaintext; encrypt to the recipient public key without
   compression; wrap the binary OpenPGP message in the `.hrtb` container; and
   publish without replacing an existing target. It prints the recipient
   fingerprint, signer fingerprint when signed, and the plaintext SHA-256.
3. `decrypt` — validate the container and outer packet sequence, decrypt,
   verify the MDC and that the supplied private key actually participated,
   verify the pinned signer when `--verify-key` is supplied, enforce the local
   anti-replay mark for verified backups, re-validate the bundle envelope, and
   only then write the plaintext bytes.
4. `inspect` — read only the outer container and unauthenticated PKESK
   advertisements. It applies the same packet-sequence check as `decrypt` but
   does not decrypt, verify the MDC, verify the signer, or prove the recipient.

Tools writes files atomically with a temp file and hard-link no-replace
publish, and does not overwrite existing outputs. Windows and Linux behavior
are exercised by native CI; network filesystems, crash durability, and
application-level rollback remain outside this interface.

The website does not call Tools, does not launch it, and does not exchange keys
or passphrases with it. Tools should hand the decrypted plaintext bundle back
to the website import path only after a successful encrypted-message decrypt
and integrity check.
