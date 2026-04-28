package hegel

import (
	"fmt"
	"testing"
)

// Compile-time check that T satisfies testing.TB.
var _ testing.TB = (*T)(nil)

// T is the test context for property tests run via [Case].
//
// It embeds *[testing.T] and overrides methods like Fatal and Skip so they
// work correctly inside a Hegel test body.
type T struct {
	*TestCase
	*testing.T
}

// Shadowed methods — override testing.T behavior for Hegel compatibility.

// Fatal logs the message via [TestCase.Note] and marks the test case as failed.
func (t *T) Fatal(args ...any) {
	msg := fmt.Sprint(args...)
	t.Note(msg)
	panic(fatalSentinel{msg: msg})
}

// Fatalf logs the formatted message via [TestCase.Note] and marks the test case as failed.
func (t *T) Fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	t.Note(msg)
	panic(fatalSentinel{msg: msg})
}

// FailNow marks the test case as failed and stops the test body.
func (t *T) FailNow() {
	panic(fatalSentinel{msg: "FailNow called"})
}

// Skip discards the current test case.
func (t *T) Skip(args ...any) {
	_ = args
	t.Assume(false)
}

// Skipf discards the current test case.
func (t *T) Skipf(format string, args ...any) {
	_, _ = format, args
	t.Assume(false)
}

// SkipNow discards the current test case.
func (t *T) SkipNow() {
	t.Assume(false)
}

// Error logs the message via [TestCase.Note] and sets the failed flag.
//
// The test case continues running but will be treated as a failure after return.
func (t *T) Error(args ...any) {
	msg := fmt.Sprint(args...)
	t.Note(msg)
	t.TestCase.failed = true
}

// Errorf logs the formatted message via [TestCase.Note] and sets the failed flag.
func (t *T) Errorf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	t.Note(msg)
	t.TestCase.failed = true
}

// Fail sets the failed flag without stopping the test case.
func (t *T) Fail() {
	t.TestCase.failed = true
}

// Failed reports whether the test case has been marked as failed.
func (t *T) Failed() bool {
	return t.TestCase.failed
}

// Log routes the message through [TestCase.Note] (only emitted on final replay).
func (t *T) Log(args ...any) {
	t.Note(fmt.Sprint(args...))
}

// Logf routes the formatted message through [TestCase.Note].
func (t *T) Logf(format string, args ...any) {
	t.Note(fmt.Sprintf(format, args...))
}

// Run aborts the test — nested sub-tests inside a Hegel property test are not supported.
func (t *T) Run(_ string, _ func(*testing.T)) bool {
	panic("nested t.Run is not supported inside a property test")
}
