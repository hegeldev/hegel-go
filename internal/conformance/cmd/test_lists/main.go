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
	if len(os.Args) <= 1 {
		panic("test_lists: missing params JSON argument")
	}
	var params struct {
		MinSize  int    `json:"min_size"`
		MaxSize  *int   `json:"max_size"`
		MinValue *int   `json:"min_value"`
		MaxValue *int   `json:"max_value"`
		Mode     string `json:"mode"`
	}
	if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
		panic("test_lists: bad params JSON: " + err.Error())
	}

	testMode := os.Getenv("HEGEL_PROTOCOL_TEST_MODE")
	useNonBasic := params.Mode == "non_basic" || strings.Contains(testMode, "collection")

	minVal := math.MinInt
	maxVal := math.MaxInt
	if params.MinValue != nil {
		minVal = *params.MinValue
	}
	if params.MaxValue != nil {
		maxVal = *params.MaxValue
	}
	elemGen := hegel.Integers[int](minVal, maxVal)

	var gen hegel.Generator[[]int]
	if useNonBasic {
		filtered := hegel.Filter(elemGen, func(v int) bool { return true })
		builder := hegel.Lists(filtered).MinSize(params.MinSize)
		if params.MaxSize != nil {
			builder = builder.MaxSize(*params.MaxSize)
		}
		gen = builder
	} else {
		builder := hegel.Lists(elemGen).MinSize(params.MinSize)
		if params.MaxSize != nil {
			builder = builder.MaxSize(*params.MaxSize)
		}
		gen = builder
	}

	n := conformance.GetTestCases()
	hegel.MustRun(func(s *hegel.TestCase) {
		defer conformance.EnsureMetric()
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
