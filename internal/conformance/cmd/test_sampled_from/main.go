// test_sampled_from is a conformance binary for sampled_from generation.
// It parses JSON params from argv[1] (options: []int), runs a hegel test,
// and writes sampled_from metrics to CONFORMANCE_METRICS_FILE.
package main

import (
	"encoding/json"
	"os"

	hegel "hegel.dev/go/hegel"
	"hegel.dev/go/hegel/internal/conformance"
)

func main() {
	if len(os.Args) <= 1 {
		panic("test_sampled_from: missing params JSON argument")
	}
	var params struct {
		Options []int64 `json:"options"`
	}
	if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
		panic("test_sampled_from: bad params JSON: " + err.Error())
	}

	gen := hegel.SampledFrom(params.Options)
	n := conformance.GetTestCases()
	hegel.MustRun(func(s *hegel.TestCase) {
		val := hegel.Draw(s, gen)
		conformance.WriteMetrics(map[string]any{
			"value": val,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
