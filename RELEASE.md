RELEASE_TYPE: minor

This release updates libhegel to [0.42.1](https://github.com/hegeldev/hegel-rust/releases/tag/libhegel-v0.42.1), including settings profile support and correct nanosecond precision for generated datetimes.

Notes and draw reports now use native test-case documents. Concurrent worker output is grouped by worker with native timing attribution, and completed documents are emitted when the reporting execution finishes. `T.Log` and `T.Logf` retain their existing `testing.T` behavior.

`WithStatefulStepCount` now configures an individual `RunStateful` invocation instead of a property run. Move it from the options passed to `Test`, `Run`, or `Workload` into the `RunStateful` call:

```go
// Before:
hegel.Test(t, func(tc *hegel.T) {
    hegel.RunStateful(tc, &machine{})
}, hegel.WithStatefulStepCount(25))

// After:
hegel.Test(t, func(tc *hegel.T) {
    hegel.RunStateful(tc, &machine{}, hegel.WithStatefulStepCount(25))
})
```

The default remains 50 rounds. `BackendAuto` now uses the backend selected by libhegel's active settings profile, including its Antithesis detection.
