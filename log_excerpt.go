package hegel

import (
	"fmt"
	"strings"
)

// formatLogExcerpt formats a server log excerpt for inclusion in error messages.
//
// Returns the last 5 unindented lines and the content between them. Runs of
// more than 10 consecutive indented lines are truncated with a summary.
func formatLogExcerpt(content string) string {
	const maxUnindented = 5
	const indentThreshold = 10
	const indentContext = 3

	lines := strings.Split(content, "\n")
	// Remove trailing empty line from final newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return "(empty)"
	}

	// Find start: walk backwards until we've seen maxUnindented unindented lines.
	unindentedSeen := 0
	startIdx := 0
	for i := len(lines) - 1; i >= 0; i-- {
		if isLogUnindented(lines[i]) {
			unindentedSeen++
			if unindentedSeen >= maxUnindented {
				startIdx = i
				break
			}
		}
	}

	// Process the relevant section, truncating long indented runs.
	relevant := lines[startIdx:]
	var output []string
	var indentRun []string

	for _, line := range relevant {
		if isLogUnindented(line) {
			flushLogIndentRun(&indentRun, &output, indentThreshold, indentContext)
			output = append(output, line)
		} else {
			indentRun = append(indentRun, line)
		}
	}
	flushLogIndentRun(&indentRun, &output, indentThreshold, indentContext)

	return strings.Join(output, "\n")
}

func isLogUnindented(line string) bool {
	return line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t")
}

func flushLogIndentRun(run *[]string, output *[]string, threshold, context int) {
	if len(*run) == 0 {
		return
	}
	if len(*run) > threshold {
		keep := context
		if keep > len(*run)/2 { //nocov
			keep = len(*run) / 2 //nocov
		}
		*output = append(*output, (*run)[:keep]...)
		hidden := len(*run) - 2*keep
		*output = append(*output, fmt.Sprintf("  [...%d lines...]", hidden))
		*output = append(*output, (*run)[len(*run)-keep:]...)
	} else {
		*output = append(*output, *run...)
	}
	*run = (*run)[:0]
}
