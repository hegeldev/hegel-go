RELEASE_TYPE: minor

This release updates the hegel-core dependency from 0.3.0 to 0.4.7.

**Breaking change:** `Text` is now a builder type (`TextGenerator`) instead of a function taking `(minSize, maxSize int)`. Migrate `Text(1, 50)` to `Text().MinSize(1).MaxSize(50)`.

New APIs:

- `TextGenerator` builder with character constraint methods: `MinSize`, `MaxSize`, `Codec`, `MinCodepoint`, `MaxCodepoint`, `Categories`, `ExcludeCategories`, `IncludeCharacters`, `ExcludeCharacters`.
- `ListGenerator.Unique()` constrains generated lists to contain only unique elements.

Text generators now exclude surrogate code points (Unicode category Cs) by default, since Go strings are UTF-8.

Internal changes:

- Handle CBOR tag 91 (HEGEL_STRING_TAG) for string values in server responses.
- Add `OneOfConformance` and `OriginDeduplicationConformance` test binaries.
