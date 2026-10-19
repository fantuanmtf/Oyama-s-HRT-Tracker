# hrt-cold

`hrt-cold` is a standalone, offline OpenPGP cold-backup CLI for the website's
`oyama-hrt-complete-backup` JSON bundle.

**Status: experimental Tools checkpoint.** The `.hrtb` container is the draft
Phase 1F format. It is **not stable**, not production-approved, and not wired
into the website. This tool does not add OpenPGP to the website and is not a
runtime dependency of the web app.

The website remains responsible for producing and importing the unencrypted
bundle. This tool validates the bundle envelope, signs and encrypts it,
decrypts and verifies it, applies a local anti-replay mark to verified backups,
and writes files atomically.

## Build and test

```sh
cd tools/hrt-cold
go build -o hrt-cold .
go vet ./...
go test ./...
```

The module is pinned to gopenpgp `v3.5.2`; the linked runtime closure is only
gopenpgp, go-crypto, circl, `x/crypto`, and `x/sys`. See
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).

## Commands

### keygen

```sh
install -m 0600 /dev/null passphrase.txt
# Put the passphrase in passphrase.txt using an editor that does not leave
# copies in shell history or process arguments.
hrt-cold keygen \
  --private-out backup-private.asc \
  --public-out backup-public.asc \
  --passphrase-file passphrase.txt
```

- Writes an armored, passphrase-locked private key (mode `0600` on Unix) and an
  armored public key (mode `0644` on Unix).
- Never overwrites an existing output. If the public-key write fails after the
  private key was published, the new private key is removed.
- Prints the full primary-key fingerprint to stdout as
  `fingerprint: <hex>`.
- The generated user ID is the fixed synthetic identity
  `Oyama HRT Cold Backup <hrt-cold@example.invalid>`.
- The generated key can sign and encrypt. Using one key for both is simplest;
  generating a second key pair and using it only as `--signer-private` /
  `--verify-key` keeps signing and decryption authority separate.
- Unix requires the passphrase file to be mode `0600` or stricter. Windows
  checks that it is a regular file and relies on the NTFS ACL; Unix mode bits
  are not a Windows ACL and are not verified there.

Keep the private key and passphrase offline and separate from backups. Losing
either makes encrypted backups unrecoverable.

### encrypt

```sh
hrt-cold encrypt \
  --recipient backup-public.asc \
  --signer-private signing-private.asc \
  --signer-passphrase-file signing-passphrase.txt \
  --in complete-backup.json \
  --out complete-backup.hrtb
```

- Reads the website complete-backup bundle (maximum 15 MiB).
- Validates the envelope: JSON structure, duplicate keys, depth, string size,
  `format == "oyama-hrt-complete-backup"`, `formatVersion == 1`, RFC 3339
  `createdAt`, `mode`, both `modes` blocks, category array shapes, tombstones,
  calibration enums, preferences object, and scalar types.
- Does **not** validate clinical record semantics. The website importer remains
  the authority for record validation, merge behavior, and conflicts.
- Unknown fields are accepted for forward compatibility.
- When the signer flags are supplied, signs the plaintext bundle before
  encrypting it. The signature covers the whole envelope, including
  `createdAt`, so provenance and content are bound together.
- Encrypts to the recipient public key without compression, wraps the binary
  OpenPGP message in the `.hrtb` container, and publishes the output without
  overwriting an existing file.
- Prints the recipient fingerprint, the signer fingerprint when signed, the
  plaintext SHA-256, and `signature: signed|unsigned`. Omitting the signer flags
  still works for legacy use but prints a warning: the backup then has no
  provenance.

### decrypt

```sh
hrt-cold decrypt \
  --private backup-private.asc \
  --passphrase-file passphrase.txt \
  --verify-key signing-public.asc \
  --in complete-backup.hrtb \
  --out restored-complete-backup.json
```

- Validates the container and the outer OpenPGP packet sequence before using
  the key.
- Requires a genuinely encrypted OpenPGP message: one or more public-key
  encrypted session-key packets, then exactly one integrity-protected encrypted
  data packet, followed immediately by end of payload. Only legal marker
  packets may appear before the encrypted data packet. Any trailing packet,
  second encrypted data packet, missing recipient, or missing encrypted data
  packet is rejected. A literal, compressed-plaintext, or signed-only payload
  is rejected without writing output.
- Verifies the OpenPGP integrity check and rejects a wrong key, wrong
  passphrase, tampered ciphertext, truncation, unknown version, reserved flags,
  or trailing bytes. The private key must actually decrypt the session key; a
  message that did not use the supplied key is not accepted.
- With `--verify-key`, requires a valid signature from that public key (or one
  of its subkeys). An unsigned message, a bad signature, or a signature from a
  different key fails closed and writes no output. Without `--verify-key`,
  decryption still works for legacy backups but prints a warning and reports
  `signatureVerified: false`.
- A verified decrypt also checks a local anti-replay high-water mark keyed by
  the signer's primary fingerprint. The state defaults to
  `<user-config>/hrt-cold/replay.json` (mode `0600`); `--replay-state FILE`
  overrides it, `--no-replay-check` disables it, and `--allow-older` overrides
  an older or same-time-different result without lowering the mark. The exact
  same bundle decrypts again as `repeat`.
- Re-validates the complete-backup envelope after decryption.
- Writes the exact decrypted bytes, mode `0600` on Unix, only after all checks
  pass. An existing output is never replaced.

### inspect

```sh
hrt-cold inspect --in complete-backup.hrtb
hrt-cold inspect --in complete-backup.hrtb --recipient backup-public.asc
```

- Reads only the outer container and the unauthenticated OpenPGP public-key
  encrypted session-key (PKESK) packets. It does not decrypt the payload and
  prints `decrypted: false`.
- Always shows magic, container version, flags, payload length, payload SHA-256,
  recipient count, `advertisedRecipientKeyID`, `advertisedRecipientFingerprint`
  when the packet carries one, and `recipientVerified: false`.
- PKESK recipient identifiers are chosen by whoever constructed the container
  and are **not authenticated**. They do not prove that the container is
  encrypted to a particular key, and a forged packet can advertise any key ID.
- Supplying `--recipient` only compares the advertised identifiers with the
  supplied public key. It prints `suppliedRecipientFingerprint`,
  `advertisedRecipientMatchesSupplied`, and any
  `advertisedRecipientMatchFingerprint` values on a best-effort basis. This is
  informational; a mismatch does not fail `inspect`, and a match is not
  recipient verification. Only a successful `decrypt` demonstrates that the
  private key participated.
- `inspect` uses the same outer packet-sequence validator as `decrypt`. It
  rejects literal, compressed-plaintext, signed-only, recipient-less, and
  payload-less containers, as well as any packet after the encrypted data
  packet, instead of reporting them as valid backups.
- `inspect` cannot detect tampering of the ciphertext because it does not
  decrypt or verify the MDC. Use `decrypt` for integrity verification.

## `.hrtb` container (experimental)

All integers are big-endian.

| Offset | Size | Field | Value |
| ---: | ---: | --- | --- |
| 0 | 4 | magic | ASCII `HRTB` |
| 4 | 2 | version | `1` |
| 6 | 2 | flags | `0` (all bits reserved) |
| 8 | 8 | payload length | unsigned 64-bit |
| 16 | `payload length` | payload | one binary OpenPGP message |

Rules enforced by this implementation:

- The total file is at most 16 MiB; the payload is at most 16 MiB minus the
  16-byte header.
- The payload length must be non-zero and must match the remaining file
  exactly. Truncation and trailing bytes are rejected.
- Unknown versions and any non-zero flags are rejected. There is no downgrade
  or fallback to plaintext: `decrypt` and `inspect` require an encrypted OpenPGP
  message. Literal, compressed-plaintext, and signed-only payloads are rejected.
- The payload is a standard OpenPGP message. This tool encrypts without
  compression. No custom cryptography or KDF is defined.
- A decryptable payload must contain at least one public-key encrypted
  session-key packet followed by exactly one integrity-protected encrypted data
  packet and then EOF. Legal marker packets may be skipped before the encrypted
  data packet; any packet after it is rejected, including an unencrypted
  literal, compressed, or signed packet.
- The plaintext bundle is at most 15 MiB, leaving room for OpenPGP overhead
  inside the 16 MiB container.

The format is aligned with the Phase 1F draft (`HRTB_FORMAT_SPEC_DRAFT.md`) but
remains experimental. The draft's separate `hrt-cold-backup` inner envelope is
not used here: this checkpoint encrypts the website's own
`oyama-hrt-complete-backup` envelope directly.

## File-safety behavior

`atomicWrite`:

1. creates a `.hrt-cold-*` temp file in the destination directory;
2. applies the requested mode, writes, `fsync`s, and closes it;
3. publishes with `os.Link` (hard link), which fails instead of replacing an
   existing target;
4. removes the temp name;
5. best-effort `fsync`s the destination directory.

Linux hard-link publish is exercised by native tests. Windows uses the same
`os.Link` path and is exercised by native `windows-latest` tests in CI. Windows
NTFS is expected to support hard links; if a filesystem does not (for example
FAT/exFAT or some network shares), the write fails closed and does not fall
back to `rename(2)`, because rename can replace an existing file. Directory
fsync and crash durability are best-effort and not proven. A process crash or
power loss can leave a `.hrt-cold-*` temp file; this checkpoint does not run
automatic stale-temp cleanup.

No failure path intentionally leaves a partial plaintext output. This is not a
claim of physical erasure: deleted files, temp files, SSD blocks, snapshots,
swap, crash dumps, and copies may retain data.

## Security limits

- Encryption is public-key OpenPGP to the supplied recipient. Signing is
  implemented but optional: without `--signer-private` the backup has no
  provenance, and without `--verify-key` decryption does not verify it.
- Recipient key IDs and fingerprints shown by `inspect` are unauthenticated
  PKESK advertisements, not proof of encryption to that key. Only a successful
  `decrypt` shows that a private key participated.
- Fingerprint display does not establish trust. Compare the full fingerprint
  through an independent channel before encrypting or pinning it.
- Anti-replay is a local high-water mark. It does not protect a fresh device, a
  deleted state file, a compromised signer key, or a stolen private key, and it
  cannot authenticate plaintext bundles imported directly into the website.
- There is no remote attestation or protection from a compromised endpoint,
  malicious binary, keylogger, or filesystem.
- Plaintext exists in memory before encryption and after decryption.
- The tool does not manage key rotation, revocation, recovery kits, or
  recovery drills.
- Backups and keys must be stored and transported separately. This tool does
  not provide physical erasure.
- This is an engineering checkpoint with regression tests, not an independent
  cryptographic audit. OpenPGP and MDC behavior are delegated to the linked
  go-crypto/gopenpgp libraries.

## Validation boundaries

The CLI validates the complete-backup **envelope** and container structure. It
does not re-implement the website's per-record sanitizers or merge rules. A
bundle that passes `hrt-cold` may still be rejected or partially handled by the
website importer. See
[`docs/backup-schema-audit/TOOLS_INTERFACE.md`](../../docs/backup-schema-audit/TOOLS_INTERFACE.md).

## Tests

Tests generate fresh keys and passphrases in temporary directories at test
time. They use only the synthetic fixture in
[`testdata/synthetic-bundle.json`](testdata/synthetic-bundle.json). No private
keys, passphrases, real health data, or production backups are committed.

Coverage includes round-trip encryption/decryption, wrong key, wrong passphrase,
tamper, truncation, unknown version, reserved flags, bad magic, trailing bytes,
unencrypted literal, compressed-plaintext, and signed-only payloads, forged
PKESK recipient key IDs, missing recipient/encrypted-payload packets, an empty
encrypted packet followed by trailing literal/compressed/signed packets,
multi-recipient and hidden-recipient messages, legal marker packets, existing
output, oversized input, malformed bundle, missing output parent, atomic
no-replace publication, concurrent writers, temp cleanup, advertised-recipient
matching, and passphrase-file mode checks on Unix. Signing coverage includes a
separate signer key, wrong and missing pinned keys, unsigned backups with
`--verify-key`, replay new/repeat/older/conflict states, and replay-state file
permissions and corruption.

## License

The repository is MIT. Third-party runtime notices are in
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).
