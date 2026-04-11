// test_lists is a conformance binary for list generation.
// It parses JSON params from argv[1] (min_size, max_size, min_value, max_value)
// and writes the raw list of elements as metrics.
//
// When HEGEL_PROTOCOL_TEST_MODE contains "collection" or mode is "non_basic",
// the element generator is wrapped with a no-op Filter to force the collection
// protocol path.
package main

import (
	"encoding/json"
	"math"
	"os"
	"strings"

	hegel "hegel.dev/go/hegel"
	"hegel.dev/go/hegel/internal/conformance"
)

func main() {
	params := map[string]any{}
	if len(os.Args) > 1 {
		if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
			panic("test_lists: bad params JSON: " + err.Error())
		}
	}
	mode := conformance.GetMode(params)

	minSize := 0
	maxSize := -1 // unbounded

	elemMinVal := math.MinInt
	elemMaxVal := math.MaxInt

	if v, ok := params["min_size"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			minSize = int(x)
		}
	}
	if v, ok := params["max_size"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			maxSize = int(x)
		}
	}
	if v, ok := params["min_value"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			elemMinVal = int(x)
		}
	}
	if v, ok := params["max_value"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			elemMaxVal = int(x)
		}
	}

	elemGen := hegel.Integers[int](elemMinVal, elemMaxVal)

	// When running collection StopTest modes or non_basic mode, force the
	// collection protocol by wrapping the element generator with a no-op
	// Filter. This makes it non-basic, so Lists uses
	// new_collection/collection_more instead of a single generate command
	// with a list schema.
	testMode := os.Getenv("HEGEL_PROTOCOL_TEST_MODE")
	needsNonBasic := mode == "non_basic" || strings.Contains(testMode, "collection")

	var gen hegel.Generator[[]int]
	if needsNonBasic {
		filtered := conformance.MakeNonBasic(elemGen)
		builder := hegel.Lists(filtered).MinSize(minSize)
		if maxSize >= 0 {
			builder = builder.MaxSize(maxSize)
		}
		gen = builder
	} else {
		builder := hegel.Lists(elemGen).MinSize(minSize)
		if maxSize >= 0 {
			builder = builder.MaxSize(maxSize)
		}
		gen = builder
	}

	n := conformance.GetTestCases()

	hegel.MustRun(func(s *hegel.TestCase) {
		items := hegel.Draw(s, gen)
		conformance.WriteMetrics(map[string]any{
			"elements": items,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
