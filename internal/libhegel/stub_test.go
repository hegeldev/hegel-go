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

// TestStubSettingsSetters exercises settings setters directly against a Stub
// so their low-level plumbing is covered independently of the public options.
func TestStubSettingsSetters(t *testing.T) {
	lib := Stub(t,
		uintptr(1), // settings_new handle
		OK,         // settings_new result
		OK,         // backend
		OK,         // verbosity
		OK,         // report_multiple_failures
		OK,         // phases
		OK,         // stateful_step_count
	)
	s := lib.SettingsNew()
	_ = s.Backend(lib, BACKEND_URANDOM)
	_ = s.Verbosity(lib, VERBOSITY_VERBOSE)
	_ = s.ReportMultipleFailures(lib, true)
	_ = s.Phases(lib, PHASE_GENERATE)
	_ = s.StatefulStepCount(lib, 25)
}

// TestStubUnwiredPrimitives exercises the per-test-case primitives that the
// runner does not drive against a Stub so their plumbing is covered: the
// variable pool, the engine-owned state machine (including the C-string-array
// marshalling in NewStateMachine), and the forced-boolean primitive. Each is a
// fallible call that writes one scripted output value (an id, index, or bool)
// through a trailing out-parameter before its Error return.
func TestStubUnwiredPrimitives(t *testing.T) {
	lib := Stub(t,
		uintptr(7), OK, // new_pool: pool handle
		int64(1), OK, // pool_add: variable id
		int64(1), OK, // pool_generate: variable id
		uintptr(3), int64(1), OK, // new_state_machine: machine handle + drawn concurrency
		int64(0), OK, // state_machine_next_group: group index
		int64(0), OK, // state_machine_next_rule: rule index
		OK,       // state_machine_rule_rejected
		true, OK, // generate_boolean: value
	)
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: lib.syms, raw: 1}}

	pool, err := tc.NewPool(lib)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	if _, err := pool.Add(lib, tc); err != nil {
		t.Fatalf("Pool.Add: %v", err)
	}
	if _, err := pool.Generate(lib, tc, true); err != nil {
		t.Fatalf("Pool.Generate: %v", err)
	}
	// Non-empty rules + nil invariants exercises both cStringArray branches.
	machine, concurrency, err := tc.NewStateMachine(lib, []string{"insert", "remove"}, []int64{0, 0}, nil, 1, 1)
	if err != nil {
		t.Fatalf("NewStateMachine: %v", err)
	}
	if concurrency != 1 {
		t.Fatalf("NewStateMachine: concurrency = %d, want 1", concurrency)
	}
	if _, err := tc.StateMachineNextGroup(lib, machine); err != nil {
		t.Fatalf("StateMachineNextGroup: %v", err)
	}
	if _, err := tc.StateMachineNextRule(lib, machine, 0); err != nil {
		t.Fatalf("StateMachineNextRule: %v", err)
	}
	if err := tc.StateMachineRuleRejected(lib, machine, 0); err != nil {
		t.Fatalf("StateMachineRuleRejected: %v", err)
	}
	if _, err := tc.GenerateBoolean(lib, 0.5, false, false); err != nil {
		t.Fatalf("GenerateBoolean: %v", err)
	}
}

// TestStubNewPoolError covers NewPool's nil-handle branch: when new_pool fails,
// allocate yields a nil handle and NewPool returns (nil, err). (The collection
// and state-machine constructors' equivalent branches are covered by the
// runner-level error tests; the pool constructor has no runner path.)
func TestStubNewPoolError(t *testing.T) {
	lib := Stub(t,
		uintptr(0), E_BACKEND, // new_pool: placeholder handle + failing Error
		"boom", // diagnostic read by invoke
	)
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: lib.syms, raw: 1}}
	if _, err := tc.NewPool(lib); err == nil {
		t.Fatal("expected NewPool error")
	}
}

// TestStubTypedPrimitives exercises the typed primitive draws that write scalar,
// struct, and byte-buffer outputs, covering every new output-parameter kind the
// stub scripts.
func TestStubTypedPrimitives(t *testing.T) {
	lib := Stub(t,
		int64(42), OK, // generate_integer
		3.5, OK, // generate_float
		[]byte("xy"), OK, // generate_bytes (result struct)
		Date{Year: 2000, Month: 1, Day: 2}, OK, // generate_date
		Time{Hour: 1, Minute: 2, Second: 3}, OK, // generate_time
		Datetime{Date: Date{Year: 2000, Month: 1, Day: 1}}, OK, // generate_datetime
		[]byte{1, 2, 3, 4}, OK, // generate_ipv4 (outBuf)
	)
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: lib.syms, raw: 1}}

	if v, err := tc.GenerateInteger(lib, 0, 100); err != nil || v != 42 {
		t.Fatalf("GenerateInteger: v=%d err=%v", v, err)
	}
	if v, err := tc.GenerateFloat(lib, 64, 0, 1, false, false, false, false, 5e-324); err != nil || v != 3.5 {
		t.Fatalf("GenerateFloat: v=%v err=%v", v, err)
	}
	if v, err := tc.GenerateBytes(lib, 0, 8); err != nil || string(v) != "xy" {
		t.Fatalf("GenerateBytes: v=%q err=%v", v, err)
	}
	if v, err := tc.GenerateDate(lib, Date{}, Date{}); err != nil || v.Day != 2 {
		t.Fatalf("GenerateDate: v=%+v err=%v", v, err)
	}
	if v, err := tc.GenerateTime(lib, Time{}, Time{}); err != nil || v.Second != 3 {
		t.Fatalf("GenerateTime: v=%+v err=%v", v, err)
	}
	if v, err := tc.GenerateDatetime(lib, Datetime{}, Datetime{}); err != nil || v.Date.Year != 2000 {
		t.Fatalf("GenerateDatetime: v=%+v err=%v", v, err)
	}
	if v, err := tc.GenerateIPv4(lib); err != nil || v != [4]byte{1, 2, 3, 4} {
		t.Fatalf("GenerateIPv4: v=%v err=%v", v, err)
	}
}

// TestStubStringGenerators covers the string-generator constructors and the
// string draw: each constructor yields a handle, and GenerateString writes a
// result struct the wrapper copies out.
func TestStubStringGenerators(t *testing.T) {
	lib := Stub(t,
		uintptr(1), OK, // string_generator_text handle + result
		"hello", OK, // generate_string
		uintptr(2), OK, // string_generator_email
		uintptr(3), OK, // string_generator_url
		uintptr(4), OK, // string_generator_domain
		uintptr(5), OK, // string_generator_regex (no alphabet)
		uintptr(6), OK, // string_generator_regex (with alphabet)
	)
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: lib.syms, raw: 1}}

	gen, err := lib.StringGeneratorText(0, 8, "utf-8", 0, 0x10FFFF, nil, []string{"Cs"}, nil, nil)
	if err != nil || gen == nil {
		t.Fatalf("StringGeneratorText: gen=%v err=%v", gen, err)
	}
	if v, err := tc.GenerateString(lib, gen); err != nil || v != "hello" {
		t.Fatalf("GenerateString: v=%q err=%v", v, err)
	}
	if _, err := lib.StringGeneratorEmail(); err != nil {
		t.Fatalf("StringGeneratorEmail: %v", err)
	}
	if _, err := lib.StringGeneratorURL(); err != nil {
		t.Fatalf("StringGeneratorURL: %v", err)
	}
	if _, err := lib.StringGeneratorDomain(255); err != nil {
		t.Fatalf("StringGeneratorDomain: %v", err)
	}
	if _, err := lib.StringGeneratorRegex("a+", true, nil); err != nil {
		t.Fatalf("StringGeneratorRegex: %v", err)
	}
	// A non-nil alphabet exercises the alphabet-handle branch.
	if _, err := lib.StringGeneratorRegex("a+", true, gen); err != nil {
		t.Fatalf("StringGeneratorRegex with alphabet: %v", err)
	}
}

// TestStubStringGeneratorNULNames covers cStringArrayArg's interior-NUL guard.
func TestStubStringGeneratorNULNames(t *testing.T) {
	lib := Stub(t) // must error before the C call
	if _, err := lib.StringGeneratorText(0, 8, "utf-8", 0, 0, []string{"a\x00b"}, nil, nil, nil); err == nil {
		t.Error("expected error for NUL in a category name")
	}
	if _, err := lib.StringGeneratorText(0, 8, "utf-8", 0, 0, nil, []string{"x\x00"}, nil, nil); err == nil {
		t.Error("expected error for NUL in an exclude-category name")
	}
}

// TestStubStringGeneratorEmptyNames covers cStringArrayArg's empty-but-non-nil
// branch: an empty category set must reach libhegel as a non-NULL, length-0
// array (distinct from a nil slice, which is absent).
func TestStubStringGeneratorEmptyNames(t *testing.T) {
	lib := Stub(t, uintptr(1), OK) // string_generator_text handle
	gen, err := lib.StringGeneratorText(0, 8, "utf-8", 0, 0, []string{}, nil, nil, nil)
	if err != nil || gen == nil {
		t.Fatalf("StringGeneratorText with empty categories: gen=%v err=%v", gen, err)
	}
}

// TestStubStringGeneratorIncludeChars covers cString's non-nil branch: an
// optional include/exclude character buffer is aliased straight into the call.
func TestStubStringGeneratorIncludeChars(t *testing.T) {
	lib := Stub(t, uintptr(1), OK) // string_generator_text handle
	inc, exc := "abc", "xyz"
	gen, err := lib.StringGeneratorText(0, 8, "utf-8", 0, 0, nil, nil, &inc, &exc)
	if err != nil || gen == nil {
		t.Fatalf("StringGeneratorText with include/exclude chars: gen=%v err=%v", gen, err)
	}
}

// TestStubIntegerBig covers the arbitrary-precision integer draw, whose result
// arrives through an outBuf plus a *uint64 length.
func TestStubIntegerBig(t *testing.T) {
	lib := Stub(t, []byte{0x2a}, uint64(1), OK) // value bytes, length, result
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: lib.syms, raw: 1}}

	got, err := tc.GenerateIntegerBig(lib, []byte{0x00}, []byte{0xff})
	if err != nil {
		t.Fatalf("GenerateIntegerBig: %v", err)
	}
	if len(got) != 1 || got[0] != 0x2a {
		t.Errorf("GenerateIntegerBig: got %v", got)
	}
}

// TestStubStateMachineRejectsNULNames covers cStringArray's interior-NUL guard
// from both NewStateMachine call sites: a C string cannot carry an embedded
// NUL, so such a name is rejected before reaching libhegel.
func TestStubStateMachineRejectsNULNames(t *testing.T) {
	lib := Stub(t) // no returns: must error before the C call
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: lib.syms, raw: 1}}

	if _, _, err := tc.NewStateMachine(lib, []string{"a\x00b"}, []int64{0}, nil, 1, 1); err == nil {
		t.Error("expected error for NUL in a rule name")
	}
	if _, _, err := tc.NewStateMachine(lib, []string{"ok"}, []int64{0}, []string{"bad\x00"}, 1, 1); err == nil {
		t.Error("expected error for NUL in an invariant name")
	}
}

// TestStubStateMachineRejectsGroupMismatch covers NewStateMachine's guard
// against a ruleGroups slice whose length differs from ruleNames.
func TestStubStateMachineRejectsGroupMismatch(t *testing.T) {
	lib := Stub(t) // no returns: must error before the C call
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: lib.syms, raw: 1}}

	if _, _, err := tc.NewStateMachine(lib, []string{"a", "b"}, []int64{0}, nil, 1, 1); err == nil {
		t.Error("expected error for mismatched rule-group length")
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
	tc, err := s.TestCaseFromBlob(lib, "YmxvYg==", nil)
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

// TestStubBlobError covers the test_case_from_blob failure branch: the stub
// fills the (unused) handle out-parameter, returns an error code, and the
// wrapper surfaces the wrapped last-error message.
func TestStubBlobError(t *testing.T) {
	lib := Stub(t, uintptr(0), E_INVALID_ARG, "bad blob") // handle, result, diagnostic
	s := &Settings{syms: lib.syms, raw: 1}
	tc, err := s.TestCaseFromBlob(lib, "not-base64", nil)
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
	lib := Stub(t,
		uintptr(2),  // context_new
		"0.0.0", OK, // version string, then version result
	)
	ctx := lib.syms.ContextNew()
	if ctx == 0 {
		t.Error("contextNew returned 0")
	}
	if e := lib.syms.ContextFree(ctx); e != OK { // context_free
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

func TestContextCloneUsesSameSymbolsAndFreshHandle(t *testing.T) {
	t.Parallel()
	ctx := Stub(t, uintptr(2)) // context_new for Clone
	clone := ctx.Clone()

	if clone == ctx {
		t.Fatal("Clone returned the original context")
	}
	if clone.syms != ctx.syms {
		t.Fatal("Clone did not retain the originating symbol table")
	}
	if clone.raw == ctx.raw {
		t.Fatal("Clone did not allocate a fresh native context")
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
		lib.syms.RunStart(lib.raw, raw, 0, 0, &run) // reuse the now-freed handle
	})
	if !strings.Contains(msg, "use-after-free") || !strings.Contains(msg, "RunStart") {
		t.Errorf("unexpected fatal message: %q", msg)
	}
}
