package hegel

import (
	"fmt"
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
		goTest()
	for _, tc := range cases {
		result := out.Tests[tc.name]
		if result.Status != "fail" {
			t.Errorf("%s status = %q, want fail; output:\n%s", tc.name, result.Status, result.Output)
		}
		for _, want := range tc.wants {
			if matched, err := regexp.MatchString(want, result.Output); err != nil {
				t.Fatalf("bad diagnostic regex %q: %v", want, err)
			} else if !matched {
				t.Errorf("%s output did not match %q:\n%s", tc.name, want, result.Output)
			}
		}
	}
}
