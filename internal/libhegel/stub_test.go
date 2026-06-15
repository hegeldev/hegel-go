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
// not (yet) drive — Verbosity, ReportMultipleFailures and Phases — directly
// against a Stub so their plumbing is covered.
func TestStubUnwiredSetters(t *testing.T) {
	lib := Stub(uintptr(1)) // settings_new handle
	s := lib.SettingsNew()
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
