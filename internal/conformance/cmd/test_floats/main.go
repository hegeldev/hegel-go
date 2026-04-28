package main

import (
	"encoding/json"
	"math"
	"os"

	hegel "hegel.dev/go/hegel"
	"hegel.dev/go/hegel/internal/conformance"
)

func main() {
	if len(os.Args) <= 1 {
		panic("test_floats: missing params JSON argument")
	}
	var params struct {
		MinValue      *float64 `json:"min_value"`
		MaxValue      *float64 `json:"max_value"`
		ExcludeMin    bool     `json:"exclude_min"`
		ExcludeMax    bool     `json:"exclude_max"`
		AllowNaN      *bool    `json:"allow_nan"`
		AllowInfinity *bool    `json:"allow_infinity"`
	}
	if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
		panic("test_floats: bad params JSON: " + err.Error())
	}

	g := hegel.Floats[float64]()
	if params.MinValue != nil {
		g = g.Min(*params.MinValue)
	}
	if params.MaxValue != nil {
		g = g.Max(*params.MaxValue)
	}
	if params.AllowNaN != nil {
		g = g.AllowNaN(*params.AllowNaN)
	}
	if params.AllowInfinity != nil {
		g = g.AllowInfinity(*params.AllowInfinity)
	}
	if params.ExcludeMin {
		g = g.ExcludeMin()
	}
	if params.ExcludeMax {
		g = g.ExcludeMax()
	}

	gen := g
	n := conformance.GetTestCases()
	hegel.MustRun(func(s *hegel.TestCase) {
		defer conformance.EnsureMetric()
		val := hegel.Draw(s, gen)
		isNaN := math.IsNaN(val)
		isInfinite := math.IsInf(val, 0)
		m := map[string]any{
			"is_nan":      isNaN,
			"is_infinite": isInfinite,
		}
		if !isNaN && !isInfinite {
			m["value"] = val
		} else {
			m["value"] = nil
		}
		conformance.WriteMetrics(m)
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
