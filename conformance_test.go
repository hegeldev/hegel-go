package hegel

import (
	"os"
	"path/filepath"
	"testing"
)

// --- GetTestCases tests ---

func TestGetTestCasesDefault(t *testing.T) {
	setEnv(t, "CONFORMANCE_TEST_CASES", "")
	n := GetTestCases()
	if n != 50 {
		t.Errorf("GetTestCases() = %d, want 50 (default)", n)
	}
}

func TestGetTestCasesValid(t *testing.T) {
	setEnv(t, "CONFORMANCE_TEST_CASES", "100")
	n := GetTestCases()
	if n != 100 {
		t.Errorf("GetTestCases() = %d, want 100", n)
	}
}

func TestGetTestCasesInvalidString(t *testing.T) {
	setEnv(t, "CONFORMANCE_TEST_CASES", "not-a-number")
	n := GetTestCases()
	if n != 50 {
		t.Errorf("GetTestCases() = %d, want 50 (default for invalid)", n)
	}
}

func TestGetTestCasesZero(t *testing.T) {
	setEnv(t, "CONFORMANCE_TEST_CASES", "0")
	n := GetTestCases()
	if n != 50 {
		t.Errorf("GetTestCases() = %d, want 50 (default for zero)", n)
	}
}

func TestGetTestCasesNegative(t *testing.T) {
	setEnv(t, "CONFORMANCE_TEST_CASES", "-5")
	n := GetTestCases()
	if n != 50 {
		t.Errorf("GetTestCases() = %d, want 50 (default for negative)", n)
	}
}

// --- WriteMetrics tests ---

func TestWriteMetricsNormal(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "metrics.jsonl")
	setEnv(t, "CONFORMANCE_METRICS_FILE", path)

	WriteMetrics(map[string]any{"value": true})
	WriteMetrics(map[string]any{"value": false})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if content == "" {
		t.Fatal("expected non-empty metrics file")
	}
	// Should have two JSON lines.
	lines := splitLines(content)
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d: %q", len(lines), content)
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				lines = append(lines, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func TestWriteMetricsNoEnvVar(t *testing.T) {
	setEnv(t, "CONFORMANCE_METRICS_FILE", "")
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic when CONFORMANCE_METRICS_FILE not set")
		}
	}()
	WriteMetrics(map[string]any{"value": 1})
}

func TestWriteMetricsOpenError(t *testing.T) {
	// Point to a directory instead of a file so OpenFile fails.
	tmp := t.TempDir()
	setEnv(t, "CONFORMANCE_METRICS_FILE", tmp) // directory, not a file

	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic when metrics file path is a directory")
		}
	}()
	WriteMetrics(map[string]any{"value": 1})
}
