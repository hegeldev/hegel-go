RELEASE_TYPE: minor

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
