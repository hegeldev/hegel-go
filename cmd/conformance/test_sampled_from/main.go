// test_sampled_from is a conformance binary for sampled_from generation.
// It parses JSON params from argv[1] (options: []int), runs a hegel test,
// and writes sampled_from metrics to CONFORMANCE_METRICS_FILE.
package main

import (
	"encoding/json"
	"os"

	hegel "github.com/antithesishq/hegel-go"
	"github.com/antithesishq/hegel-go/internal/conformance"
)

func main() {
	params := map[string]any{}
	if len(os.Args) > 1 {
		if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
			panic("test_sampled_from: bad params JSON: " + err.Error())
		}
	}

	// options is a list of integers
	var options []any
	if v, ok := params["options"]; ok {
		if arr, ok := v.([]any); ok {
			options = arr
		}
	}
	if len(options) == 0 {
		// Default fallback: use [0,1,2]
		options = []any{any(int64(0)), any(int64(1)), any(int64(2))}
	}

	// Convert to int64 values
	int64Options := make([]int64, len(options))
	for i, o := range options {
		switch x := o.(type) {
		case float64:
			int64Options[i] = int64(x)
		case int64:
			int64Options[i] = x
		default:
			int64Options[i] = 0
		}
	}

	gen := hegel.SampledFrom(int64Options)
	n := conformance.GetTestCases()
	hegel.MustRun("conformance_sampled_from", func(s *hegel.TestCase) {
		val := hegel.Draw(s, gen)
		conformance.WriteMetrics(map[string]any{
			"value": val,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
