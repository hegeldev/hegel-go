package main

import (
	hegel "github.com/antithesishq/hegel-go"
	"github.com/antithesishq/hegel-go/tests/conformance/go/metrics"
)

func main() {
	hegel.Hegel(func() {
		value := hegel.Booleans().Generate()
		metrics.Write(map[string]any{"value": value})
	}, hegel.HegelOptions{TestCases: metrics.GetTestCases()})
}
