**Important:** Hegel is in beta. We'd love for you to try it and
[report any feedback](https://github.com/hegeldev/hegel-go/issues/new).
We may make breaking changes during the beta if it makes Hegel a better
property-based testing library. See https://hegel.dev/compatibility for details.

# Hegel for Go

[![Go Reference](https://pkg.go.dev/badge/hegel.dev/go/hegel.svg)](https://pkg.go.dev/hegel.dev/go/hegel)

`hegel-go` is a property-based testing library for Go, based on [Hypothesis](https://github.com/hypothesisworks/hypothesis) using the [Hegel](https://hegel.dev/) protocol.

## Installation

```bash
go get hegel.dev/go/hegel@latest
```

Hegel requires either:

* [`uv`](https://docs.astral.sh/uv/) on your system,
* or `HEGEL_SERVER_COMMAND` set to the path of a hegel-core binary.

## Quick example

```go
package example_test

import (
	"testing"

	"hegel.dev/go/hegel"
)

func TestIntegers(t *testing.T) {
	t.Run("integers", hegel.Case(func(ht *hegel.T) {
		n := hegel.Draw(ht, hegel.Integers(0, 200))
		if n >= 50 {
			ht.Fatalf("n=%d is too large", n)
		}
	}))
}
```

See the [full documentation](https://pkg.go.dev/hegel.dev/go/hegel) for a
getting-started guide, generator reference, and API docs.
