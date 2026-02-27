// test_text is a conformance binary for text/string generation.
// It parses JSON params from argv[1] (min_size, max_size) and writes text metrics.
package main

import (
	"encoding/json"
	"os"
	"unicode/utf8"

	hegel "github.com/antithesishq/hegel-go"
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

	gen := hegel.Text(minSize, maxSize)
	n := hegel.GetTestCases()
	hegel.RunHegelTest("conformance_text", func() {
		raw := gen.Generate()
		s, _ := hegel.ExtractString(raw)
		// Count Unicode codepoints (not bytes)
		length := utf8.RuneCountInString(s)
		hegel.WriteMetrics(map[string]any{
			"length": length,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
