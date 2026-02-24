package hegel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
)

// GetTestCases returns the number of conformance test cases to run.
// It reads CONFORMANCE_TEST_CASES from the environment; defaults to 50.
func GetTestCases() int {
	val := os.Getenv("CONFORMANCE_TEST_CASES")
	if val == "" {
		return 50
	}
	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 {
		return 50
	}
	return n
}

// conformanceMetricsMu protects concurrent writes to the metrics file.
var conformanceMetricsMu sync.Mutex

// WriteMetrics appends a JSON line to the conformance metrics file.
// The metrics file path is read from CONFORMANCE_METRICS_FILE env var.
// Panics if the env var is not set or the file cannot be written.
func WriteMetrics(metrics map[string]any) {
	path := os.Getenv("CONFORMANCE_METRICS_FILE")
	if path == "" {
		panic("hegel: CONFORMANCE_METRICS_FILE env var not set")
	}
	data, err := json.Marshal(metrics)
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: WriteMetrics marshal: %v", err))
	}
	// Escape Unicode line terminators that json.Marshal leaves unescaped.
	// Python's str.splitlines() treats U+0085 (NEL), U+2028 (LS), and
	// U+2029 (PS) as line boundaries, which splits a JSONL line if these
	// appear inside a JSON string value.
	data = bytes.ReplaceAll(data, []byte("\xc2\x85"), []byte(`\u0085`))
	data = bytes.ReplaceAll(data, []byte("\xe2\x80\xa8"), []byte(`\u2028`))
	data = bytes.ReplaceAll(data, []byte("\xe2\x80\xa9"), []byte(`\u2029`))
	conformanceMetricsMu.Lock()
	defer conformanceMetricsMu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(fmt.Sprintf("hegel: WriteMetrics open: %v", err))
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		panic(fmt.Sprintf("hegel: unreachable: WriteMetrics write: %v", err))
	}
}
