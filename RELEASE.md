RELEASE_TYPE: minor

This release updates the hegel-core dependency from 0.3.0 to 0.4.7.

New APIs:

- `TextWithAlphabet` generates strings with custom character constraints (codepoint range, Unicode categories, codec).
- `ListGenerator.Unique()` constrains generated lists to contain only unique elements.
- `TestCase.IsFinal()` reports whether the current test case is the final (replay) case.

Internal changes:

- Handle CBOR tag 91 (HEGEL_STRING_TAG) for string values in server responses.
- Text generators now exclude surrogate code points (Unicode category Cs) by default, since Go strings are UTF-8.
- Add `OneOfConformance` and `OriginDeduplicationConformance` test binaries.
