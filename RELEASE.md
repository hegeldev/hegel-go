RELEASE_TYPE: patch

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
