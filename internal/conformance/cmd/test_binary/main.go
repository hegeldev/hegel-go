package main

import (
	"encoding/json"
	"os"

	hegel "hegel.dev/go/hegel"
	"hegel.dev/go/hegel/internal/conformance"
)

func main() {
	if len(os.Args) <= 1 {
		panic("test_binary: missing params JSON argument")
	}
	var params struct {
		MinSize int  `json:"min_size"`
		MaxSize *int `json:"max_size"`
	}
	if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
		panic("test_binary: bad params JSON: " + err.Error())
	}

	maxSize := -1
	if params.MaxSize != nil {
		maxSize = *params.MaxSize
	}

	gen := hegel.Binary(params.MinSize, maxSize)
	n := conformance.GetTestCases()
	hegel.MustRun(func(s *hegel.TestCase) {
		defer conformance.EnsureMetric()
		v := hegel.Draw(s, gen)
		conformance.WriteMetrics(map[string]any{
			"length": len(v),
		})
	}, hegel.WithTestCases(n))
	os.Exit(0)
}
