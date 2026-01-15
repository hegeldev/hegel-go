package main

import (
	"encoding/json"
	"fmt"
	"os"

	hegel "github.com/antithesishq/hegel-go"
	"github.com/antithesishq/hegel-go/tests/conformance/go/metrics"
)

type params struct {
	MinValue   float64 `json:"min_value"`
	MaxValue   float64 `json:"max_value"`
	ExcludeMin bool    `json:"exclude_min"`
	ExcludeMax bool    `json:"exclude_max"`
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
		gen := hegel.Floats[float64]().Min(p.MinValue).Max(p.MaxValue)
		if p.ExcludeMin {
			gen = gen.ExcludeMin()
		}
		if p.ExcludeMax {
			gen = gen.ExcludeMax()
		}
		value := gen.Generate()
		metrics.Write(map[string]any{"value": value})
	}, hegel.HegelOptions{TestCases: metrics.GetTestCases()})
}
