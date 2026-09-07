RELEASE_TYPE: minor

This release bumps our pinned libhegel ([hegel-rust](hegeldev/hegel-rust)) from [0.33.2](https://github.com/hegeldev/hegel-rust/releases/tag/v0.33.2) to [0.37.1](https://github.com/hegeldev/hegel-rust/releases/tag/v0.37.1).

This is a breaking change: `WithSingleTestCase` and Workload's `--single-test-case` flag have been removed. Property tests now always retain normal shrinking, replay, database persistence, and bounded stateful execution.
