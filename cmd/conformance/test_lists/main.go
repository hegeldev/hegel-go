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

	var minValPtr, maxValPtr *int64

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
			n := int64(x)
			minValPtr = &n
		}
	}
	if v, ok := params["max_value"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			n := int64(x)
			maxValPtr = &n
		}
	}

	var elemGen hegel.Generator
	elemGen = hegel.IntegersFrom(minValPtr, maxValPtr)

	// When running collection StopTest modes, force the collection protocol
	// by wrapping the element generator with a no-op Filter. This makes it
	// non-basic, so Lists uses new_collection/collection_more instead of a
	// single generate command with a list schema.
	testMode := os.Getenv("HEGEL_PROTOCOL_TEST_MODE")
	if strings.Contains(testMode, "collection") {
		elemGen = elemGen.Filter(func(any) bool { return true })
	}

	opts := hegel.ListsOptions{
		MinSize: minSize,
		MaxSize: maxSize,
	}
	gen := hegel.Lists(elemGen, opts)
	n := hegel.GetTestCases()

	hegel.RunHegelTest("conformance_lists", func() {
		raw := hegel.Draw(gen)
		items, _ := raw.([]any)
		size := len(items)

		var minElem, maxElem any
		if size > 0 {
			minVal := int64(math.MaxInt64)
			maxVal := int64(math.MinInt64)
			for _, item := range items {
				v, _ := hegel.ExtractInt(item)
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

		hegel.WriteMetrics(map[string]any{
			"size":        size,
			"min_element": minElem,
			"max_element": maxElem,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
