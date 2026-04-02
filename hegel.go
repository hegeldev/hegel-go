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
// # Getting started
//
// Add hegel to your module:
//
//	go get hegel.dev/go/hegel@latest
//
// Hegel requires either [uv] on your system, or the HEGEL_SERVER_COMMAND
// environment variable set to the path of a hegel-core binary.
//
// # Write your first test
//
// Hegel integrates directly with go test. Write a standard TestXxx function
// and use [Case] with t.Run:
//
//	t.Run("integers", hegel.Case(func(ht *hegel.T) {
//		n := hegel.Draw(ht, hegel.Integers(0, 200))
//		if n >= 50 {
//			ht.Fatalf("n=%d is too large", n)
//		}
//	}))
//
// [Case] returns a func(*testing.T) for use with t.Run. Inside the body,
// call [Draw] on a generator to produce a typed value. If the test body
// panics or calls ht.Fatal, Hegel shrinks the inputs to a minimal
// counterexample. Here it will report n = 50.
//
// By default Hegel runs 100 test cases. Override this with [WithTestCases]:
//
//	t.Run("my_test", hegel.Case(func(ht *hegel.T) { /* … */ }, hegel.WithTestCases(500)))
//
// # Standalone usage with Run
//
// Use [Run] outside of go test, for example in standalone binaries or
// conformance tests:
//
//	err := hegel.Run(func(s *hegel.TestCase) {
//		n := hegel.Draw(s, hegel.Integers(0, 100))
//		if n < 0 || n > 100 {
//			panic("out of range")
//		}
//	}, hegel.WithTestCases(50))
//
// [Run] returns an error on failure. Use [MustRun] to panic instead.
//
// # Drawing multiple values
//
// Call [Draw] multiple times to produce multiple values in a single test:
//
//	t.Run("multiple_values", hegel.Case(func(ht *hegel.T) {
//		n := hegel.Draw(ht, hegel.Integers(math.MinInt, math.MaxInt))
//		s := hegel.Draw(ht, hegel.Text(0, 50))
//		_ = n // n is int
//		_ = s // s is string
//	}))
//
// Because generation is imperative, you can call [Draw] at any point —
// including conditionally or in loops.
//
// # Filtering
//
// Use [Filter] on a generator for simple per-value conditions:
//
//	t.Run("even_integers", hegel.Case(func(ht *hegel.T) {
//		evenIntegers := hegel.Filter(hegel.Integers(math.MinInt, math.MaxInt), func(v int) bool {
//			return v%2 == 0
//		})
//		n := hegel.Draw(ht, evenIntegers)
//		if n%2 != 0 {
//			ht.Fatalf("%d is not even", n)
//		}
//	}))
//
// For conditions that depend on multiple generated values, use
// [TestCase.Assume] inside the test body:
//
//	t.Run("division", hegel.Case(func(ht *hegel.T) {
//		n1 := hegel.Draw(ht, hegel.Integers(-1000, 1000))
//		n2 := hegel.Draw(ht, hegel.Integers(-1000, 1000))
//		ht.Assume(n2 != 0) // discard the case where n2 is zero
//
//		// n2 is guaranteed non-zero from here
//		q, r := n1/n2, n1%n2
//		if n1 != q*n2+r {
//			ht.Fatalf("%d != %d*%d + %d", n1, q, n2, r)
//		}
//	}))
//
// Using bounds and [Map] is more efficient than [Filter] or [TestCase.Assume]
// because they avoid generating values that will be rejected.
//
// # Transforming generated values
//
// Use [Map] to transform values after generation:
//
//	t.Run("string_of_digits", hegel.Case(func(ht *hegel.T) {
//		s := hegel.Draw(ht, hegel.Map(hegel.Integers(0, 100), func(n int) string {
//			return fmt.Sprintf("%d", n)
//		}))
//		for _, c := range s {
//			if c < '0' || c > '9' {
//				ht.Fatalf("%q contains non-digit %c", s, c)
//			}
//		}
//	}))
//
// [Map] is generic — the input and output types are inferred automatically.
//
// # Dependent generation
//
// Because generation is imperative in Hegel, you can use earlier results to
// configure later generators directly:
//
//	t.Run("list_with_valid_index", hegel.Case(func(ht *hegel.T) {
//		n := hegel.Draw(ht, hegel.Integers(1, 10))
//		lst := hegel.Draw(ht, hegel.Lists(
//			hegel.Integers(math.MinInt, math.MaxInt),
//		).MinSize(n).MaxSize(n))
//		index := hegel.Draw(ht, hegel.Integers(0, n-1))
//		if index < 0 || index >= len(lst) {
//			ht.Fatal("index out of range")
//		}
//	}))
//
// You can also use [FlatMap] for dependent generation within a single
// generator expression:
//
//	t.Run("flatmap_example", hegel.Case(func(ht *hegel.T) {
//		result := hegel.Draw(ht, hegel.FlatMap(
//			hegel.Integers(1, 5),
//			func(n int) hegel.Generator[[]int] {
//				return hegel.Lists(
//					hegel.Integers(math.MinInt, math.MaxInt),
//				).MinSize(n).MaxSize(n)
//			},
//		))
//		if len(result) < 1 || len(result) > 5 {
//			ht.Fatalf("unexpected list length: %d", len(result))
//		}
//	}))
//
// # What you can generate
//
// Primitive types:
//
//	hegel.Booleans()                             // bool
//	hegel.Integers(-1000, 1000)                  // int in [min, max]
//	hegel.Integers(math.MinInt, math.MaxInt)     // unbounded int
//	hegel.Floats[float64]().Min(0).Max(1)        // float64
//	hegel.Text(0, 50)                            // Unicode string (pass maxSize < 0 for unbounded)
//	hegel.Binary(0, 64)                          // []byte (pass maxSize < 0 for unbounded)
//
// Constants and choices:
//
//	hegel.Just(42)                               // always returns 42
//	hegel.SampledFrom([]string{"a", "b", "c"})   // uniform random pick from a slice
//
// Collections:
//
//	hegel.Lists(elemGen).MinSize(1).MaxSize(10)  // []T
//	hegel.Dicts(keyGen, valGen).MaxSize(5)       // map[K]V
//
// Combinators:
//
//	hegel.OneOf(g1, g2)             // value from any of the given generators
//	hegel.Optional(g)               // *T — nil or a pointer to a value from g
//	hegel.Map(gen, fn)              // transform generated values
//	hegel.Filter(gen, pred)         // keep only values matching a predicate
//	hegel.FlatMap(gen, fn)          // dependent generation
//
// Formats and patterns:
//
//	hegel.Emails()                  // email address strings
//	hegel.URLs()                    // URL strings
//	hegel.Domains().MaxLength(63)   // domain name strings
//	hegel.Dates()                   // ISO 8601 date strings (YYYY-MM-DD)
//	hegel.Datetimes()               // ISO 8601 datetime strings
//	hegel.IPAddresses().IPv4()      // IPv4 addresses
//	hegel.FromRegex(pattern, true)  // strings matching a regular expression
//
// # Debugging with Note
//
// Use [TestCase.Note] to attach debug messages that appear only when Hegel
// replays the minimal failing example:
//
//	t.Run("example", hegel.Case(func(ht *hegel.T) {
//		x := hegel.Draw(ht, hegel.Integers(-1000, 1000))
//		y := hegel.Draw(ht, hegel.Integers(-1000, 1000))
//		ht.Note(fmt.Sprintf("trying x=%d, y=%d", x, y))
//		if x+y != y+x {
//			ht.Fatal("addition is not commutative")
//		}
//	}))
//
// # Guiding generation with Target
//
// Use [TestCase.Target] to nudge Hegel toward interesting values, making it
// more likely to find boundary failures:
//
//	t.Run("seek_large_values", hegel.Case(func(ht *hegel.T) {
//		x := hegel.Draw(ht, hegel.Integers(0, 10000))
//		ht.Target(float64(x), "maximize_x")
//		if x > 9999 {
//			ht.Fatalf("x=%d exceeds limit", x)
//		}
//	}, hegel.WithTestCases(1000)))
//
// [TestCase.Target] is advisory — Hegel will try to maximize the targeted
// metric, but it may still explore other regions of the input space.
//
// [Hypothesis]: https://github.com/hypothesisworks/hypothesis
// [Hegel]: https://hegel.dev/
// [uv]: https://docs.astral.sh/uv/
package hegel
