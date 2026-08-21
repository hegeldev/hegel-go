package hegel

import (
	"testing"
)

// These tests verify that deferred failure diagnostics retain the user's
// stack frames. Logf continues to use testing.T's helper attribution.

func TestTFatalDecoratesWithUserFileLine(t *testing.T) {
	t.Parallel()
	newTempGoProject(t).
		testBody(`ht.Fatal("BOOM-fatal")`, "hegel.WithTestCases(1)").
		expectFailure(`(?s)failure: BOOM-fatal.*hegel_test\.go:\d+`).
		goTest()
}

func TestTFatalfDecoratesWithUserFileLine(t *testing.T) {
	t.Parallel()
	newTempGoProject(t).
		testBody(`ht.Fatalf("BOOM-fatalf %d", 7)`, "hegel.WithTestCases(1)").
		expectFailure(`(?s)failure: BOOM-fatalf 7.*hegel_test\.go:\d+`).
		goTest()
}

func TestTErrorDecoratesWithUserFileLine(t *testing.T) {
	t.Parallel()
	newTempGoProject(t).
		testBody(`ht.Error("BOOM-error")`, "hegel.WithTestCases(1)").
		expectFailure(`(?s)failure: BOOM-error.*hegel_test\.go:\d+`).
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
// Draw output uses the Hegel output stream without a synthetic location.
func TestDrawOmitsUserFileLine(t *testing.T) {
	t.Parallel()
	newTempGoProject(t).
		testBody(`_ = hegel.Draw(ht, hegel.Integers(0, 100))
panic("force final replay")`, "hegel.WithTestCases(1)").
		expectFailure(`(?m)^\s+_ = hegel\.Draw\(ht, hegel\.Integers\(0, 100\)\)`).
		goTest()
}

// Notes use the Hegel output stream without a synthetic location.
func TestNoteInsideCompositeOmitsUserFileLine(t *testing.T) {
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
		expectFailure(`(?m)^\s+BOOM-composite-note`).
		goTest()
}
