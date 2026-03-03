// test_hashmaps is a conformance binary for dict/hashmap generation.
// It parses JSON params from argv[1] (min_size, max_size, key_type,
// min_key, max_key, min_value, max_value) and writes dict metrics.
package main

import (
	"encoding/json"
	"math"
	"os"

	hegel "github.com/antithesishq/hegel-go"
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

	minKey := int64(-1000)
	maxKey := int64(1000)
	minVal := int64(-1000)
	maxVal := int64(1000)

	if v, ok := params["min_key"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			minKey = int64(x)
		}
	}
	if v, ok := params["max_key"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			maxKey = int64(x)
		}
	}
	if v, ok := params["min_value"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			minVal = int64(x)
		}
	}
	if v, ok := params["max_value"]; ok && v != nil {
		if x, ok := v.(float64); ok {
			maxVal = int64(x)
		}
	}

	opts := hegel.DictOptions{
		MinSize:    minSize,
		MaxSize:    maxSize,
		HasMaxSize: true,
	}
	n := hegel.GetTestCases()

	if keyType == "string" {
		keysGen := hegel.Text(0, -1)
		valsGen := hegel.Integers(minVal, maxVal)
		gen := hegel.Dicts(keysGen, valsGen, opts)
		hegel.RunHegelTest("conformance_hashmaps", func() {
			m := hegel.Draw(gen)
			size := len(m)

			var minKeyOut, maxKeyOut, minValueOut, maxValueOut any
			if size > 0 {
				minIntVal := int64(math.MaxInt64)
				maxIntVal := int64(math.MinInt64)
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

			hegel.WriteMetrics(map[string]any{
				"size":      size,
				"min_key":   minKeyOut,
				"max_key":   maxKeyOut,
				"min_value": minValueOut,
				"max_value": maxValueOut,
			})
		}, hegel.WithTestCases(n))
	} else {
		keysGen := hegel.Integers(minKey, maxKey)
		valsGen := hegel.Integers(minVal, maxVal)
		gen := hegel.Dicts(keysGen, valsGen, opts)
		hegel.RunHegelTest("conformance_hashmaps", func() {
			m := hegel.Draw(gen)
			size := len(m)

			var minKeyOut, maxKeyOut, minValueOut, maxValueOut any
			if size > 0 {
				minIntKey := int64(math.MaxInt64)
				maxIntKey := int64(math.MinInt64)
				minIntVal := int64(math.MaxInt64)
				maxIntVal := int64(math.MinInt64)

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

			hegel.WriteMetrics(map[string]any{
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
