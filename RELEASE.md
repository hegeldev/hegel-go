RELEASE_TYPE: minor

Add `WithSeed(seed int64)` option to pin a fixed seed for property test
runs, analogous to Hypothesis's `@seed(N)`. When both `WithSeed` and
`WithDerandomize` are set, `WithSeed` takes precedence (matching
Hypothesis).
