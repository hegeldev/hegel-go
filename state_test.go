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

// runEmitting runs body against a live, engine-backed, emitting *T and
// returns the rendered report. Reports flow through the engine-hosted
// document, so an emitting test case needs real engine handles.
func runEmitting(t *testing.T, body func(ht *T)) string {
	t.Helper()
	var buf bytes.Buffer
	_ = run(func(tc TestCase) {
		body(&T{testCase: tc.(*testCase), T: t})
	}, WithSingleTestCase(), withOutput(&buf))
	return buf.String()
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

// TestTFatalEmitsIntoReport exercises the emit=true branch of Fatal: the
// message lands in the rendered report, prefixed with the user's file:line,
// and the fatalSentinel still unwinds (recovered by the case driver).
func TestTFatalEmitsIntoReport(t *testing.T) {
	t.Parallel()
	out := runEmitting(t, func(ht *T) { ht.Fatal("fatal message") })
	if !strings.Contains(out, "state_test.go:") || !strings.Contains(out, "fatal message") {
		t.Fatalf("expected a decorated fatal message in the report, got %q", out)
	}
}

func TestTFatalfEmitsIntoReport(t *testing.T) {
	t.Parallel()
	out := runEmitting(t, func(ht *T) { ht.Fatalf("fatal: %d", 42) })
	if !strings.Contains(out, "fatal: 42") {
		t.Fatalf("expected the formatted fatal message in the report, got %q", out)
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

// TestTErrorEmitsIntoReport exercises the emit=true branch of Error.
func TestTErrorEmitsIntoReport(t *testing.T) {
	t.Parallel()
	var status libhegel.Status
	out := runEmitting(t, func(ht *T) {
		ht.Error("something went wrong")
		status = ht.testCase.status
	})
	if status != libhegel.STATUS_INTERESTING {
		t.Error("expected status=INTERESTING after Error()")
	}
	if !strings.Contains(out, "something went wrong") {
		t.Fatalf("expected the message in the report, got %q", out)
	}
}

// TestTErrorfEmitsIntoReport exercises the emit=true branch of Errorf.
func TestTErrorfEmitsIntoReport(t *testing.T) {
	t.Parallel()
	var status libhegel.Status
	out := runEmitting(t, func(ht *T) {
		ht.Errorf("error: %d", 99)
		status = ht.testCase.status
	})
	if status != libhegel.STATUS_INTERESTING {
		t.Error("expected status=INTERESTING after Errorf()")
	}
	if !strings.Contains(out, "error: 99") {
		t.Fatalf("expected the formatted message in the report, got %q", out)
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

// When emit=true, Log/Logf/Note land in the rendered report with the user's
// file:line decoration.
func TestTLogEmitsWhenEmitting(t *testing.T) {
	t.Parallel()
	out := runEmitting(t, func(ht *T) {
		ht.Log("hello", " world")
		ht.Logf("value=%d", 42)
		ht.Note("a note")
	})
	for _, needle := range []string{"hello world", "value=42", "a note"} {
		if !strings.Contains(out, needle) {
			t.Errorf("expected %q in the report, got %q", needle, out)
		}
	}
}

// testCase.Note lands in the rendered report when out is set.
func TestTestCaseNoteWritesToOut(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	_ = run(func(tc TestCase) {
		tc.Note("hello world")
	}, WithSingleTestCase(), withOutput(&buf))
	if got := strings.TrimSpace(buf.String()); got != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", got)
	}
}

// testCase.Errorf writes to the report and sets the failed flag.
func TestTestCaseErrorfWritesAndFails(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	var status libhegel.Status
	_ = run(func(tc TestCase) {
		s := tc.(*testCase)
		s.Errorf("value=%d", 42)
		status = s.status
	}, WithSingleTestCase(), withOutput(&buf))
	if status != libhegel.STATUS_INTERESTING {
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
	_ = run(func(tc TestCase) {
		tc.Log("hello", " world")
	}, WithSingleTestCase(), withOutput(&buf))
	if got := strings.TrimSpace(buf.String()); got != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", got)
	}
}

// *T.reportDraw with emit=true lands in the rendered report, with an
// explain annotation appended as a trailing comment when present. The call
// site doesn't matter — formatDrawReport just needs a non-zero skip frame.
func TestTDrawEmitsIntoReport(t *testing.T) {
	t.Parallel()
	out := runEmitting(t, func(ht *T) {
		n := Draw(ht, Integers(0, 100))
		_ = n
	})
	if !strings.Contains(out, "state_test.go:") || !strings.Contains(out, "n := ") {
		t.Fatalf("expected a named draw line in the report, got %q", out)
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
