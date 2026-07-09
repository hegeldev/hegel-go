RELEASE_TYPE: patch

This patch cleans up several public-API rough edges:

- Failing `RunStateful` replays no longer print a draw-report line for Hegel's internal step-count draw (a line pointing into `stateful.go` with a huge pre-clamp integer). Replay output now contains only your own draws and the step notes.
- Generator validation messages now use the Go API's names instead of Python Hypothesis keyword names: for example `Lists: MaxSize 5 < MinSize 10` instead of `cannot have max_size=5 < min_size=10`, and `Floats: cannot combine AllowNaN(true) with Min or Max` instead of `cannot have allow_nan=true with min_value or max_value`.
- `PhaseExplicit` is now documented as reserved for future use: hegel-go currently has no way to provide explicit examples, so enabling or disabling that phase is a no-op.
- The README and package documentation described a library search order that included a sibling `../hegel-rust/target/{release,debug}` checkout, but the loader has never searched those locations. The documentation now describes the real behavior: `HEGEL_LIBHEGEL_PATH` if set, otherwise the auto-downloaded, checksum-verified pinned release.
- The CI behavior promised by `WithDatabase` (the example database is automatically disabled in CI) is implemented by the libhegel engine itself, which also derandomizes runs in CI; both defaults are now documented (on `WithDatabase` and `WithDerandomize`) and covered by tests. Explicit options always override the CI defaults.
