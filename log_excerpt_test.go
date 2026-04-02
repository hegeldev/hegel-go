package hegel

import (
	"strings"
	"testing"
)

func TestFormatLogExcerptEmpty(t *testing.T) {
	t.Parallel()
	result := formatLogExcerpt("")
	if result != "(empty)" {
		t.Errorf("formatLogExcerpt empty = %q, want %q", result, "(empty)")
	}
}

func TestFormatLogExcerptShortLog(t *testing.T) {
	t.Parallel()
	content := "Error: something went wrong\nDetails here"
	result := formatLogExcerpt(content)
	if result != content {
		t.Errorf("formatLogExcerpt short log = %q, want %q", result, content)
	}
}

func TestFormatLogExcerptTakesLastNUnindented(t *testing.T) {
	t.Parallel()
	// 10 unindented lines — only the last 5 should appear.
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "line " + strings.Repeat("x", i) // ensure different content
	}
	// Use simple numbering for clarity.
	for i := range lines {
		lines[i] = "line " + string(rune('0'+i))
	}
	content := strings.Join(lines, "\n")
	result := formatLogExcerpt(content)
	if !strings.Contains(result, "line 5") {
		t.Errorf("should include line 5: %s", result)
	}
	if !strings.Contains(result, "line 9") {
		t.Errorf("should include line 9: %s", result)
	}
	if strings.Contains(result, "line 4") {
		t.Errorf("should not include line 4: %s", result)
	}
}

func TestFormatLogExcerptTruncatesLongIndentRuns(t *testing.T) {
	t.Parallel()
	lines := []string{"Error start"}
	for i := range 20 {
		lines = append(lines, "  frame "+string(rune('A'+i)))
	}
	lines = append(lines, "Error end")
	content := strings.Join(lines, "\n")
	result := formatLogExcerpt(content)
	if !strings.Contains(result, "[...") {
		t.Errorf("should contain truncation marker: %s", result)
	}
	if !strings.Contains(result, "frame A") {
		t.Errorf("should show first frame: %s", result)
	}
	if !strings.Contains(result, "frame T") {
		t.Errorf("should show last frame: %s", result)
	}
	if strings.Contains(result, "frame K") {
		t.Errorf("should not show middle frame: %s", result)
	}
}

func TestFormatLogExcerptKeepsShortIndentRuns(t *testing.T) {
	t.Parallel()
	lines := []string{"Error"}
	for i := range 8 {
		lines = append(lines, "  frame "+string(rune('0'+i)))
	}
	lines = append(lines, "End")
	content := strings.Join(lines, "\n")
	result := formatLogExcerpt(content)
	if strings.Contains(result, "[...") {
		t.Errorf("should not truncate short run: %s", result)
	}
	if !strings.Contains(result, "frame 7") {
		t.Errorf("all frames should be present: %s", result)
	}
}
