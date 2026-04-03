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

Hegel will use [uv](https://docs.astral.sh/uv/) to install the required [hegel-core](https://github.com/hegeldev/hegel-core) server component. If `uv` is already on your path, it will use that, otherwise it will download a private copy of it to `~/.cache/hegel` and not put it on your path. See https://hegel.dev/reference/installation for details.

## Quick example

```go
package example_test

import (
	"testing"

	"hegel.dev/go/hegel"
)

func TestIntegers(t *testing.T) {
	t.Run("integers", hegel.Case(func(ht *hegel.T) {
		n := hegel.Draw(ht, hegel.Integers[int]().Min(0).Max(200))
		if n >= 50 {
			ht.Fatalf("n=%d is too large", n)
		}
	}))
}
```

See the [full documentation](https://pkg.go.dev/hegel.dev/go/hegel) for a getting-started guide, generator reference, and API docs.
