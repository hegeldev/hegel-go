// test_booleans is a conformance binary for boolean generation.
// It parses JSON params from argv[1], runs a hegel test, and writes
// boolean metrics to CONFORMANCE_METRICS_FILE for each generated value.
package main

import (
	"os"

	hegel "github.com/antithesishq/hegel-go"
	"github.com/antithesishq/hegel-go/internal/conformance"
)

func main() {
	n := conformance.GetTestCases()
	hegel.MustRun("conformance_booleans", func(s *hegel.TestCase) {
		v := hegel.Draw(s, hegel.Booleans())
		conformance.WriteMetrics(map[string]any{
			"value": v,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
