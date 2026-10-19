# Linux Website ↔ Tools Recovery E2E

**Status:** local synthetic check only. This is not a production approval, a
security audit, or a claim that the format is stable.

**Update (later 2026-10-19):** the website-side UI steps are now automated in
PR #129 `e2e/`; this document records the earlier interactive run.

**Date:** 2026-10-19
**Platform:** Linux x86_64 (local Miniflare/Workerd, not Cloudflare production)

## Pinned revisions

| Track   | Ref                     | SHA                                        |
| ------- | ----------------------- | ------------------------------------------ |
| Website | `agent/add-gpg-website` | `e6b2e4f5b2cacf84dd9d86bee28d357d80311721` |
| Tools   | `Recovery/Tools`        | `7ee628b6455dee31ca044e453dff31e6d17a7e56` |

The website code was **not modified** for this check. The only artifacts are
this report and the demo assets referenced below.

## Environment

| Tool           | Version                                                         |
| -------------- | --------------------------------------------------------------- |
| Node.js        | `v22.14.0`                                                      |
| Wrangler       | `4.149.0`                                                       |
| Go             | `go1.27.1-X:nodwarf5 linux/amd64`                               |
| Browser        | Chromium via Playwright                                         |
| Local database | D1 migrations `0000_base_tables.sql` … `0008_login_backoff.sql` |

A synthetic `JWT_SECRET` was generated with `openssl rand -hex 32` and was not
written into this report or any tracked file. The local persistence directory
was new and isolated under `/tmp`.

## Setup

```sh
# Isolated worktrees
git worktree add --detach /tmp/e2e-web e6b2e4f
git worktree add --detach /tmp/e2e-tools 7ee628b

# Website build
cd /tmp/e2e-web
npm run build

# Local D1 migrations and Worker
CI=1 npx wrangler d1 migrations apply hrt-tracker-prod --local \
  --persist-to /tmp/e2e-backend/persist
npx wrangler dev --local --ip 127.0.0.1 --port 8787 \
  --persist-to /tmp/e2e-backend/persist \
  --var "JWT_SECRET:<synthetic-secret>" \
  --show-interactive-dev-session=false

# Tools build
cd /tmp/e2e-tools/tools/hrt-cold
go build -o /tmp/e2e-tools/hrt-cold .
```

The frontend was opened at `http://127.0.0.1:8787/` and used in local (signed
out) mode. The complete-backup export/import path does not require an account.

## Synthetic data

All records were synthetic. No real health data, keys, passphrases, or tokens
are in this report or in the repository.

| Item           |                       Count / value |
| -------------- | ----------------------------------: |
| Events         | 3 (2 sublingual E2, 1 injection EV) |
| Lab results    |                 1 (`pg/ml`, `lcms`) |
| Dose templates |                                   1 |
| Quick doses    |                                   1 |
| Supplies       |  1 (vial, 5 ml × 40 mg/ml, count 3) |
| Plan items     |                                   1 |
| Weight         |                               70 kg |
| Calibration    |            `mipd` / `retrospective` |
| Deletions      |                               empty |

## Successful recovery loop

1. Seed the synthetic records in the website.
2. **Export → Complete backup** in the website UI. The download is the
   unencrypted `oyama-hrt-complete-backup` JSON; a plaintext-health-data
   confirmation is required.
3. Encrypt it with the offline Tools CLI.
4. Inspect the container (metadata only).
5. Decrypt it with the offline Tools CLI.
6. Byte-compare the website export and the restored file.
7. Clear the browser data, reload the empty website, and import the restored
   bundle.
8. Verify the records, supplies, plan, weight, and absence of credentials.

### Artifacts

| Artifact                                 |        Size | Notes                        |
| ---------------------------------------- | ----------: | ---------------------------- |
| `website-export.json`                    | 2,772 bytes | Downloaded from the website  |
| `backup.hrtb`                            | 2,938 bytes | Tools output, mode `0600`    |
| `restored.json`                          | 2,772 bytes | Tools output, mode `0600`    |
| `website-export.json` vs `restored.json` |           — | `cmp` exit 0, byte-identical |
| private key                              | 1,024 bytes | mode `0600`                  |
| public key                               |   799 bytes | mode `0644`                  |

### Commands and observed output

```sh
hrt-cold keygen --private-out backup-private.asc \
  --public-out backup-public.asc --passphrase-file passphrase.txt
# fingerprint: c5d674cb9fceea7dd54f21b2d589263dab849308

hrt-cold encrypt --recipient backup-public.asc \
  --in website-export.json --out backup.hrtb
# recipientFingerprint: c5d674cb9fceea7dd54f21b2d589263dab849308
# bundleCreatedAt: 2026-10-19T08:19:38.055Z
# bundleMode: transfem

hrt-cold inspect --in backup.hrtb --recipient backup-public.asc
# magic: HRTB
# containerVersion: 1
# containerFlags: 0
# payloadBytes: 2922
# payloadSHA256: ce97a7898e440be30d970140c9995e4c0f81156dd00535442408e2932b75a9ad
# recipientCount: 1
# advertisedRecipientKeyID: 8a267aec297cb5ed
# suppliedRecipientFingerprint: c5d674cb9fceea7dd54f21b2d589263dab849308
# advertisedRecipientMatchesSupplied: true
# advertisedRecipientMatchFingerprint: e82604869403a050d3127ccf8a267aec297cb5ed
# recipientVerified: false
# decrypted: false

hrt-cold decrypt --private backup-private.asc \
  --passphrase-file passphrase.txt \
  --in backup.hrtb --out restored.json
# privateKeyFingerprint: c5d674cb9fceea7dd54f21b2d589263dab849308

cmp website-export.json restored.json && echo RECOVERY_BYTES_IDENTICAL
# RECOVERY_BYTES_IDENTICAL
```

`inspect` reports `recipientVerified: false` by design. The advertised PKESK
key ID is unauthenticated metadata; only a successful `decrypt` shows that the
private key participated.

### Import preview and result

After clearing browser storage and reloading, the website showed empty data.
Importing `restored.json` produced this preview:

```text
Target account: this device (not signed in)
Source mode: transfem
transfem: events 3, labs 1, templates 1, quick doses 1, supplies 1, plan 1, deletions 0
transmasc: events 0, labs 0, templates 0, quick doses 0, supplies 0, plan 0, deletions 0
weight: 70 -> 70 (unchanged)
calibration: mipd/retrospective -> mipd/retrospective (unchanged)
```

The merge result was `新增 8，更新 0，删除 0` (8 added, 0 updated, 0 removed).
Storage after the import contained:

| Key                  | Value                                       |
| -------------------- | ------------------------------------------- |
| `hrt-events`         | `e2e-e1`, `e2e-e2`, `e2e-e3`                |
| `hrt-lab-results`    | `e2e-l1`                                    |
| `hrt-dose-templates` | `e2e-t1`                                    |
| `hrt-quick-doses`    | `e2e-q1`                                    |
| `hrt-supplies`       | `e2e-s1`                                    |
| `hrt-plan`           | `e2e-p1`                                    |
| `hrt-weight`         | `70`                                        |
| `auth_token`         | absent (credential keys are never exported) |

The calibration values matched the app defaults and the merge correctly treated
them as unchanged, so the `hrt-cal-method` / `hrt-cal-history-mode` keys were
not written. They load as `mipd` / `retrospective` when absent. A bundle with a
different calibration value is shown as an overwrite and requires the separate
calibration confirmation before it is written.

## Failure paths

These were exercised with synthetic inputs. Each command exits 1 and leaves no
requested output file:

| Case                                             | Result                                      |
| ------------------------------------------------ | ------------------------------------------- |
| Wrong private key                                | `openpgp: incorrect key`                    |
| Wrong passphrase                                 | key unlock failure                          |
| Tampered ciphertext                              | integrity/MDC failure                       |
| Truncated container                              | `truncated HRTB container`                  |
| Existing output                                  | `output already exists`; sentinel unchanged |
| Packet after encrypted data (trailing plaintext) | `packet after the encrypted data packet`    |
| Bytes after the container                        | `trailing bytes after HRTB payload`         |

The trailing-packet case is the regression fixed in `7ee628b` (`f73f0c9`).
`inspect` rejects the same container.

## What this check does not prove

- Production Cloudflare/D1/R2, TLS, WAF, scheduled jobs, and rate limits.
- Windows native behavior, Windows ACLs, network filesystems, or crash
  durability.
- A browser-level L3 automation suite; the UI steps were driven interactively
  with Playwright in a local browser, not committed as a test.
- Cloud-sync behavior after a restore, multi-device conflict resolution, or
  Replace (`BLOCKED`; only merge is implemented).
- OpenPGP key management, signature/provenance, anti-replay, or an independent
  cryptographic audit. `.hrtb` remains experimental and has no sender
  authentication.
- The medical simulation and its correctness. This report only exercises the
  backup/restore path.

## Demo

The rendered terminal demo is in
[`docs/DEMO_RECOVERY.md`](../DEMO_RECOVERY.md). It is a synthetic animation
generated from the outputs above; it is not a live screen recording.
