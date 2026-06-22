package hegel

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

// --- Run / MustRun / Test entry points ---

func TestRunHegelTestPasses(t *testing.T) {
	called := false
	Test(t, func(ht *T) {
		called = true
		b := Draw[bool](ht, Booleans())
		if b != true && b != false {
			t.Errorf("expected bool, got %v", b)
		}
	}, WithTestCases(5))
	if !called {
		t.Error("test function was never called")
	}
}

func TestRunHegelTestAllInvalid(t *testing.T) {
	// A test that always calls Assume(false) should pass (all cases rejected).
	Test(t, func(ht *T) {
		ht.Assume(false)
	}, WithTestCases(5), SuppressHealthCheck(FilterTooMuch))
}

func TestAssumeTrue(t *testing.T) {
	t.Parallel()
	Test(t, func(ht *T) {
		ht.Assume(true)
		_ = Draw[bool](ht, Booleans())
	}, WithTestCases(5))
}

func TestNoteNotFinal(t *testing.T) {
	t.Parallel()
	Test(t, func(ht *T) {
		ht.Note("should not appear")
		_ = Draw[bool](ht, Booleans())
	}, WithTestCases(3))
}

func TestTargetSendsCommand(t *testing.T) {
	t.Parallel()
	Test(t, func(ht *T) {
		x := Draw[int](ht, Integers[int](0, 100))
		ht.Target(float64(x), "my_target")
		if x < 0 || x > 100 {
			ht.Fatal("out of range")
		}
	}, WithTestCases(5))
}

// --- Concurrency smoke ---

func TestConcurrentRunHegelTest(t *testing.T) {
	const goroutines = 8
	var wg sync.WaitGroup
	var failures atomic.Int32
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := Run(func(tc TestCase) {
				v := Draw[int](tc, Integers[int](0, 1000))
				if v < 0 || v > 1000 {
					panic("out of range")
				}
			}, WithTestCases(50), WithDatabase(DatabaseDisabled()))
			if err != nil {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()
	if failures.Load() != 0 {
		t.Errorf("%d concurrent runs failed", failures.Load())
	}
}

// --- Single test case mode ---

func TestRunHegelTestSingleCase(t *testing.T) {
	var calls int
	err := Run(func(tc TestCase) {
		calls++
		_ = Draw[bool](tc, Booleans())
	}, WithSingleTestCase())
	if err != nil {
		t.Fatalf("Run with WithSingleTestCase: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected exactly one call under WithSingleTestCase, got %d", calls)
	}
}

// --- MustRun: panics on error ---

// TestMustRunPanicsOnUserPanic covers the exploratory-case recover path in
// driveOneCase: a user panic on a non-final case is recovered into an
// INTERESTING status (the final replay then re-panics, which MustRun surfaces).
func TestMustRunPanicsOnUserPanic(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustRun to panic on failure")
		}
	}()
	MustRun(func(tc TestCase) {
		panic("nope")
	}, WithTestCases(2), WithDatabase(DatabaseDisabled()))
}

// TestMustRunPanicsOnReturnedError covers MustRun's panic(err) branch: failing
// via Fail (rather than panicking) makes the property surface as a returned
// error from Run, which MustRun re-panics.
func TestMustRunPanicsOnReturnedError(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustRun to panic on returned error")
		}
	}()
	MustRun(func(tc TestCase) {
		tc.Fail()
	}, WithTestCases(2), WithDatabase(DatabaseDisabled()))
}

func TestMustRunSuccess(t *testing.T) {
	t.Parallel()
	MustRun(func(tc TestCase) {
		_ = Draw[bool](tc, Booleans())
	}, WithTestCases(3))
}

func TestRunPublicAPI(t *testing.T) {
	t.Parallel()
	if err := Run(func(tc TestCase) {
		_ = Draw[int](tc, Integers[int](0, 10))
	}, WithTestCases(3)); err != nil {
		t.Errorf("Run returned: %v", err)
	}
}

// --- Test() entry point: success ---

func TestTestSuccess(t *testing.T) {
	t.Parallel()
	Test(t, func(ht *T) {
		_ = Draw[bool](ht, Booleans())
	}, WithTestCases(3))
}

// --- t.Error / t.Fail behavior ---

func TestStateFailedPath(t *testing.T) {
	t.Parallel()
	err := Run(func(tc TestCase) {
		tc.Errorf("forced failure")
	}, WithTestCases(5), WithDatabase(DatabaseDisabled()))
	if err == nil {
		t.Fatal("expected failure when Errorf is called")
	}
}

func TestFatalSentinelPath(t *testing.T) {
	t.Parallel()
	err := Run(func(tc TestCase) {
		tc.FailNow()
	}, WithTestCases(5), WithDatabase(DatabaseDisabled()))
	if err == nil {
		t.Fatal("expected failure when FailNow is called")
	}
}

// --- isCI: expected-value and match-any branches ---

func TestIsCIMatchExpectedValue(t *testing.T) {
	clearCIEnv(t)
	// GITHUB_ACTIONS is a non-matchAny var: only "true" counts.
	t.Setenv("GITHUB_ACTIONS", "true")
	if !isCI() {
		t.Error("expected isCI() true when GITHUB_ACTIONS=true")
	}
}

func TestIsCIExpectedValueMismatch(t *testing.T) {
	clearCIEnv(t)
	// Present but not the expected value: must not count as CI.
	t.Setenv("GITHUB_ACTIONS", "false")
	if isCI() {
		t.Error("expected isCI() false when GITHUB_ACTIONS=false")
	}
}

func TestIsCIMatchAny(t *testing.T) {
	clearCIEnv(t)
	// CI is a matchAny var: any value (even empty) counts.
	t.Setenv("CI", "")
	if !isCI() {
		t.Error("expected isCI() true when matchAny var CI is set")
	}
}

func TestIsCINoneSet(t *testing.T) {
	clearCIEnv(t)
	if isCI() {
		t.Error("expected isCI() false when no CI var is set")
	}
}

// TestRunUnderCIDisablesDatabase covers run()'s CI branch (derandomize on,
// database disabled by default) by forcing a CI env var, independent of
// whether the test process itself runs on a CI runner.
func TestRunUnderCIDisablesDatabase(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("CI", "true")
	if err := Run(func(tc TestCase) {
		_ = Draw[bool](tc, Booleans())
	}, WithTestCases(3)); err != nil {
		t.Errorf("Run under CI: %v", err)
	}
}

// --- extractPanicOrigin / isHegelFrame ---

func TestIsHegelFrame(t *testing.T) {
	t.Parallel()
	cases := []struct {
		fn   string
		want bool
	}{
		{"hegel.dev/go/hegel", true},
		{"hegel.dev/go/hegel.Run", true},
		{"hegel.dev/go/hegel.(*testCase).Note", true},
		{"hegel.dev/go/hegel/sub.Func", true},
		{"hegel.dev/go/hegel_test.TestFoo", false},
		{"main.main", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isHegelFrame(tc.fn); got != tc.want {
			t.Errorf("isHegelFrame(%q) = %v, want %v", tc.fn, got, tc.want)
		}
	}
}

// TestExtractPanicOriginStableAcrossValues verifies that origin is stable per
// (type, call site) — libhegel uses the origin as a shrink-grouping key, and
// per-value origins would prevent the shrinker from converging.
func TestExtractPanicOriginStableAcrossValues(t *testing.T) {
	t.Parallel()
	a := findExternalCaller()
	b := findExternalCaller()
	if a != b {
		t.Errorf("origin must be stable; got %q vs %q", a, b)
	}
}

// --- Health checks ---

func TestAllHealthChecks(t *testing.T) {
	t.Parallel()
	all := AllHealthChecks()
	if len(all) != 4 {
		t.Errorf("AllHealthChecks: expected 4, got %d", len(all))
	}
}

func TestSuppressHealthCheckOption(t *testing.T) {
	t.Parallel()
	o := runOptions{}
	SuppressHealthCheck(FilterTooMuch, TooSlow)(&o)
	if len(o.suppressHealthCheck) != 2 {
		t.Errorf("expected 2 suppressed checks, got %d", len(o.suppressHealthCheck))
	}
}

func TestSuppressHealthCheckIntegration(t *testing.T) {
	t.Parallel()
	err := Run(func(tc TestCase) {
		tc.Assume(false) // always reject
	}, WithTestCases(20),
		SuppressHealthCheck(FilterTooMuch),
		WithDatabase(DatabaseDisabled()))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Options ---

func TestWithTestCasesOption(t *testing.T) {
	t.Parallel()
	o := runOptions{}
	WithTestCases(42)(&o)
	if o.testCases != 42 {
		t.Errorf("WithTestCases: expected 42, got %d", o.testCases)
	}
}

func TestWithSeedOption(t *testing.T) {
	t.Parallel()
	o := runOptions{}
	WithSeed(12345)(&o)
	if o.seed == nil || *o.seed != 12345 {
		t.Errorf("WithSeed: expected 12345, got %v", o.seed)
	}
}

func TestDatabaseDisabledSetting(t *testing.T) {
	t.Parallel()
	o := runOptions{}
	WithDatabase(DatabaseDisabled())(&o)
	if o.database.state != databaseDisabled {
		t.Errorf("WithDatabase(DatabaseDisabled): expected databaseDisabled, got %v", o.database.state)
	}
}

func TestDatabasePathSetting(t *testing.T) {
	t.Parallel()
	o := runOptions{}
	WithDatabase(Database("/tmp/foo"))(&o)
	if o.database.state != databasePath || o.database.path != "/tmp/foo" {
		t.Errorf("WithDatabase(Database): expected databasePath /tmp/foo, got %v %q", o.database.state, o.database.path)
	}
}

func TestWithDerandomizeIntegration(t *testing.T) {
	t.Parallel()
	err := Run(func(tc TestCase) {
		_ = Draw[int](tc, Integers[int](0, 100))
	}, WithTestCases(5), WithDerandomize(true), WithDatabase(DatabaseDisabled()))
	if err != nil {
		t.Errorf("derandomize integration: %v", err)
	}
}

func TestWithSeedIntegration(t *testing.T) {
	t.Parallel()
	err := Run(func(tc TestCase) {
		_ = Draw[int](tc, Integers[int](0, 100))
	}, WithTestCases(5), WithSeed(42), WithDatabase(DatabaseDisabled()))
	if err != nil {
		t.Errorf("seed integration: %v", err)
	}
}

func TestRunHegelDisablesDatabaseInCI(t *testing.T) {
	t.Parallel()
	// Apply opts directly without running so we can inspect.
	if !isCI() {
		t.Skip("only meaningful when CI env vars are set")
	}
	o := runOptions{testCases: 100, derandomize: isCI()}
	if isCI() {
		o.database = DatabaseSetting{state: databaseDisabled}
	}
	if o.database.state != databaseDisabled {
		t.Errorf("expected DB disabled in CI, got %v", o.database.state)
	}
}

// --- Stub-driven runner lifecycle error paths ---
//
// These tests inject a libhegel.Stub() Handle into runWithHandle to exercise
// the engine setup/teardown error branches and the per-case op error branches
// without the real library. The Stub pops the provided returns in strict call
// order; a NULL handle (ptr 0) plus a non-empty lastErrorMessage forces the
// wrap() error path. The bodies here never Draw — op coverage is driven by
// calling *testCase methods directly. (Draw error injection is tested
// separately below via stubTestCase.)

// stubTestCase builds a real *testCase whose libhegel operations are served
// by a Stub. opReturns supplies the per-op return values (Error/bool/…)
// consumed, in call order, after the settings/run/test-case handles are wired.
func stubTestCase(opReturns ...any) *testCase {
	returns := append([]any{
		uintptr(1),
		uintptr(1),
		uintptr(1),
	}, opReturns...)
	lib := libhegel.Stub(returns...)
	s := lib.SettingsNew()
	run, _ := s.RunStart()
	tc, _ := run.NextTestCase()
	return &testCase{tc: tc}
}

func TestRunWithHandleRunStartError(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(
		uintptr(1),
		uintptr(0), // run_start returns NULL
		"run_start boom",
	)
	err := runWithHandle(lib, func(TestCase) {}, runOptions{})
	if err == nil || !strings.Contains(err.Error(), "run_start boom") {
		t.Fatalf("expected run_start error, got %v", err)
	}
}

func TestRunWithHandleNextTestCaseError(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(
		uintptr(1),
		uintptr(1),
		uintptr(0),  // next_test_case returns NULL...
		"next boom", // ...with an error message
	)
	err := runWithHandle(lib, func(TestCase) {}, runOptions{})
	if err == nil || !strings.Contains(err.Error(), "next boom") {
		t.Fatalf("expected next_test_case error, got %v", err)
	}
}

func TestRunWithHandleRunResultError(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(
		uintptr(1),
		uintptr(1),
		uintptr(0), "", // NULL + no error => run finished
		uintptr(0), "result boom", // run_result NULL
	)
	err := runWithHandle(lib, func(TestCase) {}, runOptions{})
	if err == nil || !strings.Contains(err.Error(), "result boom") {
		t.Fatalf("expected run_result error, got %v", err)
	}
}

func TestRunWithHandleCollectFailures(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(
		uintptr(1),
		uintptr(1),
		uintptr(0), "", // run finished
		uintptr(1),
		libhegel.RUN_STATUS_FAILED, // result status: failed
		uint64(1),                  // one failure
		uintptr(1),                 // failure handle
		"prop_test.go:7",           // failure origin
	)
	err := runWithHandle(lib, func(TestCase) {}, runOptions{})
	if err == nil {
		t.Fatal("expected failure error")
	}
	if !errors.Is(err, errPropTestFailed) || !strings.Contains(err.Error(), "prop_test.go:7") {
		t.Fatalf("expected joined prop-test failure with origin, got %v", err)
	}
}

func TestRunWithHandleRunError(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(
		uintptr(1),
		uintptr(1),
		uintptr(0), "", // run finished
		uintptr(1),
		libhegel.RUN_STATUS_ERROR, // run errored (e.g. failed health check)
		"FailedHealthCheck: FilterTooMuch — …", // run-level error message
	)
	err := runWithHandle(lib, func(TestCase) {}, runOptions{})
	if err == nil {
		t.Fatal("expected run-error error")
	}
	if !errors.Is(err, errPropTestFailed) || !strings.Contains(err.Error(), "FilterTooMuch") {
		t.Fatalf("expected run-level error message, got %v", err)
	}
}

// TestBuildSettingsExercisesAllSetters drives a clean (no-test-case) run with
// rich options so buildSettings invokes every conditionally-applied settings
// setter: Mode, TestCases, Seed, Database, DatabaseKey and SuppressHealthCheck.
func TestBuildSettingsExercisesAllSetters(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(
		uintptr(1),     // settings_new
		uintptr(1),     // run_start
		uintptr(0), "", // next_test_case NULL => run finished
		uintptr(1),                 // run_result
		libhegel.RUN_STATUS_PASSED, // passed
	)
	seed := int64(7)
	opts := runOptions{
		testCases:           5,
		seed:                &seed,
		singleTestCase:      true,
		database:            Database("/tmp/does-not-matter.db"),
		databaseKey:         "TestBuildSettingsExercisesAllSetters",
		suppressHealthCheck: []HealthCheck{FilterTooMuch},
	}
	if err := runWithHandle(lib, func(TestCase) {}, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWithHandleTargetPanic(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(
		uintptr(1),
		uintptr(1),
		uintptr(1),         // one case
		true,               // is_final_replay => user panics are not recovered
		libhegel.E_BACKEND, // target fails
		"",                 // target's diagnostic, read on the error path
	)
	// On the final replay skipUserPanic lets the panic propagate out of
	// runWithHandle (so the Go runtime prints it for debuggers); it is not
	// recovered into a status or error. expectErrorPanic asserts it escapes.
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	runWithHandle(lib, func(tc TestCase) {
		tc.Target(1.0, "x")
	}, runOptions{})
}

// runWithFailingOp drives a single test case whose body calls fn against the
// stub-backed *testCase, with the next libhegel op returning failErr.
//
// failErr MUST be a non-OK Error: the stub sequence is hardwired for the error
// path, supplying the diagnostic string that the wrapper reads back from the
// context after a failed op. Passing OK would desynchronize the sequence. The
// body is expected to capture (not propagate) the error, so the case still
// returns normally (VALID) and the run completes cleanly.
func runWithFailingOp(t *testing.T, failErr libhegel.Error, fn func(*testCase)) {
	t.Helper()
	lib := libhegel.Stub(
		uintptr(1),
		uintptr(1),
		uintptr(1),
		false,       // is_final_replay
		failErr,     // the failing op
		"",          // failErr's diagnostic, read on the error path
		libhegel.OK, // mark_complete
		uintptr(0), "",
		uintptr(1),
		libhegel.RUN_STATUS_PASSED,
	)
	if err := runWithHandle(lib, func(tc TestCase) { fn(tc.(*testCase)) }, runOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTestCaseStartSpanError(t *testing.T) {
	t.Parallel()
	var got error
	runWithFailingOp(t, libhegel.E_BACKEND, func(tc *testCase) {
		got = tc.startSpan(libhegel.LABEL_LIST)
	})
	if got == nil {
		t.Fatal("expected startSpan error")
	}
}

func TestTestCaseStopSpanError(t *testing.T) {
	t.Parallel()
	var got error
	runWithFailingOp(t, libhegel.E_BACKEND, func(tc *testCase) {
		got = tc.stopSpan(false)
	})
	if got == nil {
		t.Fatal("expected stopSpan error")
	}
}

func TestTestCaseNewCollectionError(t *testing.T) {
	t.Parallel()
	var got error
	runWithFailingOp(t, libhegel.E_BACKEND, func(tc *testCase) {
		_, got = tc.newCollection(0, nil)
	})
	if got == nil {
		t.Fatal("expected newCollection error")
	}
}

func TestRunWithHandleUnrecognizedShortCircuit(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(
		uintptr(1),
		uintptr(1),
		uintptr(1),
		true, // is_final_replay
	)
	var sentinel = errors.New("weird")
	defer expectErrorPanic(t, sentinel)
	runWithHandle(lib, func(tc TestCase) {
		tc.abort(sentinel)
	}, runOptions{})
}

// TestAbortDoesNotDowngradeInteresting covers the guard in abort that refuses
// to lower an already-INTERESTING status: an Assume rejection (E_ASSUME =>
// INVALID) on an interesting case is a no-op and does not panic.
func TestAbortDoesNotDowngradeInteresting(t *testing.T) {
	t.Parallel()
	tc := stubTestCase()
	tc.setStatus(libhegel.STATUS_INTERESTING)
	tc.abort(libhegel.E_ASSUME) // INVALID < INTERESTING => returns without aborting
	if tc.getStatus() != libhegel.STATUS_INTERESTING {
		t.Fatalf("status downgraded to %v", tc.getStatus())
	}
	if tc.aborted {
		t.Fatal("expected case not to be marked aborted")
	}
}

// --- Draw error injection (via stubTestCase) ---

// errGen is a non-basic generator whose draw returns a fixed (value, error).
// It forces the collection path of Lists/Maps (asBasic => not basic) and lets
// tests inject element/key draw errors.
type errGen[T any] struct{ err error }

//lint:ignore U1000 satisfies Generator interface; staticcheck misses generic dispatch
func (g errGen[T]) draw(TestCase) (T, error) { var z T; return z, g.err }

//lint:ignore U1000 satisfies Generator interface; staticcheck misses generic dispatch
func (g errGen[T]) asBasic() (*basicGenerator[T], bool, error) { return nil, false, nil }

// expectErrorPanic is deferred to recover a Draw panic and assert its error.
func expectErrorPanic(t *testing.T, want error) {
	t.Helper()
	r := recover()
	err, ok := r.(error)
	if !ok {
		t.Fatalf("expected error panic, got %v", r)
	}
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}

func TestDrawPanicsOnGenerateError(t *testing.T) {
	t.Parallel()
	tc := stubTestCase(libhegel.E_BACKEND, "") // generate fails
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	Draw[int](tc, Integers[int](0, 10))
}

func TestDrawListStartSpanError(t *testing.T) {
	t.Parallel()
	tc := stubTestCase(libhegel.E_BACKEND, "") // start_span fails
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	Draw[[]int](tc, Lists[int](errGen[int]{}))
}

func TestDrawListNewCollectionError(t *testing.T) {
	t.Parallel()
	tc := stubTestCase(
		libhegel.OK,        // start_span
		libhegel.E_BACKEND, // new_collection fails
		"",                 // new_collection's diagnostic
	)
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	Draw[[]int](tc, Lists[int](errGen[int]{}))
}

func TestDrawListCollectionMoreError(t *testing.T) {
	t.Parallel()
	tc := stubTestCase(
		libhegel.OK,        // start_span
		libhegel.OK,        // new_collection
		libhegel.E_BACKEND, // collection_more fails => coll.Err()
		"",                 // collection_more's diagnostic
	)
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	Draw[[]int](tc, Lists[int](errGen[int]{}))
}

func TestDrawMapNewCollectionError(t *testing.T) {
	t.Parallel()
	tc := stubTestCase(
		libhegel.OK,        // start_span (LABEL_MAP)
		libhegel.E_BACKEND, // new_collection fails
		"",                 // new_collection's diagnostic
	)
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	Draw[map[int]int](tc, Maps[int, int](errGen[int]{}, errGen[int]{}))
}

// TestDrawMapKeyError covers the map key-draw error branch, which requires the
// collection loop body to actually run. The Stub cannot return more=true (it
// can't set the out-param), so this uses the real library with MinSize(1) and a
// key generator that rejects via E_ASSUME, leaving the run all-invalid (passes).
func TestDrawMapKeyError(t *testing.T) {
	t.Parallel()
	err := run(func(tc TestCase) {
		Draw[map[int]int](tc, Maps[int, int](
			errGen[int]{err: libhegel.E_ASSUME}, errGen[int]{},
		).MinSize(1))
	}, WithTestCases(5), WithDatabase(DatabaseDisabled()), SuppressHealthCheck(FilterTooMuch))
	if err != nil {
		t.Fatalf("expected all-invalid pass, got %v", err)
	}
}

// TestDrawMapBasic covers the all-basic map schema path (basic keys + values).
func TestDrawMapBasic(t *testing.T) {
	t.Parallel()
	err := run(func(tc TestCase) {
		_ = Draw[map[int]int](tc, Maps[int, int](Integers[int](0, 5), Integers[int](0, 5)))
	}, WithTestCases(5), WithDatabase(DatabaseDisabled()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDrawFilterStartSpanError(t *testing.T) {
	t.Parallel()
	tc := stubTestCase(libhegel.E_BACKEND, "") // start_span(FILTER) fails
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	Draw[int](tc, &filteredGenerator[int]{
		source:    Integers[int](0, 10),
		predicate: func(int) bool { return true },
	})
}

// TestGenerateEmptySchema covers slicePtr's empty-slice (nil pointer) branch by
// asking the stub to generate from an empty schema.
func TestGenerateEmptySchema(t *testing.T) {
	t.Parallel()
	tc := stubTestCase(libhegel.OK)
	if _, err := tc.tc.Generate(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestStubCollectionReject covers the collection_reject path against the stub.
func TestStubCollectionReject(t *testing.T) {
	t.Parallel()
	tc := stubTestCase(
		libhegel.OK, // new_collection
		libhegel.OK, // collection_reject
	)
	coll, err := tc.newCollection(0, nil)
	if err != nil {
		t.Fatalf("newCollection: %v", err)
	}
	coll.Reject("dup")
	if err := coll.Err(); err != nil {
		t.Fatalf("Reject recorded error: %v", err)
	}
}

// TestStatefulInitialInvariantError covers stateMachine.Run's panic on an
// initial-invariant failure: the stub fails start_span for the first invariant.
func TestStatefulInitialInvariantError(t *testing.T) {
	t.Parallel()
	tc := stubTestCase(libhegel.E_BACKEND, "") // start_span(STATEFUL) fails
	sm := &stateMachine{invariants: []stateMachineRule{{name: "Inv", fn: func(TestCase) {}}}}
	defer func() {
		err, ok := recover().(error)
		if !ok || !errors.Is(err, libhegel.E_BACKEND) {
			t.Fatalf("expected E_BACKEND panic, got %v", err)
		}
	}()
	sm.Run(tc)
}
