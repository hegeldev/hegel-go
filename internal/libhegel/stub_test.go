package libhegel

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// recordingTB is a minimal [testingTB] that captures the first Errorf message
// instead of failing the enclosing test, so tests can assert on the
// diagnostics Stub reports for a misconfigured or misused stub. It unwinds via
// [runtime.Goexit] after recording so the stub does not continue past the
// reported violation; callers therefore drive the failing operation under
// [captureError], on its own goroutine.
type recordingTB struct{ err string }

func (r *recordingTB) Helper() {}

func (r *recordingTB) Errorf(format string, args ...any) {
	r.err = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

// captureError runs fn on a dedicated goroutine and returns the message fn's
// stub passed to [testingTB.Errorf], or "" if it never failed. The channel
// receive synchronizes with the goroutine's exit, so reading err is race-free.
func captureError(fn func(testingTB)) string {
	tb := &recordingTB{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(tb)
	}()
	<-done
	return tb.err
}

// TestStubMissingReturnErrors covers the underflow guard in Stub's retval
// closure: popping more return values than were supplied fails with an
// index-bearing message.
func TestStubMissingReturnErrors(t *testing.T) {
	msg := captureError(func(tb testingTB) {
		lib := Stub(tb)   // no returns supplied
		lib.SettingsNew() // pops the (absent) first return => error
	})
	if !strings.Contains(msg, "missing 1'th return value") {
		t.Errorf("unexpected error message: %q", msg)
	}
}

// TestStubUnwiredSetters exercises the settings setters that the runner does
// not (yet) drive — Backend, Verbosity, ReportMultipleFailures and Phases —
// directly against a Stub so their plumbing is covered.
func TestStubUnwiredSetters(t *testing.T) {
	lib := Stub(t,
		uintptr(1), // settings_new handle
		OK,         // settings_new result
		OK,         // backend
		OK,         // verbosity
		OK,         // report_multiple_failures
		OK,         // phases
	)
	s := lib.SettingsNew()
	_ = s.Backend(lib, BACKEND_URANDOM)
	_ = s.Verbosity(lib, VERBOSITY_VERBOSE)
	_ = s.ReportMultipleFailures(lib, true)
	_ = s.Phases(lib, PHASE_GENERATE)
}

// TestStubUnwiredPrimitives exercises the per-test-case primitives that the
// runner does not drive against a Stub so their plumbing is covered: the
// variable pool, the engine-owned state machine (including the C-string-array
// marshalling in NewStateMachine), and the forced-boolean primitive. Each is a
// fallible call whose output parameters are all of the kinds the stub ignores
// (*int64 / *bool / cStringArrayPtr inputs / the int64-based *StateMachine), so
// each consumes exactly one Error return value.
func TestStubUnwiredPrimitives(t *testing.T) {
	lib := Stub(t,
		OK, // new_pool
		OK, // pool_add
		OK, // pool_generate
		OK, // new_state_machine
		OK, // state_machine_next_rule
		OK, // primitive_boolean
	)
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: lib.syms, raw: 1}}

	pool, err := tc.NewPool(lib)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	if _, err := tc.PoolAdd(lib, pool); err != nil {
		t.Fatalf("PoolAdd: %v", err)
	}
	if _, err := tc.PoolGenerate(lib, pool, true); err != nil {
		t.Fatalf("PoolGenerate: %v", err)
	}
	// Non-empty rules + nil invariants exercises both cStringArray branches.
	machine, err := tc.NewStateMachine(lib, []string{"insert", "remove"}, nil)
	if err != nil {
		t.Fatalf("NewStateMachine: %v", err)
	}
	if _, err := tc.StateMachineNextRule(lib, machine); err != nil {
		t.Fatalf("StateMachineNextRule: %v", err)
	}
	if _, err := tc.PrimitiveBoolean(lib, 0.5, false, false); err != nil {
		t.Fatalf("PrimitiveBoolean: %v", err)
	}
}

// TestStubStateMachineRejectsNULNames covers cStringArray's interior-NUL guard
// from both NewStateMachine call sites: a C string cannot carry an embedded
// NUL, so such a name is rejected before reaching libhegel.
func TestStubStateMachineRejectsNULNames(t *testing.T) {
	lib := Stub(t) // no returns: must error before the C call
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: lib.syms, raw: 1}}

	if _, err := tc.NewStateMachine(lib, []string{"a\x00b"}, nil); err == nil {
		t.Error("expected error for NUL in a rule name")
	}
	if _, err := tc.NewStateMachine(lib, []string{"ok"}, []string{"bad\x00"}); err == nil {
		t.Error("expected error for NUL in an invariant name")
	}
}

// TestStubBlobAndFailureAccessors covers the standalone-replay constructor and
// the failure accessors that the runner does not (yet) wire into
// collectFailures.
func TestStubBlobAndFailureAccessors(t *testing.T) {
	lib := Stub(t,
		uintptr(1),       // test_case_from_blob handle
		OK,               // test_case_from_blob result
		"prop_test.go:7", // failure_origin
		OK,               // failure_origin result
		"YmxvYg==",       // failure_reproduction_blob
		OK,               // failure_reproduction_blob result
	)

	s := &Settings{syms: lib.syms, raw: 1}
	tc, err := s.TestCaseFromBlob(lib, "YmxvYg==")
	if err != nil || tc == nil {
		t.Fatalf("TestCaseFromBlob: tc=%v err=%v", tc, err)
	}

	f := &Failure{pointer: &pointer[failureT]{syms: lib.syms, raw: 1}}
	if got := f.Origin(lib); got != "prop_test.go:7" {
		t.Errorf("Origin: got %q", got)
	}
	if got := f.ReproductionBlob(lib); got != "YmxvYg==" {
		t.Errorf("ReproductionBlob: got %q", got)
	}
}

// TestStubClone covers TestCase.Clone: the happy path yields a fresh handle,
// while a zero out-handle with an OK result (the engine's "no object"
// sentinel) surfaces as a nil test case and no error.
func TestStubClone(t *testing.T) {
	lib := Stub(t,
		uintptr(2), OK, // clone handle, result
		uintptr(0), OK, // sentinel handle, result
	)
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: lib.syms, raw: 1}}

	clone, err := tc.Clone(lib)
	if err != nil || clone == nil {
		t.Fatalf("Clone: clone=%v err=%v", clone, err)
	}

	nilClone, err := tc.Clone(lib)
	if err != nil || nilClone != nil {
		t.Fatalf("Clone sentinel: clone=%v err=%v", nilClone, err)
	}
}

// TestStubCloneError covers Clone's failure branch: a non-OK result surfaces
// the wrapped last-error message and a nil test case.
func TestStubCloneError(t *testing.T) {
	lib := Stub(t, uintptr(0), E_INVALID_HANDLE, "null tc") // handle, result, diagnostic
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: lib.syms, raw: 1}}

	clone, err := tc.Clone(lib)
	if err == nil || clone != nil {
		t.Fatalf("expected error, got clone=%v err=%v", clone, err)
	}
	if !strings.Contains(err.Error(), "null tc") {
		t.Errorf("expected diagnostic in error, got %q", err)
	}
}

// TestStubGenerateSuccess covers a call with multiple out-parameters:
// hegel_generate writes both the generated bytes (**byte) and their length
// (*uint64), each consuming its own scripted value in order, followed by the
// Error return value.
func TestStubGenerateSuccess(t *testing.T) {
	lib := Stub(t, "abc", uint64(3), OK) // bytes, length, then result
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: lib.syms, raw: 1}}

	got, err := tc.Generate(lib, []byte("schema"))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if string(got) != "abc" {
		t.Errorf("Generate: got %q, want %q", got, "abc")
	}
}

// TestStubBlobError covers the test_case_from_blob failure branch: the stub
// fills the (unused) handle out-parameter, returns an error code, and the
// wrapper surfaces the wrapped last-error message.
func TestStubBlobError(t *testing.T) {
	lib := Stub(t, uintptr(0), E_INVALID_ARG, "bad blob") // handle, result, diagnostic
	s := &Settings{syms: lib.syms, raw: 1}
	tc, err := s.TestCaseFromBlob(lib, "not-base64")
	if err == nil || tc != nil {
		t.Fatalf("expected error, got tc=%v err=%v", tc, err)
	}
	if !strings.Contains(err.Error(), "bad blob") {
		t.Errorf("expected diagnostic in error, got %q", err)
	}
}

// TestStubSettingsNewError covers the settings_new failure branch together with
// invoke()'s empty-message arm: SettingsNew swallows the error and returns nil.
func TestStubSettingsNewError(t *testing.T) {
	lib := Stub(t, uintptr(0), E_INTERNAL, "") // handle, result, no diagnostic
	if s := lib.SettingsNew(); s != nil {
		t.Fatalf("expected nil settings on error, got %v", s)
	}
}

// TestStubLifecycleClosures covers the resource-management closures that
// production reaches only via [Context.Close] or GC-scheduled cleanups
// (context_new/free, the *_free family, version), which no higher-level test
// drives deterministically against the stub.
func TestStubLifecycleClosures(t *testing.T) {
	lib := Stub(t, "0.0.0", OK) // version string, then version result
	if got := lib.syms.ContextNew(); got == 0 {
		t.Error("contextNew returned 0")
	}
	if e := lib.syms.ContextFree(lib.raw); e != OK { // context_free
		t.Errorf("context_free: %v", e)
	}
	// Frees run with a NULL context in production; invoke them directly here.
	if e := lib.syms.SettingsFree(0, 0); e != OK {
		t.Errorf("settings_free: %v", e)
	}
	if e := lib.syms.RunFree(0, 0); e != OK {
		t.Errorf("run_free: %v", e)
	}
	if e := lib.syms.TestCaseFree(0, 0); e != OK {
		t.Errorf("test_case_free: %v", e)
	}
	if e := lib.syms.ResultFree(0, 0); e != OK {
		t.Errorf("run_result_free: %v", e)
	}
	if e := lib.syms.FailureFree(0, 0); e != OK {
		t.Errorf("failure_free: %v", e)
	}
	if got := lib.syms.versionString(); got != "0.0.0" {
		t.Errorf("versionString: got %q", got)
	}
}

// TestGoStringNil covers goString's NULL-pointer branch (libhegel writes NULL
// for an absent string, e.g. the error message of a run that did not error).
func TestGoStringNil(t *testing.T) {
	if got := goString(nil); got != "" {
		t.Errorf("goString(nil) = %q, want empty", got)
	}
}

// TestHandleTrackerUseAfterFree exercises the ownership tracker's borrow,
// cascade and use-after-free detection directly: no libhegel symbol produces a
// borrowed handle today, so the cascade is reachable only by driving the
// tracker. Freeing an owner marks every handle borrowed from it dead, and
// checking such a handle afterwards reports a use-after-free.
func TestHandleTrackerUseAfterFree(t *testing.T) {
	h := newHandleTracker()
	h.track(settingsT(1)) // owned root

	if h.check(runT(1)) { // alive
		t.Error("alive handle reported as use-after-free")
	}

	// Freeing the NULL context (as cleanup-time frees do) must not turn a later
	// NULL argument into a spurious use-after-free.
	h.free(ctxT(0))
	if h.check(ctxT(0)) {
		t.Error("freed NULL handle reported as use-after-free")
	}

	h.free(settingsT(1)) // cascades to the borrowed run handle
}

// TestStubUseAfterFreeErrors drives the Stub closure's use-after-free guard
// end-to-end: GC runs a dropped wrapper's cleanup, freeing the underlying
// handle, and passing that handle to a later libhegel call fails with a
// diagnostic naming the symbol, argument index and handle type.
func TestStubUseAfterFreeErrors(t *testing.T) {
	msg := captureError(func(tb testingTB) {
		lib := Stub(tb, uintptr(1), OK) // settings_new
		s := lib.SettingsNew()
		raw := s.raw
		s = nil // drop the wrapper so its GC cleanup frees the handle
		_ = s
		runtime.GC()
		runtime.Gosched()
		runtime.GC()
		var run runT
		lib.syms.RunStart(lib.raw, raw, &run) // reuse the now-freed handle
	})
	if !strings.Contains(msg, "use-after-free") || !strings.Contains(msg, "RunStart") {
		t.Errorf("unexpected fatal message: %q", msg)
	}
}
