package main

import (
	"encoding/json"
	"fmt"
	"os"

	hegel "hegel.dev/go/hegel"
	"hegel.dev/go/hegel/internal/conformance"
)

func main() {
	params := map[string]any{}
	if len(os.Args) > 1 {
		if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
			panic("test_oneof: bad params JSON: " + err.Error())
		}
	}

	mode := "basic"
	if m, ok := params["mode"].(string); ok {
		mode = m
	}

	rangesRaw, ok := params["ranges"].([]any)
	if !ok || len(rangesRaw) == 0 {
		panic("test_oneof: missing or empty ranges")
	}

	type intRange struct {
		minValue int
		maxValue int
	}
	ranges := make([]intRange, len(rangesRaw))
	for i, r := range rangesRaw {
		m, ok := r.(map[string]any)
		if !ok {
			panic(fmt.Sprintf("test_oneof: range[%d] not a map", i))
		}
		ranges[i] = intRange{
			minValue: int(m["min_value"].(float64)),
			maxValue: int(m["max_value"].(float64)),
		}
	}

	var gen hegel.Generator[int]
	switch mode {
	case "basic":
		gens := make([]hegel.Generator[int], len(ranges))
		for i, r := range ranges {
			gens[i] = hegel.Integers[int](r.minValue, r.maxValue)
		}
		gen = hegel.OneOf(gens...)

	case "map_negate":
		gens := make([]hegel.Generator[int], len(ranges))
		for i, r := range ranges {
			gens[i] = hegel.Map(hegel.Integers[int](r.minValue, r.maxValue), func(v int) int { return -v })
		}
		gen = hegel.OneOf(gens...)

	case "filter_even":
		gens := make([]hegel.Generator[int], len(ranges))
		for i, r := range ranges {
			lo := r.minValue
			hi := r.maxValue
			evenLo := lo + (lo & 1)
			evenHi := hi - (hi & 1)
			halfLo := evenLo / 2
			halfHi := evenHi / 2
			mapped := hegel.Map(hegel.Integers[int](halfLo, halfHi), func(v int) int { return v * 2 })
			gens[i] = hegel.Filter(mapped, func(v int) bool { return true })
		}
		gen = hegel.OneOf(gens...)

	default:
		panic("test_oneof: unknown mode: " + mode)
	}

	n := conformance.GetTestCases()
	hegel.MustRun(func(s *hegel.TestCase) {
		val := hegel.Draw(s, gen)
		conformance.WriteMetrics(map[string]any{
			"value": val,
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
