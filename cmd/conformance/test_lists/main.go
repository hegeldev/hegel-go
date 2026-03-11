// test_lists is a conformance binary for list generation.
// It parses JSON params from argv[1] (min_size, max_size, min_value, max_value)
// and writes list metrics (size, min_element, max_element).
//
// When HEGEL_PROTOCOL_TEST_MODE contains "collection", the element generator
// is wrapped with a no-op Filter to force the collection protocol path.
package main

import (
	"encoding/json"
	"math"
	"os"
	"strings"

	hegel "github.com/antithesishq/hegel-go"
	"github.com/antithesishq/hegel-go/internal/conformance"
)

func main() {
	params := map[string]any{}
	if len(os.Args) > 1 {
		if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
			panic("test_lists: bad params JSON: " + err.Error())
		}
	}

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

	// When running collection StopTest modes, force the collection protocol
	// by wrapping the element generator with a no-op Filter. This makes it
	// non-basic, so Lists uses new_collection/collection_more instead of a
	// single generate command with a list schema.
	testMode := os.Getenv("HEGEL_PROTOCOL_TEST_MODE")
	var gen hegel.Generator[[]int]
	if strings.Contains(testMode, "collection") {
		filtered := hegel.Filter(elemGen, func(v int) bool { return true })
		gen = hegel.Lists(filtered, hegel.ListMinSize(minSize), hegel.ListMaxSize(maxSize))
	} else {
		gen = hegel.Lists(elemGen, hegel.ListMinSize(minSize), hegel.ListMaxSize(maxSize))
	}

	n := conformance.GetTestCases()

	hegel.MustRun(func(s *hegel.TestCase) {
		items := hegel.Draw(s, gen)
		size := len(items)

		var minElem, maxElem any
		if size > 0 {
			minVal := math.MaxInt
			maxVal := math.MinInt
			for _, v := range items {
				if v < minVal {
					minVal = v
				}
				if v > maxVal {
					maxVal = v
				}
			}
			minElem = minVal
			maxElem = maxVal
		}

		conformance.WriteMetrics(map[string]any{
			"size":        size,
			"min_element": minElem,
			"max_element": maxElem,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
