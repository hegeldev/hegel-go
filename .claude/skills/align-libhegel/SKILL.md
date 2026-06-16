---
name: align-libhegel
description: "How to align the Go FFI wrapper to a new libhegel (hegel-c) release. Use after bumping the pinned libhegel version (internal/libhegel/checksums.go), when `just check` fails with a symbol-resolution or version-mismatch error against libhegel, or whenever the hegel-c C API in hegel.h has changed and the Go bindings need to catch up."
---

# Aligning the Go wrapper to a new libhegel release

`libhegel` is a Rust cdylib built from hegel-rust's `hegel-c` crate. hegel-go
calls it via purego FFI. When the pinned version changes, the C API in
`hegel-c/include/hegel.h` may have added, removed, renamed, or re-typed symbols,
and the low-level wrapper in `internal/libhegel/` must be re-aligned. The main
`hegel` package's exported API must **not** change as a result — only the
internal binding layer.

## 1. Find the pinned version and fetch the matching header

The version lives in `internal/libhegel/checksums.go` as `hegelVersion`.

The release **tag is `v<VERSION>`** (note the `v` prefix — the raw path without
it 404s):

```bash
curl -sSL https://raw.githubusercontent.com/hegeldev/hegel-rust/v<VERSION>/hegel-c/include/hegel.h
```

## 2. Get the matching library — always `just check vendored`

`just check` runs integration tests (`TestLoadLibVersion`, `TestLibhegelEndToEnd`)
against the **real** library, and `TestLoadLibVersion` asserts the loaded lib
reports exactly `hegelVersion`. You need a libhegel at that exact version, and
the simplest way to get it is to let the auto-downloader fetch it:

```bash
just check vendored
```

**Always use `vendored` mode for this work — never a local build.** The
`vendored` mode leaves `HEGEL_LIBHEGEL_PATH` unset, so the loader fetches the
pinned, checksum-verified release for `hegelVersion` and caches it. A plain
`just check` would instead build whatever the sibling `../hegel-rust` checkout
happens to be on (likely an *older* tag, since you've just bumped the version),
point `HEGEL_LIBHEGEL_PATH` at that stale `.so`, and fail the version test. Don't
touch the checkout; the download is always the right version.

The cached library lands at:

```bash
${XDG_CACHE_HOME:-$HOME/.cache}/hegel-go/libhegel/<VERSION>/libhegel-linux-amd64.so
# e.g. ~/.cache/hegel-go/libhegel/0.19.1/libhegel-linux-amd64.so
```

**When you need the symbol table**, run `nm -D` against that cached `.so` — no
build required. Resolve the path with a glob so you don't have to spell out the
version:

```bash
LIB=$(ls "${XDG_CACHE_HOME:-$HOME/.cache}"/hegel-go/libhegel/*/libhegel-linux-amd64.so | tail -1)
nm -D "$LIB" | grep '^.* T hegel_' | sort
```

The `nm` output is ground truth for which `hegel_*` symbols exist — diff it
against the registration table in `libhegel.go`. A symbol the wrapper registers
but the lib no longer exports makes `registerSymbols` fail and **every**
integration test dies at load.

## 3. Compare hegel.h against `internal/libhegel/libhegel.go`

Walk every C declaration and check it against the wrapper. Categorize each:

- **Removed symbol** → delete its `Handle` field, its registration-table entry,
  its `stub.go` closure, and any wrapper method. Re-route callers.
- **Renamed/retyped symbol** (e.g. `hegel_run_result_passed` → `hegel_run_result_status`)
  → update field type, registration name, wrapper method, and the `stub.go`
  closure's signature/return type.
- **New symbol** → add a `Handle` field, a registration entry, a wrapper method,
  a `stub.go` closure, and a test (see §5).
- **Changed enum/constant values** (e.g. a new `HEGEL_LABEL_*`) → update the Go
  `const` block. Binding-invented labels that aren't in upstream must be
  renumbered to sit *past* the last upstream value so they can't collide.

## 4. Editing the binding — the four files that move together

These must stay consistent or the build/coverage breaks:

1. **`libhegel.go` — `Handle` struct**: one `func`-typed field per C symbol.
2. **`libhegel.go` — `registerSymbols` table** in `tryOpen`: `{"hegel_x", &lib.x}`.
3. **`stub.go` — the positional struct literal**: `Stub()` builds a `Handle`
   with a **positional** composite literal. Field order in the literal MUST
   match the `Handle` struct field order exactly. Adding/removing/reordering a
   field means editing the literal in lockstep. Each closure mirrors the C
   signature and either returns nothing (void setters) or pops one value via
   `retval()` (matching the dynamic type the wrapper expects: `Error`,
   `uintptr` for handles, `bool`, `string`, an enum like `RunStatus`, …).
4. **`libhegel.go` — wrapper method**: thin, typed Go method on `Settings` /
   `Run` / `TestCase` / `Result` / `Failure`. Failable calls return `error` via
   `toError(op, ...)`; handle-returning constructors go through `wrap(...)`.

### Regenerate the stringer output

Enum types are listed in the `//go:generate` directive at the top of
`libhegel.go`. After adding an enum type or changing any enum constant value:

```bash
go generate ./internal/libhegel        # uses `go tool stringer`
```

Changed constant values break the compile-time `_ = x[CONST-N]` assertions in
`libhegel_string.go` until you regenerate, so this is not optional. Add new enum
types to the `-type=` list. The generated `_string.go` is coverage-excluded.

## 5. Coverage and tests

100% per-file coverage is enforced (see the `coverage` skill). New wrapper
methods need coverage:

- Methods the runner exercises (e.g. result/status handling) are covered by the
  stub-driven tests in `runner_test.go` — update those stubs' return lists
  when a symbol's type changes (e.g. a `bool` "passed" value becomes a
  `libhegel.RunStatus`).
- Methods **not** wired into the runner (pools, state machines, blob replay,
  primitive draws) are covered by direct stub tests in
  `internal/libhegel/stub_test.go`. Construct wrapper values in-package
  (`&TestCase{lib: Stub(...), ptr: 1}`) and call the method. Cover both the
  happy path and every error branch (a guard that rejects bad input needs a
  test that trips it).
- Prefer covering a method to annotating it `// coverage-ignore`. The ratchet in
  `.github/coverage-ratchet.json` auto-tightens when the annotation count drops,
  so removing now-covered ignores is free and good.

## 6. purego pitfalls

- `const char *` return → type the field `func() string` (not `*byte`/`uintptr`).
- `const char *` argument → pass a Go `string`; purego manages the memory.
- `const char *const *` (array of C strings) is **not** handled automatically.
  Build a `[]*byte` of NUL-terminated buffers and pass `slicePtr(arr)` plus the
  length; the buffers stay alive through the call via the `*byte` pointers in
  the array (no `runtime.KeepAlive` needed for a typed `**byte` argument). A Go
  string may contain an interior NUL but a C string can't — reject such input
  with an error rather than letting C silently truncate it.

## 7. Verify

```bash
just check vendored    # lint + check-docs + test against the pinned release
```

Use `vendored` mode here too (see §2) — it guarantees the tests run against the
pinned release, not a stale local build.

Confirm the version smoke test actually ran against the real lib (not skipped):

```bash
go test -run 'TestLoadLibVersion|TestLibhegelEndToEnd' -v ./internal/libhegel/
```
