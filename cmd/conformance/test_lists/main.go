// test_lists is a conformance binary for list generation.
// It parses JSON params from argv[1] (min_size, max_size, min_value, max_value)
// and writes list metrics (size, min_element, max_element).
package main

import (
	"encoding/json"
	"math"
	"os"

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

	elemGen := hegel.IntegersFrom(minValPtr, maxValPtr)
	opts := hegel.ListsOptions{
		MinSize: minSize,
		MaxSize: maxSize,
	}
	gen := hegel.Lists(elemGen, opts)
	n := hegel.GetTestCases()

	hegel.RunHegelTest("conformance_lists", func() {
		raw := gen.Generate()
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
