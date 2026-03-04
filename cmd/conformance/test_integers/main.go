// test_integers is a conformance binary for integer generation.
// It parses JSON params from argv[1] (min_value, max_value),
// runs a hegel test, and writes integer metrics to CONFORMANCE_METRICS_FILE.
package main

import (
	"encoding/json"
	"os"

	hegel "github.com/antithesishq/hegel-go"
	"github.com/antithesishq/hegel-go/internal/conformance"
)

func main() {
	// Parse params from argv[1]
	params := map[string]any{}
	if len(os.Args) > 1 {
		if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
			panic("test_integers: bad params JSON: " + err.Error())
		}
	}

	var minPtr, maxPtr *int64
	if v, ok := params["min_value"]; ok && v != nil {
		switch x := v.(type) {
		case float64:
			n := int64(x)
			minPtr = &n
		}
	}
	if v, ok := params["max_value"]; ok && v != nil {
		switch x := v.(type) {
		case float64:
			n := int64(x)
			maxPtr = &n
		}
	}

	gen := hegel.IntegersFrom(minPtr, maxPtr)
	n := conformance.GetTestCases()
	hegel.MustRun("conformance_integers", func(s *hegel.TestCase) {
		val := hegel.Draw(s, gen)
		conformance.WriteMetrics(map[string]any{
			"value": val,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
