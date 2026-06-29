# Hegel for Go

## Build Commands

```bash
just build-libhegel     # Build libhegel.{so,dylib} in the sibling hegel-rust checkout
just test               # Run tests with coverage (fails if coverage < 100%)
just format             # Auto-format code
just lint               # Check formatting + linting
just docs               # Build API documentation
just check              # Run lint + check-docs + test (full CI check)
go test -run TestName ./...  # Run a single test
```

## What This Is

A Go implementation of the Hegel property-based testing library. Hegel is a
universal property-based testing protocol whose native engine ships as a Rust
shared library, `libhegel` (built from
[hegel-rust](https://github.com/hegeldev/hegel-rust)'s `hegel-c` crate).
hegel-go calls into libhegel directly via FFI using
[`github.com/ebitengine/purego`](https://github.com/ebitengine/purego).

## Architecture

The library is structured in layers, each building on the previous:

1. **FFI loader** (`internal/libhegel`) — purego-based dlopen of `libhegel.{so,dylib}`,
   one `*libhegel.Handle` struct with a `func`-typed field per C symbol, path
   resolution via the `HEGEL_LIBHEGEL_PATH` env var, otherwise the pinned release
   is auto-downloaded.
2. **Test runner** (`runner.go`) — `testCase` implements
   `TestCase` by routing every operation (generate, span, target, collection,
   mark_complete) to one libhegel C call.
3. **Generators** (`generators.go`, `primitives.go`, `collections.go`,
   `combinators.go`, `composite.go`) — type-safe generator abstraction, span
   system, collection protocol. Generators take a `TestCase` and produce
   typed values; they never touch libhegel directly.

A conformance suite that validates generator output against the protocol is
on the roadmap — the previous Python-server pytest harness was removed in
the libhegel transition and needs to be replaced with one that doesn't
depend on a server emitting per-test-case status.

## Public API

The user-facing surface lives in `hegel.go` (canonical package doc). Entry points:

- `hegel.Test(t, fn, opts...)` — runs a property test against a `*testing.T`
- `hegel.Run(fn, opts...) error` — runs a property test outside `*testing.T`
  (conformance binaries, standalone tools); returns an error
- `hegel.MustRun(fn, opts...)` — like Run, but panics on failure
- `hegel.Draw(ht, gen)` — draws a value inside a test body
- `hegel.T` — passed to the [Test] body; methods include `Note`, `Fatal`, `Fatalf`
- Generators: `Integers`, `Floats`, `Text`, `Booleans`, `Lists`, `Maps`, ...
- Options: `WithTestCases(n)`, `WithDatabase(path)`, ...

## Testing Philosophy

- **100% per-file code coverage** is mandatory. `just check` fails if any
  file is uncovered. Use `// coverage-ignore` only for genuinely untestable
  code (e.g. platform-specific branches, fixed-shape CBOR encode that can't
  fail). The annotation count is ratcheted in
  `.github/coverage-ratchet.json` and cannot increase without justification.
- **Function-pointer stubs** are the primary way to cover libhegel error
  paths.
- **Use the real libhegel** for integration tests.

## Locating libhegel

The library is resolved in this order; first hit wins:

1. `$HEGEL_LIBHEGEL_PATH` if set (loaded directly; no auto-download fallback if
   it fails to open)
2. Auto-download of the pinned release (`hegelVersion`) from hegel-rust's GitHub
   releases, verified against the baked-in checksum and cached per-version.

The library does **not** hunt for a sibling `hegel-rust` checkout. To test
against a local build, point `HEGEL_LIBHEGEL_PATH` at it — `just test` /
`just check` do this for you: `build-libhegel` (`cargo build --release -p
hegeltest-c` in the sibling `hegel-rust/` checkout) builds the `.so`, and the
recipe exports its path. Run `just check vendored` to bypass the local build and
exercise the auto-download path instead. `<ext>` is `so` on Linux, `dylib` on
macOS.

## Tooling Choices

- **Go version**: the oldest version supported by go.dev (1.N-1); CI tests 1.N and 1.N-1
- **FFI**: `github.com/ebitengine/purego` — runtime dlopen, no cgo
- **Test framework**: `testing` (Go stdlib) — run via `go test -race -coverprofile=coverage.out -covermode=atomic ./...`
- **Linter**: `go vet` (stdlib) + `staticcheck` v0.7.0 (2026.1) — run via `just lint`
- **Formatter**: `gofmt` (bundled with Go) — check with `gofmt -l .`, apply with `gofmt -w .`
- **Coverage tool**: [`go-test-coverage`](https://github.com/vladopajic/go-test-coverage) (via `go tool`) configured in `.testcoverage.yml` — enforces 100% per-file coverage with `// coverage-ignore` annotations for genuinely untestable code. A ratchet in `scripts/check-coverage.py` prevents annotation count from growing.
- **Documentation**: `go doc` (stdlib) — verifies all exported symbols have doc comments

## Project Conventions

- **Module path**: `hegel.dev/go/hegel`
- **Package name**: `hegel` — single package for the library, users import `hegel.dev/go/hegel`
- **File naming**: lowercase, multi-word files use underscores (e.g., `project_root.go`)
- **Test files**: `*_test.go` in the same package (white-box testing for coverage)
- **Exported symbols**: PascalCase per Go convention
- **Unexported symbols**: camelCase per Go convention
- **Error handling**: Unexported functions should return `error` for failable operations. Reserve `panic()` for two cases: (1) truly unreachable code paths (e.g. encoding a fixed-shape CBOR map), and (2) the boundary of an exported API where the caller's contract is misuse — e.g. `Map()` panics on construction with a malformed source generator, and `Draw()` panics to unwind the test body when the underlying `draw` returns a sentinel error. Internally, propagate errors with `fmt.Errorf("...: %w", err)` rather than re-panicking.
- **Doc comments**: Every exported symbol must have a doc comment starting with the symbol name
- **Coverage**: 100% enforced via `go-test-coverage` with `// coverage-ignore` for exclusions; annotation count ratcheted in `.github/coverage-ratchet.json`

## Developer Notes

### CBOR gotchas (fxamacker/cbor/v2)

- Positive integers decode as `uint64`, negative as `int64` when decoding to `any`. You MUST handle both in type switches.
- `float32` decodes as `float64` from CBOR wire format. The `float32` branch is only reachable if passed directly.
- libhegel uses `ciborium` on the Rust side. Both should round-trip cleanly; verify with new generators by exercising them in conformance.

### purego pitfalls

- Functions returning `const char*` should be typed `func() string` (not `*byte` or `uintptr`).
- C string arguments are passed as `string`. purego ensures that the memory is managed correctly.

### Coverage enforcement

- 100% coverage is mandatory. Enforced by `go-test-coverage` (configured in `.testcoverage.yml`).
- Use `// coverage-ignore` to exclude genuinely untestable code. Place it on the `if`/`switch`/`for` line to exclude the entire block, or on any line to exclude just that coverage region.
- A ratchet in `.github/coverage-ratchet.json` tracks the annotation count and prevents growth. The ratchet auto-tightens when annotations are removed.
- Use `-coverpkg=hegel.dev/go/hegel` to restrict coverage to the library package (excludes `cmd/` and `examples/`).

### Generator optimization

- `BasicGenerator.Map()` returns a new `*BasicGenerator` with the same schema and a composed transform — only one `generate` command regardless of chained `.Map()` calls.
- Non-basic generators wrapped in `Map()` produce `*MappedGenerator` with `start_span`/`stop_span` per generation.

### OneOf code paths

- All basic: a flat `{"type": "one_of", "generators": [...]}` schema. libhegel
  returns `[index, value]` and the synthesized parse fn dispatches to the
  matching per-branch parse using `index`. No tagged-tuple wrapping.
- Any non-basic: falls back to `oneOfGenerator.draw`, which generates an index
  via `tc.generate(schema)` (an integer in `[0, n-1]`) and recursively draws
  from the chosen branch under a span. The same shape applies to `Optional`,
  whose schema is a 2-branch one_of (null vs. inner value).
