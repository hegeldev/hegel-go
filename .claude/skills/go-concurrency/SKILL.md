---
name: go-concurrency
description: "Go concurrency patterns and pitfalls for this codebase. Use when writing or reviewing code that involves goroutines, channels, mutexes, shared state, subprocess management, or anything that runs concurrently. Also use proactively when writing new code — ask yourself: can this be called from multiple goroutines? Is this resource process-global?"
---

# Go Concurrency Review

This codebase multiplexes concurrent property tests over a shared subprocess via channels. Concurrency bugs here are subtle and the race detector only catches about half of real-world Go concurrency bugs. Think carefully.

## Core principle: be explicit about ownership

Every piece of mutable state must have a clear owner. Either:
- **One goroutine owns it** — no synchronization needed, but document it (e.g., "only called from readLoop")
- **A mutex protects it** — every access, including reads, must hold the lock
- **A channel mediates it** — data flows from producer to consumer with happens-before guarantees
- **It's immutable after initialization** — set before any goroutine can read it, preferably via sync.Once

If you can't immediately name the owner of a field, that's a bug.

## This codebase's concurrency architecture

See `references/architecture.md` for the full map of what's protected by what. The key patterns:

1. **Single readLoop goroutine per connection** — only one goroutine reads the socket. Packets are dispatched to per-channel inboxes via non-blocking select.
2. **writerMu serializes socket writes** — any goroutine can write, but all writes go through the mutex.
3. **hegelSession.mu protects subprocess lifecycle** — start/cleanup are serialized; concurrent tests multiplex over the shared connection via separate channels.
4. **processExited channel as a happens-before barrier** — the monitor goroutine writes crash state, then closes the channel. Readers after `<-processExited` see the state without additional locking.

## Red flags to watch for

### Lazy initialization without clear goroutine assignment
If initialization happens "on first use" and multiple goroutines might be the first user, you have a race. Use `sync.Once` or initialize before spawning goroutines. The lazy pattern is especially tempting for error messages, log excerpts, and diagnostic data — but those are read under failure conditions when timing is least predictable.

### Reading shared state "just for diagnostics"
Logging, error messages, and crash reports still need synchronization. `s.logFile.Name()` races with `s.logFile = nil` in cleanup. The fix is to capture the value at initialization time, or have the goroutine that detects the event compute the message before signaling.

### Function closures capturing mutable references
`conn.crashMessageFn = s.serverCrashMessage` captures `s` and reads `s.logFile` — any field the method touches is shared state. Prefer capturing immutable values (strings, ints) rather than pointers to mutable structs.

### Process-global resources in parallel tests
Environment variables, working directory, temp file paths, and network ports are process-global. `t.Setenv` panics if called after `t.Parallel()`. If tests need different env values, pass config structs instead.

### File I/O racing with Close
Even though `os.File.Read` is safe to call concurrently with `Close` at the syscall level, the logical race remains: after Close, the fd is freed and may be reused by another open. Read the file contents into a variable while the file is known to be open, then use the variable.

### select with multiple ready cases
Go's `select` picks a uniformly random ready case. If you need priority (e.g., "check done before processing"), use a nested select with a non-blocking check first.

### Goroutine leaks
Every goroutine must have a clear termination path. A goroutine blocked on a channel send/receive that will never complete leaks forever. Common in error paths where cleanup skips closing a channel.

## Cross-process concurrency

`go test ./...` runs separate binaries per package. `t.Parallel()` runs tests concurrently within a binary. Both levels matter:

- **Within-process**: Shared Go variables, the global session, mutexes work
- **Cross-process**: File locks, temp dirs, network ports, the uv binary cache, installed binaries in `.hegel/venv` — all races here require filesystem-level coordination (atomic rename, unique paths per process)

See `references/testing.md` for testing-specific patterns.

## The happens-before relationship

The Go memory model guarantees sequential consistency for **data-race-free** programs. Key rules:
- Channel send happens-before the corresponding receive completes
- For **unbuffered** channels, receive happens-before send completes (NOT true for buffered)
- `mutex.Unlock()` happens-before the next `Lock()` returns
- `once.Do(f)` completion happens-before any `Do()` call returns
- `close(ch)` happens-before `<-ch` returns zero value
- Goroutine creation (`go f()`) happens-before `f()` starts
- Goroutine exit is **NOT synchronized** — you cannot observe completion without explicit sync

Corollary: if you write to a variable, then close a channel, any goroutine that receives from that channel is guaranteed to see the write. This is the pattern used by `processExited`.
