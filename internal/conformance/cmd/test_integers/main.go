package main

import (
	"encoding/json"
	"math"
	"os"

	hegel "hegel.dev/go/hegel"
	"hegel.dev/go/hegel/internal/conformance"
)

func main() {
	if len(os.Args) <= 1 {
		panic("test_integers: missing params JSON argument")
	}
	var params struct {
		MinValue *int `json:"min_value"`
		MaxValue *int `json:"max_value"`
	}
	if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
		panic("test_integers: bad params JSON: " + err.Error())
	}

	minVal := math.MinInt
	maxVal := math.MaxInt
	if params.MinValue != nil {
		minVal = *params.MinValue
	}
	if params.MaxValue != nil {
		maxVal = *params.MaxValue
	}

	gen := hegel.Integers[int](minVal, maxVal)
	n := conformance.GetTestCases()
	hegel.MustRun(func(s *hegel.TestCase) {
		val := hegel.Draw(s, gen)
		conformance.WriteMetrics(map[string]any{
			"value": val,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
