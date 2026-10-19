# Building the native scaffold

## 1. Requirements

Milestone 0:

- CMake >= 3.16
- A C11 compiler and a C++17 compiler
- A POSIX shell for `./configure` (Windows: use Git Bash, or call CMake
  directly)
- `ctest` for the header checks

Future milestones add the selected OpenPGP backend (see
[`CRYPTO.md`](CRYPTO.md)). Do not add a crypto dependency to the M0 build; M0
must stay dependency-free so the ABI and packaging can be reviewed alone.

## 2. Configure

```sh
./configure \
  --prefix=/usr/local \
  --build-dir=build \
  --build-type=Release \
  --enable-werror
```

Options:

| Option | Meaning |
| --- | --- |
| `--prefix=PATH` | install prefix, default `/usr/local` |
| `--build-dir=DIR` | build directory; relative paths are resolved against the tree |
| `--build-type=TYPE` | `Release`, `RelWithDebInfo`, `Debug`, `MinSizeRel` |
| `--enable-sanitizers` | ASan + UBSan; GCC/Clang only, rejected on MSVC |
| `--disable-sanitizers` | explicit default |
| `--enable-werror` | warnings as errors |
| `--with-crypto=BACKEND` | planned backend: `rnp`, `openssl`, `botan`, `libgcrypt`, `none` |
| `-h`, `--help` | usage |

`configure` is a small hand-written wrapper around CMake, not Autoconf. It
checks the backend name and build type, then runs:

```sh
cmake -S . -B build \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_INSTALL_PREFIX=/usr/local \
  -DHRTCOLD_CRYPTO_BACKEND=rnp
```

`CC`, `CXX`, and `CMAKE` environment variables override the defaults.

## 3. Build, test, install

```sh
cmake --build build --config Release
(cd build && ctest -C Release --output-on-failure)
cmake --install build
```

Milestone 0 builds:

- `hrtcold_header_c` — compiles, links, and runs a C11 program that includes
  `<hrtcold/hrtcold.h>`.
- `hrtcold_header_cpp` — the same from C++17.

No library binary is produced. `hrtcold::native` is an interface target that
exports the include directory.

## 4. Install layout

```text
${prefix}/include/hrtcold/hrtcold.h
${prefix}/<libdir>/cmake/hrtcold/hrtcoldConfig.cmake
${prefix}/<libdir>/cmake/hrtcold/hrtcoldConfigVersion.cmake
${prefix}/<libdir>/cmake/hrtcold/hrtcoldTargets.cmake
${prefix}/<libdir>/pkgconfig/hrtcold.pc
```

`<libdir>` comes from CMake's `GNUInstallDirs` (`lib`, `lib64`, or a multiarch
directory on Linux; `lib` on Windows). The CMake package config is
relocatable. The pkg-config file records the prefix chosen by
`./configure --prefix`; if you install with a different `cmake --install
--prefix`, regenerate or adjust the `.pc` file.

CMake consumers:

```cmake
find_package(hrtcold 0.1 CONFIG REQUIRED)
target_link_libraries(my_tool PRIVATE hrtcold::native)
```

pkg-config consumers:

```sh
pkg-config --cflags hrtcold
```

The `.pc` file currently has an empty `Libs:` line because M0 exports no
binary. It will gain the real library flags with the implementation.

## 5. Direct CMake use

`configure` is optional:

```sh
cmake -S . -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build
(cd build && ctest --output-on-failure)
```

Useful cache options:

- `-DHRTCOLD_BUILD_HEADER_CHECKS=OFF`
- `-DHRTCOLD_ENABLE_SANITIZERS=ON`
- `-DHRTCOLD_ENABLE_WERROR=ON`
- `-DHRTCOLD_CRYPTO_BACKEND=rnp`

## 6. Windows

From Git Bash:

```sh
bash configure --build-dir=build --enable-werror
cmake --build build --config Release
(cd build && ctest -C Release --output-on-failure)
```

CMake selects a Visual Studio or Ninja generator as available. MSVC uses
`/W4` and `/WX`; sanitizers are not offered on that toolchain. The same C and
C++ header checks must pass natively on Windows in CI.

## 7. Sanitizers and fuzzing

M0 supports ASan/UBSan on GCC/Clang:

```sh
./configure --build-dir=build-asan --build-type=Debug --enable-sanitizers
cmake --build build-asan
(cd build-asan && ctest --output-on-failure)
```

Fuzzing harnesses are planned for M1/M2 (see [`TEST_PLAN.md`](TEST_PLAN.md)).
Do not enable sanitizers in release packaging.

## 8. CI

`.github/workflows/tools-hrt-cold.yml` runs the native configure/build/test
matrix on Ubuntu and Windows alongside the existing Go tests. The Linux job
enables sanitizers and warnings-as-errors; Windows uses the native compiler
without sanitizers. The workflow triggers on changes to `configure`,
`CMakeLists.txt`, `cmake/`, `include/`, `src/native/`, `tests/native/`, and
`docs/native/`.

## 9. Future crypto dependency

M2 will add the chosen OpenPGP backend. Keep it behind CMake options and do not
make the M0 header checks depend on it. Before adding it, record:

- exact version and checksum/lockfile policy,
- licence and notice updates,
- Windows build instructions,
- known-answer and negative test evidence,
- whether static or dynamic linking is the default.

See [`CRYPTO.md`](CRYPTO.md) for the candidate decision.
