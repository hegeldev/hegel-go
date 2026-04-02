# Coverage Patterns and Techniques

Detailed patterns for achieving 100% test coverage through better code design.

## Genuinely Unreachable Code

Code that should never execute under any circumstances.

**Fix**: Make it an explicit panic.

```go
// Bad: Silent unreachable code
func process(state State) error {
    switch state {
    case StateA:
        return handleA()
    case StateB:
        return handleB()
    default:
        return nil // "Can't happen" - but coverage sees it
    }
}

// Good: Explicit unreachable
func process(state State) error {
    switch state {
    case StateA:
        return handleA()
    case StateB:
        return handleB()
    default:
        panic("hegel: unreachable: unexpected state")
    }
}
```

The `panic("hegel: unreachable: ...")` documents intent, will fail loudly if your assumption is wrong, and is automatically excluded by the coverage script.

## Hard-to-Test Dependencies

Code that interacts with external systems (filesystem, network, time, environment).

**Fix**: Extract and inject dependencies.

### Extract Functions

```go
// Bad: Monolithic function
func deploy() error {
    output, err := exec.Command("git", "push").Output()
    if err != nil {
        return fmt.Errorf("git push failed: %w", err)
    }
    // ... more logic
    return nil
}

// Good: Extract the testable logic
func checkCommandSuccess(output []byte, err error) error {
    if err != nil {
        return fmt.Errorf("command failed: %w", err)
    }
    return nil
}

func TestCheckCommandSuccess(t *testing.T) {
    err := checkCommandSuccess(nil, fmt.Errorf("exit status 1"))
    if err == nil {
        t.Fatal("expected error")
    }
}
```

### Use Interfaces for Dependency Injection

```go
// Bad: Hardcoded dependency
func getCurrentTime() time.Time {
    return time.Now() // Can't test time-dependent logic
}

// Good: Inject the dependency
type Clock interface {
    Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

type mockClock struct{ t time.Time }

func (c mockClock) Now() time.Time { return c.t }

func isExpired(clock Clock, expiry time.Time) bool {
    return clock.Now().After(expiry)
}

func TestIsExpired(t *testing.T) {
    mock := mockClock{t: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)}
    past := time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)
    if !isExpired(mock, past) {
        t.Fatal("expected expired")
    }
}
```

### Parameterize Over Environment

For functions that read env vars, platform information, or global state, extract the logic into a parameterized version and leave a thin wrapper:

```go
// Hard to test — reads env vars directly
func cacheDir() string {
    if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
        return filepath.Join(xdg, "myapp")
    }
    // ...
}

// Testable — takes values as parameters
func cacheDirFrom(xdg string, home string) string {
    if xdg != "" {
        return filepath.Join(xdg, "myapp")
    }
    // ...
}

// Thin wrapper calls the testable version
func cacheDir() string {
    return cacheDirFrom(os.Getenv("XDG_CACHE_HOME"), os.Getenv("HOME"))
}
```

### Manipulate PATH to Mock Commands

For code that shells out to external commands:

```go
func TestDeployHandlesGitFailure(t *testing.T) {
    dir := t.TempDir()
    gitPath := filepath.Join(dir, "git")
    os.WriteFile(gitPath, []byte("#!/bin/sh\nexit 1\n"), 0o755)

    // Prepend our mock to PATH
    originalPath := os.Getenv("PATH")
    t.Setenv("PATH", dir+":"+originalPath)

    err := deploy()
    if err == nil {
        t.Fatal("expected error from git failure")
    }
}
```

## Error Handling Branches

Error paths that are hard to trigger.

**Fix**: Design for testability.

```go
// Bad: Can't test parsing logic without triggering IO
func readAndParse(path string) (Data, error) {
    content, err := os.ReadFile(path)
    if err != nil {
        return Data{}, err
    }
    value, err := strconv.ParseUint(strings.TrimSpace(string(content)), 10, 64)
    if err != nil {
        return Data{}, err
    }
    return Data{Value: value}, nil
}

// Good: Separate IO from parsing
func parse(content string) (Data, error) {
    // All parsing logic here - easy to test with bad input
}

func readAndParse(path string) (Data, error) {
    content, err := os.ReadFile(path)
    if err != nil {
        return Data{}, err
    }
    return parse(string(content))
}

func TestParseInvalidInput(t *testing.T) {
    _, err := parse("not valid")
    if err == nil {
        t.Fatal("expected error for invalid input")
    }
}
```

## Common Anti-Patterns to Avoid

### Don't: Suppress with Annotations

```go
// Bad: Hiding the problem
func someFunction() error {
    if errorCondition() {
        return err //nocov
    }
}
```

Either figure out how to trigger the error condition in tests, or if the error is genuinely impossible to trigger, mark it unreachable.

### Don't: Mock Everything

```go
// Bad: Testing mocks, not code
func TestWithAllMocks(t *testing.T) {
    mockDB := NewMockDB()
    mockHTTP := NewMockHTTP()
    mockFS := NewMockFS()
    // At this point, what are you even testing?
}
```
