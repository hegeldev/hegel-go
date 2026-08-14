# AGENTS.md

## Repository overview

`hegel-go` is the Go client for the Hegel property-based testing protocol. The
public module is `hegel.dev/go/hegel`; the native engine is loaded through
`internal/libhegel` using `purego`.

## Environment

- Go module minimum: Go 1.25 (`go.mod`).
- Devbox Go: `/usr/local/go` from `go.dev/dl`.
- User-installed Go tools use `GOBIN=$HOME/.local/bin`; that directory is on
  `PATH`.
- The checkout lives at `$WORKSPACE/hegel-go` (`$WORKSPACE=$HOME/workspace`).
- Run `git lfs pull` after cloning. Vendored native libraries under
  `internal/libhegel/libs/` must contain real LFS objects, not pointer files.
- `just test` uses a sibling `$WORKSPACE/hegel-rust` build when present and
  otherwise falls back to the vendored `libhegel` binary.

## Build, run, and test

```bash
# Compile all packages.
go build ./...

# Run the full local/CI-equivalent check: formatting, vet/staticcheck, docs,
# race-enabled tests, coverage threshold, and coverage ratchet.
just check

# Test against the checked-in native library explicitly.
just check vendored

# Run tests only (builds sibling hegel-rust first when available).
just test
just test vendored

# Run a focused test.
go test -run '^TestName$' ./...

# Format and lint.
just format
just lint

# Serve package documentation locally (default port 6060).
just docs
```

This repository is a library and has no main application to run. Use tests for
normal development or `just docs` to inspect the API.

## Project conventions

- Keep the user-facing API in package `hegel`; internal FFI details belong in
  `internal/libhegel`.
- Use lowercase filenames with underscores for multiple words. Tests are
  white-box `*_test.go` files in the same package.
- Use PascalCase for exported symbols and camelCase for unexported symbols.
- Every exported symbol needs a doc comment beginning with its name.
- Return and wrap internal errors with `fmt.Errorf("context: %w", err)`.
  Reserve panics for unreachable states or documented exported-API misuse.
- Maintain 100% per-file coverage. Use `// coverage-ignore` only for genuinely
  untestable paths; `.github/coverage-ratchet.json` prevents annotation growth.
- Cover libhegel failures with function-pointer stubs and use the real native
  library for integration behavior.
- For CBOR decoded into `any`, handle positive integers as `uint64`, negative
  integers as `int64`, and wire-format `float32` values as `float64`.
- For `purego`, model `const char *` returns and C string arguments as Go
  `string` values.

## Before submitting changes

1. Run `gofmt` (normally `just format`).
2. Add or update tests for every behavior change.
3. Run `just check vendored`; run `just check` as well when a sibling
   `hegel-rust` checkout is available.
4. Confirm `git status` contains no generated coverage files or accidental
   native-library changes.
5. Use focused commits and never bypass Git LFS for files under
   `internal/libhegel/libs/`.
