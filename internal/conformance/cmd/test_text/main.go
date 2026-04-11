// test_text is a conformance binary for text/string generation.
// It parses JSON params from argv[1] (min_size, max_size) and writes text metrics.
package main

import (
	"encoding/json"
	"os"
	"unicode/utf8"

	hegel "hegel.dev/go/hegel"
	"hegel.dev/go/hegel/internal/conformance"
)

func main() {
	params := map[string]any{}
	if len(os.Args) > 1 {
		if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
			panic("test_text: bad params JSON: " + err.Error())
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

	n := conformance.GetTestCases()
	hegel.MustRun(func(s *hegel.TestCase) {
		val := hegel.Draw(s, hegel.Text(minSize, maxSize))
		// Count Unicode codepoints (not bytes)
		length := utf8.RuneCountInString(val)
		conformance.WriteMetrics(map[string]any{
			"length": length,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
