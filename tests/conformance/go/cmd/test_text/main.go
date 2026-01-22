package main

import (
	"encoding/json"
	"fmt"
	"os"
	"unicode/utf8"

	hegel "github.com/antithesishq/hegel-go"
	"github.com/antithesishq/hegel-go/tests/conformance/go/metrics"
)

type params struct {
	MinSize int  `json:"min_size"`
	MaxSize *int `json:"max_size"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: test_text '<json_params>'")
		os.Exit(1)
	}

	var p params
	if err := json.Unmarshal([]byte(os.Args[1]), &p); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse params: %v\n", err)
		os.Exit(1)
	}

	hegel.Hegel(func() {
		gen := hegel.Text().MinSize(p.MinSize)
		if p.MaxSize != nil {
			gen = gen.MaxSize(*p.MaxSize)
		}
		value := gen.Generate()
		// Count Unicode codepoints, not bytes
		length := utf8.RuneCountInString(value)
		metrics.Write(map[string]any{"length": length})
	}, hegel.HegelOptions{TestCases: metrics.GetTestCases()})
}
