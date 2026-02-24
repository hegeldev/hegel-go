# hegel-go

A Go SDK for [Hegel](https://github.com/antithesishq/hegel-core) — a universal
property-based testing framework powered by [Hypothesis](https://hypothesis.works/).

## What is Hegel?

Hegel generates random inputs to your test functions, then shrinks any
counterexample it finds down to the smallest case that still triggers the
failure. Unlike traditional fuzz testing, it understands structure: you tell
Hegel *what kind* of data to produce and it handles the rest.

The Hegel backend runs as a Python subprocess (`hegel`). This SDK communicates
with it over a Unix socket using a compact binary protocol, so Go tests get
the full power of Hypothesis's generation and shrinking engine.

## Installation

```bash
# Install the hegel backend
pip install hegel-sdk   # or: just setup

# Add the Go SDK to your module
go get github.com/antithesishq/hegel-go
```

See [`just setup`](justfile) for the full development setup (creates a `.venv`
with the `hegel` binary and installs Go tools).

## Quick start

```go
package mypackage_test

import (
    "testing"

    hegel "github.com/antithesishq/hegel-go"
)

func TestAddCommutative(t *testing.T) {
    hegel.RunHegelTest("add_commutative", func() {
        a := hegel.GenerateInt(-1000, 1000)
        b := hegel.GenerateInt(-1000, 1000)
        if a+b != b+a {
            panic("addition is not commutative!")
        }
    })
}
```

Run it with `go test` as normal. Hegel will generate 100 random input pairs
and report the minimal counterexample if it finds one.

## Core API

### Running tests

| Function | Description |
|---|---|
| `RunHegelTest(name, fn, opts...)` | Run a property test; panic on failure |
| `RunHegelTestE(name, fn, opts...)` | Run a property test; return error on failure |
| `WithTestCases(n)` | Option: set number of test cases (default 100) |

### Generating values (inside test body)

| Function | Description |
|---|---|
| `GenerateBool()` | Generate a boolean |
| `GenerateInt(min, max)` | Generate an integer in [min, max] |
| `gen.Generate()` | Generate a value from any [Generator] |

### Generators

| Constructor | Description |
|---|---|
| `Integers(min, max)` | Bounded integers |
| `IntegersUnbounded()` | Unbounded integers |
| `IntegersFrom(&min, &max)` | Integers with optional bounds |
| `Floats(min, max, allowNaN, allowInf, exclMin, exclMax)` | Floating-point numbers |
| `Booleans(p)` | Boolean with probability `p` of true |
| `Text(minSize, maxSize)` | Unicode strings (maxSize < 0 = unbounded) |
| `Binary(minSize, maxSize)` | Byte slices |
| `Just(value)` | Constant generator |
| `SampledFrom(values)` | Uniform pick from a slice |
| `MustSampledFrom(values)` | Like SampledFrom, panics if empty |
| `Lists(elements, opts)` | Slices of generated elements |
| `Dicts(keys, values, opts)` | Maps of generated key-value pairs |
| `Tuples2/3/4(g1, g2, ...)` | Fixed-length tuples |
| `OneOf(g1, g2, ...)` | Union of generators |
| `Optional(g)` | Either nil or a value from g |
| `IPAddresses(opts)` | IPv4 or IPv6 address strings |

All generators implement `.Map(fn)` to transform their output.

### Test control (inside test body)

| Function | Description |
|---|---|
| `Assume(condition)` | Discard test case if condition is false |
| `Note(message)` | Print message only when replaying the failing case |
| `Target(value, label)` | Guide Hegel toward interesting values |

## Example programs

See the [`examples/`](examples/) directory:

- [`examples/basic/`](examples/basic/) — primitive generators and property assertions
- [`examples/collections/`](examples/collections/) — lists, dicts, and combinators
- [`examples/strings/`](examples/strings/) — a real-world string-processing scenario

## Documentation

```bash
go doc github.com/antithesishq/hegel-go          # package overview
go doc github.com/antithesishq/hegel-go Generator # specific type
```

For a step-by-step tutorial, see [docs/getting-started.md](docs/getting-started.md).

## Build commands

```bash
just setup       # Install dependencies (hegel binary + Go tools)
just check       # Full CI: lint + docs + tests with 100% coverage
just test        # Run tests only
just lint        # Format check + go vet + staticcheck
just conformance # Run cross-language conformance tests
```
