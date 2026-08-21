package hegel

import (
	"bytes"
	"strings"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

// makeFakeT creates a *T with a zero testCase and a real *testing.T
// for unit testing T methods in state.go.
//
// emit defaults to false so the embedded *testing.T isn't exercised.
func makeFakeT(t *testing.T) *T {
	return &T{
		testCase: &testCase{},
		T:        t,
	}
}

// makeEmittingT creates a *T with a testCase.out set to buf.
// Use when the test wants to assert routing through the embedded *testing.T
// (which we can't capture directly) and/or testCase.out.
func makeEmittingT(t *testing.T, buf *bytes.Buffer) *T {
	return &T{
		testCase: &testCase{out: buf},
		T:        t,
	}
}

func expectInvocationFailure(t *testing.T, message string) {
	t.Helper()
	got := recover()
	err, ok := got.(*invocationError)
	if !ok || err.kind != "failure" || err.Error() != message {
		t.Fatalf("panic = %#v, want failure %q", got, message)
	}
}

// =============================================================================
// T.Fatal / T.Fatalf / T.FailNow — panic with fatalSentinel
// =============================================================================

func TestTFatalPanicsWithSentinel(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer expectInvocationFailure(t, "fatal message")
	ht.Fatal("fatal message")
}

// TestTFatalEmitsViaTestingT exercises the emit=true branch of Fatal: the
// message routes through t.T.Log so file:line decoration walks back to user
// code. We can't capture t.T.Log output from here, so this test merely
// proves the path runs without panic from t.Log and that the fatalSentinel
// still unwinds.
func TestTFatalEmitsViaTestingT(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	ht := makeEmittingT(t, &buf)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected Fatalf to panic")
		}
	}()
	const message = "fatal message"
	ht.Fatal(message)
	if !strings.Contains(buf.String(), message) {
		t.Fatalf("expected %s to contain %s", buf.String(), message)
	}
}

func TestTFatalfPanicsWithSentinel(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	ht := makeEmittingT(t, &buf)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected Fatalf to panic")
		}
	}()
	ht.Fatalf("fatal: %d", 42)
	if !strings.Contains(buf.String(), "fatal: 42") {
		t.Fatalf("expected %s to contain %s", buf.String(), "fatal: 42")
	}
}

func TestTFailNowAborts(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer expectInvocationFailure(t, "test case aborted with FailNow")
	ht.FailNow()
}

func TestTSkipPanicsWithShortCircuit(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer expectErrorPanic(t, libhegel.E_ASSUME)
	ht.Skip("skipping")
}

func TestTSkipfPanicsWithShortCircuit(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer expectErrorPanic(t, libhegel.E_ASSUME)
	ht.Skipf("skip: %d", 1)
}

func TestTSkipNowPanicsWithShortCircuit(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer expectErrorPanic(t, libhegel.E_ASSUME)
	ht.SkipNow()
}

// =============================================================================
// T.Error / T.Errorf — abort the invocation
// =============================================================================

func TestTErrorAborts(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer expectInvocationFailure(t, "something went wrong")
	ht.Error("something went wrong")
}

func TestTErrorfAborts(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer expectInvocationFailure(t, "error: 99")
	ht.Errorf("error: %d", 99)
}

// TestTErrorEmitsViaTestingT exercises the emit=true branch of Error.
func TestTErrorEmitsViaTestingT(t *testing.T) {
	t.Parallel()
	ht := makeEmittingT(t, &bytes.Buffer{})
	defer expectInvocationFailure(t, "something went wrong")
	ht.Error("something went wrong")
}

// TestTErrorfEmitsViaTestingT exercises the emit=true branch of Errorf.
func TestTErrorfEmitsViaTestingT(t *testing.T) {
	t.Parallel()
	ht := makeEmittingT(t, &bytes.Buffer{})
	defer expectInvocationFailure(t, "error: 99")
	ht.Errorf("error: %d", 99)
}

// =============================================================================
// T.Fail aborts immediately.
// =============================================================================

func TestTFailAborts(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer expectInvocationFailure(t, "test case aborted with Fail")
	ht.Fail()
}

func TestTFailedIsFalse(t *testing.T) {
	t.Parallel()
	if makeFakeT(t).Failed() {
		t.Error("Failed() = true, want false")
	}
}

// =============================================================================
// T.Log / T.Logf / T.Note — route through t.T.Log when emit=true, no-op otherwise
// =============================================================================

// When emit=false, Log/Logf/Note are silent.
func TestTLogSilentWhenNotEmitting(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	ht.Log("hello", " world")
	ht.Logf("value=%d", 42)
	ht.Note("a note")
}

// When emit=true, Log/Logf route through t.T.Log. We can't capture t.T.Log
// output directly; this test just exercises those branches. Note is promoted
// from testCase and writes to the normal Hegel output stream.
func TestTLogEmitsWhenEmitting(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	ht := makeEmittingT(t, &out)
	ht.Log("hello", " world")
	ht.Logf("value=%d", 42)
	ht.Note("a note")
	if got := strings.TrimSpace(out.String()); got != "a note" {
		t.Errorf("Note output = %q, want %q", got, "a note")
	}
}

// testCase.Note writes to s.out when set.
func TestTestCaseNoteWritesToOut(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	s := &testCase{out: &buf}
	s.Note("hello world")
	if got := strings.TrimSpace(buf.String()); got != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", got)
	}
}

// testCase.Errorf defers its diagnostic and aborts.
func TestTestCaseErrorfDefersOutputAndFails(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	s := &testCase{out: &buf}
	defer func() {
		r := recover()
		err, ok := r.(*invocationError)
		if !ok || err.kind != "failure" {
			t.Fatalf("unexpected panic: %v", r)
		}
		if got := buf.String(); got != "" {
			t.Errorf("output before completion = %q, want none", got)
		}
	}()
	s.Errorf("value=%d", 42)
}

// testCase.Log routes through Note.
func TestTestCaseLogWritesToOut(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	s := &testCase{out: &buf}
	s.Log("hello", " world")
	if got := strings.TrimSpace(buf.String()); got != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", got)
	}
}

// *T promotes testCase.reportDraw and writes through the Hegel output stream.
func TestTReportDrawEmits(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	ht := makeEmittingT(t, &out)
	ht.reportDraw(0, 42)
	if got := out.String(); !strings.Contains(got, " = 42") {
		t.Fatalf("draw output = %q", got)
	}
}

// =============================================================================
// T.Run — panics with "not supported"
// =============================================================================

func TestTRunPanics(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected Run to panic")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T: %v", r, r)
		}
		if msg != "nested t.Run is not supported inside a property test" {
			t.Errorf("unexpected panic message: %q", msg)
		}
	}()
	ht.Run("sub", func(_ *testing.T) {})
}
