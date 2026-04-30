RELEASE_TYPE: minor

This release removes `hegel.Case` in favor of a new `hegel.Test`. `hegel.Test` is now the recommended way to write Hegel tests.

```go
// before
func TestA(t *testing.T) {
	t.Run("test_name", hegel.Case(func(ht *hegel.T) {
		hegel.Draw(ht, hegel.Integers(-1000, 1000))
	}))
}

// after
func TestA(t *testing.T) {
	hegel.Test(t, func(ht *hegel.T) {
		hegel.Draw(ht, hegel.Integers(-1000, 1000))
	})
}
```
