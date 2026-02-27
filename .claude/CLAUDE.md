# Hegel SDK for go

## Build Commands

```bash
just setup              # Install dependencies and hegel binary
just test               # Run tests with coverage (fails if coverage < 100%)
just format             # Auto-format code
just lint               # Check formatting + linting
just docs               # Build API documentation
just check              # Run lint + docs + test (full CI check)
just build-conformance  # Compile conformance binaries to bin/conformance/
just conformance        # Build conformance binaries + run Python conformance test suite
```

Tests must use `PATH="$(pwd)/.venv/bin:$PATH"` (absolute path) so the `hegel` binary is found.

## What This Is

A go implementation of the Hegel property-based testing SDK. Hegel is a
universal property-based testing framework powered by Hypothesis on the backend.
SDKs communicate with the `hegel` binary (a Python server) via Unix sockets using
a custom binary protocol.

## SDK Architecture

The SDK is structured in layers, each building on the previous:

1. **Protocol Layer** — Binary wire protocol with 20-byte header, CBOR payload, CRC32
2. **Connection & Channels** — Unix socket multiplexing with demand-driven reader
3. **Test Runner** — Spawns `hegel` subprocess, manages test lifecycle
4. **Generators** — Type-safe generator abstraction, span system, collection protocol
5. **Conformance** — Test binaries that validate SDK correctness against the framework

### Key Pattern: Demand-Driven Reader

The Connection uses a demand-driven model: when a Channel needs a message, it
acquires a reader lock and reads packets from the socket until its inbox has data.
No background threads — reading is triggered by the consumer that needs data.

### Key Pattern: Thread-Local Channel State

The current data channel is stored in thread-local (or context-var) state so that
generator functions (`generate()`, `assume()`, `note()`, `target()`) don't need a
channel parameter. The test runner sets the current channel before calling the test
body.

### Key Pattern: Global Lazy Session

The `hegel` subprocess is managed by a global session that starts lazily on first
use and shuts down automatically on process exit. Users never construct connections
or sessions manually — `run_hegel_test()` is a plain free function.

## Testing Philosophy

- **100% code coverage** is mandatory. `just check` fails if any line is uncovered.
  Use `HEGEL_PROTOCOL_TEST_MODE` (see below) to cover error paths — do NOT use `# nocov`.
- **Use the real `hegel` binary** for integration tests. Never write a mock server.
  The real binary runs as a subprocess, so there is zero threading contention.
  In-process mocks with threads cause deadlocks — they have wasted hundreds of
  agent turns in previous SDK generations.
- **Socket pairs** (`socketpair()`) for unit testing Connection/Channel in isolation.

### HEGEL_PROTOCOL_TEST_MODE — Error Injection

Set the `HEGEL_PROTOCOL_TEST_MODE` environment variable before calling `RunHegelTestE` to
trigger server-side error injection:

| Mode                          | What it does                                      |
|-------------------------------|---------------------------------------------------|
| `stop_test_on_generate`       | StopTest on 1st generate of 2nd test case         |
| `stop_test_on_mark_complete`  | StopTest in response to mark_complete             |
| `stop_test_on_collection_more`| StopTest during collection_more                   |
| `stop_test_on_new_collection` | StopTest during new_collection                    |
| `error_response`              | RequestError on first generate                    |
| `empty_test`                  | test_done immediately, no test cases run          |

## Critical: StopTest Handling

When the server sends StopTest, the SDK MUST:
1. Raise a language-specific exception (DataExhausted/StopTest) to unwind the test body
2. NOT send `mark_complete` after receiving StopTest
3. Track a per-test-case `test_aborted` flag to suppress further commands

Failing to handle StopTest correctly causes `FlakyStrategyDefinition` errors.

## Wire Protocol

- **Header**: 5 big-endian uint32: `magic(0x4845474C)`, `CRC32`, `channel_id`,
  `message_id`, `payload_length`
- **Payload**: CBOR-encoded bytes
- **Terminator**: single byte `0x0A`
- **Reply bit**: `message_id | (1 << 31)` marks a message as a reply
- **Client channel IDs**: odd — allocated as `(counter << 1) | 1`
- **CRC32**: computed over the full 20-byte header (checksum field zeroed) + payload

## Tooling Choices

- **Go version**: 1.23.x (installed via `actions/setup-go@v5` in CI)
- **Test framework**: `testing` (Go stdlib) — run via `go test -race -coverprofile=coverage.out -covermode=atomic ./...`
- **Linter**: `go vet` (stdlib) + `staticcheck` v0.7.0 (2026.1) — run via `just lint`
- **Formatter**: `gofmt` (bundled with Go) — check with `gofmt -l .`, apply with `gofmt -w .`
- **Coverage tool**: `go test -coverprofile` + `scripts/check-coverage.py` — custom Python script that parses coverage profiles, filters false positives, and fails if any real line is uncovered
- **Documentation**: `go doc` (stdlib) — verifies all exported symbols have doc comments

## Project Conventions

- **Module path**: `github.com/antithesishq/hegel-go`
- **Package name**: `hegel` — single package for the library, users import `github.com/antithesishq/hegel-go`
- **File naming**: lowercase, multi-word files use underscores (e.g., `test_runner.go`)
- **Test files**: `*_test.go` in the same package (white-box testing for coverage)
- **Exported symbols**: PascalCase per Go convention
- **Unexported symbols**: camelCase per Go convention
- **Error handling**: Return `error` for failable operations; `panic()` for truly unreachable code paths
- **Doc comments**: Every exported symbol must have a doc comment starting with the symbol name
- **Coverage**: 100% enforced — `scripts/check-coverage.py` runs after tests; false positives (closing braces, unreachable panics) are filtered automatically
- **Test execution**: Tests use `PATH="$(pwd)/.venv/bin:$PATH"` (absolute path) to find the `hegel` binary — relative paths don't work with `exec.LookPath`

## Developer Notes

### CBOR gotchas (fxamacker/cbor/v2)

- Positive integers decode as `uint64`, negative as `int64` when decoding to `any`. You MUST handle both in type switches.
- `float32` decodes as `float64` from CBOR wire format. The `float32` branch is only reachable if passed directly.

### net.Pipe() in tests

- `net.Pipe()` is synchronous (unbuffered) — always write in a goroutine to avoid deadlocks.
- Returns `io.ErrClosedPipe` (not `io.EOF`) when the other end closes.

### Coverage enforcement

- 100% coverage is mandatory. `scripts/check-coverage.py` filters false positives automatically.
- Use `panic("hegel: unreachable: ...")` for truly unreachable code paths — the false-positive filter recognizes the "unreachable" keyword.
- The `if err != nil {` line must be immediately followed by the `panic(` line (no comments between them) for the filter to detect it.
- Use `-coverpkg=github.com/antithesishq/hegel-go` to restrict coverage to the library package (excludes `cmd/` and `examples/`).

### Test isolation with HEGEL_PROTOCOL_TEST_MODE

- Test-mode hegel handles exactly ONE `run_test` then exits. `RunHegelTestE` creates a fresh temporary session when this env var is set.
- Test-mode sessions suppress stderr to avoid Python tracebacks in test output.

### Protocol field names

- The `run_test` command and server responses use `"channel_id"` for the test channel ID.
- Always cross-check field names against the published hegel-core server.

### Generator optimization

- `BasicGenerator.Map()` returns a new `*BasicGenerator` with the same schema and a composed transform — only one `generate` command regardless of chained `.Map()` calls.
- Non-basic generators wrapped in `Map()` produce `*MappedGenerator` with `start_span`/`stop_span` per generation.

### OneOf code paths

- Path 1 (all basic, all identity): simple `{"one_of": [...]}` schema.
- Path 2 (all basic, some transforms): tagged-tuple schema with dispatch.
- Path 3 (any non-basic): `*CompositeOneOfGenerator` with span wrapping.
