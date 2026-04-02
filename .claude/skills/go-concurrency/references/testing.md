# Concurrency in Tests

## Two levels of test parallelism

1. **Within a test binary**: `t.Parallel()` runs tests concurrently in the same process. Shared Go variables (the global session, package state) are accessible.
2. **Across test binaries**: `go test ./...` runs separate binaries per package as separate OS processes. No shared Go state, but shared filesystem, network, and environment.

## Rules for parallel tests

### Never use `t.Setenv` with `t.Parallel()`
`t.Setenv` panics if the test or any ancestor is parallel. Environment variables are process-global. If a test needs a different env, either:
- Don't mark it parallel
- Pass config via a struct parameter instead of env vars
- Run the code that reads the env in a subprocess

### Use `t.TempDir()` for file isolation
Each call returns a unique directory, cleaned up automatically. Never use hardcoded temp paths.

### net.Pipe() deadlock risk
`net.Pipe()` is synchronous — writes block until reads occur. Always write in a goroutine:
```go
server, client := net.Pipe()
go func() {
    server.Write(data)
    server.Close()
}()
// Now safe to read from client
```
`net.Pipe()` returns `io.ErrClosedPipe` (not `io.EOF`) on close.

### Socket pair tests are fully isolated
Tests using `socketPair()` get their own connection and channels. No shared state with other tests except process-global resources.

### The global session is shared
All integration tests that use `runHegel` share `globalSession`. The session mutex ensures one subprocess at a time, but concurrent tests multiplex over the same connection. Test isolation comes from each test getting its own channel, not from process isolation.

### HEGEL_PROTOCOL_TEST_MODE tests get isolated sessions
When this env var is set, `runHegel` creates a temporary single-use session. These tests must NOT be parallel with each other since they use `os.Setenv`.

## Race detector limitations

`go test -race` detects data races that actually execute during the test run. It does NOT detect:
- Races in untested code paths
- Deadlocks (Go's deadlock detector only catches all-goroutines-blocked)
- Logical race conditions where synchronization exists but logic is wrong
- Goroutine leaks

The race detector has a mutex blind spot: it tracks happens-before via lock acquisitions, so an unguarded write after releasing a lock may be masked by a subsequent lock acquisition in another goroutine.

## Cross-process resource coordination

For resources shared between test processes:
- **Unique paths**: Include PID or use `os.MkdirTemp` for per-process isolation
- **Atomic rename**: Install to temp dir, rename into place (used for uv binary)
- **Retry with backoff**: For port binding or file lock acquisition
- **Per-PID log files**: The server log uses `server.{pid}.log` to avoid interleaving
