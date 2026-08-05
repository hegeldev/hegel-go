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
			}, WithTestCases(50), WithDatabase(""))
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
	}, WithTestCases(2), WithDatabase(""))
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
	}, WithTestCases(2), WithDatabase(""))
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
	}, WithTestCases(5), WithDatabase(""))
	if err == nil {
		t.Fatal("expected failure when Errorf is called")
	}
}

func TestFatalSentinelPath(t *testing.T) {
	t.Parallel()
	err := Run(func(tc TestCase) {
		tc.FailNow()
	}, WithTestCases(5), WithDatabase(""))
	if err == nil {
		t.Fatal("expected failure when FailNow is called")
	}
}

// --- findCaller / isHegelFrame ---

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

func anyFrame(string) bool {
	return true
}

// TestFindCallerStableAcrossValues verifies that origin is stable per call
// site. libhegel uses the origin as a shrink-grouping key, and per-value
// origins would prevent the shrinker from converging.
func TestFindCallerStableAcrossValues(t *testing.T) {
	t.Parallel()
	var a, b string
	for i := range 2 {
		origin := findCaller(1, anyFrame)
		if i == 0 {
			a = origin
		} else {
			b = origin
		}
	}
	if a != b {
		t.Errorf("origin must be stable; got %q vs %q", a, b)
	}
	if !strings.Contains(a, " (0x") || !strings.HasSuffix(a, ")") {
		t.Errorf("origin = %q, want file:line (0xpc)", a)
	}
}

func TestFindCallerDistinguishesCallsitesInSameFunction(t *testing.T) {
	t.Parallel()
	a := findCaller(1, anyFrame)
	b := findCaller(1, anyFrame)
	if a == b {
		t.Errorf("origin must distinguish callsites in the same function; got %q twice", a)
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

func TestAllPhases(t *testing.T) {
	t.Parallel()
	all := AllPhases()
	if len(all) != 5 {
		t.Errorf("AllPhases: expected 5, got %d", len(all))
	}
}

// TestSettingsOptionsRecordApplier checks that each settings-backed option
// records exactly one applier. The value each applier sets is exercised
// end-to-end by TestBuildSettingsExercisesAllSetters (against a stub) and the
// per-option integration tests (against the real engine).
func TestSettingsOptionsRecordApplier(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		opt  Option
	}{
		{"WithTestCases", WithTestCases(42)},
		{"WithStatefulStepCount", WithStatefulStepCount(25)},
		{"WithSeed", WithSeed(12345)},
		{"WithDerandomize", WithDerandomize(true)},
		{"WithDatabase", WithDatabase("/tmp/foo")},
		{"WithBackend", WithBackend(BackendURandom)},
		{"WithVerbosity", WithVerbosity(VerbosityVerbose)},
		{"WithReportMultipleFailures", WithReportMultipleFailures(true)},
		{"WithPhases", WithPhases(PhaseGenerate, PhaseShrink)},
		{"SuppressHealthCheck", SuppressHealthCheck(FilterTooMuch, TooSlow)},
	}
	for _, tc := range cases {
		o := applyOpts([]Option{tc.opt})
		if len(o.settingsAppliers) != 1 {
			t.Errorf("%s: recorded %d appliers, want 1", tc.name, len(o.settingsAppliers))
		}
	}
}

func TestSuppressHealthCheckIntegration(t *testing.T) {
	t.Parallel()
	err := Run(func(tc TestCase) {
		tc.Assume(false) // always reject
	}, WithTestCases(20),
		SuppressHealthCheck(FilterTooMuch),
		WithDatabase(""))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Options ---

func TestWithDerandomizeIntegration(t *testing.T) {
	t.Parallel()
	err := Run(func(tc TestCase) {
		_ = Draw[int](tc, Integers[int](0, 100))
	}, WithTestCases(5), WithDerandomize(true), WithDatabase(""))
	if err != nil {
		t.Errorf("derandomize integration: %v", err)
	}
}

func TestWithSeedIntegration(t *testing.T) {
	t.Parallel()
	err := Run(func(tc TestCase) {
		_ = Draw[int](tc, Integers[int](0, 100))
	}, WithTestCases(5), WithSeed(42), WithDatabase(""))
	if err != nil {
		t.Errorf("seed integration: %v", err)
	}
}

func TestWithBackendIntegration(t *testing.T) {
	t.Parallel()
	for _, b := range []Backend{BackendAuto, BackendDefault} {
		err := Run(func(tc TestCase) {
			_ = Draw[int](tc, Integers[int](0, 100))
		}, WithTestCases(5), WithBackend(b), WithDatabase(""))
		if err != nil {
			t.Errorf("backend %v integration: %v", b, err)
		}
	}
}

func TestWithVerbosityIntegration(t *testing.T) {
	t.Parallel()
	err := Run(func(tc TestCase) {
		_ = Draw[int](tc, Integers[int](0, 100))
	}, WithTestCases(5), WithVerbosity(VerbosityQuiet), WithDatabase(""))
	if err != nil {
		t.Errorf("verbosity integration: %v", err)
	}
}

func TestWithPhasesIntegration(t *testing.T) {
	t.Parallel()
	// Restrict to generation only; a passing property still completes.
	err := Run(func(tc TestCase) {
		_ = Draw[int](tc, Integers[int](0, 100))
	}, WithTestCases(5), WithPhases(PhaseGenerate), WithDatabase(""))
	if err != nil {
		t.Errorf("phases integration: %v", err)
	}
}

// TestWithReportMultipleFailuresIntegration checks the option is accepted by
// the engine on a passing property. (The distinct-failure count it controls is
// keyed off per-call-site origins, which collapse to one in these white-box
// tests, so the count itself isn't observable here.)
func TestWithReportMultipleFailuresIntegration(t *testing.T) {
	t.Parallel()
	err := Run(func(tc TestCase) {
		_ = Draw[int](tc, Integers[int](0, 100))
	}, WithTestCases(5), WithReportMultipleFailures(true), WithDatabase(""))
	if err != nil {
		t.Errorf("report-multiple-failures integration: %v", err)
	}
}

// --- Stub-driven runner lifecycle error paths ---
//
// These tests inject a libhegel.Stub(t, ) Context into runWithContext to exercise
// the engine setup/teardown error branches and the per-case op error branches
// without the real library. The Stub pops the provided returns in strict call
// order: each call consumes one value per output parameter (handle / count /
// status / string) followed by its Error return, and — when that Error is not
// OK — the diagnostic string read back via context_last_error. A handle
// constructor therefore consumes its handle value, then OK; on failure it
// consumes a (placeholder) handle, the Error, then the diagnostic. The bodies
// here never Draw — op coverage is driven by calling *testCase methods
// directly. (Draw error injection is tested separately below via
// newStubTestCase.)

// newStubTestCase builds a real *testCase whose libhegel operations are served
// by a Stub. opReturns supplies the per-op return values (Error/bool/…)
// consumed, in call order, after the settings/run/test-case handles are wired.
func newStubTestCase(t testing.TB, opReturns ...any) *testCase {
	returns := append([]any{
		uintptr(1), libhegel.OK, // settings_new
		uintptr(1), libhegel.OK, // run_start
		uintptr(1), libhegel.OK, // next_test_case
	}, opReturns...)
	lib := libhegel.Stub(t, returns...)
	s := lib.SettingsNew()
	run, _ := s.RunStart(lib, nil)
	tc, _ := run.NextTestCase(lib)
	return &testCase{ctx: lib, tc: tc}
}

// newRealTestCase returns a testCase backed by the real libhegel engine, for
// tests that assert the engine's own validation of draw parameters (rather than
// a Go-side pre-check). The first test case of a fresh run is sufficient.
func newRealTestCase(t testing.TB) *testCase {
	t.Helper()
	ctx := libhegel.NewContext()
	s := ctx.SettingsNew()
	run, err := s.RunStart(ctx, nil)
	if err != nil {
		t.Fatalf("RunStart: %v", err)
	}
	tc, err := run.NextTestCase(ctx)
	if err != nil {
		t.Fatalf("NextTestCase: %v", err)
	}
	return &testCase{ctx: ctx, tc: tc}
}

func TestRunWithHandleRunStartError(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,                                      // derandomize
		uintptr(0), libhegel.E_BACKEND, "run_start boom", // run_start fails
	)
	err := runWithContext(lib, func(TestCase) {}, applyOpts([]Option{WithDerandomize(false)}))
	if err == nil || !strings.Contains(err.Error(), "run_start boom") {
		t.Fatalf("expected run_start error, got %v", err)
	}
}

func TestRunWithHandleNextTestCaseError(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		uintptr(1), libhegel.OK, // run_start
		uintptr(0), libhegel.E_BACKEND, "next boom", // next_test_case fails
	)
	err := runWithContext(lib, func(TestCase) {}, applyOpts([]Option{WithDerandomize(false)}))
	if err == nil || !strings.Contains(err.Error(), "next boom") {
		t.Fatalf("expected next_test_case error, got %v", err)
	}
}

func TestRunWithHandleRunResultError(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		uintptr(1), libhegel.OK, // run_start
		uintptr(0), libhegel.OK, // next_test_case NULL => run finished
		uintptr(0), libhegel.E_BACKEND, "result boom", // run_result fails
	)
	err := runWithContext(lib, func(TestCase) {}, applyOpts([]Option{WithDerandomize(false)}))
	if err == nil || !strings.Contains(err.Error(), "result boom") {
		t.Fatalf("expected run_result error, got %v", err)
	}
}

func TestRunWithHandleCollectFailures(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		uintptr(1), libhegel.OK, // run_start
		uintptr(0), libhegel.OK, // next_test_case NULL => run finished
		uintptr(1), libhegel.OK, // run_result
		libhegel.RUN_STATUS_FAILED, libhegel.OK, // result status: failed
		uint64(1), libhegel.OK, // one failure
		uintptr(1), libhegel.OK, // failure handle
		"blob-data", libhegel.OK, // reproduction blob (replay)
		uintptr(1), libhegel.OK, // test_case_from_blob handle (replay)
		"prop_test.go:7", libhegel.OK, // failure origin
	)
	err := runWithContext(lib, func(TestCase) {}, applyOpts([]Option{WithDerandomize(false)}))
	if err == nil {
		t.Fatal("expected failure error")
	}
	if !errors.Is(err, errPropTestFailed) || !strings.Contains(err.Error(), "prop_test.go:7") {
		t.Fatalf("expected joined prop-test failure with origin, got %v", err)
	}
}

// TestRunWithContextSingleModeFailure covers the single-test-case failure
// branch: replay is skipped (no shrink/blob phase), so a FAILED run surfaces as
// the bare errPropTestFailed sentinel without a TestCaseFromBlob round-trip.
func TestRunWithContextSingleModeFailure(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		libhegel.OK,             // mode (single-test-case)
		uintptr(1), libhegel.OK, // run_start
		uintptr(1), libhegel.OK, // next_test_case: one case
		libhegel.OK,             // mark_complete
		uintptr(0), libhegel.OK, // next_test_case: run finished
		uintptr(1), libhegel.OK, // run_result
		libhegel.RUN_STATUS_FAILED, libhegel.OK, // result status
	)
	err := runWithContext(lib, func(tc TestCase) { tc.(*testCase).Fail() }, applyOpts([]Option{WithDerandomize(false), WithSingleTestCase()}))
	if !errors.Is(err, errPropTestFailed) {
		t.Fatalf("expected single-mode prop-test failure, got %v", err)
	}
}

// TestRunWithContextReplayBlobError covers replayCounterexample's error branch:
// TestCaseFromBlob fails while reproducing the counterexample, and that error
// is surfaced instead of the joined origin error.
func TestRunWithContextReplayBlobError(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		uintptr(1), libhegel.OK, // run_start
		uintptr(0), libhegel.OK, // next_test_case NULL => run finished
		uintptr(1), libhegel.OK, // run_result
		libhegel.RUN_STATUS_FAILED, libhegel.OK, // result status
		uint64(1), libhegel.OK, // one failure
		uintptr(1), libhegel.OK, // failure handle
		"bad-blob", libhegel.OK, // reproduction blob
		uintptr(0), libhegel.E_BACKEND, "replay boom", // test_case_from_blob fails
	)
	err := runWithContext(lib, func(TestCase) {}, applyOpts([]Option{WithDerandomize(false)}))
	if err == nil || !strings.Contains(err.Error(), "replay boom") {
		t.Fatalf("expected replay error, got %v", err)
	}
}

func TestRunWithHandleFailureError(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		uintptr(1), libhegel.OK, // run_start
		uintptr(0), libhegel.OK, // next_test_case NULL => run finished
		uintptr(1), libhegel.OK, // run_result
		libhegel.RUN_STATUS_FAILED, libhegel.OK, // result status: not passed
		uint64(1), libhegel.OK, // one failure
		uintptr(0), libhegel.E_BACKEND, "failure boom", // failure fetch fails
	)
	err := runWithContext(lib, func(TestCase) {}, applyOpts([]Option{WithDerandomize(false)}))
	if err == nil || !strings.Contains(err.Error(), "failure boom") {
		t.Fatalf("expected failure-fetch error, got %v", err)
	}
}

func TestRunWithHandleRunError(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		uintptr(1), libhegel.OK, // run_start
		uintptr(0), libhegel.OK, // next_test_case NULL => run finished
		uintptr(1), libhegel.OK, // run_result
		libhegel.RUN_STATUS_ERROR, libhegel.OK, // result status: run errored (e.g. failed health check)
		"FailedHealthCheck: FilterTooMuch — …", libhegel.OK, // run-level error message
	)
	err := runWithContext(lib, func(TestCase) {}, applyOpts([]Option{WithDerandomize(false)}))
	if err == nil {
		t.Fatal("expected run-error error")
	}
	if !errors.Is(err, errPropTestFailed) || !strings.Contains(err.Error(), "FilterTooMuch") {
		t.Fatalf("expected run-level error message, got %v", err)
	}
}

// TestBuildSettingsExercisesAllSetters drives a clean (no-test-case) run with
// every settings-backed option so buildSettings invokes each setter applier:
// TestCases, StatefulStepCount, Derandomize, Seed, Database, DatabaseKey,
// SuppressHealthCheck, Backend, Verbosity, ReportMultipleFailures, Phases and
// Mode.
func TestBuildSettingsExercisesAllSetters(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // test_cases
		libhegel.OK,             // stateful_step_count
		libhegel.OK,             // derandomize
		libhegel.OK,             // seed
		libhegel.OK,             // database
		libhegel.OK,             // database_key
		libhegel.OK,             // suppress_health_check
		libhegel.OK,             // backend
		libhegel.OK,             // verbosity
		libhegel.OK,             // report_multiple_failures
		libhegel.OK,             // phases
		libhegel.OK,             // mode (single-test-case)
		uintptr(1), libhegel.OK, // run_start
		uintptr(0), libhegel.OK, // next_test_case NULL => run finished
		uintptr(1), libhegel.OK, // run_result
		libhegel.RUN_STATUS_PASSED, libhegel.OK, // result status: passed
	)
	// Apply every settings-backed option, in the same order the stub expects
	// each setter to fire.
	opts := applyOpts([]Option{
		WithTestCases(5),
		WithStatefulStepCount(25),
		WithDerandomize(false),
		WithSeed(7),
		WithDatabase("/tmp/does-not-matter.db"),
		withDatabaseKey("TestBuildSettingsExercisesAllSetters"),
		SuppressHealthCheck(FilterTooMuch),
		WithBackend(BackendURandom),
		WithVerbosity(VerbosityVerbose),
		WithReportMultipleFailures(true),
		WithPhases(PhaseGenerate, PhaseShrink),
		WithSingleTestCase(),
	})
	if err := runWithContext(lib, func(TestCase) {}, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestBuildSettingsSetterError covers the setter-error branch in buildSettings:
// a rejected settings option surfaces as an error from runWithContext rather
// than being silently dropped. A single derandomize applier reaches a setter
// before run_start.
func TestBuildSettingsSetterError(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.E_BACKEND, "setter boom", // derandomize fails (diagnostic read by invoke)
	)
	err := runWithContext(lib, func(TestCase) {}, applyOpts([]Option{WithDerandomize(false)}))
	if err == nil || !strings.Contains(err.Error(), "setter boom") {
		t.Fatalf("expected settings-setter error, got %v", err)
	}
}

// TestRunWithHandleMarkCompleteError covers the mark_complete error branch in
// the run loop: a failure to record a test case's status is surfaced rather
// than swallowed.
func TestRunWithHandleMarkCompleteError(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		uintptr(1), libhegel.OK, // run_start
		uintptr(1), libhegel.OK, // next_test_case: one case
		libhegel.E_BACKEND, "mark boom", // mark_complete fails (diagnostic read by invoke)
	)
	err := runWithContext(lib, func(TestCase) {}, applyOpts([]Option{WithDerandomize(false)}))
	if err == nil || !strings.Contains(err.Error(), "mark boom") {
		t.Fatalf("expected mark_complete error, got %v", err)
	}
}

func TestRunWithHandleTargetPanic(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		libhegel.OK,             // mode (single-test-case)
		uintptr(1), libhegel.OK, // run_start
		uintptr(1), libhegel.OK, // next_test_case: one case
		libhegel.E_BACKEND, "boom", // target fails (diagnostic read by invoke)
	)
	// In single-test-case mode skipUserPanic lets the panic propagate out of
	// runWithContext (so the Go runtime prints it for debuggers); it is not
	// recovered into a status or error. expectErrorPanic asserts it escapes.
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	runWithContext(lib, func(tc TestCase) {
		tc.Target(1.0, "x")
	}, applyOpts([]Option{WithDerandomize(false), WithSingleTestCase()}))
}

// stubOpCase drives a single test case whose body calls fn against the
// stub-backed *testCase, with one op return op. The body returns normally
// (VALID), so the case completes cleanly.
// stubOpCase drives fn against a stubbed test case. ops supplies the failing
// operation's scripted values — its output-parameter placeholder(s) followed by
// the failing Error code — in call order.
func stubOpCase(t *testing.T, fn func(*testCase), ops ...any) {
	t.Helper()
	returns := []any{
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		uintptr(1), libhegel.OK, // run_start
		uintptr(1), libhegel.OK, // next_test_case: one case
	}
	returns = append(returns, ops...) // the failing op's outputs + Error
	returns = append(returns,
		"boom",                  // diagnostic read by invoke after the failing op
		libhegel.OK,             // mark_complete
		uintptr(0), libhegel.OK, // next_test_case NULL => run finished
		uintptr(1), libhegel.OK, // run_result
		libhegel.RUN_STATUS_PASSED, libhegel.OK, // result status
	)
	lib := libhegel.Stub(t, returns...)
	if err := runWithContext(lib, func(tc TestCase) { fn(tc.(*testCase)) }, applyOpts([]Option{WithDerandomize(false)})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTestCaseStartSpanError(t *testing.T) {
	t.Parallel()
	var got error
	stubOpCase(t, func(tc *testCase) {
		got = tc.startSpan(libhegel.LABEL_LIST)
	}, libhegel.E_BACKEND)
	if got == nil {
		t.Fatal("expected startSpan error")
	}
}

func TestTestCaseStopSpanError(t *testing.T) {
	t.Parallel()
	var got error
	stubOpCase(t, func(tc *testCase) {
		got = tc.stopSpan(false)
	}, libhegel.E_BACKEND)
	if got == nil {
		t.Fatal("expected stopSpan error")
	}
}

func TestTestCaseNewCollectionError(t *testing.T) {
	t.Parallel()
	var got error
	// new_collection writes its *Collection out-param (placeholder) before
	// returning the failing Error.
	stubOpCase(t, func(tc *testCase) {
		_, got = tc.newCollection(0, nil)
	}, libhegel.Collection(0), libhegel.E_BACKEND)
	if got == nil {
		t.Fatal("expected newCollection error")
	}
}

func TestRunWithHandleUnrecognizedShortCircuit(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		libhegel.OK,             // mode (single-test-case)
		uintptr(1), libhegel.OK, // run_start
		uintptr(1), libhegel.OK, // next_test_case: one case
	)
	var sentinel = errors.New("weird")
	defer expectErrorPanic(t, sentinel)
	runWithContext(lib, func(tc TestCase) {
		tc.abort(sentinel)
	}, applyOpts([]Option{WithDerandomize(false), WithSingleTestCase()}))
}

// TestAbortDoesNotDowngradeInteresting covers the guard in abort that refuses
// to lower an already-INTERESTING status: an Assume rejection (E_ASSUME =>
// INVALID) on an interesting case is a no-op and does not panic.
func TestAbortDoesNotDowngradeInteresting(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t)
	tc.setStatus(libhegel.STATUS_INTERESTING)
	tc.abort(libhegel.E_ASSUME) // INVALID < INTERESTING => returns without aborting
	if tc.getStatus() != libhegel.STATUS_INTERESTING {
		t.Fatalf("status downgraded to %v", tc.getStatus())
	}
	if tc.aborted {
		t.Fatal("expected case not to be marked aborted")
	}
}

// --- Draw error injection (via newStubTestCase) ---

// errGen is a generator whose draw returns a fixed (zero, error). It lets tests
// inject element/key draw errors into the collection loop of Lists/Maps.
type errGen[T any] struct{ err error }

//lint:ignore U1000 satisfies Generator interface; staticcheck misses generic dispatch
func (g errGen[T]) draw(TestCase) (T, error) { var z T; return z, g.err }

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
	// generate_integer writes its int64 out-parameter (placeholder) before
	// returning the failing Error.
	tc := newStubTestCase(t, int64(0), libhegel.E_BACKEND, "boom")
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	Draw[int](tc, Integers[int](0, 10))
}

func TestDrawListStartSpanError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t, libhegel.E_BACKEND, "boom") // start_span fails
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	Draw[[]int](tc, Lists[int](errGen[int]{}))
}

func TestDrawListNewCollectionError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		libhegel.OK,            // start_span
		libhegel.Collection(0), // new_collection out-param placeholder
		libhegel.E_BACKEND,     // new_collection fails
		"boom",                 // diagnostic read by invoke
	)
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	Draw[[]int](tc, Lists[int](errGen[int]{}))
}

func TestDrawListCollectionMoreError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		libhegel.OK,            // start_span
		libhegel.Collection(0), // new_collection out-param placeholder
		libhegel.OK,            // new_collection
		false,                  // collection_more out-param placeholder
		libhegel.E_BACKEND,     // collection_more fails => coll.Err()
		"boom",                 // diagnostic read by invoke
	)
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	Draw[[]int](tc, Lists[int](errGen[int]{}))
}

func TestDrawMapNewCollectionError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		libhegel.OK,            // start_span (LABEL_MAP)
		libhegel.Collection(0), // new_collection out-param placeholder
		libhegel.E_BACKEND,     // new_collection fails
		"boom",                 // diagnostic read by invoke
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
	}, WithTestCases(5), WithDatabase(""), SuppressHealthCheck(FilterTooMuch))
	if err != nil {
		t.Fatalf("expected all-invalid pass, got %v", err)
	}
}

// TestDrawMapValueError covers the map value-draw error branch. It mirrors
// [TestDrawMapKeyError] but lets the key draw succeed (a real generator) so the
// loop reaches the value draw, which then rejects via E_ASSUME. Without this the
// branch is only ever hit by luck — when the real engine returns a stop/assume
// sentinel mid-draw — which makes its coverage flaky.
func TestDrawMapValueError(t *testing.T) {
	t.Parallel()
	err := run(func(tc TestCase) {
		Draw[map[int]int](tc, Maps[int, int](
			Integers[int](0, 5), errGen[int]{err: libhegel.E_ASSUME},
		).MinSize(1))
	}, WithTestCases(5), WithDatabase(""), SuppressHealthCheck(FilterTooMuch))
	if err != nil {
		t.Fatalf("expected all-invalid pass, got %v", err)
	}
}

// TestDrawMapBasic covers the map collection path with primitive keys + values.
func TestDrawMapBasic(t *testing.T) {
	t.Parallel()
	err := run(func(tc TestCase) {
		_ = Draw[map[int]int](tc, Maps[int, int](Integers[int](0, 5), Integers[int](0, 5)))
	}, WithTestCases(5), WithDatabase(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDrawFilterStartSpanError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t, libhegel.E_BACKEND, "boom") // start_span(FILTER) fails
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	Draw[int](tc, &filteredGenerator[int]{
		source:    Integers[int](0, 10),
		predicate: func(int) bool { return true },
	})
}

// TestStubCollectionReject covers the collection_reject path against the stub.
func TestStubCollectionReject(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		libhegel.Collection(0), // new_collection out-param placeholder
		libhegel.OK,            // new_collection
		libhegel.OK,            // collection_reject
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

// TestStatefulNewStateMachineError covers stateMachine.Run's panic when the
// engine rejects new_state_machine registration.
func TestStatefulNewStateMachineError(t *testing.T) {
	t.Parallel()
	// new_state_machine writes its *StateMachine out-param (placeholder) before
	// returning the failing Error.
	tc := newStubTestCase(t, libhegel.StateMachine(0), libhegel.E_BACKEND, "boom")
	sm := &stateMachine{rules: []stateMachineRule{{name: "Rule", fn: func(TestCase) {}}}}
	defer func() {
		err, ok := recover().(error)
		if !ok || !errors.Is(err, libhegel.E_BACKEND) {
			t.Fatalf("expected E_BACKEND panic, got %v", err)
		}
	}()
	sm.Run(tc)
}

// TestStatefulInitialInvariantError covers stateMachine.Run's panic on an
// initial-invariant failure: new_state_machine succeeds, then the stub fails
// start_span for the first invariant.
func TestStatefulInitialInvariantError(t *testing.T) {
	t.Parallel()
	// new_state_machine writes its *StateMachine out-param (placeholder) + OK to
	// register the machine; E_BACKEND then fails start_span(STATEFUL).
	tc := newStubTestCase(t, libhegel.StateMachine(0), libhegel.OK, libhegel.E_BACKEND, "boom")
	sm := &stateMachine{invariants: []stateMachineRule{{name: "Inv", fn: func(TestCase) {}}}}
	defer func() {
		err, ok := recover().(error)
		if !ok || !errors.Is(err, libhegel.E_BACKEND) {
			t.Fatalf("expected E_BACKEND panic, got %v", err)
		}
	}()
	sm.Run(tc)
}
