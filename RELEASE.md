RELEASE_TYPE: patch

Add a `WithSeed(seed int64)` option to set a fixed seed for a test:

```go
hegel.Test(t, func(ht *hegel.T) {
    ...
}, hegel.WithSeed(42))
