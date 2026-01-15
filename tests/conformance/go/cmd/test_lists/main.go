package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"

	hegel "github.com/antithesishq/hegel-go"
	"github.com/antithesishq/hegel-go/tests/conformance/go/metrics"
)

type params struct {
	MinSize  int `json:"min_size"`
	MaxSize  int `json:"max_size"`
	MinValue int `json:"min_value"`
	MaxValue int `json:"max_value"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: test_lists '<json_params>'")
		os.Exit(1)
	}

	var p params
	if err := json.Unmarshal([]byte(os.Args[1]), &p); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse params: %v\n", err)
		os.Exit(1)
	}

	hegel.Hegel(func() {
		elemGen := hegel.Integers[int]().Min(p.MinValue).Max(p.MaxValue)
		value := hegel.Slices(elemGen).MinSize(p.MinSize).MaxSize(p.MaxSize).Generate()

		m := map[string]any{"size": len(value)}
		if len(value) > 0 {
			m["min_element"] = slices.Min(value)
			m["max_element"] = slices.Max(value)
		}
		metrics.Write(m)
	}, hegel.HegelOptions{TestCases: metrics.GetTestCases()})
}
