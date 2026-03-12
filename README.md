# hegel-go

A Go SDK for [Hegel](https://github.com/hegeldev/hegel-core) — universal
property-based testing powered by [Hypothesis](https://hypothesis.works/).

Hegel generates random inputs for your tests, finds failures, and automatically
shrinks them to minimal counterexamples.

## Installation

```bash
go get github.com/hegeldev/hegel-go@latest
```

The SDK requires the `hegel` CLI on your PATH:

```bash
pip install "hegel @ git+https://github.com/hegeldev/hegel-core"
```

## Quick Start

```go
package mypackage_test

import (
    "testing"

    hegel "github.com/hegeldev/hegel-go"
)

func TestAddCommutative(t *testing.T) {
    t.Run("add_commutative", hegel.Case(func(t *hegel.T) {
        a, _ := hegel.Draw(t, hegel.Integers(-1000, 1000))
        b, _ := hegel.Draw(t, hegel.Integers(-1000, 1000))
        if a+b != b+a {
            t.Fatal("addition is not commutative!")
        }
    }))
}
```

Run with `go test` as normal. Hegel by default generates 100 random input pairs
and reports the minimal counterexample if it finds one.

For a full walkthrough, see [docs/getting-started.md](docs/getting-started.md).

## Development

```bash
just setup       # Install dependencies (hegel binary + Go tools)
just check       # Full CI: lint + docs + tests with 100% coverage
just test        # Run tests only
just conformance # Run cross-language conformance tests
```
