# hegel-go

> [!IMPORTANT]
> If you've found this repository, congratulations! You're getting a sneak peak at an upcoming property-based testing library from [Antithesis](https://antithesis.com/), built on [Hypothesis](https://hypothesis.works/).
>
> We are still making rapid changes and progress.  Feel free to experiment, but don't expect stability from Hegel just yet!

## Installation

```bash
go get github.com/hegeldev/hegel-go@latest
```

### Hegel server

The SDK automatically manages the `hegel` server binary. On first use it
creates a project-local `.hegel/venv` virtualenv and installs the pinned
version of [hegel-core](https://github.com/antithesishq/hegel-core) into it.
Subsequent runs reuse the cached binary unless the pinned version changes.

To use your own `hegel` binary instead (e.g. a local development build), set
the `HEGEL_CMD` environment variable:

```bash
export HEGEL_CMD=/path/to/hegel
```

The SDK requires [`uv`](https://docs.astral.sh/uv/) to be installed for
automatic server management.

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

See [docs/getting-started.md](docs/getting-started.md) for more.

## Development

```bash
just setup       # install dependencies
just test        # run tests
just check       # run PR checks: lint + tests + docs
```
