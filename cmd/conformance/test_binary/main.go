// test_binary is a conformance binary for binary/byte-slice generation.
// It parses JSON params from argv[1] (min_size, max_size) and writes binary metrics.
package main

import (
	"encoding/json"
	"os"

	hegel "github.com/hegeldev/hegel-go"
	"github.com/hegeldev/hegel-go/internal/conformance"
)

func main() {
	params := map[string]any{}
	if len(os.Args) > 1 {
		if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
			panic("test_binary: bad params JSON: " + err.Error())
		}
	}

	minSize := 0
	maxSize := -1 // unbounded

	if v, ok := params["min_size"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			minSize = int(x)
		}
	}
	if v, ok := params["max_size"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			maxSize = int(x)
		}
	}

	gen := hegel.Binary(minSize, maxSize)
	n := conformance.GetTestCases()
	hegel.MustRun(func(s *hegel.TestCase) {
		v := hegel.Draw(s, gen)
		length := len(v)
		conformance.WriteMetrics(map[string]any{
			"length": length,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
