package main

import (
	"encoding/json"
	"fmt"
	"os"

	hegel "github.com/antithesishq/hegel-go"
	"github.com/antithesishq/hegel-go/tests/conformance/go/metrics"
)

type params struct {
	MinSize int  `json:"min_size"`
	MaxSize *int `json:"max_size"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: test_binary '<json_params>'")
		os.Exit(1)
	}

	var p params
	if err := json.Unmarshal([]byte(os.Args[1]), &p); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse params: %v\n", err)
		os.Exit(1)
	}

	hegel.Hegel(func() {
		gen := hegel.Binary().MinSize(p.MinSize)
		if p.MaxSize != nil {
			gen = gen.MaxSize(*p.MaxSize)
		}
		value := gen.Generate()
		metrics.Write(map[string]any{"length": len(value)})
	}, hegel.HegelOptions{TestCases: metrics.GetTestCases()})
}
