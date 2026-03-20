// test_floats is a conformance binary for float generation.
// It parses JSON params from argv[1] and writes float metrics.
package main

import (
	"encoding/json"
	"math"
	"os"

	hegel "github.com/hegeldev/hegel-go"
	"github.com/hegeldev/hegel-go/internal/conformance"
)

func main() {
	params := map[string]any{}
	if len(os.Args) > 1 {
		if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
			panic("test_floats: bad params JSON: " + err.Error())
		}
	}

	g := hegel.Floats[float64]()

	if v, ok := params["min_value"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			g = g.Min(x)
		}
	}
	if v, ok := params["max_value"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			g = g.Max(x)
		}
	}
	if v, ok := params["allow_nan"]; ok && v != nil {
		if x, ok := v.(bool); ok {
			g = g.AllowNaN(x)
		}
	}
	if v, ok := params["allow_infinity"]; ok && v != nil {
		if x, ok := v.(bool); ok {
			g = g.AllowInfinity(x)
		}
	}
	if v, ok := params["exclude_min"]; ok {
		if x, ok := v.(bool); ok && x {
			g = g.ExcludeMin()
		}
	}
	if v, ok := params["exclude_max"]; ok {
		if x, ok := v.(bool); ok && x {
			g = g.ExcludeMax()
		}
	}

	gen := g
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
