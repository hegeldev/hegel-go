# Hegel for Go

## Build Commands

```bash
just build-libhegel                              # Build the sibling hegel-rust library, if present
just test vendored                               # Run tests with 100% coverage enforcement
just format                                      # Auto-format Go code
just lint                                        # Check formatting, go fix, vet, and staticcheck
just check-docs                                  # Run go doc over the package
just docs                                        # Serve API docs locally; open the printed URL
just check vendored                              # Run lint, check-docs, and tests
go test -run '^TestLoadLibVersion$' ./internal/libhegel  # Run one named test
```

Without `vendored`, `just test` and `just check` build the sibling hegel-rust
checkout when present. They can fail if its C API differs from the pinned
libhegel version. Use the vendored commands above for a reproducible check;
drop `vendored` when iterating on a combined hegel-rust and hegel-go change,
so tests use the freshly built sibling Rust library. `just docs` keeps running
until stopped. A plain `go test` uses the vendored library unless
`HEGEL_LIBHEGEL_PATH` is set.

## What This Is

A Go implementation of the Hegel property-based testing library. Hegel is a
universal property-based testing protocol whose native engine ships as a Rust
shared library, `libhegel` (built from
[hegel-rust](https://github.com/hegeldev/hegel-rust)'s `hegel-c` crate).

## Architecture

The library is structured in layers, each building on the previous:

1. **FFI loader** (`internal/libhegel`): uses dlopen or similar to load libhegel
   at runtime. Contains a low-level API which is not necessarily idiomatic but
   very close to the exposed C API. All GC integration is handled here.
2. **Test runner**: `testCase` implements
   `TestCase` by routing every operation (generate, span, target, collection,
   mark_complete) to one libhegel C call.
3. **Generator[T]**: type-safe generator abstraction. Generators take a `TestCase`
   and produce typed values.

## Testing Philosophy

- **100% per-file code coverage** is mandatory. `just check` fails if any
  file is uncovered. Use `// coverage-ignore` only for genuinely untestable
  code. The annotation count is ratcheted in
  `.github/coverage-ratchet.json` and cannot increase without justification.
- **Function-pointer stubs** are the primary way to cover libhegel error
  paths.
- **Use the real libhegel** for integration tests.

### Mutation testing

When adding or changing tests, run the normal Go tests and mutation test the
code they exercise with gomutants. Run:

```sh
go tool gomutants --changed-since origin/main ./...
```

The `--changed-since` flag limits mutations to changed lines. Use the
appropriate base ref if the branch does not target `main`.

Before adding a test for a surviving mutant, state the intended guarantee it
threatens and where that guarantee comes from, such as the protocol, documented
behavior, or an agreed security assumption. An internal invariant is worth
testing when it protects that guarantee. Assert the invariant at a stable
boundary; do not turn incidental implementation choices into requirements
merely to improve the mutation score. Explain survivors that do not reveal a
meaningful test gap.

Report the mutation command, survivors and explanations, and any run you could
not finish. Keep settings at their defaults; tune workers or timeouts only if a
run has resource or timeout problems. Start mutation testing early when a run may
take a while, and let it run in the background while doing independent review work.

## Project Conventions

- **Module path**: `hegel.dev/go/hegel`
- **Package name**: `hegel` — single package for the library, users import `hegel.dev/go/hegel`
- **File naming**: lowercase, multi-word files use underscores (e.g., `project_root.go`)
- **Test files**: `*_test.go` in the same package (white-box testing for coverage)
- **Error handling**: Unexported functions should return `error` for failable operations. Reserve `panic()` for two cases: (1) truly unreachable code paths (e.g. encoding a fixed-shape CBOR map), and (2) the boundary of an exported API where the caller's contract is misuse — e.g. `Map()` panics on construction with a malformed source generator, and `Draw()` panics to unwind the test body when the underlying `draw` returns a sentinel error. Internally, propagate errors with `fmt.Errorf("...: %w", err)` rather than re-panicking.
- **Doc comments**: Every exported symbol must have a doc comment, usually starting with the symbol name.
  The first paragraph must consist of a single short sentence. Functions should have two more paragraphs: one describing inputs if necessary (for example sentinel values) and one describing outputs (including important sentinel errors).
- **Coverage**: 100% enforced via `go-test-coverage` with `// coverage-ignore` for exclusions; annotation count ratcheted in `.github/coverage-ratchet.json`

## Developer Notes

### purego pitfalls

- Functions returning `const char*` should be typed `func() string` (not `*byte` or `uintptr`).
- C string arguments are passed as `string`. purego ensures that the memory is managed correctly.

## Restating libhegel invariants

Do not duplicate input validation which is done by hegel-rust. Only perform validation
if it is necessary to make libhegel calls safe, for example when converting int to uint64.
