> [!IMPORTANT]
> We're excited you're checking out Hegel! Hegel is in beta, and we'd love for you to try it and [report any feedback](https://github.com/hegeldev/hegel-go/issues/new).
>
> As part of our beta, we may make breaking changes if it makes Hegel a better property-based testing library. If that instability bothers you, please check back in a few months for a stable release!
>
> See https://hegel.dev/compatibility for more details.

# Hegel for Go

* [Documentation](https://pkg.go.dev/hegel.dev/go/hegel)
* [Hegel website](https://hegel.dev)

`hegel-go` is a property-based testing library for Go. `hegel-go` is based on [Hypothesis](https://github.com/hypothesisworks/hypothesis), using the [Hegel](https://hegel.dev/) protocol.

## Installation

To install: `go get hegel.dev/go/hegel@latest`.

Hegel will use [uv](https://docs.astral.sh/uv/) to install the required [hegel-core](https://github.com/hegeldev/hegel-core) server component. If `uv` is already on your path, it will use that, otherwise it will download a private copy of it to ~/.cache/hegel and not put it on your path. See https://hegel.dev/reference/installation for details.

## Quickstart

Here's a quick example of how to write a Hegel test:

```go
package example_test

import (
	"slices"
	"testing"

	"hegel.dev/go/hegel"
)

func mySort(ls []int) []int {
	result := make([]int, len(ls))
	copy(result, ls)
	slices.Sort(result)
	result = slices.Compact(result)
	return result
}

func TestMatchesBuiltin(t *testing.T) {
	t.Run("matches builtin", hegel.Case(func(ht *hegel.T) {
		slice1 := hegel.Draw(ht, hegel.Lists(hegel.Integers[int]()))
		slice2 := mySort(slice1)
		slices.Sort(slice1)
		if !slices.Equal(slice1, slice2) {
			ht.Fatalf("slices not equal: %v != %v", slice1, slice2)
		}
	}))
}
```

This test will fail when run with `go test`! Hegel will produce a minimal failing test case for us:

```
Draw 1: [0 0]
    example_test.go:25: slices not equal: [0 0] != [0]
```

Hegel reports the minimal example showing that our sort is incorrectly dropping duplicates. If we remove `result = slices.Compact(result)` from `mySort()`, this test will then pass (because it's just comparing the standard sort against itself).
