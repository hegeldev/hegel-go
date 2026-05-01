# Changelog

## 0.3.3 - 2026-05-01

This release adds `hegel.Composite`, for defining custom generators:

```go
type Person struct {
    Name           string
    Age            int
    DrivingLicense bool
}

personGen := hegel.Composite(func(tc *hegel.TestCase) Person {
    age := hegel.Draw(tc, hegel.Integers(0, 120))
    name := hegel.Draw(tc, hegel.Text())
    p := Person{Age: age, Name: name}
    if age >= 18 {
        p.DrivingLicense = hegel.Draw(tc, hegel.Booleans())
    }
    return p
})

hegel.Test(t, func(ht *hegel.T) {
    p := hegel.Draw(ht, personGen)
    // ...
})
```

## 0.3.2 - 2026-05-01

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

## 0.3.1 - 2026-04-30

Internal refactor.

## 0.3.0 - 2026-04-30

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

## 0.2.1 - 2026-04-29

Internal refactor of `oneOf`.

## 0.2.0 - 2026-04-28

This release renames the `hegel.Dicts` generator to `hegel.Maps`.

This release also changes `Text` to a builder pattern, matching our other generator APIs:

```go
// before
hegel.Text(1, 50)

// after
hegel.Text().MinSize(1).MaxSize(50)
```

This release also adds more configuration parameters to `Text()`:

```go
hegel.Text().Codec("ascii")
hegel.Text().Alphabet("abc")
hegel.Text().MinCodepoint(0x20).MaxCodepoint(0x7E)
hegel.Text().Categories([]string{"L", "Nd"})
hegel.Text().ExcludeCategories([]string{"Cc"})
hegel.Text().IncludeCharacters("@#$")
hegel.Text().ExcludeCharacters("\n\t")
```

As well as a new `Characters()` generator:

```go
c := hegel.Draw(tc, hegel.Characters())
c := hegel.Draw(tc, hegel.Characters().Codec("ascii"))
```

## 0.1.3 - 2026-04-16

Fix an error when using `Integers` with the full unsigned bounds.

## 0.1.2 - 2026-04-09

This patch lowers the minimum Go version from 1.26 to 1.25.

## 0.1.1 - 2026-04-07

Fix documentation syntax.

## 0.0.1 - 2026-03-03

Initial release!
