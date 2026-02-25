# Getting Started with Hegel (Go)

## Install Hegel

```bash
# Install the hegel backend
pip install "git+ssh://git@github.com/antithesishq/hegel-core.git"

# Add the Go SDK to your module
go get github.com/antithesishq/hegel-go
```

If you are working inside this repository, `just setup` handles both steps.

## Write your first test

Create `example_test.go`:

```go
package example_test

import (
	"fmt"

	hegel "github.com/antithesishq/hegel-go"
)

func Example_integers() {
	hegel.RunHegelTest("integers", func() {
		n, _ := hegel.ExtractInt(hegel.IntegersUnbounded().Generate())
		fmt.Printf("called with %d\n", n)
	}, hegel.WithTestCases(5))
}
```

`RunHegelTest` runs the test body many times with different random inputs. Inside
the body, call `.Generate()` on a generator to produce a value. If the test body
panics, Hegel shrinks the inputs to a minimal counterexample.

By default Hegel runs **100 test cases**. Override this with `WithTestCases`:

```go
hegel.RunHegelTest("my_test", func() { /* … */ }, hegel.WithTestCases(500))
```

## Running in a test suite

Hegel integrates directly with `go test`. Write a standard `TestXxx` function
and call `RunHegelTest` inside it:

```go
func TestBoundedIntegers(t *testing.T) {
	hegel.RunHegelTest("bounded_integers", func() {
		n, _ := hegel.ExtractInt(hegel.Integers(0, 200).Generate())
		if n >= 50 {
			panic(fmt.Sprintf("n=%d is too large", n))
		}
	})
}
```

When a test fails, Hegel shrinks the counterexample to the smallest value that
still triggers the failure — here it will report `n = 50`.

## Generating multiple values

Call `.Generate()` multiple times to produce multiple values in a single test:

```go
hegel.RunHegelTest("multiple_values", func() {
	n, _ := hegel.ExtractInt(hegel.IntegersUnbounded().Generate())
	s := hegel.Text(0, 50).Generate()

	_ = n // n is an int64
	_ = s // s is a string
})
```

Because generation is imperative, you can call `.Generate()` at any point —
including conditionally or in loops.

## Filtering

Use `.Filter` on a generator for simple per-value conditions:

```go
hegel.RunHegelTest("even_integers", func() {
	n, _ := hegel.ExtractInt(
		hegel.IntegersUnbounded().Filter(func(v any) bool {
			i, _ := hegel.ExtractInt(v)
			return i%2 == 0
		}).Generate(),
	)
	if n%2 != 0 {
		panic(fmt.Sprintf("%d is not even", n))
	}
})
```

For conditions that depend on multiple generated values, use `Assume` inside
the test body:

```go
hegel.RunHegelTest("division", func() {
	n1, _ := hegel.ExtractInt(hegel.Integers(-1000, 1000).Generate())
	n2, _ := hegel.ExtractInt(hegel.Integers(-1000, 1000).Generate())

	hegel.Assume(n2 != 0) // discard the case where n2 is zero

	// n2 is guaranteed non-zero from here
	q, r := n1/n2, n1%n2
	if n1 != q*n2+r {
		panic(fmt.Sprintf("%d != %d*%d + %d", n1, q, n2, r))
	}
})
```

Using bounds and `.Map` is more efficient than `.Filter` or `Assume` because
they avoid generating values that will be rejected.

## Transforming generated values

Use `.Map` to transform values after generation:

```go
hegel.RunHegelTest("string_of_digits", func() {
	s := hegel.Integers(0, 100).Map(func(v any) any {
		n, _ := hegel.ExtractInt(v)
		return fmt.Sprintf("%d", n)
	}).Generate().(string)

	for _, c := range s {
		if c < '0' || c > '9' {
			panic(fmt.Sprintf("%q contains non-digit %c", s, c))
		}
	}
})
```

`.Map` receives and returns `any`. Use the `ExtractInt`, `ExtractString`, etc.
helpers to safely unwrap values.

## Dependent generation

Because generation is imperative in Hegel, you can use earlier results to
configure later generators directly:

```go
hegel.RunHegelTest("list_with_valid_index", func() {
	n, _ := hegel.ExtractInt(hegel.Integers(1, 10).Generate())
	lst := hegel.Lists(
		hegel.IntegersUnbounded(),
		hegel.ListsOptions{MinSize: int(n), MaxSize: int(n)},
	).Generate().([]any)
	index, _ := hegel.ExtractInt(hegel.Integers(0, n-1).Generate())

	if index < 0 || index >= int64(len(lst)) {
		panic("index out of range")
	}
})
```

You can also use `FlatMap` for dependent generation within a single generator
expression:

```go
hegel.RunHegelTest("flatmap_example", func() {
	result := hegel.FlatMap(
		hegel.Integers(1, 5),
		func(v any) hegel.Generator {
			n, _ := hegel.ExtractInt(v)
			return hegel.Lists(
				hegel.IntegersUnbounded(),
				hegel.ListsOptions{MinSize: int(n), MaxSize: int(n)},
			)
		},
	).Generate().([]any)

	if len(result) < 1 || len(result) > 5 {
		panic(fmt.Sprintf("unexpected list length: %d", len(result)))
	}
})
```

## What you can generate

### Primitive types

```go
hegel.Booleans(0.5)                          // bool with probability p of true
hegel.Integers(-1000, 1000)                  // int64 in [min, max]
hegel.IntegersUnbounded()                    // unbounded int64
hegel.Floats(min, max, nan, inf, eMin, eMax) // float64 (use nil to omit bounds)
hegel.Text(0, 50)                            // Unicode string (pass maxSize < 0 for unbounded)
hegel.Binary(0, 64)                          // []byte (pass maxSize < 0 for unbounded)
```

### Constants and choices

```go
hegel.Just(42)                                // always returns 42
hegel.MustSampledFrom([]any{1, 2, 3})        // uniform random pick from a slice
```

### Collections

```go
hegel.Lists(elemGen, hegel.ListsOptions{MinSize: 1, MaxSize: 10}) // []any
hegel.Dicts(keyGen, valGen, hegel.DictOptions{MaxSize: 5})         // map[any]any
hegel.Tuples2(g1, g2)                                              // []any of length 2
hegel.Tuples3(g1, g2, g3)                                          // []any of length 3
hegel.Tuples4(g1, g2, g3, g4)                                      // []any of length 4
```

### Combinators

```go
hegel.OneOf(g1, g2)             // value from any of the given generators
hegel.Optional(g)               // nil or a value from g
gen.Map(fn)                     // transform generated values
gen.Filter(pred)                // keep only values matching a predicate
hegel.FlatMap(gen, fn)          // dependent generation
```

### Formats and patterns

```go
hegel.Emails()                  // email address strings
hegel.URLs()                    // URL strings
hegel.Domains(opts)             // domain name strings
hegel.Dates()                   // ISO 8601 date strings (YYYY-MM-DD)
hegel.Times()                   // time strings
hegel.Datetimes()               // ISO 8601 datetime strings
hegel.IPAddresses(opts)         // IPv4 or IPv6 address strings
hegel.FromRegex(pattern, true)  // strings matching a regular expression
```

## Debugging with Note

Use `Note` to attach debug messages that appear only when Hegel replays the
minimal failing example:

```go
hegel.RunHegelTest("example", func() {
	x, _ := hegel.ExtractInt(hegel.Integers(-1000, 1000).Generate())
	y, _ := hegel.ExtractInt(hegel.Integers(-1000, 1000).Generate())

	hegel.Note(fmt.Sprintf("trying x=%d, y=%d", x, y))

	if x+y != y+x {
		panic("addition is not commutative")
	}
})
```

## Guiding generation with Target

Use `Target` to nudge Hegel toward interesting values, making it more likely to
find boundary failures:

```go
hegel.RunHegelTest("seek_large_values", func() {
	x := hegel.Integers(0, 10000).Generate().(int64)
	hegel.Target(float64(x), "maximize_x")

	if x > 9999 {
		panic(fmt.Sprintf("x=%d exceeds limit", x))
	}
}, hegel.WithTestCases(1000))
```

`Target` is advisory — Hegel will try to maximize the targeted metric, but it
may still explore other regions of the input space.

## Next steps

- Browse the [`examples/`](../examples/) directory for runnable programs.
- Read the full API reference: `go doc github.com/antithesishq/hegel-go`
