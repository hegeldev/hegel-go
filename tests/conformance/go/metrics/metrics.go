package metrics

import (
	"encoding/json"
	"os"
	"strconv"
)

var metricsFile *os.File

func init() {
	if path := os.Getenv("CONFORMANCE_METRICS_FILE"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			metricsFile = f
		}
	}
}

func GetTestCases() int {
	if val := os.Getenv("CONFORMANCE_TEST_CASES"); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return 50
}

func Write(m map[string]any) {
	if metricsFile != nil {
		data, _ := json.Marshal(m)
		metricsFile.Write(append(data, '\n'))
	}
}
