# hrt-cold native core

**Status: Milestone 0 — contract and build system only. No native core
implementation is present yet.**

The working Tools implementation is still the Go CLI in
[`tools/hrt-cold`](../../tools/hrt-cold/README.md). This directory specifies the
planned C/C++ core, its ABI, build system, crypto backend decision, and test
plan. The C/C++ decision is intentional: C/C++ has the broadest library and
toolchain compatibility, while Rust was deferred to avoid adding a second
build ecosystem.

## What exists now

- `./configure` — a hand-written wrapper around CMake with the usual
  `--prefix`, `--build-dir`, `--build-type`, sanitizer, and crypto-backend
  options.
- `CMakeLists.txt` — configures an interface-only `hrtcold::native` target,
  installs the public header and package metadata, and builds C11/C++17 header
  compatibility checks.
- `include/hrtcold/hrtcold.h` — Milestone 0 C ABI vocabulary: version, status,
  limits, and fixed-size error detail. It declares no implemented symbols.
- `tests/native/header_c.c`, `tests/native/header_cpp.cpp` — compile/link/run
  checks for the header from both languages.

## What does not exist yet

No container parser, JSON validator, OpenPGP encryption/decryption, signature
verification, replay store, or file writer is implemented in C/C++. Do not link
against this target expecting those functions. The Go CLI remains the authority
until a native implementation passes the differential tests in
[`TEST_PLAN.md`](TEST_PLAN.md).

## Quick start

```sh
./configure --prefix=/usr/local --build-dir=build --enable-werror
cmake --build build --config Release
(cd build && ctest -C Release --output-on-failure)
cmake --install build
```

Milestone 0 builds only the header checks. `--enable-sanitizers` adds ASan and
UBSan on GCC/Clang; Windows/MSVC sanitizer builds are intentionally rejected.

## Document map

- [`DESIGN.md`](DESIGN.md) — scope, architecture, module boundaries, milestones,
  and security invariants.
- [`ABI.md`](ABI.md) — planned C entry points, ownership, threading, error
  model, and versioning rules.
- [`BUILD.md`](BUILD.md) — configure/CMake options, install layout, package
  consumers, and CI.
- [`CRYPTO.md`](CRYPTO.md) — OpenPGP backend requirements and candidate
  comparison.
- [`TEST_PLAN.md`](TEST_PLAN.md) — unit, differential, fuzz, sanitizer, and
  platform coverage.

The website ↔ Tools format boundary is documented separately in
[`../backup-schema-audit/TOOLS_INTERFACE.md`](../backup-schema-audit/TOOLS_INTERFACE.md).
