RELEASE_TYPE: patch

This release adds `hegel.WithSingleTestCase` (and a matching `--single-test-case` flag on `hegel.Workload`) for long-running workloads or tests whose body is not safely re-runnable on the same inputs — code with external side effects, time-dependent behavior, or execution under Antithesis. Shrinking, replay, and the example database are disabled, and `hegel.RunStateful` loops indefinitely instead of capping at the usual step count.
