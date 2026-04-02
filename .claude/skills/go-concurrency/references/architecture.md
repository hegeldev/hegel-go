# Concurrency Architecture Map

This documents what protects what in the codebase. When modifying any of these components, verify you're holding the right lock or running in the right goroutine.

## connection (connection.go)

| Field | Protection | Notes |
|-------|-----------|-------|
| `reader` | readLoop goroutine (exclusive) | Only readLoop reads the socket |
| `writer` / `writerMu` | writerMu Mutex | Any goroutine can write; all writes serialize through writerMu |
| `controlCh` / `controlMu` | controlMu Mutex | Prevents concurrent handshakes on channel 0 |
| `done` | Closed once by readLoop defer | Read-only signal; safe to select on from any goroutine |
| `processExited` | Closed once by monitor goroutine | Read-only signal; crashMessage is set before close |
| `crashMessage` | Written once before processExited close | Safe to read after `<-processExited` (happens-before) |
| `channels` map | readLoop goroutine (writes via dispatch) | Channels registered before readLoop can dispatch to them |
| `nextChannelID` | session mutex (only NewChannel is called under session lock) | Verify if this assumption holds |

## channel (connection.go)

| Field | Protection | Notes |
|-------|-----------|-------|
| `inbox` | Buffered channel (64) | readLoop sends, consumer receives |
| `dropped` | `droppedOnce` sync.Once | Closed when inbox overflows; signals fatal error |
| `responses` map | Consumer goroutine only | Only the goroutine calling processOneMessage touches this |
| `closed` | Consumer goroutine only | Set when shutdownSentinel received |

## hegelSession (runner.go)

| Field | Protection | Notes |
|-------|-----------|-------|
| `conn`, `cli`, `process` | `mu` Mutex | All set in start(), cleared in cleanupLocked() |
| `logFile` | `mu` Mutex | Opened in start(), closed in cleanupLocked() |
| `hegelCmd` | Set before start(), read-only after | Not protected; safe because it's set at construction |
| `suppressStderr` | Set before start(), read-only after | Same |
| `processExited` | `mu` for write, channel semantics for read | Set in start() under lock; read via channel receive |

## Process-global state

| Resource | Protection | Risk |
|----------|-----------|------|
| `globalSession` | Package-level var, protected by its own `mu` | All test code goes through this |
| `hegelDirOnce` / project root | `sync.Once` + `hegelDirMu` | Computed once per process |
| Environment variables | Process-global, no protection | Never use `os.Setenv` in parallel tests |
| `.hegel/venv` directory | Filesystem | Cross-process races on binary installation |
| `.hegel/server.{pid}.log` | Per-PID file path | Different processes get different files |
| uv binary cache (`~/.cache/hegel/uv`) | Atomic rename | Multiple processes may install concurrently |

## Goroutine lifecycle

| Goroutine | Spawned by | Terminated by | Leak risk |
|-----------|-----------|---------------|-----------|
| readLoop | `newConnection()` | Reader EOF/error | Low — reader always closes eventually |
| process monitor | `hegelSession.start()` | `cmd.Wait()` returns | Low — process always exits eventually |
| async SendPacket | `channel.Close()` | Write completes or errors | Low — fire-and-forget, bounded work |
| test goroutines | Various tests | WaitGroup / test completion | Medium — ensure WaitGroup.Done() is deferred |
