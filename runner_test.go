package hegel

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

func TestMain(m *testing.M) {
	if err := os.Setenv("HEGEL_STATISTICS", ""); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

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
		_ = Draw[bool](ht, Booleans())
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
	for range goroutines {
		wg.Go(func() {
			err := Run(func(tc TestCase) {
				v := Draw[int](tc, Integers[int](0, 1000))
				if v < 0 || v > 1000 {
					panic("out of range")
				}
			}, WithTestCases(50), WithDatabase(""))
			if err != nil {
				failures.Add(1)
			}
		})
	}
	wg.Wait()
	if failures.Load() != 0 {
		t.Errorf("%d concurrent runs failed", failures.Load())
	}
}

// --- Test-case count ---

func TestRunHegelTestOneCase(t *testing.T) {
	var calls int
	err := Run(func(tc TestCase) {
		calls++
		_ = Draw[bool](tc, Booleans())
	}, WithTestCases(1))
	if err != nil {
		t.Fatalf("Run with WithTestCases(1): %v", err)
	}
	if calls != 1 {
		t.Errorf("expected exactly one call under WithTestCases(1), got %d", calls)
	}
}

// --- MustRun: panics on error ---

// TestMustRunPanicsOnUserPanic covers the exploratory-case recover path in
// testCase.run: a user panic on a non-final case is recovered into an
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

func TestRunReportsSamePackageLocation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("libhegel's Antithesis reporter does not write sdk.jsonl on Windows")
	}
	sdkDir := t.TempDir()
	t.Setenv("ANTITHESIS_OUTPUT_DIR", sdkDir)
	if err := Run(func(TestCase) {}, WithTestCases(1), WithDatabase("")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(sdkDir, "sdk.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	report := string(data)
	if !strings.Contains(report, `"class":"hegel.dev/go/hegel"`) || !strings.Contains(report, `"function":"TestRunReportsSamePackageLocation"`) {
		t.Fatalf("unexpected Antithesis report:\n%s", report)
	}
	if !strings.Contains(report, `runner_test.go`) {
		t.Fatalf("report does not contain the property source file:\n%s", report)
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

// TestFindCallerStableAcrossValues verifies that origin is stable per call
// site. libhegel uses the origin as a shrink-grouping key, and per-value
// origins would prevent the shrinker from converging.
func TestFindCallerStableAcrossValues(t *testing.T) {
	t.Parallel()
	var a, b string
	for i := range 2 {
		var pcs [1]uintptr
		runtime.Callers(1, pcs[:])
		origin := findCallerInPCs(pcs[:], anyFrame)
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
	var aPC, bPC [1]uintptr
	runtime.Callers(1, aPC[:])
	runtime.Callers(1, bPC[:])
	a := findCallerInPCs(aPC[:], anyFrame)
	b := findCallerInPCs(bPC[:], anyFrame)
	if a == b {
		t.Errorf("origin must distinguish callsites in the same function; got %q twice", a)
	}
}

func TestFindCallerInPCsWithoutMatchingFrame(t *testing.T) {
	t.Parallel()
	var pcs [1]uintptr
	runtime.Callers(1, pcs[:])
	if got, want := findCallerInPCs(pcs[:], func(string) bool { return false }), "<unknown>:0 (0x0)"; got != want {
		t.Fatalf("origin = %q, want %q", got, want)
	}
}

func TestFindCallerLocationInPCs(t *testing.T) {
	t.Parallel()
	pcs := make([]uintptr, 8)
	n := runtime.Callers(1, pcs)
	location, ok := findCallerLocationInPCs(pcs[:n], anyFrame)
	if !ok {
		t.Fatal("expected caller location")
	}
	if !strings.HasSuffix(location.file, "runner_test.go") {
		t.Errorf("file = %q, want runner_test.go", location.file)
	}
	if location.line == 0 {
		t.Error("line = 0, want source line")
	}
	if got, want := location.class, "hegel.dev/go/hegel"; got != want {
		t.Errorf("class = %q, want %q", got, want)
	}
	if got, want := location.function, "TestFindCallerLocationInPCs"; got != want {
		t.Errorf("function = %q, want %q", got, want)
	}
}

func TestFindCallerLocationInPCsWithoutMatchingFrame(t *testing.T) {
	t.Parallel()
	var pcs [1]uintptr
	runtime.Callers(1, pcs[:])
	if _, ok := findCallerLocationInPCs(pcs[:], func(string) bool { return false }); ok {
		t.Fatal("unexpected caller location")
	}
}

func TestFormatInvocationResultIncludesWorkerAndStack(t *testing.T) {
	t.Parallel()
	pcs := make([]uintptr, 8)
	n := runtime.Callers(1, pcs)
	err := &workerError{
		worker: 2,
		err: &invocationError{
			cause: "boom",
			kind:  "failure",
			pcs:   pcs[:n],
		},
	}
	var out strings.Builder
	formatInvocationResult(&out, err)
	if got := out.String(); !strings.Contains(got, "failure in worker 2: boom") || !strings.Contains(got, "TestFormatInvocationResultIncludesWorkerAndStack") {
		t.Fatalf("diagnostic = %q", got)
	}
	if got, want := err.Error(), "boom"; got != want {
		t.Fatalf("worker error = %q, want %q", got, want)
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
		{"WithSeed", WithSeed(12345)},
		{"WithDerandomize", WithDerandomize(true)},
		{"WithDatabase", WithDatabase("/tmp/foo")},
		{"WithBackend", WithBackend(BackendURandom)},
		{"WithVerbosity", WithVerbosity(VerbosityVerbose)},
		{"WithReportMultipleFailures", WithReportMultipleFailures(true)},
		{"WithStatistics", WithStatistics(true)},
		{"WithReproductionBlob", WithReproductionBlob(true)},
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

func TestWithProfileRecordsName(t *testing.T) {
	o := applyOpts([]Option{WithProfile("base"), WithProfile("workload")})
	if o.profile != "workload" {
		t.Fatalf("profile = %q, want workload", o.profile)
	}
	if len(o.settingsAppliers) != 0 {
		t.Fatalf("WithProfile recorded %d settings appliers, want 0", len(o.settingsAppliers))
	}
}

func TestSuppressHealthCheckIntegration(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	err := Run(func(tc TestCase) {
		_ = Draw[bool](tc, Booleans())
		tc.Assume(calls.Add(1) > 100)
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
	for _, b := range []Backend{BackendDefault, BackendURandom} {
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

func TestStatisticsReporting(t *testing.T) {
	var out strings.Builder
	err := run(1, func(tc TestCase) {
		_ = Draw(tc, Booleans())
		tc.Event("visited")
		tc.EventValue("size", 42)
	}, WithTestCases(5), WithDatabase(""), WithDerandomize(true), WithStatistics(true), withOutput(&out))
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "* visited: 100.0% of test cases") {
		t.Errorf("statistics missing event occurrence:\n%s", got)
	}
	if !strings.Contains(got, "* size: count 5") || !strings.Contains(got, "median 42") {
		t.Errorf("statistics missing value distribution:\n%s", got)
	}
}

func TestStatisticsDisabledByDefault(t *testing.T) {
	t.Setenv("HEGEL_STATISTICS", "")
	var out strings.Builder
	err := run(1, func(tc TestCase) {
		_ = Draw(tc, Booleans())
		tc.Event("visited")
	}, WithTestCases(1), WithDatabase(""), withOutput(&out))
	if err != nil {
		t.Fatal(err)
	}
	if got := out.String(); strings.Contains(got, "Statistics") {
		t.Fatalf("unexpected statistics output:\n%s", got)
	}
}

func TestStatisticsEnvironmentOverride(t *testing.T) {
	t.Setenv("HEGEL_STATISTICS", "1")
	var out strings.Builder
	err := run(1, func(tc TestCase) {
		_ = Draw(tc, Booleans())
		tc.Event("visited")
	}, WithTestCases(1), WithDatabase(""), WithStatistics(false), withOutput(&out))
	if err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "* visited: 100.0% of test cases") {
		t.Fatalf("statistics environment override was not applied:\n%s", got)
	}
}

func TestStatisticsEnvironmentDisabledValues(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"zero", "0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("HEGEL_STATISTICS", test.value)
			ctx := libhegel.NewContext()
			settings, err := (runOptions{}).buildSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			got, err := settings.GetShowStatistics(ctx)
			if err != nil || got {
				t.Fatalf("show statistics = %v, %v; want false", got, err)
			}
		})
	}
}

func TestEventValueRejectsNonFiniteValues(t *testing.T) {
	err := Run(func(tc TestCase) {
		tc.EventValue("bad", math.NaN())
	}, WithTestCases(1), WithDatabase(""))
	if err == nil || !strings.Contains(err.Error(), "finite value") {
		t.Fatalf("expected finite-value error, got %v", err)
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
	s, _ := lib.SettingsNew()
	run, _ := s.RunStart(lib, nil)
	tc, _ := run.NextTestCase(lib)
	state, _ := newTestCase(lib, tc, nil, captureUserPanics)
	return state
}

func TestFrameworkLogWritesWithoutLocation(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	tc := newEmittingTestCase(t, &out)
	tc.log("Round %d", 3)
	if _, err := tc.run(func(TestCase) {}); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "Round 3\n"; got != want {
		t.Fatalf("log output = %q, want %q", got, want)
	}
}

func TestDrawReportOmitsLocation(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	tc := newEmittingTestCase(t, &out)
	tc.reportDraw(0, 42)
	if _, err := tc.run(func(TestCase) {}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, " = 42\n") || strings.Contains(got, "runner_test.go:") {
		t.Fatalf("draw output = %q, want draw report without location", got)
	}
}

func TestDrawReportPropagatesPrinterErrors(t *testing.T) {
	tests := []struct {
		name    string
		returns []any
	}{
		{name: "prefix", returns: []any{libhegel.E_BACKEND, "boom"}},
		{name: "value", returns: []any{libhegel.OK, libhegel.E_BACKEND, "boom"}},
		{name: "hard break", returns: []any{libhegel.OK, libhegel.OK, libhegel.E_BACKEND, "boom"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			returns := append([]any{uintptr(1), libhegel.OK}, tt.returns...)
			ctx := libhegel.Stub(t, returns...)
			printer, err := ctx.PrinterNew(nil)
			if err != nil {
				t.Fatal(err)
			}
			tc := &testCase{ctx: ctx, printer: printer}
			defer func() {
				err, ok := recover().(error)
				if !ok || !errors.Is(err, libhegel.E_BACKEND) {
					t.Fatalf("reportDraw panic = %v, want E_BACKEND", err)
				}
			}()
			tc.reportDraw(0, 42)
		})
	}
}

func TestTClonePropagatesError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t, uintptr(0), libhegel.E_BACKEND, "clone boom")
	ht := &T{testCase: tc, T: t}
	clone, err := ht.clone()
	if clone != nil || err == nil || !strings.Contains(err.Error(), "clone boom") {
		t.Fatalf("clone = %v, err = %v, want clone error", clone, err)
	}
}

// newRealTestCase returns a testCase backed by the real libhegel engine, for
// tests that assert the engine's own validation of draw parameters (rather than
// a Go-side pre-check). The first test case of a fresh run is sufficient.
func newRealTestCase(t testing.TB) *testCase {
	t.Helper()
	ctx := libhegel.NewContext()
	s, err := ctx.SettingsNew()
	if err != nil {
		t.Fatalf("SettingsNew: %v", err)
	}
	run, err := s.RunStart(ctx, nil)
	if err != nil {
		t.Fatalf("RunStart: %v", err)
	}
	tc, err := run.NextTestCase(ctx)
	if err != nil {
		t.Fatalf("NextTestCase: %v", err)
	}
	state, _ := newTestCase(ctx, tc, nil, false)
	return state
}

func TestTestCaseCloneInheritsExecutionPolicy(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		uintptr(1), libhegel.OK, // run_start
		uintptr(1), libhegel.OK, // next_test_case
		uintptr(1), libhegel.OK, // printer
		uintptr(2), libhegel.OK, // test_case_clone
		uintptr(2),              // context_new for the cloned wrapper
		uintptr(2), libhegel.OK, // nested printer
	)
	s, _ := lib.SettingsNew()
	run, _ := s.RunStart(lib, nil)
	raw, _ := run.NextTestCase(lib)
	var output strings.Builder
	parent, err := newTestCase(lib, raw, &output, true)
	if err != nil {
		t.Fatal(err)
	}

	cloned, err := parent.clone()
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	clone := cloned.(*testCase)
	if clone == parent || clone.tc == parent.tc {
		t.Fatal("clone did not receive an independent handle")
	}
	if clone.ctx == parent.ctx {
		t.Fatal("clone shares its parent's error-reporting context")
	}
	if parent.out != &output || clone.out != nil || clone.panicPolicy != parent.panicPolicy {
		t.Fatalf("clone changed output ownership or execution policy")
	}
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
		false, libhegel.OK, // print blob
		uintptr(1), libhegel.OK, // test_case_from_blob handle (replay)
		libhegel.OK,                   // mark_complete replay
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

func TestRunWithContextPrintsReproductionBlob(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		uintptr(1), libhegel.OK, // run_start
		uintptr(0), libhegel.OK, // next_test_case: run finished
		uintptr(1), libhegel.OK, // run_result
		libhegel.RUN_STATUS_FAILED, libhegel.OK,
		uint64(1), libhegel.OK, // failure count
		uintptr(1), libhegel.OK, // failure
		"blob-data", libhegel.OK,
		true, libhegel.OK, // print blob
		uintptr(1), libhegel.OK, // test_case_from_blob
		uintptr(1), libhegel.OK, // printer
		libhegel.OK,     // mark_complete
		libhegel.OK,     // resolve
		"", libhegel.OK, // printer value
		"prop_test.go:7", libhegel.OK,
	)
	var output strings.Builder
	opts := applyOpts([]Option{WithDerandomize(false)})
	opts.output = &output
	err := runWithContext(lib, func(TestCase) {}, opts)
	if !errors.Is(err, errPropTestFailed) {
		t.Fatalf("runWithContext error = %v, want property failure", err)
	}
	if got := output.String(); !strings.Contains(got, "reproduction blob: blob-data") {
		t.Fatalf("output = %q, want reproduction blob", got)
	}
}

func TestRunWithContextGetPrintBlobError(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		uintptr(1), libhegel.OK, // run_start
		uintptr(0), libhegel.OK, // next_test_case: run finished
		uintptr(1), libhegel.OK, // run_result
		libhegel.RUN_STATUS_FAILED, libhegel.OK,
		uint64(1), libhegel.OK, // failure count
		uintptr(1), libhegel.OK, // failure
		"blob-data", libhegel.OK,
		false, libhegel.E_BACKEND, "print blob boom",
	)
	err := runWithContext(lib, func(TestCase) {}, applyOpts([]Option{WithDerandomize(false)}))
	if err == nil || !strings.Contains(err.Error(), "print blob boom") {
		t.Fatalf("error = %v, want print-blob error", err)
	}
}

func TestRunWithContextReproductionBlobWriteError(t *testing.T) {
	t.Parallel()
	want := errors.New("write blob failed")
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		uintptr(1), libhegel.OK, // run_start
		uintptr(0), libhegel.OK, // next_test_case: run finished
		uintptr(1), libhegel.OK, // run_result
		libhegel.RUN_STATUS_FAILED, libhegel.OK,
		uint64(1), libhegel.OK, // failure count
		uintptr(1), libhegel.OK, // failure
		"blob-data", libhegel.OK,
		true, libhegel.OK, // print blob
		uintptr(1), libhegel.OK, // test_case_from_blob
		uintptr(1), libhegel.OK, // printer
		libhegel.OK,     // mark_complete
		libhegel.OK,     // resolve
		"", libhegel.OK, // printer value
	)
	opts := applyOpts([]Option{WithDerandomize(false)})
	opts.output = nonemptyWriteError{want}
	err := runWithContext(lib, func(TestCase) {}, opts)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

type nonemptyWriteError struct{ err error }

func (w nonemptyWriteError) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return 0, w.err
}

func TestRunWithContextReplayPropagatesUserPanic(t *testing.T) {
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
		"blob-data", libhegel.OK, // reproduction blob
		true, libhegel.OK, // print blob
		uintptr(1), libhegel.OK, // test_case_from_blob handle
		uintptr(1), libhegel.OK, // printer
		libhegel.OK,     // resolve during panic unwinding
		"", libhegel.OK, // printer value
	)
	var output strings.Builder
	opts := applyOpts([]Option{WithDerandomize(false)})
	opts.output = &output

	defer func() {
		if got := recover(); got != "replay panic" {
			t.Fatalf("panic = %v, want replay panic", got)
		}
		if got := output.String(); !strings.Contains(got, "reproduction blob: blob-data") {
			t.Fatalf("output = %q, want reproduction blob", got)
		}
	}()
	_ = runWithContext(lib, func(TestCase) {
		panic("replay panic")
	}, opts)
}

func TestRunWithContextReplayMarkCompleteError(t *testing.T) {
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
		"blob-data", libhegel.OK, // reproduction blob
		false, libhegel.OK, // print blob
		uintptr(1), libhegel.OK, // test_case_from_blob handle
		libhegel.E_BACKEND, "replay mark boom", // mark_complete replay
	)

	err := runWithContext(lib, func(TestCase) {}, applyOpts([]Option{WithDerandomize(false)}))
	if err == nil || !strings.Contains(err.Error(), "replay mark boom") {
		t.Fatalf("expected replay mark_complete error, got %v", err)
	}
}

// TestRunWithContextOneCaseFailure verifies that a one-test-case failure
// has no reproduction blob and therefore skips final replay.
func TestRunWithContextOneCaseFailure(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		libhegel.OK,             // settings
		uintptr(1), libhegel.OK, // run_start
		uintptr(1), libhegel.OK, // next_test_case: one case
		false, libhegel.OK, // is_nondeterministic
		libhegel.OK,             // mark_complete
		uintptr(0), libhegel.OK, // next_test_case: run finished
		uintptr(1), libhegel.OK, // run_result
		libhegel.RUN_STATUS_FAILED, libhegel.OK, // result status
		uint64(1), libhegel.OK, // one failure
		uintptr(1), libhegel.OK, // failure handle
		"", libhegel.OK, // no reproduction blob
	)
	err := runWithContext(lib, func(tc TestCase) { tc.(*testCase).Fail() }, applyOpts([]Option{WithDerandomize(false), WithTestCases(1)}))
	if !errors.Is(err, errPropTestFailed) {
		t.Fatalf("expected single-mode prop-test failure, got %v", err)
	}
}

// TestRunWithContextNondeterministicFailure verifies that an empty reproduction
// blob identifies a nondeterministic failure and skips final replay.
func TestRunWithContextNondeterministicFailure(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		uintptr(1), libhegel.OK, // run_start
		uintptr(0), libhegel.OK, // next_test_case: run finished
		uintptr(1), libhegel.OK, // run_result
		libhegel.RUN_STATUS_FAILED, libhegel.OK, // result status
		uint64(1), libhegel.OK, // one failure
		uintptr(1), libhegel.OK, // failure handle
		"", libhegel.OK, // no reproduction blob
	)
	err := runWithContext(lib, func(TestCase) {}, applyOpts([]Option{WithDerandomize(false)}))
	if !errors.Is(err, errPropTestFailed) {
		t.Fatalf("expected nondeterministic prop-test failure, got %v", err)
	}
}

func TestRunWithContextEmitsNondeterministicFailureOutput(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		uintptr(1), libhegel.OK, // run_start
		uintptr(1), libhegel.OK, // next_test_case: one case
		true, libhegel.OK, // is_nondeterministic
		uintptr(1), libhegel.OK, // printer
		libhegel.OK,                     // note
		libhegel.OK,                     // mark_complete
		libhegel.OK,                     // resolve
		"failure output\n", libhegel.OK, // value
		uintptr(0), libhegel.OK, // next_test_case: run finished
		uintptr(1), libhegel.OK, // run_result
		libhegel.RUN_STATUS_FAILED_NONDETERMINISTIC, libhegel.OK, // result status
	)
	var output strings.Builder
	opts := applyOpts([]Option{WithDerandomize(false)})
	opts.output = &output
	err := runWithContext(lib, func(tc TestCase) {
		tc.Log("failure output")
		tc.abort(&invocationError{status: libhegel.STATUS_INTERESTING, cause: "failed", kind: "failure"})
	}, opts)
	if err != nil {
		t.Fatalf("runWithContext: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "failure output") {
		t.Fatalf("output = %q, want nondeterministic failure output", got)
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
		false, libhegel.OK, // print blob
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

func TestBuildSettingsExercisesAllSetters(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // test_cases
		libhegel.OK,             // derandomize
		libhegel.OK,             // seed
		libhegel.OK,             // database
		libhegel.OK,             // database_key
		libhegel.OK,             // suppress_health_check
		libhegel.OK,             // backend
		libhegel.OK,             // verbosity
		libhegel.OK,             // report_multiple_failures
		libhegel.OK,             // show_statistics
		libhegel.OK,             // print_blob
		libhegel.OK,             // phases
		libhegel.OK,             // settings
		uintptr(1), libhegel.OK, // run_start
		uintptr(0), libhegel.OK, // next_test_case NULL => run finished
		uintptr(1), libhegel.OK, // run_result
		libhegel.RUN_STATUS_PASSED, libhegel.OK, // result status: passed
	)
	// Apply every settings-backed option, in the same order the stub expects
	// each setter to fire.
	opts := applyOpts([]Option{
		WithTestCases(5),
		WithDerandomize(false),
		WithSeed(7),
		WithDatabase("/tmp/does-not-matter.db"),
		withDatabaseKey("TestBuildSettingsExercisesAllSetters"),
		SuppressHealthCheck(FilterTooMuch),
		WithBackend(BackendURandom),
		WithVerbosity(VerbosityVerbose),
		WithReportMultipleFailures(true),
		WithStatistics(true),
		WithReproductionBlob(true),
		WithPhases(PhaseGenerate, PhaseShrink),
		WithTestCases(1),
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
		false, libhegel.OK, // is_nondeterministic
		libhegel.E_BACKEND, "mark boom", // mark_complete fails (diagnostic read by invoke)
	)
	err := runWithContext(lib, func(TestCase) {}, applyOpts([]Option{WithDerandomize(false)}))
	if err == nil || !strings.Contains(err.Error(), "mark boom") {
		t.Fatalf("expected mark_complete error, got %v", err)
	}
}

func TestRunWithHandleIsNondeterministicError(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK,
		libhegel.OK, // Draw span
		uintptr(1), libhegel.OK,
		uintptr(1), libhegel.OK,
		false, libhegel.E_BACKEND, "capture boom",
	)
	err := runWithContext(lib, func(TestCase) {}, applyOpts([]Option{WithDerandomize(false)}))
	if err == nil || !strings.Contains(err.Error(), "capture boom") {
		t.Fatalf("expected is_nondeterministic error, got %v", err)
	}
}

func TestRunWithHandleTargetError(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		libhegel.OK,             // settings
		uintptr(1), libhegel.OK, // run_start
		uintptr(1), libhegel.OK, // next_test_case: one case
		false, libhegel.OK, // is_nondeterministic
		libhegel.E_BACKEND, "boom", // target fails (diagnostic read by invoke)
	)
	err := runWithContext(lib, func(tc TestCase) {
		tc.Target(1.0, "x")
	}, applyOpts([]Option{WithDerandomize(false), WithTestCases(1)}))
	if !errors.Is(err, libhegel.E_BACKEND) || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected target backend error, got %v", err)
	}
}

func TestEventAbortsOnEngineError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t, libhegel.E_BACKEND, "boom")
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	tc.Event("visited")
}

func TestEventValueAbortsOnEngineError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t, libhegel.E_BACKEND, "boom")
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	tc.EventValue("size", 42)
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
		false, libhegel.OK, // is_nondeterministic
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
		got = tc.startSpan(labelFromName("list"))
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
	}, uintptr(1), libhegel.E_BACKEND)
	if got == nil {
		t.Fatal("expected newCollection error")
	}
}

func TestRunWithHandleUnrecognizedShortCircuit(t *testing.T) {
	t.Parallel()
	lib := libhegel.Stub(t,
		uintptr(1), libhegel.OK, // settings_new
		libhegel.OK,             // derandomize
		libhegel.OK,             // settings
		uintptr(1), libhegel.OK, // run_start
		uintptr(1), libhegel.OK, // next_test_case: one case
		false, libhegel.OK, // is_nondeterministic
	)
	var sentinel = errors.New("weird")
	err := runWithContext(lib, func(tc TestCase) {
		tc.abort(sentinel)
	}, applyOpts([]Option{WithDerandomize(false), WithTestCases(1)}))
	if !errors.Is(err, sentinel) {
		t.Fatalf("runWithContext() error = %v, want %v", err, sentinel)
	}
}

func TestInvocationErrorFormatsCause(t *testing.T) {
	t.Parallel()
	err := &invocationError{cause: "failure details"}
	if got, want := err.Error(), "failure details"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestInvokeUsesIndependentAbortBoundaries(t *testing.T) {
	t.Parallel()
	tc := &testCase{panicPolicy: captureUserPanics}

	outer := tc.invoke(func(outerTC TestCase) {
		inner := outerTC.invoke(func(innerTC TestCase) {
			innerTC.Assume(false)
		})
		if !errors.Is(inner, libhegel.E_ASSUME) {
			t.Fatalf("inner result = %v, want E_ASSUME", inner)
		}
		outerTC.FailNow()
	})

	var outcome *invocationError
	if !errors.As(outer, &outcome) || outcome.status != libhegel.STATUS_INTERESTING {
		t.Fatalf("outer result = %v, want INTERESTING invocationError", outer)
	}
}

// TestInvokeReturnsStopTest covers an engine abort at an invocation boundary.
func TestInvokeReturnsStopTest(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t)
	result := tc.invoke(func(tc TestCase) {
		tc.(*testCase).abort(libhegel.E_STOP_TEST)
	})
	if !errors.Is(result, libhegel.E_STOP_TEST) {
		t.Fatalf("result = %v, want E_STOP_TEST", result)
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
	tc := newStubTestCase(t,
		libhegel.OK,
		int64(0), libhegel.E_BACKEND, "boom",
	)
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
		libhegel.OK,        // start_span
		uintptr(1),         // new_collection out-param placeholder
		libhegel.E_BACKEND, // new_collection fails
		"boom",             // diagnostic read by invoke
	)
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	Draw[[]int](tc, Lists[int](errGen[int]{}))
}

func TestDrawListCollectionMoreError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		libhegel.OK,        // start_span
		uintptr(1),         // new_collection out-param placeholder
		libhegel.OK,        // new_collection
		false,              // collection_more out-param placeholder
		libhegel.E_BACKEND, // collection_more fails => coll.Err()
		"boom",             // diagnostic read by invoke
	)
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	Draw[[]int](tc, Lists[int](errGen[int]{}))
}

func TestDrawMapNewCollectionError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		libhegel.OK,        // start_span
		uintptr(1),         // new_collection out-param placeholder
		libhegel.E_BACKEND, // new_collection fails
		"boom",             // diagnostic read by invoke
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
	err := run(1, func(tc TestCase) {
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
	err := run(1, func(tc TestCase) {
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
	err := run(1, func(tc TestCase) {
		_ = Draw[map[int]int](tc, Maps[int, int](Integers[int](0, 5), Integers[int](0, 5)))
	}, WithTestCases(5), WithDatabase(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDrawFilterStartSpanError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		libhegel.OK,                // Draw span
		libhegel.E_BACKEND, "boom", // filter attempt span
	)
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
		uintptr(1),  // new_collection out-param placeholder
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

// TestStatefulNewStateMachineError covers stateMachine.Run's panic when the
// engine rejects new_state_machine registration.
func TestStatefulNewStateMachineError(t *testing.T) {
	t.Parallel()
	// new_state_machine writes its *StateMachine and concurrency out-params
	// (placeholders) before returning the failing Error.
	tc := newStubTestCase(t, uintptr(0), int64(0), libhegel.E_BACKEND, "boom")
	sm := &stateMachine{rules: []stateMachineRule{{name: "Rule", fn: func(TestCase) {}}}, ruleGroups: []int64{0}}
	defer func() {
		err, ok := recover().(error)
		if !ok || !errors.Is(err, libhegel.E_BACKEND) {
			t.Fatalf("expected E_BACKEND panic, got %v", err)
		}
	}()
	sm.Run(tc)
}

// TestStatefulNextGroupError covers stateMachine.Run's panic when the engine
// rejects the round draw (state_machine_next_group).
func TestStatefulNextGroupError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(1), int64(1), libhegel.OK, // new_state_machine
		int64(0), libhegel.E_BACKEND, "boom", // next_group fails
	)
	sm := &stateMachine{rules: []stateMachineRule{{name: "Rule", fn: func(TestCase) {}}}, ruleGroups: []int64{0}}
	defer func() {
		err, ok := recover().(error)
		if !ok || !errors.Is(err, libhegel.E_BACKEND) {
			t.Fatalf("expected E_BACKEND panic, got %v", err)
		}
	}()
	sm.Run(tc)
}

// TestStatefulInitialInvariantError covers a panic from an initial invariant.
func TestStatefulInitialInvariantError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(1), int64(1), libhegel.OK, // new_state_machine
	)
	sm := &stateMachine{invariants: []stateMachineRule{{name: "Inv", fn: func(TestCase) {
		panic(libhegel.E_BACKEND)
	}}}}
	defer func() {
		err, ok := recover().(error)
		if !ok || !errors.Is(err, libhegel.E_BACKEND) {
			t.Fatalf("expected E_BACKEND panic, got %v", err)
		}
	}()
	sm.Run(tc)
}

func TestStatefulFinalInvariantError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(1), int64(1), libhegel.OK, // new_state_machine
		libhegel.StateMachineDone, libhegel.OK, // state_machine_next_group
	)
	calls := 0
	sm := &stateMachine{invariants: []stateMachineRule{{name: "Inv", fn: func(tc TestCase) {
		calls++
		if calls == 2 {
			tc.abort(libhegel.E_BACKEND)
		}
	}}}}
	defer expectErrorPanic(t, libhegel.E_BACKEND)
	sm.Run(tc)
}

func TestBuildSettingsCreationError(t *testing.T) {
	t.Parallel()
	ctx := libhegel.Stub(t, uintptr(0), libhegel.E_INVALID_ARG, "invalid profile")
	settings, err := (runOptions{}).buildSettings(ctx)
	if settings != nil || err == nil || !strings.Contains(err.Error(), "invalid profile") {
		t.Fatalf("buildSettings = %v, %v; want profile error", settings, err)
	}
}

func TestBuildSettingsNamedProfileError(t *testing.T) {
	t.Parallel()
	ctx := libhegel.Stub(t, uintptr(0), libhegel.E_INVALID_ARG, "unknown profile")
	settings, err := (runOptions{profile: "missing"}).buildSettings(ctx)
	if settings != nil || err == nil || !strings.Contains(err.Error(), "unknown profile") {
		t.Fatalf("buildSettings = %v, %v; want named profile error", settings, err)
	}
}

func TestWithProfileSelectsNamedProfile(t *testing.T) {
	t.Setenv("HEGEL_DEFAULT_PROFILE", "base")
	ctx := libhegel.NewContext()
	settings, err := applyOpts([]Option{WithProfile("workload")}).buildSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := settings.GetBackend(ctx)
	if err != nil || got != BackendURandom {
		t.Fatalf("backend = %v, %v; want %v", got, err, BackendURandom)
	}
}

func TestWithEmptyProfileUsesDefault(t *testing.T) {
	t.Setenv("HEGEL_DEFAULT_PROFILE", "workload")
	ctx := libhegel.NewContext()
	settings, err := applyOpts([]Option{WithProfile("")}).buildSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := settings.GetBackend(ctx)
	if err != nil || got != BackendURandom {
		t.Fatalf("backend = %v, %v; want %v", got, err, BackendURandom)
	}
}

func TestDefaultBackendUsesProfile(t *testing.T) {
	for _, test := range []struct {
		profile string
		want    Backend
	}{
		{"base", BackendDefault}, {"workload", BackendURandom},
	} {
		t.Run(test.profile, func(t *testing.T) {
			t.Setenv("HEGEL_DEFAULT_PROFILE", test.profile)
			ctx := libhegel.NewContext()
			settings, err := (runOptions{}).buildSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			got, err := settings.GetBackend(ctx)
			if err != nil || got != test.want {
				t.Fatalf("backend = %v, %v; want %v", got, err, test.want)
			}
		})
	}
}
