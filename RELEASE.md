RELEASE_TYPE: patch

This patch updates libhegel to [0.42.1](https://github.com/hegeldev/hegel-rust/releases/tag/libhegel-v0.42.1), including settings profile support and correct nanosecond precision for generated datetimes.

Notes and draw reports now use native test-case documents. Concurrent worker output is grouped by worker with native timing attribution, and completed documents are emitted when the reporting execution finishes. `T.Log` and `T.Logf` retain their existing `testing.T` behavior.
