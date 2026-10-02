package hegel

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"
)

// testing.T source attribution requires a separate test process.
func TestTDiagnosticsWithUserFileLine(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		wants []string
	}{
		{
			name: "TestFatal",
			body: `_ = hegel.Draw(ht, hegel.Integers(0, 100))
ht.Fatal("BOOM-fatal")`,
			wants: []string{
				`(?s)failure: BOOM-fatal.*hegel_test\.go:\d+`,
				`(?m)^\s+_ = hegel\.Draw\(ht, hegel\.Integers\(0, 100\)\)`,
			},
		},
		{
			name:  "TestFatalf",
			body:  `ht.Fatalf("BOOM-fatalf %d", 7)`,
			wants: []string{`(?s)failure: BOOM-fatalf 7.*hegel_test\.go:\d+`},
		},
		{
			name: "TestError",
			body: `c := hegel.Composite(func(tc hegel.TestCase) int {
	tc.(*hegel.T).Helper()
	tc.Note("BOOM-composite-note")
	return 0
})
_ = hegel.Draw(ht, c)
ht.Error("BOOM-error")`,
			wants: []string{
				`(?s)failure: BOOM-error.*hegel_test\.go:\d+`,
				`(?m)^\s+BOOM-composite-note`,
			},
		},
		{
			name:  "TestLogf",
			body:  "ht.Logf(\"BOOM-logf %d\", 9)\npanic(\"force final replay\")",
			wants: []string{`hegel_test\.go:\d+: BOOM-logf 9`},
		},
	}

	var source strings.Builder
	source.WriteString(`package temptest

import (
	"testing"

	"hegel.dev/go/hegel"
)
`)
	for _, tc := range cases {
		fmt.Fprintf(&source, `
func %s(t *testing.T) {
	hegel.Test(t, func(ht *hegel.T) {
		%s
	}, hegel.WithTestCases(1))
}
`, tc.name, tc.body)
	}

	out := newTempGoProject(t).
		writeFile("hegel_test.go", source.String()).
		expectFailure(`"Action":"fail"`).
		goTest("-json")

	outputs := make(map[string]string)
	failed := make(map[string]bool)
	decoder := json.NewDecoder(strings.NewReader(out.Stdout))
	for {
		var event struct {
			Action string
			Test   string
			Output string
		}
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("decode go test output: %v", err)
		}
		outputs[event.Test] += event.Output
		if event.Action == "fail" {
			failed[event.Test] = true
		}
	}
	for _, tc := range cases {
		if !failed[tc.name] {
			t.Errorf("%s did not fail; output:\n%s", tc.name, outputs[tc.name])
		}
		for _, want := range tc.wants {
			if matched, err := regexp.MatchString(want, outputs[tc.name]); err != nil {
				t.Fatalf("bad diagnostic regex %q: %v", want, err)
			} else if !matched {
				t.Errorf("%s output did not match %q:\n%s", tc.name, want, outputs[tc.name])
			}
		}
	}
}
