package main

import (
	"os"

	hegel "hegel.dev/go/hegel"
	"hegel.dev/go/hegel/internal/conformance"
)

func main() {
	n := conformance.GetTestCases()
	hegel.MustRun(func(s *hegel.TestCase) {
		defer conformance.EnsureMetric()
		v := hegel.Draw(s, hegel.Booleans())
		conformance.WriteMetrics(map[string]any{
			"value": v,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
