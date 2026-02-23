# Hegel SDK for go

## Build Commands

```bash
just setup   # Install dependencies and hegel binary
just test    # Run tests with coverage (fails if coverage < 100%)
just format  # Auto-format code
just lint    # Check formatting + linting
just docs    # Build API documentation
just check   # Run lint + docs + test (full CI check)
```

Tests must use `PATH=".venv/bin:$PATH"` so the `hegel` binary is found.

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
  Use `HEGEL_TEST_MODE` (see below) to cover error paths — do NOT use `# nocov`.
- **Use the real `hegel` binary** for integration tests. Never write a mock server.
  The real binary runs as a subprocess, so there is zero threading contention.
  In-process mocks with threads cause deadlocks — they have wasted hundreds of
  agent turns in previous SDK generations.
- **Socket pairs** (`socketpair()`) for unit testing Connection/Channel in isolation.

### HEGEL_TEST_MODE — Error Injection

Set the `HEGEL_TEST_MODE` environment variable before calling `run_hegel_test` to
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
- **Test execution**: Tests use `PATH=".venv/bin:$PATH"` to find the `hegel` binary

## Lessons Learned

*(Updated by each stage as knowledge accumulates — gotchas, non-obvious patterns,
decisions made and why, things that would have saved time to know up front)*

### Stage 2: Binary Wire Protocol

**CBOR library: fxamacker/cbor/v2**
- Use `github.com/fxamacker/cbor/v2` — well-maintained, RFC 8949 compliant, familiar `Marshal`/`Unmarshal` API.
- **Critical gotcha**: `fxamacker/cbor` decodes positive integers as `uint64`, not `int64`, when decoding to `any`. Negative integers decode as `int64`. This means you MUST handle both `uint64` and `int64` in type switch extractors. A test that encodes `int64(42)` and decodes to `any` will produce a `uint64` — the `case int64:` branch won't fire for positive values from CBOR. Test with negative integers to exercise the `int64` branch.
- Similarly, `float32` is decoded as `float64` from CBOR wire format. The `case float32:` branch in `ExtractFloat` is only reachable if a `float32` is passed directly, not via CBOR decode.

**net.Pipe() is synchronous (unbuffered)**
- `net.Pipe()` blocks `Write` until the other side `Read`s. Any test that writes then reads sequentially on the same goroutine will deadlock.
- **Pattern**: always write in a goroutine: `go func() { errCh <- WritePacket(writer, pkt) }()`, then read on the main goroutine.
- Use `sendRaw(conn, data)` helper (which spawns a goroutine) for error-case tests that write raw bytes.
- `net.Pipe()` returns `io.ErrClosedPipe` (not `io.EOF`) when the other end closes. Handle this in `recvExact`.

**check-coverage.py had two bugs (fixed in Stage 2)**
- **Bug 1**: The regex captured `numStatements` instead of `executionCount` from the coverage profile. Format is `file:range numStatements execCount`; the script was using the 4th capture group as "count" but that was `numStatements`. Fixed by capturing the last field for `execCount`.
- **Bug 2**: `is_false_positive` tried to `open(file)` using the Go module path (e.g., `github.com/antithesishq/hegel-go/cbor.go`) which doesn't exist on disk. Fixed by stripping module prefix to find the local path.

**Unreachable panic coverage**
- `if err != nil { panic("unreachable: ...") }` guards that are truly unreachable (e.g., `cbor.DecOptions{}.DecMode()`) must be handled as false positives by the coverage script.
- The script now detects `if err != nil {` followed by a panic with "unreachable" as a false positive.
- Keep the word "unreachable" in the panic message so the false-positive filter can identify it.

**CRC32**: Use stdlib `hash/crc32.ChecksumIEEE` — matches Python's `zlib.crc32(data) & 0xFFFFFFFF`. The IEEE polynomial is the standard one.

**Packet struct with []byte field**: Go structs containing `[]byte` are not comparable with `==`. Use a `packetsEqual(a, b Packet) bool` helper that calls `bytes.Equal` for the payload field.

### Stage 3: Connection and Channel Abstractions

**staticcheck ST1005 — error strings must not be capitalized**
- Go convention (enforced by staticcheck ST1005): error strings must start with lowercase.
- Any `fmt.Errorf("Bad ...", ...)` or `fmt.Errorf("Cannot ...", ...)` will fail lint.
- Panics are NOT error strings and can be capitalized.
- Remember to update test `mustContain` assertions to match the lowercase strings.

**net.Pipe() returns io.ErrClosedPipe, not io.EOF**
- Already known from Stage 2, but also relevant here: when testing handshake error paths by closing one end, the error will be `io.ErrClosedPipe` not `io.EOF`.

**demand-driven reader: TryLock busy-wait**
- `sync.Mutex.TryLock()` is available in Go 1.18+. The pattern is: try lock in a loop, sleeping 1ms between attempts, checking `until()` after each failed attempt.
- The `until()` function returning true WHILE waiting for the lock (before acquiring it) causes `runReader` to return early — this path needs a dedicated test.

**Unreachable panic placement — no comment between `if err != nil {` and `panic(`**
- The `check-coverage.py` false-positive filter detects `if err != nil {` followed DIRECTLY by a `panic(` containing "unreachable". If there's a comment between them (e.g., `// unreachable: ...`), the filter won't recognize the region as a false positive.
- **Pattern**: Place the comment BEFORE the `if err != nil {` block, not inside it, and put the panic on the line immediately after `if err != nil {`.
- Example (correct): `if err != nil { panic(fmt.Sprintf("hegel: unreachable: ...", err)) }`

**processOneMessage: ch.responses nil check**
- `recvResponseRaw` initializes `ch.responses` at line 468 BEFORE calling `processOneMessage`. So the `if ch.responses == nil` guard inside `processOneMessage` (for reply routing) is never reached via `recvResponseRaw`.
- To cover it: directly call `processOneMessage` with a reply packet in the inbox, bypassing `recvResponseRaw`.

**Channel.SendReplyError: always-encodable encode**
- `map[string]any{"error": string, "type": string}` is always CBOR-encodable. The `if err != nil` guard after `EncodeCBOR` is unreachable. Use the `panic("hegel: unreachable: ...")` pattern so the false-positive filter handles it.

**isTimeout(nil): must be exercised**
- `isTimeout` is only called inside `runReader` when `err != nil`, so the `if err == nil { return false }` branch is never hit during normal use. Add a direct test `TestIsTimeoutNil` calling `isTimeout(nil)`.

**CloseRead() interface coverage**
- `net.Pipe()` connections don't implement `CloseRead()`. To cover the `CloseRead()` branch in `Connection.Close()`, wrap the connection in a test struct that embeds `net.Conn` and implements `CloseRead() error`.

**dispatch: unknown channel — IsReply=true vs IsReply=false paths**
- Two separate branches: `IsReply=true` to an unknown channel → silently drop (line 169-184 `!pkt.IsReply` is false). `IsReply=false` to unknown channel → send error reply (lines 171-182). Both need distinct tests.
- The client must drain the server's error reply for the request case, otherwise the server's `SendPacket` blocks on the synchronous `net.Pipe`.

**TestHandleRequestsStopFnImmediate: stopFn returning true immediately**
- Use `HandleRequests` with `stopFn = func() bool { return true }` to cover the early-exit path.
