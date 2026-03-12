// test_integers is a conformance binary for integer generation.
// It parses JSON params from argv[1] (min_value, max_value),
// runs a hegel test, and writes integer metrics to CONFORMANCE_METRICS_FILE.
package main

import (
	"encoding/json"
	"math"
	"os"

	hegel "github.com/hegeldev/hegel-go"
	"github.com/hegeldev/hegel-go/internal/conformance"
)

func main() {
	// Parse params from argv[1]
	params := map[string]any{}
	if len(os.Args) > 1 {
		if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
			panic("test_integers: bad params JSON: " + err.Error())
		}
	}

	minVal := math.MinInt
	maxVal := math.MaxInt
	if v, ok := params["min_value"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			minVal = int(x)
		}
	}
	if v, ok := params["max_value"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			maxVal = int(x)
		}
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
