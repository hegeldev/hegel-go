# Concurrency Patterns and Anti-Patterns

## Pattern: Channel-close as happens-before barrier

Write state, then close a channel. Readers after `<-ch` see the write.

```go
// Writer goroutine
conn.crashMessage = computeMessage()
close(processExited)

// Reader goroutine (any number)
<-processExited
msg := conn.crashMessage  // guaranteed to see the write
```

This works because `close(ch)` happens-before `<-ch` returns the zero value. No mutex needed.

## Pattern: sync.Once for lazy initialization

```go
var initOnce sync.Once
var result *Thing

func getResult() *Thing {
    initOnce.Do(func() {
        result = expensiveComputation()
    })
    return result
}
```

`once.Do(f)` completion happens-before any `Do()` return. All callers block until `f()` finishes. But: if `f()` panics, the Once is "used up" — subsequent calls are no-ops. Use `sync.OnceValue` (Go 1.21+) if you need panic propagation.

## Pattern: Atomic rename for cross-process safety

```go
tmpDir, _ := os.MkdirTemp(parentDir, "install-*")
defer os.RemoveAll(tmpDir)
// ... install binary to tmpDir ...
os.Rename(filepath.Join(tmpDir, "binary"), targetPath)
// rename is atomic on same filesystem; last writer wins
```

Multiple processes can race — all install, one rename succeeds, the rest fail harmlessly.

## Anti-pattern: Closure capturing mutable struct pointer

```go
// BAD: closure reads s.logFile which may be nil'd by cleanup
conn.crashMessageFn = s.serverCrashMessage

// GOOD: capture immutable value
logPath := logFile.Name()
// ... use logPath in goroutine or closure
```

Methods on mutable structs access all fields of the struct. A closure that calls a method on `*s` is implicitly sharing all of `s`'s state.

## Anti-pattern: Lazy error messages reading shared state

```go
// BAD: reads logFile at crash time, races with cleanup
func (s *session) crashMessage() string {
    if s.logFile != nil {          // race: cleanup may nil this
        path = s.logFile.Name()    // race: logFile may be closed
    }
    content, _ := os.ReadFile(path) // race: file may be closed
    return format(content)
}

// GOOD: compute at the point where state is known-good
go func() {
    cmd.Wait()
    conn.crashMessage = computeCrashMessage(logPath)
    close(processExited)
}()
```

The goroutine that detects the event should compute the diagnostic message immediately, before signaling. Other goroutines read the pre-computed result.

## Anti-pattern: Defensive nil checks masking design bugs

```go
// BAD: if processExited is nil, we have a design bug — don't hide it
if ch.conn.processExited != nil {
    select { case <-ch.conn.processExited: ... }
}

// GOOD: let it panic so the bug is found
select { case <-ch.conn.processExited: ... }
```

Nil checks on channels that "should always be set" hide initialization bugs. A nil channel blocks forever in select, which is a deadlock — but a panic gives a stack trace.

## Anti-pattern: Custom PATH search instead of exec.LookPath

```go
// BAD: reimplements stdlib, misses platform details (PATHEXT on Windows, etc.)
func findInPath(name string) string {
    for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
        if _, err := os.Stat(filepath.Join(dir, name)); err == nil { ... }
    }
}

// GOOD: stdlib handles platform differences
path, err := exec.LookPath("uv")
```

## Anti-pattern: select priority assumption

```go
// BAD: if both done and data are ready, Go picks randomly
select {
case <-done: return
case v := <-data: process(v)
}

// GOOD: check done first with non-blocking receive
select {
case <-done: return
default:
}
select {
case <-done: return
case v := <-data: process(v)
}
```

## Subprocess management rules

1. **Call cmd.Wait() exactly once**, from exactly one goroutine. Multiple calls race on internal state.
2. **Read all pipe output before calling Wait()**. Wait closes pipes while readers may still be blocked.
3. **Use cmd.WaitDelay** (Go 1.20+) to bound how long Wait blocks for child-inherited pipe fds.
4. **PID reuse**: Process.Kill() on a dead process may kill an unrelated process that reused the PID. Kill then Wait.
5. **The monitor goroutine pattern**: One goroutine calls Wait(), captures state, closes a signal channel. All other goroutines learn about exit via the channel.
