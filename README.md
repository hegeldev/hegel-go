# hegel-go

> [!IMPORTANT]
> If you've found this repository, congratulations! You're getting a sneak peek at an upcoming property-based testing library from [Antithesis](https://antithesis.com/), built on [Hypothesis](https://hypothesis.works/).
>
> We are still making rapid changes and progress.  Feel free to experiment, but don't expect stability from Hegel just yet!

## Installation

Install the `hegel-go` library:

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
        a, _ := hegel.Draw(t, hegel.Integers(-1000, 1000))
        b, _ := hegel.Draw(t, hegel.Integers(-1000, 1000))
        if a+b != b+a {
            t.Fatal("addition is not commutative!")
        }
    }))
}
```

See [docs/getting-started.md](docs/getting-started.md) for more on how to use Hegel.