package libhegel

import (
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
	lib := Stub(
		uintptr(1), // settings_new handle
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
	lib := Stub() // no returns: must error before the C call
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
	lib := Stub(
		uintptr(1),       // test_case_from_blob handle
		"prop_test.go:7", // failure_origin
		"YmxvYg==",       // failure_reproduction_blob
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

// TestStubBlobError covers the test_case_from_blob failure branch: the stub
// returns an error code and the wrapper surfaces a wrapped error.
func TestStubBlobError(t *testing.T) {
	lib := Stub(E_INVALID_ARG, "bad blob")
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
// check()'s empty-message arm: SettingsNew swallows the error and returns nil.
func TestStubSettingsNewError(t *testing.T) {
	lib := Stub(E_INTERNAL, "") // settings_new fails with no diagnostic
	if s := lib.SettingsNew(); s != nil {
		t.Fatalf("expected nil settings on error, got %v", s)
	}
}

// TestStubLifecycleClosures covers the resource-management closures that
// production reaches only via [Context.Close] or GC-scheduled cleanups
// (context_new/free, the *_free family, version), which no higher-level test
// drives deterministically against the stub.
func TestStubLifecycleClosures(t *testing.T) {
	lib := Stub("0.0.0") // version reader
	if got := lib.syms.contextNew(); got == 0 {
		t.Error("contextNew returned 0")
	}
	if e := lib.syms.contextFree(lib.raw); e != OK { // context_free
		t.Errorf("context_free: %v", e)
	}
	// Frees run with a NULL context in production; invoke them directly here.
	if e := lib.syms.settingsFree(0, 0); e != OK {
		t.Errorf("settings_free: %v", e)
	}
	if e := lib.syms.runFree(0, 0); e != OK {
		t.Errorf("run_free: %v", e)
	}
	if e := lib.syms.testCaseFree(0, 0); e != OK {
		t.Errorf("test_case_free: %v", e)
	}
	if got := lib.syms.Version(); got != "0.0.0" {
		t.Errorf("Version: got %q", got)
	}
}

// TestGoStringNil covers goString's NULL-pointer branch (libhegel writes NULL
// for an absent string, e.g. the error message of a run that did not error).
func TestGoStringNil(t *testing.T) {
	if got := goString(nil); got != "" {
		t.Errorf("goString(nil) = %q, want empty", got)
	}
}
