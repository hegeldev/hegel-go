RELEASE_TYPE: minor

This release changes `hegel.TestCase` from a struct to an interface. Code that previously named the type as `*hegel.TestCase` should now use `hegel.TestCase`:

```go
// before
personGen := hegel.Composite(func(tc *hegel.TestCase) Person { ... })

// after
personGen := hegel.Composite(func(tc hegel.TestCase) Person { ... })
```
