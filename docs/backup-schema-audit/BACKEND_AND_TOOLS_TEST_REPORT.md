# Backend and Tools Test Report

**Date:** 2026-10-19
**Type:** read-only verification plus a local smoke test. This is not a security
audit, not production approval, and does not claim the absence of
vulnerabilities.

## Pinned revisions

| Track           | Ref                     | SHA                                        | Worktree                                |
| --------------- | ----------------------- | ------------------------------------------ | --------------------------------------- |
| Tools           | `Recovery/Tools`        | `7ee628b6455dee31ca044e453dff31e6d17a7e56` | `/tmp/opencode/hrt-cold-p0`             |
| Website/backend | `agent/add-gpg-website` | `5bfbb6bc83c4f1155744131afe04f14851249609` | `/tmp/opencode/hrt-ai-workflow-context` |

Both worktrees were clean before the checks. No production code, schema,
dependency, Docker, license, database, or release file was changed. The only
repository change is this report plus a pointer in `HANDOFF.md`.

## Environment

| Tool       | Version                           |
| ---------- | --------------------------------- |
| Go         | `go1.27.1-X:nodwarf5 linux/amd64` |
| Node.js    | `v22.14.0`                        |
| npm        | `10.9.2`                          |
| TypeScript | `5.8.3`                           |
| Vitest     | `4.1.11`                          |
| Vite       | `6.4.4`                           |
| Wrangler   | `4.149.0`                         |

Node.js was taken from `/home/haiyan/.local/share/nodejs/bin`; it was not on the
default non-interactive `PATH`.

## Summary

| Area                                                                    | Status          | Evidence                                                                                                 |
| ----------------------------------------------------------------------- | --------------- | -------------------------------------------------------------------------------------------------------- |
| Tools formatting/vet/build/module verification                          | **PASS**        | `gofmt`, `go vet`, `go build`, `go mod verify` all exit 0                                                |
| Tools unit tests                                                        | **PASS**        | `go test -count=1 ./...`: 32 `Test*` functions, exit 0                                                   |
| Tools race tests                                                        | **PASS**        | `go test -race -count=1 ./...`: exit 0, 18.354s                                                          |
| Synthetic CLI recovery loop                                             | **PASS**        | keygen → encrypt → inspect → decrypt → byte-identical compare                                            |
| Synthetic failure cases                                                 | **PASS**        | wrong key/passphrase, tamper, truncate, existing output, trailing packet/bytes all exit 1 with no output |
| Website type check                                                      | **PASS**        | `npx tsc --noEmit`, exit 0, no output                                                                    |
| Website tests                                                           | **PASS**        | 19 files, 417 passed, 4 expected fail, exit 0                                                            |
| Website production build                                                | **PASS**        | Vite build exit 0; existing >500 kB chunk warning only                                                   |
| Local Worker migrations/startup                                         | **PASS**        | all 9 migrations applied locally; Worker ready on loopback                                               |
| Public `/api/transparency`                                              | **PASS (note)** | 200, aggregate JSON, security headers; `Cache-Control` differs from the workflow document                |
| Unauthenticated `/api/content`, `/api/shares`                           | **PASS**        | 401, `SESSION_INVALID`, `X-Session-Invalid: 1`, no data                                                  |
| `/share` headers/referrer policy                                        | **PASS**        | 200 SPA shell, `Referrer-Policy: no-referrer`, `X-Robots-Tag`, CSP, HSTS, frame/nosniff                  |
| Production/Cloudflare runtime behavior                                  | **NOT_TESTED**  | local Miniflare/Workerd only                                                                             |
| Browser referrer enforcement, passkeys, R2, scheduled jobs, rate limits | **NOT_TESTED**  | outside this run                                                                                         |
| Independent cryptographic/Tools audit                                   | **BLOCKED**     | requires separate review                                                                                 |
| L3 browser backup E2E                                                   | **NOT_TESTED**  | no browser harness in this run                                                                           |

No **FAIL** result was observed in the executed checks.

## 1. Tools checks

Commands run in `/tmp/opencode/hrt-cold-p0/tools/hrt-cold`:

```sh
gofmt -l .
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
go build ./...
go mod verify
```

| Command                        | Result                                     |
| ------------------------------ | ------------------------------------------ |
| `gofmt -l .`                   | no output, exit 0                          |
| `go vet ./...`                 | no output, exit 0                          |
| `go test -count=1 ./...`       | `ok ... 7.536s`, exit 0; 32 test functions |
| `go test -race -count=1 ./...` | `ok ... 18.354s`, exit 0                   |
| `go build ./...`               | exit 0                                     |
| `go mod verify`                | `all modules verified`, exit 0             |

`go build ./...` created an ignored `tools/hrt-cold/hrt-cold` binary. It was
removed after the checks; no tracked file changed. `git status` remained clean.

## 2. Synthetic CLI recovery loop

All material was created under `/tmp/opencode/hrt-cold-synth.OsKG72` (mode 700).
The input was the repository fixture
`tools/hrt-cold/testdata/synthetic-bundle.json`. The passphrase was generated
with `openssl rand -base64 32` into a mode-0600 file and was never placed in a
command argument or the report.

### Successful path

The actual order was keygen → encrypt → inspect → decrypt → compare, because
`inspect` requires an existing `.hrtb` container.

| Step                             | Exit | Result / artifact                                                                                                                                                                                          |
| -------------------------------- | ---: | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `keygen`                         |    0 | private key mode 600, 1024 bytes; public key mode 644, 799 bytes; fingerprint `e6013d862efd9c4fe46dbc1475ab3f25c7961c69`                                                                                   |
| second `keygen` (wrong-key test) |    0 | fingerprint `04da8e1138387e49601ca589a40639a09f97f3ff`                                                                                                                                                     |
| `encrypt`                        |    0 | `bundle.hrtb`, mode 600, 2642 bytes, SHA-256 `c27f2097af584556573769937b30077bea94c58d4c3e2b057465546d0b7890ea`                                                                                            |
| `inspect`                        |    0 | `payloadBytes: 2626`; `payloadSHA256: 14a9e73dfc6140d0c9bf40c7524c88631bebc1f6a9f20cc44423433bbea5a2f9`; 1 recipient; advertised key ID `69fbd917ad2601cf`; `recipientVerified: false`; `decrypted: false` |
| `inspect --recipient`            |    0 | `suppliedRecipientFingerprint: e6013d...`; `advertisedRecipientMatchesSupplied: true`; still `recipientVerified: false`                                                                                    |
| `decrypt`                        |    0 | `restored.json`, mode 600, 2476 bytes, SHA-256 `f9bfe87717ec02e8274ac765e088a506b8d4ed57c280da22b608fef4a2aac305`                                                                                          |
| `cmp bundle.json restored.json`  |    0 | byte-identical; both SHA-256 values above                                                                                                                                                                  |
| `help`                           |    0 | usage printed                                                                                                                                                                                              |

Representative commands:

```sh
hrt-cold keygen  --private-out key-private.asc --public-out key-public.asc --passphrase-file pass.txt
hrt-cold encrypt --recipient key-public.asc --in bundle.json --out bundle.hrtb
hrt-cold inspect --in bundle.hrtb --recipient key-public.asc
hrt-cold decrypt --private key-private.asc --passphrase-file pass.txt --in bundle.hrtb --out restored.json
cmp bundle.json restored.json
```

### Failure paths

Each command below returned exit 1, wrote the expected error to stderr, and did
not create the requested output file.

| Case                                                                          | Exit | Observed error (abridged)                                                                                                                              |
| ----------------------------------------------------------------------------- | ---: | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| wrong private key                                                             |    1 | `error: decrypt (wrong key or corrupt container): openpgp: incorrect key`                                                                              |
| wrong passphrase                                                              |    1 | `error: unlock private key (wrong passphrase or unsupported key): gopenpgp: error in unlocking key`                                                    |
| tampered ciphertext (last payload byte flipped)                               |    1 | `error: verify encrypted payload (tampered, truncated, or corrupt container): openpgp: decryption with session key failed: parsing error`              |
| truncated container (last byte removed)                                       |    1 | `error: truncated HRTB container: declared 2626 payload bytes, got 2625`                                                                               |
| existing output file                                                          |    1 | `error: output already exists: .../existing-output.json`; sentinel SHA-256 remained `d812286338742da87a8edc612bfc3f64ff92846f9d71306db7e075a899dd45c6` |
| inner trailing packet (literal packet after encrypted data, length corrected) |    1 | `error: container payload has a packet after the encrypted data packet: unexpected *packet.LiteralData packet`; `inspect` also exit 1                  |
| trailing bytes after container                                                |    1 | `error: trailing bytes after HRTB payload: 1 extra bytes`; `inspect` also exit 1                                                                       |

Artifact hashes:

| Artifact               | SHA-256                                                            |
| ---------------------- | ------------------------------------------------------------------ |
| `bundle.hrtb`          | `c27f2097af584556573769937b30077bea94c58d4c3e2b057465546d0b7890ea` |
| `tampered.hrtb`        | `21951e65b0d639bee31f14f6a174a70983225bba53415996b9ccf2addccff077` |
| `truncated.hrtb`       | `eb66167dc7697a44bf3fe170f82a212f2f7612f62d16f0ddff112bfd92efaf9d` |
| `trailing-packet.hrtb` | `cec53674ba60966315392257809045fdafa8706b94e5b5bd2ac921f6fbd28463` |
| `trailing-bytes.hrtb`  | `f1959d5e9da19b615fa01ca5229cafe0be0bc1483e71b198fdccdd507d8f50a3` |

The tampered/truncated/trailing copies were created with Python's default mode
0644; they are synthetic test inputs, not CLI outputs. CLI-created private keys,
containers, and plaintext outputs were mode 0600; the generated public key was 0644.

### Hygiene

- No private key, passphrase, or plaintext was committed. `git status` in the
  Tools worktree was clean after the checks.
- No `.asc`, passphrase, or plaintext file was written outside
  `/tmp/opencode/hrt-cold-synth.OsKG72`.
- The CLI is local-only; no network command or service was used.
- The synthetic private keys, passphrase files, and decrypted copy were deleted
  after the run. Logs and ciphertext artifacts remain under the temp directory.
- Note: `/tmp/opencode/hrt-cold-manual-20261019` pre-existed from an earlier
  manual run and was not created, read as input, or modified by this run.

## 3. Website checks

Commands run in `/tmp/opencode/hrt-ai-workflow-context` after
`npm ci --no-audit --no-fund` (554 packages, exit 0):

```sh
npx tsc --noEmit
npx vitest run
npm run build
```

| Command            | Result                                                                   |
| ------------------ | ------------------------------------------------------------------------ |
| `npx tsc --noEmit` | exit 0, no output                                                        |
| `npx vitest run`   | 19 files passed; 417 passed, 4 expected fail (421 total); 23.05s; exit 0 |
| `npm run build`    | Vite production build pass; PWA service worker generated; exit 0         |

The only build warning was the existing chunk larger than 500 kB warning for
`dist/assets/index-C_VNukil.js` (1,526.47 kB, 480.03 kB gzip).

`dist/` and `node_modules/` were untracked/ignored; `git status` reported no
tracked changes after the checks.

## 4. Local Worker smoke test

### Setup

```sh
BASE=/tmp/opencode/hrt-backend-smoke-20261019
CI=1 npx wrangler d1 migrations apply hrt-tracker-prod --local --persist-to "$BASE/persist"
npx wrangler dev --local --ip 127.0.0.1 --port 8787 \
  --persist-to "$BASE/persist" \
  --var "JWT_SECRET:$SYNTHETIC_JWT_SECRET" \
  --show-interactive-dev-session=false
```

- `SYNTHETIC_JWT_SECRET` was generated with `openssl rand -hex 32` and is not
  reproduced in this report.
- The persistence directory was new and isolated. No `.dev.vars` was created.
- All migrations `0000_base_tables.sql` through
  `0008_login_backoff.sql` applied successfully to the local D1 database.
- Wrangler started on `127.0.0.1:8787` and logged `Ready`. The log showed local
  request handling only; no remote deployment or remote database command was
  run.
- The Worker was stopped after the probes. The local persistence tree contains
  only Miniflare/Workerd SQLite files under the temp directory.

### Probe results

| Request                                   | Status | Result                                                                                                                        |
| ----------------------------------------- | -----: | ----------------------------------------------------------------------------------------------------------------------------- |
| `GET /api/transparency`                   |    200 | 209-byte aggregate JSON; SHA-256 `1116b6089361fe3663d2b132cbcd96ad09b9ec19f888af3f729b735825dd6ba8`; empty synthetic database |
| `OPTIONS /api/transparency`               |    200 | Empty body; CORS origin/method/header values present                                                                          |
| `GET /api/content` (no token)             |    401 | `{"code":"SESSION_INVALID","message":"Unauthorized"}`, `X-Session-Invalid: 1`, `Cache-Control: no-store`                      |
| `POST /api/content` (no token)            |    401 | Same fail-closed response; no write                                                                                           |
| `GET /api/content` (invalid bearer token) |    401 | `{"code":"SESSION_INVALID","message":"Invalid token"}`, `X-Session-Invalid: 1`                                                |
| `POST /api/shares` (no token)             |    401 | Same fail-closed response; no write                                                                                           |
| `GET /api/shares` (no token)              |    401 | Same fail-closed response; no share metadata                                                                                  |
| `GET /share`                              |    200 | 1964-byte SPA shell; SHA-256 `745d143b2e8863069f3650ab7f97653d8efb7b7cf01a7d0b2c990ba7032da63e`                               |
| `GET /share/synthetic-token`              |    200 | Same SPA shell/headers                                                                                                        |
| `GET /`                                   |    200 | Same SPA shell; no share-specific referrer/robots headers                                                                     |
| `GET /api/not-a-route` (no token)         |    401 | Fail-closed before route handling; no route enumeration                                                                       |

`/api/transparency` response headers included `Content-Type: application/json`,
`Access-Control-Allow-Origin: *`, `Strict-Transport-Security`,
`Content-Security-Policy`, `Permissions-Policy`,
`X-Content-Type-Options: nosniff`, and `X-Frame-Options: DENY`.

**Note — cache policy mismatch:** `/api/transparency` returned
`Cache-Control: public, max-age=15`, not the `Cache-Control: no-store` value
listed as expected evidence for B-03 in
`AI_WORKFLOW_AND_CONTEXT.md`. The body contains aggregate counts only and no
per-user data, so this may be intentional public caching. A human should confirm
whether the 15-second public cache is the intended policy and update either the
implementation or the test matrix.

**Note — preflight headers:** the `OPTIONS` response carries CORS headers only;
it does not repeat the security headers. This is normal for a bodyless preflight
and was not treated as a failure.

### `/share` security headers

Both `/share` and `/share/synthetic-token` returned:

- `Referrer-Policy: no-referrer`
- `X-Robots-Tag: noindex, nofollow, noarchive`
- `Content-Security-Policy` with `frame-ancestors 'none'`, `object-src 'none'`,
  `base-uri 'self'`, and `script-src 'self'`
- `Strict-Transport-Security: max-age=31536000`
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Permissions-Policy: camera=(), microphone=(), geolocation=()`

The `/` SPA route had the same general security headers but deliberately did not
have the share-specific `Referrer-Policy` or `X-Robots-Tag`.

This verifies the response headers only. Browser enforcement of
`Referrer-Policy` (including navigation from `/share` to third-party links),
CSP behavior, and mixed-content behavior were **NOT_TESTED**.

## 5. Backend test coverage observations

Read-only review of the test suite found no direct `worker.fetch` coverage for
the routes exercised above:

- `tests/workerHardening.test.ts` covers module exports, client-IP parsing,
  passkey origin validation, and wrongly typed credentials.
- `tests/sharePasswordLockout.test.ts` exercises migration statements and
  lockout SQL behavior, not the full `/api/shares` fetch route.
- `tests/routes.test.ts` is about pharmacokinetic routes, not HTTP routes.
- No test asserts `/api/transparency` status/body/cache policy, unauthenticated
  `/api/content` or `/api/shares` responses, `X-Session-Invalid`, or `/share`
  security headers.

This is a coverage gap, not a demonstrated defect. The local smoke test provides
one observed response per path, not regression protection.

## 6. Residual risks and limits

- Local Miniflare/Workerd is not the production Cloudflare runtime. Production
  D1, R2, rate-limit bindings, scheduled jobs, TLS/WAF, and deployed routing are
  **NOT_TESTED**.
- No authenticated user flow, role boundary, admin route, share creation/access,
  passkey/WebAuthn ceremony, session expiry/revocation, or cross-user access was
  tested. The 401 checks only prove that the selected endpoints reject missing
  and syntactically invalid bearer tokens before data access.
- No malformed-body, oversized-body, malformed-ID, rate-limit, or load tests were
  run. The AI workflow plan explicitly excludes load/DoS testing.
- The `/api/transparency` cache-policy mismatch needs owner confirmation.
- The Tools `.hrtb` format remains experimental. `inspect` reports
  unauthenticated PKESK advertisements; only a successful `decrypt` shows the
  supplied key participated. MDC/OpenPGP behavior is delegated to
  gopenpgp/go-crypto and has not had an independent cryptographic audit.
- Tools Windows durability/ACL behavior was **NOT_TESTED** in this run; native
  Windows CI exists but is separate evidence.
- No L3 browser export/import/merge test, production restore, Replace
  transaction, signing, key rotation, or key recovery was exercised.
- No claim is made that these systems have no vulnerabilities.

## 7. Recommended next steps

1. Confirm the intended `/api/transparency` cache policy and reconcile
   `Cache-Control: public, max-age=15` with the B-03 expectation.
2. Add Worker fetch-level regression tests for transparency, unauthenticated
   content/shares, `X-Session-Invalid`, and `/share` headers, using a mocked D1
   binding and synthetic requests.
3. Re-run the backend smoke against the reviewed branch head after any route,
   header, or auth change; keep the isolated `--local --persist-to` setup.
4. Have a human/security reviewer cover authenticated authorization boundaries,
   passkey flows, R2, scheduled cleanup, rate limiting, and production runtime
   differences.
5. Commission an independent review of the Tools OpenPGP container/format before
   any release or website integration claim.

## 8. Evidence locations

- Tools worktree: `/tmp/opencode/hrt-cold-p0`
- Synthetic CLI run: `/tmp/opencode/hrt-cold-synth.OsKG72` (`run.sh`,
  `run.log`, `logs/`)
- Backend smoke: `/tmp/opencode/hrt-backend-smoke-20261019` (`logs/`,
  `http/`, `persist/`)
- These directories are temporary and are not part of the repository.
