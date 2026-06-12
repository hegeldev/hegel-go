> [!IMPORTANT]
> We're excited you're checking out Hegel! Hegel is in beta, and we'd love for you to try it and [report any feedback](https://github.com/hegeldev/hegel-go/issues/new).
>
> As part of our beta, we may make breaking changes if it makes Hegel a better property-based testing library. If that instability bothers you, please check back in a few months for a stable release!
>
> See https://hegel.dev/compatibility for more details.

# Hegel for Go

* [Documentation](https://pkg.go.dev/hegel.dev/go/hegel)
* [Hegel website](https://hegel.dev)

Hegel is a property-based testing library for Go. Hegel is based on [Hypothesis](https://github.com/hypothesisworks/hypothesis), using the [Hegel protocol](https://hegel.dev/).

## Installation

To install: `go get hegel.dev/go/hegel@latest`.

Hegel for Go drives [libhegel](https://github.com/hegeldev/hegel-rust) — the native Rust engine. At runtime hegel-go looks for `libhegel.so` (Linux), `libhegel.dylib` (macOS), or `libhegel.dll` (Windows) at `$HEGEL_LIBHEGEL_PATH` first, then in a sibling `../hegel-rust/target/release/` (and `../hegel-rust/target/debug/`) checkout relative to your project root, and finally falls back to downloading the matching version from hegel-rust's GitHub releases on first use (cached under `~/.cache/hegel-go/libhegel/<version>/`, SHA-256 verified). Set `HEGEL_LIBHEGEL_NO_DOWNLOAD=1` to opt out of the download fallback.

Supported platforms (those with a published libhegel artifact): Linux amd64/arm64, macOS arm64 (Apple Silicon), and Windows amd64/arm64.

During development, build the library locally:

```
just build-libhegel    # cargo build --release -p hegeltest-c in ../hegel-rust/
```

## Quickstart

Here's a quick example of how to write a Hegel test:

```go
package example_test

import (
	"math"
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
	hegel.Test(t, func(ht *hegel.T) {
		slice1 := hegel.Draw(ht, hegel.Lists(hegel.Integers(math.MinInt, math.MaxInt)))
		slice2 := mySort(slice1)
		slices.Sort(slice1)
		if !slices.Equal(slice1, slice2) {
			ht.Fatalf("slices not equal: %v != %v", slice1, slice2)
		}
	})
}
```

This test will fail when run with `go test`! Hegel will produce a minimal failing test case for us:

```
example_test.go:46: slice1 := hegel.Draw(ht, hegel.Lists(hegel.Integers(math.MinInt, math.MaxInt))) = []int{0, 0}
example_test.go:50: slices not equal: [0 0] != [0]
```

Hegel reports the minimal example showing that our sort is incorrectly dropping duplicates. If we remove `result = slices.Compact(result)` from `mySort()`, this test will then pass (because it's just comparing the standard sort against itself).
