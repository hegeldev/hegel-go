package main

import (
	"encoding/json"
	"fmt"
	"os"

	hegel "github.com/antithesishq/hegel-go"
	"github.com/antithesishq/hegel-go/tests/conformance/go/metrics"
)

type params struct {
	MinValue int `json:"min_value"`
	MaxValue int `json:"max_value"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: test_integers '<json_params>'")
		os.Exit(1)
	}

	var p params
	if err := json.Unmarshal([]byte(os.Args[1]), &p); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse params: %v\n", err)
		os.Exit(1)
	}

	hegel.Hegel(func() {
		value := hegel.Integers[int]().Min(p.MinValue).Max(p.MaxValue).Generate()
		metrics.Write(map[string]any{"value": value})
	}, hegel.HegelOptions{TestCases: metrics.GetTestCases()})
}
