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
			panic("test_text: bad params JSON: " + err.Error())
		}
	}

	minSize := 0
	maxSize := -1

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

	intKeys := map[string]bool{
		"min_codepoint": true,
		"max_codepoint": true,
	}

	alphabetParams := map[string]any{}
	for k, v := range params {
		if k == "min_size" || k == "max_size" || k == "mode" {
			continue
		}
		if intKeys[k] {
			if f, ok := v.(float64); ok {
				alphabetParams[k] = int64(f)
				continue
			}
		}
		alphabetParams[k] = v
	}

	var gen hegel.Generator[string]
	if len(alphabetParams) > 0 {
		gen = hegel.TextWithAlphabet(minSize, maxSize, alphabetParams)
	} else {
		gen = hegel.Text(minSize, maxSize)
	}

	n := conformance.GetTestCases()
	hegel.MustRun(func(s *hegel.TestCase) {
		val := hegel.Draw(s, gen)
		codepoints := make([]any, 0, len(val))
		for _, r := range val {
			codepoints = append(codepoints, int64(r))
		}
		conformance.WriteMetrics(map[string]any{
			"codepoints": codepoints,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
