RELEASE_TYPE: patch

This release adds more configuration parameters to `Text()`:

```go
hegel.Text(0, 100).Codec("ascii")
hegel.Text(0, 100).Alphabet("abc")
hegel.Text(0, 100).MinCodepoint(0x20).MaxCodepoint(0x7E)
hegel.Text(0, 100).Categories([]string{"L", "Nd"})
hegel.Text(0, 100).ExcludeCategories([]string{"Cc"})
hegel.Text(0, 100).IncludeCharacters("@#$")
hegel.Text(0, 100).ExcludeCharacters("\n\t")
```

As well as a new `Characters()` generator:

```go
c := hegel.Draw(tc, hegel.Characters())
c := hegel.Draw(tc, hegel.Characters().Codec("ascii"))
```
