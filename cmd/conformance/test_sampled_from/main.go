// test_sampled_from is a conformance binary for sampled_from generation.
// It parses JSON params from argv[1] (options: []int), runs a hegel test,
// and writes sampled_from metrics to CONFORMANCE_METRICS_FILE.
package main

import (
	"encoding/json"
	"os"

	hegel "github.com/antithesishq/hegel-go"
)

func main() {
	params := map[string]any{}
	if len(os.Args) > 1 {
		if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
			panic("test_sampled_from: bad params JSON: " + err.Error())
		}
	}

	// options is a list of integers
	var rawOptions []any
	if v, ok := params["options"]; ok {
		if arr, ok := v.([]any); ok {
			rawOptions = arr
		}
	}

	// Convert float64 JSON values to []int64
	var intOptions []int64
	if len(rawOptions) == 0 {
		// Default fallback: use [0,1,2]
		intOptions = []int64{0, 1, 2}
	} else {
		intOptions = make([]int64, len(rawOptions))
		for i, o := range rawOptions {
			switch x := o.(type) {
			case float64:
				intOptions[i] = int64(x)
			}
		}
	}

	gen := hegel.MustSampledFrom(intOptions)
	n := hegel.GetTestCases()
	hegel.RunHegelTest("conformance_sampled_from", func() {
		val := hegel.Draw(gen)
		hegel.WriteMetrics(map[string]any{
			"value": val,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
