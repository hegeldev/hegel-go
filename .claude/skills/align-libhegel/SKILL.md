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

## The context-based ABI

Every fallible libhegel call follows one convention, and the whole wrapper is
shaped around it:

- The **first argument** is a `hegel_context_t` (an error-reporting context).
- The **return value** is an `Error` result code (`OK` is 0; failures are
  negative — `E_INVALID_HANDLE`, `E_INVALID_ARG`, …).
- Any **value the call produces** (a handle, a count, a bool, a byte buffer) is
  written through a **trailing out-parameter**, not returned.
- On a non-`OK` return, the human-readable message is read back from the context
  via `hegel_context_last_error`.

So `symbols` (the loaded library) is immutable and shared across goroutines,
while the per-call error state lives on a `*Context`. The wrapper threads a
`*Context` through every method and funnels every call through two helpers:

- `ctx.invoke(op, fn)` — runs `fn(ctxT)`, and if it returns non-`OK`, reads
  `hegel_context_last_error` and wraps it into a Go `error`.
- `allocate(ctx, op, new, free)` — `invoke` for handle constructors: maps a
  zero out-handle to a `nil` result (the engine's "no object" sentinel, e.g. a
  finished run) and registers a GC cleanup that calls `free`.

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
# e.g. ~/.cache/hegel-go/libhegel/0.23.0/libhegel-linux-amd64.so
```

**When you need the symbol table**, run `nm -D` against that cached `.so` — no
build required. Resolve the path with a glob so you don't have to spell out the
version:

```bash
LIB=$(ls "${XDG_CACHE_HOME:-$HOME/.cache}"/hegel-go/libhegel/*/libhegel-linux-amd64.so | tail -1)
nm -D "$LIB" | grep '^.* T hegel_' | sort
```

The `nm` output is ground truth for which `hegel_*` symbols exist — diff it
against the registration table in `tryOpen` (`libhegel.go`). A symbol the wrapper
registers but the lib no longer exports makes `registerSymbols` fail and
**every** integration test dies at load.

## 3. Compare hegel.h against `internal/libhegel/libhegel.go`

Walk every C declaration and check it against the wrapper. Categorize each:

- **Removed symbol** → delete its `symbols` field, its registration-table entry,
  its `stub.go` closure, and any wrapper method. Re-route callers.
- **Renamed/retyped symbol** (e.g. `hegel_run_result_passed`, a `bool`, became
  `hegel_run_result_status`, a `hegel_run_status_t` written through an out-param)
  → update the field signature, the registration name, the wrapper method, and
  the `stub.go` closure's signature/out-param handling.
- **New symbol** → add a `symbols` field, a registration entry, a wrapper method,
  a `stub.go` closure, and a test (see §5).
- **Changed signature** (a new arg, or a value that moved from return to
  out-param) → remember the ABI convention: ctx first, `Error` return,
  produced value through a trailing out-pointer. The field type, wrapper, and
  stub closure all change together.
- **Changed enum/constant values** (e.g. a new `HEGEL_LABEL_*`, or a reordered
  `hegel_status_t`) → update the Go `const` block. Binding-invented labels that
  aren't in upstream (`LABEL_COMPOSITE`, `LABEL_STATEFUL`) must be renumbered to
  sit *past* the last upstream value so they can't collide.

## 4. Editing the binding — the four things that move together

These must stay consistent or the build/coverage breaks:

1. **`libhegel.go` — `symbols` struct**: one `func`-typed field per C symbol.
   Mirror the C signature exactly: `func(ctxT, <args>, <*outParam>) Error` for a
   fallible call. The two exceptions are the C functions that return a value
   directly rather than through a result code — only `hegel_context_last_error`
   (`func(ctxT) string`) currently does this.
2. **`libhegel.go` — `registerSymbols` table** in `tryOpen`: `{"hegel_x", &syms.x}`.
3. **`stub.go` — the positional struct literal**: `Stub(returns ...any)` builds a
   `symbols` with a **positional** composite literal (its first field is
   `handle dlhandle`, set to `0`) and returns a `*Context`. Field order in the
   literal MUST match the `symbols` struct field order exactly — adding,
   removing, or reordering a field means editing the literal in lockstep. Each
   closure mirrors the new C signature: it takes `ctxT` + args + out-pointers,
   writes any produced value into the out-param, and returns an `Error`. Values
   are supplied by the caller via `returns ...any` and popped in strict call
   order through the closures' helpers:
     - `retval()` — pop the next value (an `Error` for a plain fallible op, or a
       typed scalar like `RunStatus` / `uint64` to write into a `*out`).
     - `retHandle()` — for handle constructors: pop either an `Error` (the call
       fails) or a `uintptr` (the produced handle; `0` = NULL/"no object").
     - `writeStr(out **byte)` — pop a Go string and write it into a `const char**`
       out-param as a NUL-terminated buffer.
4. **`libhegel.go` — wrapper method**: a thin, typed Go method that threads a
   `*Context`. Handle constructors live on `Context` (e.g. `SettingsNew`) or on
   the parent handle (e.g. `Settings.RunStart`, `Run.NextTestCase`) and go
   through `allocate(ctx, op, new, free)`. Other fallible calls go through
   `ctx.invoke(op, fn)`. Reader methods (`Result.Status`, `Result.FailureCount`,
   `Failure.Origin`, …) use the reusable `out*` scratch fields on the wrapper
   struct (`TestCase`, `Result`, `Failure`) so the hot per-draw path doesn't
   allocate a fresh out-param on every call.

### Regenerate the stringer output

Enum types are listed in the `//go:generate` directive at the top of
`libhegel.go` (currently
`-type=Error,Status,Mode,Backend,Verbosity,RunStatus,HealthCheck,Phase,Label`).
After adding an enum type or changing any enum constant value:

```bash
go generate ./internal/libhegel        # uses `go tool stringer`
```

Changed constant values break the compile-time `_ = x[CONST-N]` assertions in
`libhegel_string.go` until you regenerate, so this is not optional. Add new enum
types to the `-type=` list. The generated `_string.go` is coverage-excluded.

## 5. Coverage and tests

100% per-file coverage is enforced (see the `coverage` skill). New or changed
wrapper methods need coverage:

- Methods the runner exercises (result/status handling, generate, spans,
  mark_complete) are covered by the stub-driven tests in `runner_test.go` —
  update those `Stub(...)` return lists when a symbol's output type or call
  order changes (e.g. a `bool` "passed" value becoming a `RunStatus`).
- Methods **not** wired into the runner (pools, state machines, blob replay,
  primitive draws, the unwired settings setters) are covered by direct stub
  tests in `internal/libhegel/stub_test.go`. Build a `*Context` with
  `lib := Stub(...)`, construct the wrapper value in-package against its symbol
  table —
  `tc := &TestCase{pointer: &pointer[testCaseT]{syms: lib.syms, raw: 1}}` or
  `s := &Settings{syms: lib.syms, raw: 1}` — and call the method, passing `lib`
  as the `*Context`. Cover both the happy path and every error branch (a guard
  that rejects bad input, like `cStringArray`'s interior-NUL check, needs a test
  that trips it).
- Prefer covering a method to annotating it `// coverage-ignore`. The ratchet in
  `.github/coverage-ratchet.json` auto-tightens when the annotation count drops,
  so removing now-covered ignores is free and good.

## 6. purego pitfalls

- A symbol that **returns** `const char *` directly → type the field
  `func(ctxT) string` (not `*byte`/`uintptr`). Only `hegel_context_last_error`
  does this today.
- A symbol that **writes** a string through a `const char **` out-param → type
  that argument `**byte` and read it back with `goString`, which copies the
  NUL-terminated buffer immediately (a NULL pointer — libhegel's "no string" —
  yields `""`). This is how `hegel_version`, `hegel_run_result_error`, and the
  failure getters return strings.
- Produced **handles and scalars** come back through trailing out-params
  (`*settingsT`, `*runT`, `*uint64`, `*bool`, `*int64`, `*Collection`,
  `*RunStatus`), never as the return value — the return is always the `Error`
  code.
- `const char *` **argument** → pass a Go `string`; purego manages the memory.
- `const char *const *` (array of C strings) is **not** handled automatically.
  `cStringArray` builds a `[]*byte` of NUL-terminated buffers; pass
  `slicePtr(arr)` plus the length. The buffers stay alive through the call via
  the `*byte` pointers in the array (no `runtime.KeepAlive` needed for a typed
  `**byte` argument). A Go string may contain an interior NUL but a C string
  can't — `cStringArray` rejects such input with an error rather than letting C
  silently truncate it.

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
