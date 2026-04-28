package main

import (
	"encoding/json"
	"fmt"
	"os"

	hegel "hegel.dev/go/hegel"
	"hegel.dev/go/hegel/internal/conformance"
)

func buggyFunction(n int) {
	panic(fmt.Sprintf("bug: %d", n))
}

func callPathA(n int) { buggyFunction(n) }
func callPathB(n int) { buggyFunction(n) }

func main() {
	if len(os.Args) <= 1 {
		panic("test_origin_dedup: missing params JSON argument")
	}
	var params struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
		panic("test_origin_dedup: bad params JSON: " + err.Error())
	}

	n := conformance.GetTestCases()

	switch params.Mode {
	case "value_in_error_message":
		_ = hegel.Run(func(s *hegel.TestCase) {
			v := hegel.Draw(s, hegel.Integers[int](0, 1000))
			if !s.IsFinal() {
				conformance.WriteMetrics(map[string]any{"value": v})
			}
			panic(fmt.Sprintf("failing with value %d", v))
		}, hegel.WithTestCases(n))

	case "multiple_call_sites":
		_ = hegel.Run(func(s *hegel.TestCase) {
			v := hegel.Draw(s, hegel.Integers[int](1, 1000))
			if !s.IsFinal() {
				conformance.WriteMetrics(map[string]any{"value": v})
			}
			if v%2 == 0 {
				callPathA(v)
			} else {
				callPathB(v)
			}
		}, hegel.WithTestCases(n))

	default:
		panic("test_origin_dedup: unknown mode: " + params.Mode)
	}
	os.Exit(0)
}
