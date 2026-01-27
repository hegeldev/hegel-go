package main

import (
	"encoding/json"
	"fmt"
	"os"

	hegel "github.com/antithesishq/hegel-go"
	"github.com/antithesishq/hegel-go/tests/conformance/go/metrics"
)

type params struct {
	MinSize  int    `json:"min_size"`
	MaxSize  int    `json:"max_size"`
	KeyType  string `json:"key_type"`
	MinKey   int    `json:"min_key"`
	MaxKey   int    `json:"max_key"`
	MinValue int    `json:"min_value"`
	MaxValue int    `json:"max_value"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: test_hashmaps '<json_params>'")
		os.Exit(1)
	}

	var p params
	if err := json.Unmarshal([]byte(os.Args[1]), &p); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse params: %v\n", err)
		os.Exit(1)
	}

	hegel.Hegel(func() {
		m := make(map[string]any)

		if p.KeyType == "integer" {
			dict := hegel.Maps(
				hegel.Integers[int]().Min(p.MinKey).Max(p.MaxKey),
				hegel.Integers[int]().Min(p.MinValue).Max(p.MaxValue),
			).MinSize(p.MinSize).MaxSize(p.MaxSize).Generate()

			m["size"] = len(dict)
			if len(dict) > 0 {
				var minKey, maxKey, minVal, maxVal *int
				for k, v := range dict {
					k, v := k, v // capture for pointers
					if minKey == nil || k < *minKey {
						minKey = &k
					}
					if maxKey == nil || k > *maxKey {
						maxKey = &k
					}
					if minVal == nil || v < *minVal {
						minVal = &v
					}
					if maxVal == nil || v > *maxVal {
						maxVal = &v
					}
				}
				m["min_key"] = *minKey
				m["max_key"] = *maxKey
				m["min_value"] = *minVal
				m["max_value"] = *maxVal
			}
		} else {
			// string keys
			dict := hegel.Maps(
				hegel.Text(),
				hegel.Integers[int]().Min(p.MinValue).Max(p.MaxValue),
			).MinSize(p.MinSize).MaxSize(p.MaxSize).Generate()

			m["size"] = len(dict)
			if len(dict) > 0 {
				var minVal, maxVal *int
				for _, v := range dict {
					v := v // capture for pointer
					if minVal == nil || v < *minVal {
						minVal = &v
					}
					if maxVal == nil || v > *maxVal {
						maxVal = &v
					}
				}
				m["min_value"] = *minVal
				m["max_value"] = *maxVal
			}
		}
		metrics.Write(m)
	}, hegel.HegelOptions{TestCases: metrics.GetTestCases()})
}
