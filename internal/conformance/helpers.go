// Package conformance provides shared helpers for the hegel conformance test binaries.
package conformance

import (
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
		panic("CONFORMANCE_METRICS_FILE env var not set")
	}
	data, err := json.Marshal(metrics)
	if err != nil {
		panic(fmt.Sprintf("unreachable: WriteMetrics marshal: %v", err))
	}
	conformanceMetricsMu.Lock()
	defer conformanceMetricsMu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(fmt.Sprintf("WriteMetrics open: %v", err))
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		panic(fmt.Sprintf("unreachable: WriteMetrics write: %v", err))
	}
}
