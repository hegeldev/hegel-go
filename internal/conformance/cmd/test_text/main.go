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

	g := hegel.Text()

	if v, ok := params["min_size"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			g = g.MinSize(int(x))
		}
	}
	if v, ok := params["max_size"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			g = g.MaxSize(int(x))
		}
	}
	if v, ok := params["codec"].(string); ok {
		g = g.Codec(v)
	}
	if v, ok := params["min_codepoint"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			g = g.MinCodepoint(int64(x))
		}
	}
	if v, ok := params["max_codepoint"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			g = g.MaxCodepoint(int64(x))
		}
	}
	if v, ok := params["categories"]; ok && v != nil {
		if arr, ok := v.([]any); ok {
			cats := make([]string, 0, len(arr))
			for _, c := range arr {
				if s, ok := c.(string); ok {
					cats = append(cats, s)
				}
			}
			g = g.Categories(cats...)
		}
	}
	if v, ok := params["exclude_categories"]; ok && v != nil {
		if arr, ok := v.([]any); ok {
			cats := make([]string, 0, len(arr))
			for _, c := range arr {
				if s, ok := c.(string); ok {
					cats = append(cats, s)
				}
			}
			g = g.ExcludeCategories(cats...)
		}
	}
	if v, ok := params["include_characters"].(string); ok {
		g = g.IncludeCharacters(v)
	}
	if v, ok := params["exclude_characters"].(string); ok {
		g = g.ExcludeCharacters(v)
	}

	n := conformance.GetTestCases()
	hegel.MustRun(func(s *hegel.TestCase) {
		val := hegel.Draw(s, g)
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
