# Oyama HRT Tracker — Tools

This branch contains the offline Tools line of work for the Oyama HRT Tracker
complete-backup format. It intentionally does not contain the website
application, website assets, or website CI.

## Contents

- [`tools/hrt-cold`](tools/hrt-cold/README.md) — experimental offline OpenPGP
  cold-backup CLI: bundle validation, sign/encrypt, decrypt/verify, local
  anti-replay state, and atomic no-replace writes.
- [`docs/backup-schema-audit/TOOLS_INTERFACE.md`](docs/backup-schema-audit/TOOLS_INTERFACE.md)
  — the website ↔ Tools hand-off format and boundary.
- [`.github/workflows/tools-hrt-cold.yml`](.github/workflows/tools-hrt-cold.yml)
  — native Linux and Windows tests for the CLI.

The `.hrtb` container, the optional signing/provenance flow, and the CLI are
experimental and not production-approved. See the tool README for build,
usage, security limits, and test commands.
