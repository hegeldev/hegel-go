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
		panic("test_hashmaps: missing params JSON argument")
	}
	var params struct {
		MinSize  int    `json:"min_size"`
		MaxSize  int    `json:"max_size"`
		KeyType  string `json:"key_type"`
		MinKey   int    `json:"min_key"`
		MaxKey   int    `json:"max_key"`
		MinValue int    `json:"min_value"`
		MaxValue int    `json:"max_value"`
		Mode     string `json:"mode"`
	}
	if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
		panic("test_hashmaps: bad params JSON: " + err.Error())
	}

	// In non_basic mode, wrap the value generator with a no-op Filter so that
	// the value generator becomes non-basic, forcing Maps down the
	// new_collection/collection_more protocol path.
	intValsGen := hegel.Integers[int](params.MinValue, params.MaxValue)
	var valsGen hegel.Generator[int] = intValsGen
	if params.Mode == "non_basic" {
		valsGen = hegel.Filter(intValsGen, func(v int) bool { return true })
	}
	n := conformance.GetTestCases()

	switch params.KeyType {
	case "string":
		var keysGen hegel.Generator[string] = hegel.Text()
		if params.Mode == "non_basic" {
			keysGen = hegel.Filter(keysGen, func(v string) bool { return true })
		}
		gen := hegel.Maps(keysGen, valsGen).MinSize(params.MinSize).MaxSize(params.MaxSize)

		hegel.MustRun(func(s *hegel.TestCase) {
			m := hegel.Draw(s, gen)
			size := len(m)

			var minKeyOut, maxKeyOut, minValueOut, maxValueOut any
			if size > 0 {
				minIntVal := math.MaxInt
				maxIntVal := math.MinInt
				minStrKey := ""
				maxStrKey := ""
				firstKey := true

				for k, v := range m {
					if v < minIntVal {
						minIntVal = v
					}
					if v > maxIntVal {
						maxIntVal = v
					}
					if firstKey || k < minStrKey {
						minStrKey = k
					}
					if firstKey || k > maxStrKey {
						maxStrKey = k
					}
					firstKey = false
				}

				minKeyOut = minStrKey
				maxKeyOut = maxStrKey
				minValueOut = minIntVal
				maxValueOut = maxIntVal
			}

			conformance.WriteMetrics(map[string]any{
				"size":      size,
				"min_key":   minKeyOut,
				"max_key":   maxKeyOut,
				"min_value": minValueOut,
				"max_value": maxValueOut,
			})
		}, hegel.WithTestCases(n))
	case "integer":
		var keysGen hegel.Generator[int] = hegel.Integers[int](params.MinKey, params.MaxKey)
		if params.Mode == "non_basic" {
			keysGen = hegel.Filter(keysGen, func(v int) bool { return true })
		}
		gen := hegel.Maps(keysGen, valsGen).MinSize(params.MinSize).MaxSize(params.MaxSize)

		hegel.MustRun(func(s *hegel.TestCase) {
			m := hegel.Draw(s, gen)
			size := len(m)

			var minKeyOut, maxKeyOut, minValueOut, maxValueOut any
			if size > 0 {
				minIntKey := math.MaxInt
				maxIntKey := math.MinInt
				minIntVal := math.MaxInt
				maxIntVal := math.MinInt

				for k, v := range m {
					if v < minIntVal {
						minIntVal = v
					}
					if v > maxIntVal {
						maxIntVal = v
					}
					if k < minIntKey {
						minIntKey = k
					}
					if k > maxIntKey {
						maxIntKey = k
					}
				}

				minKeyOut = minIntKey
				maxKeyOut = maxIntKey
				minValueOut = minIntVal
				maxValueOut = maxIntVal
			}

			conformance.WriteMetrics(map[string]any{
				"size":      size,
				"min_key":   minKeyOut,
				"max_key":   maxKeyOut,
				"min_value": minValueOut,
				"max_value": maxValueOut,
			})
		}, hegel.WithTestCases(n))
	default:
		panic("test_hashmaps: unknown key_type: " + params.KeyType)
	}
	os.Exit(0)
}
