# Getting Started with Hegel (Go)

This tutorial walks through the main features of the Hegel Go SDK.
It is a Go adaptation of the [Python Getting Started guide](https://github.com/antithesishq/hegel-core).

> **API note:** The Python SDK uses a decorator (`@hegel`) to mark test
> functions. In Go there are no decorators, so you pass your test body as a
> plain `func()` to `RunHegelTest`. The rest of the model — calling
> `.Generate()` directly inside the body — is identical.

---

## Install

```bash
# Install the hegel backend (Python subprocess)
pip install hegel-sdk

# Add the Go SDK to your module
go get github.com/antithesishq/hegel-go
```

If you are working inside this repository, `just setup` handles everything.

---

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
        n, _ := hegel.ExtractInt(hegel.Integers(-1<<31, 1<<31-1).Generate())
        fmt.Printf("called with %d\n", n)
        if n != n { // always false — just checking the type
            panic("unreachable")
        }
    }, hegel.WithTestCases(5))
}
```

`RunHegelTest` is the entry point for a property test. The second argument is
the *test body* — a plain `func()` that Hegel calls once per generated input.
Inside the body, call `.Generate()` on a generator to obtain a random value.
When the test runs, Hegel generates random inputs and checks that the body
never panics.

By default Hegel runs **100 test cases**. Override this with `WithTestCases`:

```go
hegel.RunHegelTest("my_test", func() { /* … */ }, hegel.WithTestCases(500))
```

> **Python equivalent:** `@hegel(test_cases=500)`

---

## Running in a test suite

Hegel integrates directly with `go test`. Write a standard `TestXxx` function
and call `RunHegelTest` inside it:

```go
func TestIntegerBound(t *testing.T) {
    err := hegel.RunHegelTestE("integer_bound", func() {
        n, _ := hegel.ExtractInt(hegel.Integers(0, 200).Generate())
        if n >= 50 {
            panic(fmt.Sprintf("n=%d is too large", n))
        }
    })
    if err == nil {
        t.Fatal("expected a failure")
    }
}
```

When a test fails, Hegel shrinks the counterexample to the smallest value that
still triggers the failure — here it will report `n=50`.

> **Note:** Use `RunHegelTestE` when you need the error value. `RunHegelTest`
> panics on failure, which `go test` turns into a test failure automatically.

---

## Generating multiple values

Call generators multiple times to produce multiple values in one test:

```go
hegel.RunHegelTest("multiple_values", func() {
    n, _ := hegel.ExtractInt(hegel.Integers(-1000, 1000).Generate())
    s := hegel.Text(0, 50).Generate()

    _ = n // n is an int64
    _ = s // s is a string
})
```

> **Hypothesis comparison:** In Hypothesis you pass strategies as arguments to
> `@given` and receive them as function parameters. In Hegel you call
> `.Generate()` directly inside the test body, which means you can generate
> values at any point — including conditionally or inside loops.

---

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

`.Filter` retries up to 3 times. If all attempts fail, the test case is
discarded (equivalent to `Assume(false)`).

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

Schema bounds and `.Map` are more efficient than `.Filter` or `Assume`
because they avoid generating values that will be rejected.

---

## Transforming generated values

Use `.Map` to apply a function to each generated value:

```go
hegel.RunHegelTest("string_of_digits", func() {
    // Generate an integer 0–100 and convert it to its decimal string.
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

> **Go note:** `.Map` receives and returns `any`. Use the `ExtractInt`,
> `ExtractString`, etc. helpers from the `hegel` package to safely unwrap
> values. The type assertion at the end (`.Generate().(string)`) is safe here
> because the transform always produces a string.

---

## Dependent generation

Because generation is imperative in Hegel, you can use earlier results to
configure later generators directly — no `@composite` or `data()` required:

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

> **Hypothesis comparison:** This pattern requires `@composite` or `data()` in
> Hypothesis. In Hegel it falls out naturally from the imperative style.

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

---

## What you can generate

### Primitive types

| Go call | Description |
|---|---|
| `Booleans(p).Generate()` | `bool` with probability `p` of true |
| `Integers(min, max).Generate()` | `int64` in [min, max] |
| `IntegersUnbounded().Generate()` | Unbounded `int64` |
| `IntegersFrom(&min, &max).Generate()` | `int64` with optional bounds (pass `nil` to omit) |
| `Floats(min, max, nan, inf, exclMin, exclMax).Generate()` | `float64`; use `nil` pointers to omit bounds |
| `Text(minSize, maxSize).Generate()` | Unicode `string`; pass `maxSize < 0` for unbounded |
| `Binary(minSize, maxSize).Generate()` | `[]byte`; pass `maxSize < 0` for unbounded |

### Constants and choices

| Go call | Description |
|---|---|
| `Just(v).Generate()` | Always returns `v` |
| `MustSampledFrom([]any{1, 2, 3}).Generate()` | Uniform random pick from a slice |

### Collections

| Go call | Description |
|---|---|
| `Lists(elemGen, ListsOptions{}).Generate()` | `[]any` |
| `Dicts(keyGen, valGen, DictOptions{}).Generate()` | `map[any]any` |
| `Tuples2(g1, g2).Generate()` | `[]any` of length 2 |
| `Tuples3(g1, g2, g3).Generate()` | `[]any` of length 3 |
| `Tuples4(g1, g2, g3, g4).Generate()` | `[]any` of length 4 |

### Combinators

| Go call | Description |
|---|---|
| `OneOf(g1, g2, ...).Generate()` | Value from any of the given generators |
| `Optional(g).Generate()` | Either `nil` or a value from `g` |
| `gen.Map(fn)` | Transform generated values |
| `gen.Filter(pred)` | Keep only values matching a predicate |
| `FlatMap(gen, fn)` | Dependent generation — `fn` returns a new generator |

### Formats and addresses

| Go call | Description |
|---|---|
| `Emails().Generate()` | Email address string |
| `URLs().Generate()` | URL string |
| `Domains(opts).Generate()` | Domain name string |
| `Dates().Generate()` | ISO 8601 date string (`YYYY-MM-DD`) |
| `Times().Generate()` | Time string |
| `Datetimes().Generate()` | ISO 8601 datetime string |
| `IPAddresses(IPAddressOptions{}).Generate()` | IPv4 or IPv6 address string |
| `FromRegex(pattern, fullmatch).Generate()` | String matching a regular expression |

---

## Debugging with Note

Use `Note` to attach debug messages that appear *only* when Hegel replays the
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

> **Python equivalent:** `note(f"trying x={x}, y={y}")`

---

## Guiding generation with Target

Use `Target` to nudge Hegel toward large or small values, making it more
likely to find boundary failures:

```go
hegel.RunHegelTest("seek_large_values", func() {
    x := hegel.Integers(0, 10000).Generate().(int64)
    hegel.Target(float64(x), "maximize_x")

    if x > 9999 {
        panic(fmt.Sprintf("x=%d exceeds limit", x))
    }
}, hegel.WithTestCases(1000))
```

> **Python equivalent:** `target(x, "maximize_x")`

`Target` is advisory — Hegel will *try* to maximize (or, if negative,
minimize) the targeted metric, but it may still explore other regions.

---

## Next steps

- Browse the [`examples/`](../examples/) directory for runnable programs.
- Read the full API reference: `go doc github.com/antithesishq/hegel-go`
- Explore the [Hypothesis documentation](https://hypothesis.readthedocs.io/)
  for deeper background on the underlying engine.
