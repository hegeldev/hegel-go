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

// =============================================================================
// T.Fatal / T.Fatalf / T.FailNow — panic with fatalSentinel
// =============================================================================

func TestTFatalPanicsWithSentinel(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer expectErrorPanic(t, errTestCaseAborted)
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

func TestTFailNowPanicsWithSentinel(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer expectErrorPanic(t, errTestCaseAborted)
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
// T.Error / T.Errorf — set failed flag, call Note
// =============================================================================

func TestTErrorSetsFailed(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	ht.Error("something went wrong")
	if ht.testCase.status != libhegel.STATUS_INTERESTING {
		t.Error("expected status=INTERESTING after Error()")
	}
}

func TestTErrorfSetsFailed(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	ht.Errorf("error: %d", 99)
	if ht.testCase.status != libhegel.STATUS_INTERESTING {
		t.Error("expected status=INTERESTING after Errorf()")
	}
}

// TestTErrorEmitsViaTestingT exercises the emit=true branch of Error.
func TestTErrorEmitsViaTestingT(t *testing.T) {
	t.Parallel()
	ht := makeEmittingT(t, &bytes.Buffer{})
	ht.Error("something went wrong")
	if ht.testCase.status != libhegel.STATUS_INTERESTING {
		t.Error("expected status=INTERESTING after Error()")
	}
}

// TestTErrorfEmitsViaTestingT exercises the emit=true branch of Errorf.
func TestTErrorfEmitsViaTestingT(t *testing.T) {
	t.Parallel()
	ht := makeEmittingT(t, &bytes.Buffer{})
	ht.Errorf("error: %d", 99)
	if ht.testCase.status != libhegel.STATUS_INTERESTING {
		t.Error("expected status=INTERESTING after Errorf()")
	}
}

// =============================================================================
// T.Fail / T.Failed — sets/reads failed flag
// =============================================================================

func TestTFailSetsFailed(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	if ht.Failed() {
		t.Error("expected Failed() to be false initially")
	}
	ht.Fail()
	if !ht.Failed() {
		t.Error("expected Failed() to be true after Fail()")
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

// When emit=true, Log/Logf/Note route through t.T.Log. We can't capture
// t.T.Log output directly; this test just exercises the branch.
func TestTLogEmitsWhenEmitting(t *testing.T) {
	t.Parallel()
	ht := makeEmittingT(t, &bytes.Buffer{})
	ht.Log("hello", " world")
	ht.Logf("value=%d", 42)
	ht.Note("a note")
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

// testCase.Errorf writes to s.out and sets the failed flag.
func TestTestCaseErrorfWritesAndFails(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	s := &testCase{out: &buf}
	s.Errorf("value=%d", 42)
	if s.status != libhegel.STATUS_INTERESTING {
		t.Error("expected status=INTERESTING after Errorf")
	}
	if got := strings.TrimSpace(buf.String()); got != "value=42" {
		t.Errorf("expected %q, got %q", "value=42", got)
	}
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

// *T.reportDraw with emit=true and a captured Draw site routes through
// t.T.Log. We can't capture t.T.Log output from here, but we can exercise
// the code path. The call site doesn't matter — formatDrawReport just
// needs a non-zero skip frame.
func TestTReportDrawEmits(t *testing.T) {
	t.Parallel()
	ht := makeEmittingT(t, &bytes.Buffer{})
	ht.reportDraw(0, 42)
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
