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
	if len(os.Args) <= 1 {
		panic("test_text: missing params JSON argument")
	}
	var params struct {
		MinSize           int      `json:"min_size"`
		MaxSize           *int     `json:"max_size"`
		Codec             *string  `json:"codec"`
		MinCodepoint      *int     `json:"min_codepoint"`
		MaxCodepoint      *int     `json:"max_codepoint"`
		Categories        []string `json:"categories"`
		ExcludeCategories []string `json:"exclude_categories"`
		IncludeCharacters *string  `json:"include_characters"`
		ExcludeCharacters *string  `json:"exclude_characters"`
	}
	if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
		panic("test_text: bad params JSON: " + err.Error())
	}

	g := hegel.Text().MinSize(params.MinSize)
	if params.MaxSize != nil {
		g = g.MaxSize(*params.MaxSize)
	}
	if params.Codec != nil {
		g = g.Codec(*params.Codec)
	}
	if params.MinCodepoint != nil {
		g = g.MinCodepoint(rune(*params.MinCodepoint))
	}
	if params.MaxCodepoint != nil {
		g = g.MaxCodepoint(rune(*params.MaxCodepoint))
	}
	if params.Categories != nil {
		g = g.Categories(params.Categories)
	}
	if params.ExcludeCategories != nil {
		g = g.ExcludeCategories(params.ExcludeCategories)
	}
	if params.IncludeCharacters != nil {
		g = g.IncludeCharacters(*params.IncludeCharacters)
	}
	if params.ExcludeCharacters != nil {
		g = g.ExcludeCharacters(*params.ExcludeCharacters)
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
