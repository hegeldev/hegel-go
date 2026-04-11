// test_booleans is a conformance binary for boolean generation.
// It parses JSON params from argv[1], runs a hegel test, and writes
// boolean metrics to CONFORMANCE_METRICS_FILE for each generated value.
package main

import (
	"encoding/json"
	"os"

	hegel "hegel.dev/go/hegel"
	"hegel.dev/go/hegel/internal/conformance"
)

func main() {
	params := map[string]any{}
	if len(os.Args) > 1 {
		if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
			panic("test_booleans: bad params JSON: " + err.Error())
		}
	}
	mode := conformance.GetMode(params)

	gen := hegel.Booleans()
	var finalGen hegel.Generator[bool]
	if mode == "non_basic" {
		finalGen = conformance.MakeNonBasic(gen)
	} else {
		finalGen = gen
	}

	n := conformance.GetTestCases()
	hegel.MustRun(func(s *hegel.TestCase) {
		v := hegel.Draw(s, finalGen)
		conformance.WriteMetrics(map[string]any{
			"value": v,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
