package hegel

import (
	"fmt"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

// Compile-time check that T satisfies testing.TB.
var _ testing.TB = (*T)(nil)

// Compile-time checks that *testCase and *T both satisfy [TestCase]. This
// is what lets a [Composite] callback typed `func(TestCase) T` be invoked
// from either entry point.
var (
	_ TestCase = (*testCase)(nil)
	_ TestCase = (*T)(nil)
)

// Compile-time checks that *testCase satisfies the TestingT interfaces used by
// popular assertion libraries (testify, gotest.tools, gomega). This lets
// assertions be used directly inside [Composite] callbacks and [Run] bodies,
// where only a *testCase is available.
var _ interface {
	Errorf(format string, args ...any)
	FailNow()
} = (*testCase)(nil)

var _ interface {
	Fail()
	FailNow()
	Log(args ...any)
} = (*testCase)(nil)

// T is the test context for property tests run via [Test].
//
// It embeds *[testing.T] and overrides methods like Fatal and Skip so they
// work correctly inside a Hegel test body.
type T struct {
	*testCase
	*testing.T
}

// Shadowed methods — override testing.T behavior for Hegel compatibility.

// Fatal logs the message via the embedded [*testing.T] and marks the test
// case as failed.
func (t *T) Fatal(args ...any) {
	msg := fmt.Sprint(args...)
	if t.out != nil {
		t.Helper()
		t.T.Log(msg)
	}
	t.abort(errTestCaseAborted)
}

// Fatalf logs the formatted message via the embedded [*testing.T] and marks
// the test case as failed.
func (t *T) Fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if t.out != nil {
		t.Helper()
		t.T.Log(msg)
	}
	t.abort(errTestCaseAborted)
}

// Skip discards the current test case.
func (t *T) Skip(_ ...any) {
	t.Assume(false)
}

// Skipf discards the current test case.
func (t *T) Skipf(_ string, _ ...any) {
	t.Assume(false)
}

// SkipNow discards the current test case.
func (t *T) SkipNow() {
	t.Assume(false)
}

// Error logs the message via the embedded [*testing.T] and sets the failed flag.
//
// The test case continues running but will be treated as a failure after return.
func (t *T) Error(args ...any) {
	msg := fmt.Sprint(args...)
	if t.out != nil {
		t.Helper()
		t.T.Log(msg)
	}
	t.testCase.Fail()
}

// Errorf logs the formatted message via the embedded [*testing.T] and sets
// the failed flag.
//
// The test case continues running but will be treated as a failure after return.
func (t *T) Errorf(format string, args ...any) {
	if t.out != nil {
		t.Helper()
		t.T.Logf(format, args...)
	}
	t.testCase.Fail()
}

// Failed reports whether the test case has been marked as failed.
func (t *T) Failed() bool {
	return t.testCase.status == libhegel.STATUS_INTERESTING
}

// Log routes the message through the embedded [*testing.T].
func (t *T) Log(args ...any) {
	if t.out != nil {
		t.Helper()
		t.T.Log(fmt.Sprint(args...))
	}
}

// Logf routes the formatted message through the embedded [*testing.T].
func (t *T) Logf(format string, args ...any) {
	if t.out != nil {
		t.Helper()
		t.T.Logf(format, args...)
	}
}

// Note routes the message through the embedded [*testing.T].
func (t *T) Note(message string) {
	if t.out != nil {
		t.Helper()
		t.T.Log(message)
	}
}

// Run aborts the test — nested sub-tests inside a Hegel property test are not supported.
func (t *T) Run(_ string, _ func(*testing.T)) bool {
	panic("nested t.Run is not supported inside a property test")
}

func (t *T) reportDraw(skip int, value any, comment string) {
	if t.out == nil {
		return
	}
	_, msg := formatDrawReport(skip+1, value)
	t.Helper()
	t.T.Log(msg + formatExplainComment(comment))
}
