// test_floats is a conformance binary for float generation.
// It parses JSON params from argv[1] and writes float metrics.
package main

import (
	"encoding/json"
	"math"
	"os"

	hegel "github.com/antithesishq/hegel-go"
	"github.com/antithesishq/hegel-go/internal/conformance"
)

func main() {
	params := map[string]any{}
	if len(os.Args) > 1 {
		if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
			panic("test_floats: bad params JSON: " + err.Error())
		}
	}

	var minPtr, maxPtr *float64
	var allowNaN, allowInfinity *bool

	if v, ok := params["min_value"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			minPtr = &x
		}
	}
	if v, ok := params["max_value"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			maxPtr = &x
		}
	}
	if v, ok := params["allow_nan"]; ok && v != nil {
		if x, ok := v.(bool); ok {
			allowNaN = &x
		}
	}
	if v, ok := params["allow_infinity"]; ok && v != nil {
		if x, ok := v.(bool); ok {
			allowInfinity = &x
		}
	}

	excludeMin := false
	excludeMax := false
	if v, ok := params["exclude_min"]; ok {
		if x, ok := v.(bool); ok {
			excludeMin = x
		}
	}
	if v, ok := params["exclude_max"]; ok {
		if x, ok := v.(bool); ok {
			excludeMax = x
		}
	}

	gen := hegel.Floats(minPtr, maxPtr, allowNaN, allowInfinity, excludeMin, excludeMax)
	n := conformance.GetTestCases()
	hegel.MustRun(func(s *hegel.TestCase) {
		val := hegel.Draw(s, gen)
		isNaN := math.IsNaN(val)
		isInfinite := math.IsInf(val, 0)
		m := map[string]any{
			"is_nan":      isNaN,
			"is_infinite": isInfinite,
		}
		if !isNaN && !isInfinite {
			m["value"] = val
		} else {
			m["value"] = nil
		}
		conformance.WriteMetrics(m)
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
