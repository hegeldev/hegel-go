package main

import (
	"encoding/json"
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

	minSize := 0
	maxSize := -1
	elemMinVal := -1000
	elemMaxVal := 1000
	unique := false

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
	if v, ok := params["unique"]; ok {
		if x, ok := v.(bool); ok {
			unique = x
		}
	}

	mode := "basic"
	if m, ok := params["mode"].(string); ok {
		mode = m
	}

	testMode := os.Getenv("HEGEL_PROTOCOL_TEST_MODE")
	useNonBasic := mode == "non_basic" || strings.Contains(testMode, "collection")

	elemGen := hegel.Integers[int](elemMinVal, elemMaxVal)

	var gen hegel.Generator[[]int]
	if useNonBasic && !unique {
		filtered := hegel.Filter(elemGen, func(v int) bool { return true })
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
		if unique {
			builder = builder.Unique()
		}
		gen = builder
	}

	n := conformance.GetTestCases()
	hegel.MustRun(func(s *hegel.TestCase) {
		items := hegel.Draw(s, gen)
		elements := make([]any, len(items))
		for i, v := range items {
			elements[i] = v
		}
		conformance.WriteMetrics(map[string]any{
			"elements": elements,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
