# Oyama HRT Tracker — Tools

Repository: <https://github.com/xunxunProjects/Oyama-s-HRT-Tracker-Tools> (private).

This repository contains the offline Tools line of work for the Oyama HRT
Tracker complete-backup format. It intentionally does not contain the website
application, website assets, or website CI.

## Contents

- [`tools/hrt-cold`](tools/hrt-cold/README.md) — experimental offline OpenPGP
  cold-backup CLI: bundle validation, sign/encrypt, decrypt/verify, local
  anti-replay state, and atomic no-replace writes.
- [`docs/native`](docs/native/README.md) — Milestone 0 design, C ABI, crypto
  backend analysis, build instructions, and test plan for the planned C/C++
  native core. No native implementation is present yet.
- [`configure`](configure) and [`CMakeLists.txt`](CMakeLists.txt) — the
  configure/CMake build contract for that native core; Milestone 0 builds only
  C/C++ header compatibility checks.
- [`docs/backup-schema-audit/TOOLS_INTERFACE.md`](docs/backup-schema-audit/TOOLS_INTERFACE.md)
  — the website ↔ Tools hand-off format and boundary.
- [`.github/workflows/tools-hrt-cold.yml`](.github/workflows/tools-hrt-cold.yml)
  — Linux and Windows tests for the Go CLI and the native scaffold.

The `.hrtb` container, the optional signing/provenance flow, and the CLI are
experimental and not production-approved. The Go CLI remains the working
implementation; the C/C++ core is docs-first and code-later. See the tool README
for build, usage, security limits, and test commands.
