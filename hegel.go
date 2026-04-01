// Package hegel is a property-based testing library for Go. It is based on
// [Hypothesis], using the [Hegel] protocol.
//
// Hegel runs your test body many times with different generated inputs,
// and when a failure is found it automatically shrinks the inputs to a
// minimal counterexample.
//
// Use [Case] to write property tests inside go test, or [Run] for
// standalone usage. Inside the test body, call [Draw] with a [Generator]
// to produce typed random values.
//
// Generators include primitives like [Integers], [Floats], [Booleans],
// and [Text]; collections like [Lists] and [Dicts]; and combinators like
// [Map], [Filter], [FlatMap], and [OneOf]. See each function's
// documentation for details.
//
// [Hypothesis]: https://github.com/hypothesisworks/hypothesis
// [Hegel]: https://hegel.dev/
package hegel
