# Changelog

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
