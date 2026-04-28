// Package conformance provides shared helpers for the hegel conformance test binaries.
package conformance

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
)

// GetTestCases returns the number of conformance test cases to run,
// read from CONFORMANCE_TEST_CASES. Panics if the env var is missing
// or not a positive integer.
func GetTestCases() int {
	val := os.Getenv("CONFORMANCE_TEST_CASES")
	if val == "" {
		panic("CONFORMANCE_TEST_CASES env var not set")
	}
	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 {
		panic("CONFORMANCE_TEST_CASES must be a positive integer, got: " + val)
	}
	return n
}

// conformanceMetricsMu protects concurrent writes to the metrics file
// and the metricsWrittenThisCase flag.
var conformanceMetricsMu sync.Mutex

// metricsWrittenThisCase tracks whether the current test case has called
// WriteMetrics. EnsureMetric reads and clears it.
var metricsWrittenThisCase bool

// WriteMetrics appends a JSON line to the conformance metrics file.
// The metrics file path is read from CONFORMANCE_METRICS_FILE env var.
// Panics if the env var is not set or the file cannot be written.
func WriteMetrics(metrics map[string]any) {
	writeMetricsLine(metrics)
	conformanceMetricsMu.Lock()
	metricsWrittenThisCase = true
	conformanceMetricsMu.Unlock()
}

// writeMetricsLine appends a JSON line to the metrics file without touching
// the metricsWrittenThisCase flag. EnsureMetric uses this directly so its
// synthetic {} write doesn't bleed the flag into the next test case.
func writeMetricsLine(metrics map[string]any) {
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

// EnsureMetric guarantees that exactly one metrics line is written for the
// current test case. If the test body already called WriteMetrics, this is a
// no-op. Otherwise it writes an empty {}.
//
// Conformance binaries should `defer conformance.EnsureMetric()` at the top
// of their test body.
func EnsureMetric() {
	conformanceMetricsMu.Lock()
	written := metricsWrittenThisCase
	metricsWrittenThisCase = false
	conformanceMetricsMu.Unlock()
	if !written {
		writeMetricsLine(map[string]any{})
	}
}
