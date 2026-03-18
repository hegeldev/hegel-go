package hegel

import (
	"testing"
)

// makeFakeT creates a *T with a zero TestCase and a real *testing.T
// for unit testing T methods in state.go.
func makeFakeT(t *testing.T) *T {
	return &T{
		TestCase: &TestCase{},
		T:        t,
	}
}

// =============================================================================
// T.Fatal / T.Fatalf / T.FailNow — panic with fatalSentinel
// =============================================================================

func TestTFatalPanicsWithSentinel(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected Fatal to panic")
		}
		fs, ok := r.(fatalSentinel)
		if !ok {
			t.Fatalf("expected fatalSentinel, got %T: %v", r, r)
		}
		if fs.msg != "fatal message" {
			t.Errorf("expected msg %q, got %q", "fatal message", fs.msg)
		}
	}()
	ht.Fatal("fatal message")
}

func TestTFatalfPanicsWithSentinel(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected Fatalf to panic")
		}
		fs, ok := r.(fatalSentinel)
		if !ok {
			t.Fatalf("expected fatalSentinel, got %T: %v", r, r)
		}
		if fs.msg != "fatal: 42" {
			t.Errorf("expected msg %q, got %q", "fatal: 42", fs.msg)
		}
	}()
	ht.Fatalf("fatal: %d", 42)
}

func TestTFailNowPanicsWithSentinel(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected FailNow to panic")
		}
		fs, ok := r.(fatalSentinel)
		if !ok {
			t.Fatalf("expected fatalSentinel, got %T: %v", r, r)
		}
		if fs.msg != "FailNow called" {
			t.Errorf("expected msg %q, got %q", "FailNow called", fs.msg)
		}
	}()
	ht.FailNow()
}

// =============================================================================
// T.Skip / T.Skipf / T.SkipNow — call Assume(false), panic with assumeRejected
// =============================================================================

func TestTSkipPanicsWithAssumeRejected(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected Skip to panic")
		}
		if _, ok := r.(assumeRejected); !ok {
			t.Fatalf("expected assumeRejected, got %T: %v", r, r)
		}
	}()
	ht.Skip("skipping")
}

func TestTSkipfPanicsWithAssumeRejected(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected Skipf to panic")
		}
		if _, ok := r.(assumeRejected); !ok {
			t.Fatalf("expected assumeRejected, got %T: %v", r, r)
		}
	}()
	ht.Skipf("skip: %d", 1)
}

func TestTSkipNowPanicsWithAssumeRejected(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected SkipNow to panic")
		}
		if _, ok := r.(assumeRejected); !ok {
			t.Fatalf("expected assumeRejected, got %T: %v", r, r)
		}
	}()
	ht.SkipNow()
}

// =============================================================================
// T.Error / T.Errorf — set failed flag, call Note
// =============================================================================

func TestTErrorSetsFailedAndCallsNote(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	// Make state final so Note actually fires (verify no panic).
	ht.TestCase.isFinal = true
	noted := false
	ht.TestCase.noteFn = func(msg string) { noted = true }
	ht.Error("something went wrong")
	if !ht.TestCase.failed {
		t.Error("expected failed to be true after Error()")
	}
	if !noted {
		t.Error("expected Note to be called by Error()")
	}
}

func TestTErrorfSetsFailedAndCallsNote(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	ht.TestCase.isFinal = true
	noted := false
	ht.TestCase.noteFn = func(msg string) { noted = true }
	ht.Errorf("error: %d", 99)
	if !ht.TestCase.failed {
		t.Error("expected failed to be true after Errorf()")
	}
	if !noted {
		t.Error("expected Note to be called by Errorf()")
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
// T.Log / T.Logf — route through Note
// =============================================================================

func TestTLogCallsNote(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	ht.TestCase.isFinal = true
	var gotMsg string
	ht.TestCase.noteFn = func(msg string) { gotMsg = msg }
	ht.Log("hello", " world")
	if gotMsg != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", gotMsg)
	}
}

func TestTLogfCallsNote(t *testing.T) {
	t.Parallel()
	ht := makeFakeT(t)
	ht.TestCase.isFinal = true
	var gotMsg string
	ht.TestCase.noteFn = func(msg string) { gotMsg = msg }
	ht.Logf("value=%d", 42)
	if gotMsg != "value=42" {
		t.Errorf("expected %q, got %q", "value=42", gotMsg)
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
		if msg != "hegel: nested t.Run is not supported inside a property test" {
			t.Errorf("unexpected panic message: %q", msg)
		}
	}()
	ht.Run("sub", func(_ *testing.T) {})
}
