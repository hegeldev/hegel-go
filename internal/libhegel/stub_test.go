package libhegel

import (
	"runtime"
	"strings"
	"testing"
)

// TestStubMissingReturnPanics covers the underflow guard in Stub's retval
// closure: popping more return values than were supplied panics with an
// index-bearing message.
func TestStubMissingReturnPanics(t *testing.T) {
	lib := Stub() // no returns supplied
	defer func() {
		r := recover()
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %v", r)
		}
		if !strings.Contains(msg, "missing 1'th return value") {
			t.Errorf("unexpected panic message: %q", msg)
		}
	}()
	lib.SettingsNew() // pops the (absent) first return => panic
}

// TestStubUnwiredSetters exercises the settings setters that the runner does
// not (yet) drive — Backend, Verbosity, ReportMultipleFailures and Phases —
// directly against a Stub so their plumbing is covered.
func TestStubUnwiredSetters(t *testing.T) {
	lib := Stub(uintptr(1)) // settings_new handle
	s := lib.SettingsNew()
	s.Backend(BACKEND_URANDOM)
	s.Verbosity(VERBOSITY_VERBOSE)
	s.ReportMultipleFailures(true)
	s.Phases(PHASE_GENERATE)
}

// TestStubUnwiredPrimitives exercises the per-test-case primitives that the
// runner does not (yet) drive — the variable pool, the engine-owned state
// machine, and the forced-boolean primitive — against a Stub so their plumbing
// (including the C-string-array marshalling in NewStateMachine) is covered.
func TestStubUnwiredPrimitives(t *testing.T) {
	lib := Stub(
		OK, // new_pool
		OK, // pool_add
		OK, // pool_generate
		OK, // new_state_machine
		OK, // state_machine_next_rule
		OK, // primitive_boolean
	)
	tc := &TestCase{lib: lib, ptr: 1}

	pool, err := tc.NewPool()
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	if _, err := tc.PoolAdd(pool); err != nil {
		t.Fatalf("PoolAdd: %v", err)
	}
	if _, err := tc.PoolGenerate(pool, true); err != nil {
		t.Fatalf("PoolGenerate: %v", err)
	}
	// Non-empty rules + nil invariants exercises both cStringArray branches.
	machine, err := tc.NewStateMachine([]string{"insert", "remove"}, nil)
	if err != nil {
		t.Fatalf("NewStateMachine: %v", err)
	}
	if _, err := tc.StateMachineNextRule(machine); err != nil {
		t.Fatalf("StateMachineNextRule: %v", err)
	}
	if _, err := tc.PrimitiveBoolean(0.5, false, false); err != nil {
		t.Fatalf("PrimitiveBoolean: %v", err)
	}
}

// TestStubStateMachineRejectsNULNames covers cStringArray's interior-NUL guard
// from both NewStateMachine call sites: a C string cannot carry an embedded
// NUL, so such a name is rejected before reaching libhegel.
func TestStubStateMachineRejectsNULNames(t *testing.T) {
	tc := &TestCase{lib: Stub(), ptr: 1} // no returns: must error before the C call

	if _, err := tc.NewStateMachine([]string{"a\x00b"}, nil); err == nil {
		t.Error("expected error for NUL in a rule name")
	}
	if _, err := tc.NewStateMachine([]string{"ok"}, []string{"bad\x00"}); err == nil {
		t.Error("expected error for NUL in an invariant name")
	}
}

// TestStubBlobAndFailureAccessors covers the standalone-replay constructor and
// the failure accessors that the runner does not (yet) wire into
// collectFailures.
func TestStubBlobAndFailureAccessors(t *testing.T) {
	lib := Stub(
		uintptr(1), // test_case_from_blob handle
		"boom",     // failure_panic_message
		"YmxvYg==", // failure_reproduction_blob
	)

	s := &Settings{lib: lib, ptr: 1}
	tc, err := s.TestCaseFromBlob("YmxvYg==")
	if err != nil || tc == nil {
		t.Fatalf("TestCaseFromBlob: tc=%v err=%v", tc, err)
	}

	f := &Failure{lib: lib, ptr: 1}
	if got := f.PanicMessage(); got != "boom" {
		t.Errorf("PanicMessage: got %q", got)
	}
	if got := f.ReproductionBlob(); got != "YmxvYg==" {
		t.Errorf("ReproductionBlob: got %q", got)
	}
}

// TestStubWithContextDiagnostic covers withContext's non-empty-diagnostic arm:
// a failed op whose context records a message wraps that message into the error.
func TestStubWithContextDiagnostic(t *testing.T) {
	tc := &TestCase{lib: Stub(E_BACKEND, "boom"), ptr: 1}
	err := tc.StopSpan(false)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected error carrying the diagnostic, got %v", err)
	}
}

// TestContextRawPointerNil covers rawPointer's nil-context branch, which guards
// the wrap closures against a context that failed to allocate.
func TestContextRawPointerNil(t *testing.T) {
	var c *context
	if got := c.rawPointer(); got != 0 {
		t.Fatalf("expected 0 for a nil context, got %d", got)
	}
}

// TestStubFailureOutOfRange covers Result.Failure's NULL return (an out-of-range
// index, or a NULL result) and, through it, wrap's nil-context NULL path and
// libContext.lastError's nil-context branch: hegel_run_result_failure takes no
// context, so wrap is handed a nil one and a NULL handle yields a nil *Failure.
func TestStubFailureOutOfRange(t *testing.T) {
	r := &Result{lib: Stub(uintptr(0)), ptr: 1} // failure handle NULL
	if f := r.Failure(0); f != nil {
		t.Fatalf("expected nil failure for an out-of-range index, got %v", f)
	}
}

// TestStubBlobRejectedNoDiagnostic covers wrap's NULL-with-no-diagnostic path:
// a from_blob that returns NULL while the context records no diagnostic yields
// (nil, nil) — a blob rejected without a diagnostic. The stub supplies the NULL
// handle followed by the empty diagnostic string that contextLastError pops.
func TestStubBlobRejectedNoDiagnostic(t *testing.T) {
	s := &Settings{lib: Stub(uintptr(0), ""), ptr: 1} // from_blob returns NULL, no diagnostic
	tc, err := s.TestCaseFromBlob("bad")
	if tc != nil || err != nil {
		t.Fatalf("expected (nil, nil) for a rejected blob with no diagnostic, got tc=%v err=%v", tc, err)
	}
}

// TestStubBlobFreeCleanup covers the standalone-test-case free closure, which
// (unlike the run-owned test case) must thread the context into
// hegel_test_case_free. It only runs on GC finalization, so the test forces a
// collection and waits for the cleanup to fire.
func TestStubBlobFreeCleanup(t *testing.T) {
	s := &Settings{lib: Stub(uintptr(1)), ptr: 1} // from_blob returns a handle
	tc, err := s.TestCaseFromBlob("YmxvYg==")
	if err != nil || tc == nil {
		t.Fatalf("TestCaseFromBlob: tc=%v err=%v", tc, err)
	}

	// Attach a sentinel cleanup to the same wrapper; when it fires the wrapper
	// has been collected and its internal free closure has been scheduled too.
	done := make(chan struct{})
	runtime.AddCleanup(tc, func(struct{}) { close(done) }, struct{}{})

	tc = nil // drop the only reference so the wrapper becomes collectable
	for {
		runtime.GC()
		select {
		case <-done:
			return
		default:
			runtime.Gosched()
		}
	}
}
