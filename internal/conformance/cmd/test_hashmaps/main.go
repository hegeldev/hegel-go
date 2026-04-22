// test_hashmaps is a conformance binary for dict/hashmap generation.
// It parses JSON params from argv[1] (min_size, max_size, key_type,
// min_key, max_key, min_value, max_value) and writes dict metrics.
package main

import (
	"encoding/json"
	"math"
	"os"

	hegel "hegel.dev/go/hegel"
	"hegel.dev/go/hegel/internal/conformance"
)

func main() {
	params := map[string]any{}
	if len(os.Args) > 1 {
		if err := json.Unmarshal([]byte(os.Args[1]), &params); err != nil {
			panic("test_hashmaps: bad params JSON: " + err.Error())
		}
	}

	minSize := 0
	maxSize := 10

	if v, ok := params["min_size"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			minSize = int(x)
		}
	}
	if v, ok := params["max_size"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			maxSize = int(x)
		}
	}

	// Key type: "string" or "integer"
	keyType := "integer"
	if v, ok := params["key_type"]; ok {
		if s, ok := v.(string); ok {
			keyType = s
		}
	}

	minKey := int(-1000)
	maxKey := int(1000)
	minVal := int(-1000)
	maxVal := int(1000)

	if v, ok := params["min_key"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			minKey = int(x)
		}
	}
	if v, ok := params["max_key"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			maxKey = int(x)
		}
	}
	if v, ok := params["min_value"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			minVal = int(x)
		}
	}
	if v, ok := params["max_value"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			maxVal = int(x)
		}
	}

	valsGen := hegel.Integers[int](minVal, maxVal)
	n := conformance.GetTestCases()

	if keyType == "string" {
		keysGen := hegel.Text()
		gen := hegel.Dicts(keysGen, valsGen).MinSize(minSize).MaxSize(maxSize)

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
	} else {
		keysGen := hegel.Integers[int](minKey, maxKey)
		gen := hegel.Dicts(keysGen, valsGen).MinSize(minSize).MaxSize(maxSize)

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
	}
	os.Exit(0)
}
