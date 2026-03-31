**Important:** Hegel is in beta. We'd love for you to try it and
[report any feedback](https://github.com/hegeldev/hegel-go/issues/new).
We may make breaking changes during the beta if it makes Hegel a better
property-based testing library. See https://hegel.dev/compatibility for details.

# Hegel for Go

* [Documentation](https://pkg.go.dev/hegel.dev/go/hegel)
* [Hegel website](https://hegel.dev)

`hegel-go` is a property-based testing library for Go. `hegel-go` is based on [Hypothesis](https://github.com/hypothesisworks/hypothesis), using the [Hegel](https://hegel.dev/) protocol.

## Installation

```bash
go get hegel.dev/go/hegel@latest
```

Hegel requires either:

* [`uv`](https://docs.astral.sh/uv/) on your system,
* or `HEGEL_SERVER_COMMAND` set to the path of a hegel-core binary.

## Quickstart

Here's a quick example of how to write a Hegel test:

```go
package mypackage_test

import (
    "testing"

    "hegel.dev/go/hegel"
)

func TestAddCommutative(t *testing.T) {
    t.Run("add_commutative", hegel.Case(func(t *hegel.T) {
        a := hegel.Draw(t, hegel.Integers(-1000, 1000))
        b := hegel.Draw(t, hegel.Integers(-1000, 1000))
        if a+b != b+a {
            t.Fatal("addition is not commutative!")
        }
    }))
}
```

See the [full documentation](https://pkg.go.dev/hegel.dev/go/hegel) for a complete getting-started guide.
