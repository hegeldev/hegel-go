package main

import (
	"encoding/json"
	"os"

	hegel "hegel.dev/go/hegel"
	"hegel.dev/go/hegel/internal/conformance"
)

func main() {
	if len(os.Args) <= 1 {
		panic("test_oneof: missing params JSON argument")
	}
	var params struct {
		Mode   string `json:"mode"`
		Ranges []struct {
			MinValue int `json:"min_value"`
			MaxValue int `json:"max_value"`
		} `json:"ranges"`
	}
	if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
		panic("test_oneof: bad params JSON: " + err.Error())
	}
	if len(params.Ranges) == 0 {
		panic("test_oneof: missing or empty ranges")
	}

	gens := make([]hegel.Generator[int], len(params.Ranges))
	switch params.Mode {
	case "basic":
		for i, r := range params.Ranges {
			gens[i] = hegel.Integers[int](r.MinValue, r.MaxValue)
		}
	case "map_negate":
		for i, r := range params.Ranges {
			base := hegel.Integers[int](r.MinValue, r.MaxValue)
			gens[i] = hegel.Map(base, func(v int) int { return -v })
		}
	case "filter_even":
		for i, r := range params.Ranges {
			base := hegel.Integers[int](r.MinValue, r.MaxValue)
			gens[i] = hegel.Filter(base, func(v int) bool { return v%2 == 0 })
		}
	default:
		panic("test_oneof: unknown mode: " + params.Mode)
	}

	gen := hegel.OneOf(gens...)

	n := conformance.GetTestCases()
	hegel.MustRun(func(s *hegel.TestCase) {
		defer conformance.EnsureMetric()
		val := hegel.Draw(s, gen)
		conformance.WriteMetrics(map[string]any{
			"value": val,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
