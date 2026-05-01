RELEASE_TYPE: patch

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
