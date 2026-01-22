package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"

	hegel "github.com/antithesishq/hegel-go"
	"github.com/antithesishq/hegel-go/tests/conformance/go/metrics"
)

type params struct {
	MinValue      *float64 `json:"min_value"`
	MaxValue      *float64 `json:"max_value"`
	ExcludeMin    bool     `json:"exclude_min"`
	ExcludeMax    bool     `json:"exclude_max"`
	AllowNan      bool     `json:"allow_nan"`
	AllowInfinity bool     `json:"allow_infinity"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: test_floats '<json_params>'")
		os.Exit(1)
	}

	var p params
	if err := json.Unmarshal([]byte(os.Args[1]), &p); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse params: %v\n", err)
		os.Exit(1)
	}

	hegel.Hegel(func() {
		gen := hegel.Floats[float64]()
		if p.MinValue != nil {
			gen = gen.Min(*p.MinValue)
		}
		if p.MaxValue != nil {
			gen = gen.Max(*p.MaxValue)
		}
		if p.ExcludeMin {
			gen = gen.ExcludeMin()
		}
		if p.ExcludeMax {
			gen = gen.ExcludeMax()
		}
		if p.AllowNan {
			gen = gen.AllowNan()
		}
		if p.AllowInfinity {
			gen = gen.AllowInfinity()
		}
		value := gen.Generate()
		metrics.Write(map[string]any{
			"value":       value,
			"is_nan":      math.IsNaN(value),
			"is_infinite": math.IsInf(value, 0),
		})
	}, hegel.HegelOptions{TestCases: metrics.GetTestCases()})
}
