# Changelog

## 0.6.16 - 2026-07-09

This patch fixes a rare crash. A wrapper's GC cleanup could free its
native libhegel handle while a call on that same handle was still
executing, because the raw handle is invisible to the collector and the
wrapper's last use can precede the call's return (observed once as a
segfault inside `hegel_mark_complete`). Every native call now pins its
handle's wrapper with `runtime.KeepAlive`, and a test enforces the rule
for all future bindings.

## 0.6.15 - 2026-07-08

This patch bumps our pinned libhegel ([hegel-rust](hegeldev/hegel-rust)) from [0.27.0](https://github.com/hegeldev/hegel-rust/releases/tag/v0.27.0) to [0.28.0](https://github.com/hegeldev/hegel-rust/releases/tag/v0.28.0).

## 0.6.14 - 2026-07-07

This patch bumps our pinned libhegel ([hegel-rust](hegeldev/hegel-rust)) from [0.25.0](https://github.com/hegeldev/hegel-rust/releases/tag/v0.25.0) to [0.27.0](https://github.com/hegeldev/hegel-rust/releases/tag/v0.27.0).

## 0.6.13 - 2026-07-06

This patch bumps our pinned libhegel ([hegel-rust](hegeldev/hegel-rust)) from [0.24.0](https://github.com/hegeldev/hegel-rust/releases/tag/v0.24.0) to [0.25.0](https://github.com/hegeldev/hegel-rust/releases/tag/v0.25.0).

## 0.6.12 - 2026-07-03

This patch bumps our pinned libhegel ([hegel-rust](hegeldev/hegel-rust)) from [0.23.2](https://github.com/hegeldev/hegel-rust/releases/tag/v0.23.2) to [0.24.0](https://github.com/hegeldev/hegel-rust/releases/tag/v0.24.0).

## 0.6.11 - 2026-07-03

Update include the callers PC in failure origin so that hegel-rust considers different PCs
as individual errors.

## 0.6.10 - 2026-06-30

Fix a use-after-free due to the GC collecting a hegel-rust object too soon.

## 0.6.9 - 2026-06-29

This release changes `WithDatabase` to take a plain `string` path instead of a `DatabaseSetting` value. The `DatabaseSetting` type and its `Database` and `DatabaseDisabled` constructors have been removed. A non-empty path persists failing examples to that directory; an empty path disables persistence.

```go
// before
hegel.Test(t, body, hegel.WithDatabase(hegel.Database("examples")))
hegel.Test(t, body, hegel.WithDatabase(hegel.DatabaseDisabled()))

// after
hegel.Test(t, body, hegel.WithDatabase("examples"))
hegel.Test(t, body, hegel.WithDatabase("")) // empty path disables persistence
```

This release also exposes four libhegel engine settings that previously had no public option:

- `WithBackend` selects the randomness backend (`BackendAuto`, `BackendDefault`, `BackendURandom`) — for example to read fresh entropy on every draw when running under Antithesis.
- `WithVerbosity` sets how much the engine logs (`VerbosityQuiet` through `VerbosityDebug`).
- `WithReportMultipleFailures` asks the engine to report every distinct counterexample it finds rather than stopping at the first.
- `WithPhases` restricts a run to specific phases (`PhaseExplicit`, `PhaseReuse`, `PhaseGenerate`, `PhaseTarget`, `PhaseShrink`); `AllPhases` returns them all.

Passing `SuppressHealthCheck` more than once now keeps the last call's set of checks rather than accumulating across calls — pass every check to suppress in a single call.

## 0.6.8 - 2026-06-29

This patch bumps our pinned libhegel ([hegel-rust](hegeldev/hegel-rust)) from [0.23.1](https://github.com/hegeldev/hegel-rust/releases/tag/v0.23.1) to [0.23.2](https://github.com/hegeldev/hegel-rust/releases/tag/v0.23.2).

## 0.6.7 - 2026-06-26

This patch bumps our pinned libhegel ([hegel-rust](hegeldev/hegel-rust)) from [0.23.0](https://github.com/hegeldev/hegel-rust/releases/tag/v0.23.0) to [0.23.1](https://github.com/hegeldev/hegel-rust/releases/tag/v0.23.1).

## 0.6.6 - 2026-06-25

This release hands rule selection in stateful tests (`RunStateful`) to the
libhegel engine. Previously hegel-go drew the next rule itself; it now asks the
engine via the state-machine protocol, which applies swarm testing: each test
case enables a random subset of the rules and draws only from that subset, with
the restrictions shrinking away in minimal counterexamples. This tends to
surface bugs that only appear under particular combinations of rules.

## 0.6.5 - 2026-06-25

Update vendored libhegel to v0.23.0.

## 0.6.4 - 2026-06-16

This patch bumps our pinned libhegel ([hegel-rust](hegeldev/hegel-rust)) from [0.19.1](https://github.com/hegeldev/hegel-rust/releases/tag/v0.19.1) to [0.19.2](https://github.com/hegeldev/hegel-rust/releases/tag/v0.19.2).

## 0.6.3 - 2026-06-16

This patch bumps our pinned libhegel ([hegel-rust](hegeldev/hegel-rust)) from [0.17.4](https://github.com/hegeldev/hegel-rust/releases/tag/v0.17.4) to [0.19.1](https://github.com/hegeldev/hegel-rust/releases/tag/v0.19.1).

## 0.6.2 - 2026-06-15

This release fixes two issues in stateful testing:

- Calling `Assume()` inside an invariant now rejects only that invariant
  invocation rather than the entire test case, matching the behaviour of
  `Assume()` inside a rule.
- The number of steps taken during a stateful run is now drawn with a
  skewed distribution clamped to the maximum step count, aligning step
  selection with the other Hegel libraries so that shrinking behaves
  consistently.

## 0.6.1 - 2026-06-15

Overhaul panic handling to improve the experience when debugging hegel tests.

## 0.6.0 - 2026-06-12

This release replaces the Python-server backend with a direct FFI binding to libhegel, the native Rust engine shipped by [hegel-rust](https://github.com/hegeldev/hegel-rust).

The public API is unchanged: `hegel.Test`, `hegel.Run`, `hegel.MustRun`, `hegel.Workload`, every generator, and every option work exactly as before.

What changes is the installation requirement. Instead of `uv` and `hegel-core`, hegel-go now needs `libhegel.so` (Linux) or `libhegel.dylib` (macOS) at runtime. It is resolved in this order:

1. `$HEGEL_LIBHEGEL_PATH` if set.
2. `../hegel-rust/target/release/libhegel.<ext>` (and the `debug` build) relative to your project root, for local development against a sibling hegel-rust checkout.
3. The platform-appropriate binary fetched on first use from the hegel-rust GitHub release matching the embedded version, cached under `~/.cache/hegel-go/libhegel/<version>/`.

The auto-download is verified against a SHA-256 digest baked into the library (not one fetched from the release, which would offer no protection against a tampered release); set `HEGEL_LIBHEGEL_NO_DOWNLOAD` to opt out.

## 0.5.3 - 2026-05-21

This patch improves the output of failing tests. When a test fails, Hegel now emits a line for every `hegel.Draw` call made during the final replay, echoing the call site, the source statement, and the drawn value:

```
    example_test.go:46: slice1 := hegel.Draw(...) = []int{0, 0}
```

If the source file isn't available, a synthesized statement is used instead:

```
    example_test.go:46: hegel.Draw[[]int](...) = []int{0, 0}
```

`hegel.T`'s methods, `hegel.Draw`, and other internal frames are also marked as test helpers, so `file:line` decoration from `t.Log`, `t.Fatal`, and friends now points at the user's test body rather than into Hegel's internals.

## 0.5.2 - 2026-05-20

This release adds `hegel.WithSingleTestCase` (and a matching `--single-test-case` flag on `hegel.Workload`) for long-running workloads or tests whose body is not safely re-runnable on the same inputs — code with external side effects, time-dependent behavior, or execution under Antithesis. Shrinking, replay, and the example database are disabled, and `hegel.RunStateful` loops indefinitely instead of capping at the usual step count.

## 0.5.1 - 2026-05-16

Add a `WithSeed(seed int64)` option to set a fixed seed for a test:

```go
hegel.Test(t, func(ht *hegel.T) {
    ...
}, hegel.WithSeed(42))

## 0.5.0 - 2026-05-13

This release changes `hegel.TestCase` from a struct to an interface. Code that previously named the type as `*hegel.TestCase` should now use `hegel.TestCase`:

```go
// before
personGen := hegel.Composite(func(tc *hegel.TestCase) Person { ... })

// after
personGen := hegel.Composite(func(tc hegel.TestCase) Person { ... })
```

## 0.4.0 - 2026-05-13

This release adds `hegel.Workload`, for running a property test as a standalone CLI binary outside of `go test` — for example as a workload in a soak run or fuzzing harness.

## 0.3.5 - 2026-05-11

This patch bumps our pinned hegel-core from [0.6.0](https://github.com/hegeldev/hegel-core/releases/tag/v0.6.0) to [0.8.2](https://github.com/hegeldev/hegel-core/releases/tag/v0.8.2).

## 0.3.4 - 2026-05-08

This release adds support for stateful property testing via `hegel.RunStateful`.

This release also makes `*hegel.TestCase` compatible with the `TestingT` interfaces used by popular assertion libraries (testify, gotest.tools, gomega). Assertions from those libraries can now be used directly inside `Composite` callbacks, `Run` bodies, and stateful rules, where only a `*TestCase` is available.

## 0.3.3 - 2026-05-01

This release adds `hegel.Composite`, for defining custom generators:

```go
type Person struct {
    Name           string
    Age            int
    DrivingLicense bool
}

personGen := hegel.Composite(func(tc *hegel.TestCase) Person {
    age := hegel.Draw(tc, hegel.Integers(0, 120))
    name := hegel.Draw(tc, hegel.Text())
    p := Person{Age: age, Name: name}
    if age >= 18 {
        p.DrivingLicense = hegel.Draw(tc, hegel.Booleans())
    }
    return p
})

hegel.Test(t, func(ht *hegel.T) {
    p := hegel.Draw(ht, personGen)
    // ...
})
```

## 0.3.2 - 2026-05-01

This release adds the `WithDatabase` option, which controls the location of the test case database:

```go
hegel.Test(t, func(ht *hegel.T) {
    ...
}, hegel.WithDatabase(hegel.Database("my_custom_directory")))

// disable the database
hegel.Test(t, func(ht *hegel.T) {
    ...
}, hegel.WithDatabase(hegel.DatabaseDisabled()))
```

This release also adds the `WithDerandomize` option, which can be set to make the test run deterministically:

```go
hegel.Test(t, func(ht *hegel.T) {
    ...
}, hegel.WithDerandomize(true))
```

## 0.3.1 - 2026-04-30

Internal refactor.

## 0.3.0 - 2026-04-30

This release removes `hegel.Case` in favor of a new `hegel.Test`. `hegel.Test` is now the recommended way to write Hegel tests.

```go
// before
func TestA(t *testing.T) {
	t.Run("test_name", hegel.Case(func(ht *hegel.T) {
		hegel.Draw(ht, hegel.Integers(-1000, 1000))
	}))
}

// after
func TestA(t *testing.T) {
	hegel.Test(t, func(ht *hegel.T) {
		hegel.Draw(ht, hegel.Integers(-1000, 1000))
	})
}
```

## 0.2.1 - 2026-04-29

Internal refactor of `oneOf`.

## 0.2.0 - 2026-04-28

This release renames the `hegel.Dicts` generator to `hegel.Maps`.

This release also changes `Text` to a builder pattern, matching our other generator APIs:

```go
// before
hegel.Text(1, 50)

// after
hegel.Text().MinSize(1).MaxSize(50)
```

This release also adds more configuration parameters to `Text()`:

```go
hegel.Text().Codec("ascii")
hegel.Text().Alphabet("abc")
hegel.Text().MinCodepoint(0x20).MaxCodepoint(0x7E)
hegel.Text().Categories([]string{"L", "Nd"})
hegel.Text().ExcludeCategories([]string{"Cc"})
hegel.Text().IncludeCharacters("@#$")
hegel.Text().ExcludeCharacters("\n\t")
```

As well as a new `Characters()` generator:

```go
c := hegel.Draw(tc, hegel.Characters())
c := hegel.Draw(tc, hegel.Characters().Codec("ascii"))
```

## 0.1.3 - 2026-04-16

Fix an error when using `Integers` with the full unsigned bounds.

## 0.1.2 - 2026-04-09

This patch lowers the minimum Go version from 1.26 to 1.25.

## 0.1.1 - 2026-04-07

Fix documentation syntax.

## 0.0.1 - 2026-03-03

Initial release!
