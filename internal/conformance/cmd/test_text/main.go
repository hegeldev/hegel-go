// test_text is a conformance binary for text/string generation.
// It parses JSON params from argv[1] (min_size, max_size, and character
// filtering options) and writes codepoint metrics.
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

	g := hegel.Text(minSize, maxSize)

	if v, ok := params["codec"]; ok && v != nil {
		if x, ok := v.(string); ok {
			g = g.Codec(x)
		}
	}
	if v, ok := params["min_codepoint"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			g = g.MinCodepoint(rune(x))
		}
	}
	if v, ok := params["max_codepoint"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			g = g.MaxCodepoint(rune(x))
		}
	}
	if v, ok := params["categories"]; ok && v != nil {
		if arr, ok := v.([]any); ok {
			cats := make([]string, len(arr))
			for i, c := range arr {
				cats[i] = c.(string)
			}
			g = g.Categories(cats)
		}
	}
	if v, ok := params["exclude_categories"]; ok && v != nil {
		if arr, ok := v.([]any); ok {
			cats := make([]string, len(arr))
			for i, c := range arr {
				cats[i] = c.(string)
			}
			g = g.ExcludeCategories(cats)
		}
	}
	if v, ok := params["include_characters"]; ok && v != nil {
		if x, ok := v.(string); ok {
			g = g.IncludeCharacters(x)
		}
	}
	if v, ok := params["exclude_characters"]; ok && v != nil {
		if x, ok := v.(string); ok {
			g = g.ExcludeCharacters(x)
		}
	}

	gen := g
	n := conformance.GetTestCases()
	hegel.MustRun(func(s *hegel.TestCase) {
		val := hegel.Draw(s, gen)
		codepoints := make([]int, 0, len(val))
		for _, r := range val {
			codepoints = append(codepoints, int(r))
		}
		conformance.WriteMetrics(map[string]any{
			"codepoints": codepoints,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
