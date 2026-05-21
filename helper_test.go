package hegel

import "testing"

// These tests verify that the t.Helper() calls inside T.Fatal, T.Fatalf, and
// T.Logf cause t.Log to decorate output with the user's test file:line, not
// the hegel-go source location. The output flows:
//
//   user test → ht.Fatal/Fatalf/Logf → noteFn closure → t.Log
//
// All intermediate frames between t.Log and the user's test body need to be
// marked as helpers; otherwise t.Log reports the first non-helper frame.

func TestTFatalDecoratesWithUserFileLine(t *testing.T) {
	t.Parallel()
	newTempGoProject(t).
		testBody(`ht.Fatal("BOOM-fatal")`, "hegel.WithTestCases(1)").
		expectFailure(`hegel_test\.go:\d+: BOOM-fatal`).
		goTest()
}

func TestTFatalfDecoratesWithUserFileLine(t *testing.T) {
	t.Parallel()
	newTempGoProject(t).
		testBody(`ht.Fatalf("BOOM-fatalf %d", 7)`, "hegel.WithTestCases(1)").
		expectFailure(`hegel_test\.go:\d+: BOOM-fatalf 7`).
		goTest()
}

func TestTErrorDecoratesWithUserFileLine(t *testing.T) {
	t.Parallel()
	newTempGoProject(t).
		testBody(`ht.Error("BOOM-error")`, "hegel.WithTestCases(1)").
		expectFailure(`hegel_test\.go:\d+: BOOM-error`).
		goTest()
}

func TestTLogfDecoratesWithUserFileLine(t *testing.T) {
	t.Parallel()
	newTempGoProject(t).
		testBody(`ht.Logf("BOOM-logf %d", 9)
panic("force final replay")`, "hegel.WithTestCases(1)").
		expectFailure(`hegel_test\.go:\d+: BOOM-logf 9`).
		goTest()
}

// TestDrawDecoratesWithUserFileLine verifies the helper marking on Draw lets
// the noteFn-driven t.Log decoration point at the user's file. When Draw runs
// under *T the formatter omits its own file:line prefix so the t.Log
// decoration is the only one present.
func TestDrawDecoratesWithUserFileLine(t *testing.T) {
	t.Parallel()
	newTempGoProject(t).
		testBody(`_ = hegel.Draw(ht, hegel.Integers(0, 100))
panic("force final replay")`, "hegel.WithTestCases(1)").
		expectFailure(`(?m)^\s+hegel_test\.go:\d+: _ = hegel\.Draw\(ht, hegel\.Integers\(0, 100\)\)`).
		goTest()
}

// TestNoteInsideCompositeDecoratesWithUserFileLine verifies that tc.Note()
// called from within a Composite generator's callback decorates with the
// user's test file, not an internal hegel source location.
//
// The path is: user's Composite callback → testCase.Note → noteFn closure →
// t.Log. Every frame between t.Log and the user's body needs to be marked as
// a helper; testCase.Note and compositeGenerator.draw are the two non-helper
// frames that this case exposes.
func TestNoteInsideCompositeDecoratesWithUserFileLine(t *testing.T) {
	t.Parallel()
	newTempGoProject(t).
		testBody(`c := hegel.Composite(func(tc hegel.TestCase) int {
	tc.(*hegel.T).Helper()
	tc.Note("BOOM-composite-note")
	return 0
})
_ = hegel.Draw(ht, c)
ht.Fail()
`, "hegel.WithSingleTestCase()").
		expectFailure(`(?m)^\s+hegel_test\.go:\d+: BOOM-composite-note`).
		goTest()
}
