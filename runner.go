package hegel

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

// panicPolicy controls whether a panic from user code is converted into an
// interesting test case for the engine or allowed to escape to the caller.
type panicPolicy bool

const (
	captureUserPanics   panicPolicy = false
	propagateUserPanics panicPolicy = true
)

// testCase holds the per-test-case context.
//
// It is compatible with most popular TestingT interfaces from assert libraries.
type testCase struct {
	ctx         *libhegel.Context
	tc          *libhegel.TestCase
	out         io.Writer // Final destination, owned only by the root; nil on clones.
	printer     *libhegel.Printer
	depth       int
	panicPolicy panicPolicy
	abortFn     func(error)
}

func newTestCase(ctx *libhegel.Context, tc *libhegel.TestCase, out io.Writer, policy panicPolicy) (*testCase, error) {
	s := &testCase{
		ctx:         ctx,
		tc:          tc,
		out:         out,
		panicPolicy: policy,
	}
	if out != nil {
		printer, err := tc.Printer(ctx, nil)
		if err != nil {
			return nil, err
		}
		s.printer = printer
	}
	return s, nil
}

type invocationError struct {
	status libhegel.Status
	cause  any
	pcs    []uintptr
	kind   string
}

func (e *invocationError) Error() string {
	return fmt.Sprint(e.cause)
}

func failureInvocationError(message string) *invocationError {
	err := interestingInvocationError(message)
	err.kind = "failure"
	return err
}

func (e *invocationError) Unwrap() error {
	err, _ := e.cause.(error)
	return err
}

func interestingInvocationError(cause any) *invocationError {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(3, pcs)
	pcs = pcs[:n]
	return &invocationError{
		status: libhegel.STATUS_INTERESTING,
		cause:  cause,
		pcs:    pcs,
		kind:   "panic",
	}
}

// errPropTestFailed is the marker error for "property failed during this run".
// testCase.run records individual failures; runWithContext collects the run.
var errPropTestFailed = errors.New("property test failed")

// Assume rejects the current test case if condition is false.
func (s *testCase) Assume(condition bool) {
	if !condition {
		s.abort(libhegel.E_ASSUME)
	}
}

func (s *testCase) Note(message string) {
	if s.printer != nil {
		if err := s.tc.Note(s.ctx, message); err != nil {
			s.abort(err)
		}
	}
}

func (s *testCase) Event(label string) {
	if err := s.tc.Event(s.ctx, label); err != nil {
		s.abort(err)
	}
}

func (s *testCase) EventValue(label string, value float64) {
	if err := s.tc.EventValue(s.ctx, value, label); err != nil {
		s.abort(err)
	}
}

func (s *testCase) log(format string, args ...any) {
	if s.printer != nil {
		s.Note(fmt.Sprintf(format, args...))
	}
}

func (s *testCase) reportDraw(skip int, value any) {
	if s.printer == nil {
		return
	}
	msg := formatDrawReport(skip+1, value)
	s.Note(msg)
}

func (s *testCase) Errorf(format string, args ...any) {
	s.abort(failureInvocationError(fmt.Sprintf(format, args...)))
}

func (s *testCase) Fail() {
	s.abort(failureInvocationError("test case aborted with Fail"))
}

func (s *testCase) FailNow() {
	s.abort(failureInvocationError("test case aborted with FailNow"))
}

func (s *testCase) Log(args ...any) {
	s.Note(fmt.Sprint(args...))
}

func (s *testCase) Target(value float64, label string) {
	err := s.tc.Target(s.ctx, value, label)
	if err != nil {
		s.abort(err)
	}
}

func (s *testCase) abort(err error) {
	if s.abortFn != nil {
		s.abortFn(err)
	}
	panic(err)
}

func (s *testCase) engine() (*libhegel.Context, *libhegel.TestCase) {
	return s.ctx, s.tc
}

// clone returns a test-case wrapper backed by an independent libhegel stream.
func (s *testCase) clone() (TestCase, error) {
	tc, err := s.tc.Clone(s.ctx)
	if err != nil {
		return nil, err
	}
	clone := &testCase{ctx: s.ctx.Clone(), tc: tc, panicPolicy: s.panicPolicy}
	if s.printer != nil {
		clone.printer, err = tc.Printer(clone.ctx, nil)
		if err != nil {
			tc.Free()
			return nil, err
		}
	}
	return clone, nil
}

func (s *testCase) free() {
	s.tc.Free()
}

func (s *testCase) stateMachineNew(ruleNames []string, ruleGroups []int64, invariantNames []string, invariantAlwaysCheck []bool, maxConcurrency, stepCount int) (*libhegel.StateMachine, int64, error) {
	machine, concurrency, err := s.tc.NewStateMachine(s.ctx, ruleNames, ruleGroups, nil, invariantNames, invariantAlwaysCheck, 1, int64(maxConcurrency), int64(stepCount))
	return machine, concurrency, err
}

func (s *testCase) stateMachineNextGroup(machine *libhegel.StateMachine) (libhegel.StateMachineGroup, error) {
	return s.tc.StateMachineNextGroup(s.ctx, machine)
}

func (s *testCase) stateMachineNextRule(machine *libhegel.StateMachine, worker int64) (int64, error) {
	return s.tc.StateMachineNextRule(s.ctx, machine, worker)
}

func (s *testCase) stateMachineRuleRejected(machine *libhegel.StateMachine, worker int64) error {
	return s.tc.StateMachineRuleRejected(s.ctx, machine, worker)
}

func (s *testCase) stateMachineShouldCheckInvariant(machine *libhegel.StateMachine, invariant int64) (bool, error) {
	return s.tc.StateMachineShouldCheckInvariant(s.ctx, machine, invariant)
}

func (s *testCase) startSpan(spanLabel libhegel.Label) error {
	err := s.tc.StartSpan(s.ctx, spanLabel)
	if err != nil {
		return err
	}
	s.depth++
	return nil
}

func (s *testCase) stopSpan(discard bool) error {
	err := s.tc.StopSpan(s.ctx, discard)
	if err != nil {
		return err
	}
	s.depth--
	return nil
}

func (s *testCase) inSpan() bool {
	return s.depth > 0
}

func (s *testCase) newCollection(minSize int, maxSize *int) (*collection, error) {
	maxVal := uint64(math.MaxUint64)
	if maxSize != nil {
		maxVal = uint64(*maxSize)
	}
	id, err := s.tc.NewCollection(s.ctx, uint64(minSize), maxVal)
	if err != nil {
		return nil, err
	}
	return &collection{ctx: s.ctx, tc: s.tc, id: id}, nil
}

// --- collection protocol ---

// collection manages an engine-side collection (list/set/map) generation session.
//
// Errors from More and Reject are stashed on err. Callers iterate with
// `for coll.More(s) { ... }` and check `coll.Err()` once after the loop.
type collection struct {
	ctx      *libhegel.Context
	tc       *libhegel.TestCase
	id       *libhegel.Collection
	finished bool
	err      error
}

// Err returns the first error encountered by More or Reject, or nil.
func (c *collection) Err() error {
	return c.err
}

// More asks the engine whether another element should be generated.
//
// Returns false once the collection is finished or an error has been
// recorded; check Err after the loop to distinguish those cases.
func (c *collection) More() bool {
	if c.finished || c.err != nil {
		return false
	}
	more, err := c.tc.CollectionMore(c.ctx, c.id)
	if err != nil {
		c.err = err
		return false
	}
	if !more {
		c.finished = true
	}
	return more
}

// Reject tells the engine that the last generated element should not count.
// reason is an optional human-readable explanation (e.g. "duplicate key")
// surfaced to the engine for diagnostics.
//
// Errors are recorded on the collection and surfaced via Err.
func (c *collection) Reject(reason string) {
	if c.finished || c.err != nil {
		return
	}
	if err := c.tc.CollectionReject(c.ctx, c.id, reason); err != nil {
		c.err = err
	}
}

// testBody is the internal representation of a test function.
// It receives the [TestCase] for the current test case.
type testBody func(TestCase)

// --- Health checks ---

// HealthCheck identifies a health check that can be suppressed during a run.
//
// Health checks detect common issues with test configuration that would
// otherwise cause tests to run inefficiently or not at all.
type HealthCheck = libhegel.HealthCheck

const (
	// FilterTooMuch indicates too many test cases are being filtered out via [TestCase.Assume].
	FilterTooMuch = libhegel.HC_FILTER_TOO_MUCH
	// TooSlow indicates test execution is too slow.
	TooSlow = libhegel.HC_TOO_SLOW
	// TestCasesTooLarge indicates generated test cases are too large.
	TestCasesTooLarge = libhegel.HC_TEST_CASES_TOO_LARGE
	// LargeInitialTestCase indicates the smallest natural input is very large.
	LargeInitialTestCase = libhegel.HC_LARGE_INITIAL_TEST_CASE
)

// AllHealthChecks returns all health check variants.
func AllHealthChecks() []HealthCheck {
	return []HealthCheck{FilterTooMuch, TooSlow, TestCasesTooLarge, LargeInitialTestCase}
}

// --- Engine knobs ---

// Backend selects the engine's source of randomness. Pass one to [WithBackend].
type Backend = libhegel.Backend

const (
	// BackendDefault expands a single seeded PRNG; runs are reproducible from
	// the seed and shrinking / replay work as usual.
	BackendDefault = libhegel.BACKEND_DEFAULT
	// BackendURandom reads fresh entropy from /dev/urandom on every draw.
	// Intended for running under Antithesis; you almost certainly don't want it
	// otherwise.
	BackendURandom = libhegel.BACKEND_URANDOM
)

// Verbosity controls how much the engine logs during a run. Pass one to
// [WithVerbosity].
type Verbosity = libhegel.Verbosity

const (
	// VerbosityQuiet suppresses engine logging.
	VerbosityQuiet = libhegel.VERBOSITY_QUIET
	// VerbosityNormal is the default logging level.
	VerbosityNormal = libhegel.VERBOSITY_NORMAL
	// VerbosityVerbose enables verbose logging.
	VerbosityVerbose = libhegel.VERBOSITY_VERBOSE
	// VerbosityDebug enables debug logging.
	VerbosityDebug = libhegel.VERBOSITY_DEBUG
)

// Phase identifies one phase of a property-test run. Phases can be combined and
// passed to [WithPhases] to restrict which phases the engine runs.
type Phase = libhegel.Phase

const (
	// PhaseExplicit runs explicitly-provided examples.
	PhaseExplicit = libhegel.PHASE_EXPLICIT
	// PhaseReuse replays examples from the example database.
	PhaseReuse = libhegel.PHASE_REUSE
	// PhaseGenerate generates new examples.
	PhaseGenerate = libhegel.PHASE_GENERATE
	// PhaseTarget runs targeted-property search.
	PhaseTarget = libhegel.PHASE_TARGET
	// PhaseShrink shrinks failing examples.
	PhaseShrink = libhegel.PHASE_SHRINK
)

// AllPhases returns all phase variants.
func AllPhases() []Phase {
	return []Phase{PhaseExplicit, PhaseReuse, PhaseGenerate, PhaseTarget, PhaseShrink}
}

// --- Test runner options ---

// settingApplier mutates a libhegel settings object. Options that map to an
// engine setting append one of these; [runOptions.buildSettings] runs them in
// order, so a later applier overrides an earlier one (this is how user options
// override the CI defaults seeded by [run]).
type settingApplier func(*libhegel.Context, *libhegel.Settings) error

// runOptions holds options for property tests.
//
// Settings-backed options add an applier only when set, which preserves engine
// defaults. Options that select the initial settings or affect runner behavior
// use dedicated fields.
type runOptions struct {
	settingsAppliers []settingApplier
	profile          string

	// output receives note/draw-report output during the final replay of
	// interesting cases. nil means no output.
	output io.Writer
}

// addSetting appends a libhegel settings mutation applied at build time.
func (o *runOptions) addSetting(apply settingApplier) {
	o.settingsAppliers = append(o.settingsAppliers, apply)
}

// Option is a functional option for Test and Run.
type Option func(*runOptions)

// WithProfile uses the named profile as the base settings. Other options
// override the profile. An empty name uses the process-wide default profile.
func WithProfile(name string) Option {
	return func(o *runOptions) { o.profile = name }
}

// WithTestCases sets the number of test cases to run.
func WithTestCases(n int) Option {
	return func(o *runOptions) {
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.TestCases(ctx, uint64(n))
		})
	}
}

// SuppressHealthCheck suppresses the given health checks so they do not cause
// test failure.
//
// Each call sets the complete suppression mask; calls do not accumulate, so
// when SuppressHealthCheck is passed more than once the last call wins. Pass
// every check to suppress in a single call.
func SuppressHealthCheck(checks ...HealthCheck) Option {
	var mask HealthCheck
	for _, hc := range checks {
		mask |= hc
	}
	return func(o *runOptions) {
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.SuppressHealthCheck(ctx, mask)
		})
	}
}

// WithDatabase configures example-database persistence for this test. A
// non-empty path persists failing examples to that directory; an empty path
// disables persistence entirely, so no failing examples are saved or replayed.
//
// The default (when WithDatabase is not specified) is to use libhegel's default
// database location, except in CI environments where the database is
// automatically disabled.
func WithDatabase(path string) Option {
	return func(o *runOptions) {
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.Database(ctx, path)
		})
	}
}

// WithDerandomize sets whether to use a fixed seed for reproducible runs.
func WithDerandomize(derandomize bool) Option {
	return func(o *runOptions) {
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.Derandomize(ctx, derandomize)
		})
	}
}

// WithSeed sets a fixed random seed for the test, making it deterministic.
func WithSeed(seed int64) Option {
	return func(o *runOptions) {
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.Seed(ctx, uint64(seed), true)
		})
	}
}

// WithBackend selects the engine's randomness backend. See [Backend]. The
// default comes from the active libhegel settings profile.
func WithBackend(b Backend) Option {
	return func(o *runOptions) {
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.Backend(ctx, b)
		})
	}
}

// WithVerbosity sets how much the engine logs during a run. See [Verbosity].
// The active profile supplies the default; the base profile uses [VerbosityNormal].
func WithVerbosity(v Verbosity) Option {
	return func(o *runOptions) {
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.Verbosity(ctx, v)
		})
	}
}

// WithReportMultipleFailures sets whether the engine reports every distinct
// counterexample it finds rather than stopping at the first.
func WithReportMultipleFailures(report bool) Option {
	return func(o *runOptions) {
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.ReportMultipleFailures(ctx, report)
		})
	}
}

// WithStatistics controls end-of-run statistics for events recorded with
// [TestCase.Event] and [TestCase.EventValue]. Statistics are disabled by default.
// A nonempty HEGEL_STATISTICS value other than "0" enables statistics unless
// WithStatistics is set.
func WithStatistics(show bool) Option {
	return func(o *runOptions) {
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.ShowStatistics(ctx, show)
		})
	}
}

// WithReproductionBlob controls whether failure output includes the base64
// reproduction blob for the counterexample. The blob remains available to the
// runner for replay regardless of this setting.
func WithReproductionBlob(show bool) Option {
	return func(o *runOptions) {
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.PrintBlob(ctx, show)
		})
	}
}

// WithPhases restricts the run to the given test phases. See [Phase] and
// [AllPhases]. The active profile supplies the default; the base profile runs all phases.
func WithPhases(phases ...Phase) Option {
	var mask Phase
	for _, p := range phases {
		mask |= p
	}
	return func(o *runOptions) {
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.Phases(ctx, mask)
		})
	}
}

// withDatabaseKey sets the example-database key. Unexported: only [Test]
// supplies a key, deriving it from t.Name(). The key is applied unconditionally;
// libhegel ignores it when the database is disabled.
func withDatabaseKey(key string) Option {
	return func(o *runOptions) {
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.DatabaseKey(ctx, key)
		})
	}
}

// withOutput sets the writer that receives note and draw-report output during
// the final replay of interesting cases. Unexported: [Run] sets it to
// [os.Stdout], [Test] to t.Output(), [Workload] to its stdout. Tests use it to
// inspect output.
func withOutput(w io.Writer) Option {
	return func(o *runOptions) { o.output = w }
}

// Run runs a property test and returns any error.
//
// Note output goes to stdout. For use in standalone binaries and conformance tests.
func Run(fn func(TestCase), opts ...Option) error {
	return run(1, fn, append(opts, withOutput(os.Stdout))...)
}

// MustRun runs a property test and panics if it fails.
func MustRun(fn func(TestCase), opts ...Option) {
	if err := run(1, fn, append(opts, withOutput(os.Stdout))...); err != nil {
		panic(err)
	}
}

// Test runs a property test against t.
func Test(t *testing.T, fn func(*T), opts ...Option) {
	t.Helper()

	body := func(tc TestCase) {
		ht := &T{testCase: tc.(*testCase), T: t}
		fn(ht)
	}
	allOpts := append(opts, withDatabaseKey(t.Name()), withOutput(t.Output()))

	if err := run(1, body, allOpts...); err != nil { // coverage-ignore (run's error is covered via Run; this only delegates to stdlib testing.T)
		if errors.Is(err, errPropTestFailed) {
			t.Fail()
		} else {
			t.Fatal(err)
		}
	}
}

// run runs a property after skipping callerSkip stack frames above itself to
// find the property definition. The example-database key is supplied by
// [Test], and [withOutput] routes output.
func run(callerSkip int, fn testBody, opts ...Option) error {
	var o runOptions
	for _, opt := range opts {
		opt(&o)
	}

	var pcs [32]uintptr
	n := runtime.Callers(2+callerSkip, pcs[:])
	if location, ok := findCallerLocationInPCs(pcs[:n], anyFrame); ok {
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.TestLocation(ctx, location.file, uint32(location.line), location.class, location.function)
		})
	}

	ctx := libhegel.NewContext()
	return runWithContext(ctx, fn, o)
}

func runWithContext(ctx *libhegel.Context, fn testBody, opts runOptions) error {
	s, err := opts.buildSettings(ctx)
	if err != nil {
		return err
	}

	run, err := s.RunStart(ctx, opts.output)
	if err != nil {
		return err
	}

	var nondeterministicOutput *bytes.Buffer
	for {
		tc, err := run.NextTestCase(ctx)
		if err != nil {
			return err
		}
		if tc == nil {
			break
		}

		nondeterministic, err := tc.IsNondeterministic(ctx)
		if err != nil {
			return err
		}

		var out io.Writer
		policy := captureUserPanics
		if nondeterministic {
			// Buffer non-deterministic test cases, because we only know whether
			// we should output them after they have run. At which point we
			// can't recreate them because replay isn't available.
			//
			// NB: It doesn't make sense to propagate user panics when buffering
			// output because we can't flush the buffer on a panic.
			out = new(bytes.Buffer)
		}

		state, err := newTestCase(ctx, tc, out, policy)
		if err != nil {
			return err
		}

		failed, err := state.run(fn)
		if err != nil {
			return err
		}

		if failed && nondeterministic {
			nondeterministicOutput = out.(*bytes.Buffer)
		}
	}

	result, err := run.RunResult(ctx)
	if err != nil {
		return err
	}

	switch result.Status(ctx) {
	case libhegel.RUN_STATUS_PASSED:
		return nil
	case libhegel.RUN_STATUS_ERROR:
		// The run itself failed (a health check, a nondeterministic test, an
		// engine panic) and produced no verdict on the property. There are no
		// counterexamples to collect; the diagnostic lives in the run-level
		// error message.
		return fmt.Errorf("%w: %s", errPropTestFailed, result.ErrorMessage(ctx))
	case libhegel.RUN_STATUS_FAILED_NONDETERMINISTIC:
		// We hit a non-deterministic failure, so replay is not available.
		// Output the last cached run.
		_, err := io.Copy(opts.output, nondeterministicOutput)
		return err
	default:
		return replayFailures(ctx, s, result, fn, opts)
	}
}

// buildSettings constructs the libhegel settings for this run by applying every
// recorded setting in order; later appliers override earlier ones.
//
// Each applier is a fallible libhegel call; a non-nil error means the option
// was rejected and must be surfaced rather than silently dropped. Errors are
// collected (nil entries dropped by errors.Join) so a bad option is reported
// instead of being lost.
func (o runOptions) buildSettings(ctx *libhegel.Context) (*libhegel.Settings, error) {
	var (
		s   *libhegel.Settings
		err error
	)
	if o.profile == "" {
		s, err = ctx.SettingsNew()
	} else {
		s, err = ctx.SettingsNewForProfile(o.profile)
	}
	if err != nil {
		return nil, err
	}

	var errs []error
	for _, apply := range o.settingsAppliers {
		errs = append(errs, apply(ctx, s))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return s, nil
}

// invoke runs fn with an invocation-local abort boundary.
//
// Returns an invocationError if fn panics or trips an assertion,
// or a libhegel.
func (s *testCase) invoke(fn testBody) (result error) {
	aborted := false

	defer func() {
		// Do not call recover at all when panics must propagate. Recovering and
		// re-panicking the value would replace the user's original panic stack.
		if !aborted && s.panicPolicy == propagateUserPanics {
			return
		}

		recovered := recover()
		if aborted {
			result = recovered.(error)
			return
		}
		if recovered != nil {
			result = interestingInvocationError(recovered)
		}
	}()

	scoped := *s
	scoped.abortFn = func(err error) {
		aborted = true
		panic(err)
	}

	fn(&scoped)
	return nil
}

func (s *testCase) run(fn testBody) (failed bool, err error) {
	var result error
	defer func() {
		// Rejected and overrun cases are probes, not results. Their native
		// documents must remain private even when this case owns an output
		// destination.
		if errors.Is(result, libhegel.E_ASSUME) || errors.Is(result, libhegel.E_STOP_TEST) {
			return
		}
		err = errors.Join(err, s.flushNativeOutput())
		formatInvocationResult(s.out, result)
	}()

	result = s.invoke(fn)
	if result == nil {
		return false, s.tc.MarkComplete(s.ctx, libhegel.STATUS_VALID, "")
	}

	var status libhegel.Status
	var origin string
	switch {
	case errors.Is(result, libhegel.E_ASSUME):
		status = libhegel.STATUS_INVALID
	case errors.Is(result, libhegel.E_STOP_TEST):
		status = libhegel.STATUS_OVERRUN
	default:
		var outcome *invocationError
		if !errors.As(result, &outcome) {
			return true, result
		}
		status = outcome.status
		origin = findCallerInPCs(outcome.pcs, isNotHegelFrame)
	}

	return true, s.tc.MarkComplete(s.ctx, status, origin)

}

func (s *testCase) flushNativeOutput() error {
	if s.out == nil {
		return nil
	}
	// Match the Rust frontend: no deferred regions is a harmless resolve error;
	// Value still reports layout errors after resolution.
	_ = s.printer.Resolve(s.ctx)
	value, err := s.printer.Value(s.ctx)
	if err != nil {
		return err
	}
	_, err = io.WriteString(s.out, value)
	return err
}

// replayFailures walks the failures of a result and replays fn against them.
//
// Returns an error describing the failures.
func replayFailures(ctx *libhegel.Context, s *libhegel.Settings, result *libhegel.Result, fn testBody, opts runOptions) error {
	var origins []string
	for i := range result.FailureCount(ctx) {
		fail, err := result.Failure(ctx, i)
		if err != nil {
			return err
		}
		blob := fail.ReproductionBlob(ctx)
		if blob == "" {
			return errPropTestFailed
		}
		printBlob, err := s.GetPrintBlob(ctx)
		if err != nil {
			return err
		}
		tc, err := s.TestCaseFromBlob(ctx, blob, opts.output)
		if err != nil {
			return err
		}
		state, err := newTestCase(ctx, tc, opts.output, propagateUserPanics)
		if err != nil {
			return err
		}
		if err := replayFailure(state, fn, opts.output, printBlob, blob); err != nil {
			return err
		}
		origins = append(origins, fail.Origin(ctx))
	}
	return fmt.Errorf("%w: %d failures %v", errPropTestFailed, len(origins), origins)
}

// replayFailure appends the reproduction blob when enabled, even if replay panics.
func replayFailure(state *testCase, fn testBody, out io.Writer, printBlob bool, blob string) (err error) {
	defer func() {
		if !printBlob || out == nil {
			return
		}
		_, writeErr := fmt.Fprintf(out, "reproduction blob: %s\n", blob)
		err = errors.Join(err, writeErr)
	}()
	_, err = state.run(fn)
	return err
}

type callerLocation struct {
	file     string
	line     int
	class    string
	function string
}

func findCallerLocationInPCs(pcs []uintptr, filter func(string) bool) (callerLocation, bool) {
	frame, ok := findCallerFrameInPCs(pcs, filter)
	if !ok {
		return callerLocation{}, false
	}
	function := strings.TrimPrefix(path.Ext(frame.Function), ".")
	class, _ := strings.CutSuffix(frame.Function, "."+function)
	return callerLocation{
		file:     frame.File,
		line:     frame.Line,
		class:    class,
		function: function,
	}, true
}

func findCallerFrameInPCs(pcs []uintptr, filter func(string) bool) (runtime.Frame, bool) {
	frames := runtime.CallersFrames(pcs)
	for {
		frame, more := frames.Next()
		if filter(frame.Function) {
			return frame, true
		}
		if !more {
			return runtime.Frame{}, false
		}
	}
}

func anyFrame(string) bool {
	return true
}

// findCallerInPCs returns the first matching frame as "<file>:<line> (<pc>)".
// The result is used as libhegel's stable shrink-grouping key.
func findCallerInPCs(pcs []uintptr, filter func(string) bool) string {
	frame, ok := findCallerFrameInPCs(pcs, filter)
	if !ok {
		return "<unknown>:0 (0x0)"
	}
	return fmt.Sprintf("%s:%d (%#x)", frame.File, frame.Line, frame.PC)
}

func formatInvocationResult(out io.Writer, err error) {
	var outcome *invocationError
	if !errors.As(err, &outcome) || out == nil {
		return
	}
	header := outcome.kind
	if worker, ok := errors.AsType[*workerError](err); ok {
		header = fmt.Sprintf("%s in worker %d", header, worker.worker)
	}
	fmt.Fprintf(out, "%s: %s\n\n", header, outcome.Error())
	frames := runtime.CallersFrames(outcome.pcs)
	for {
		frame, more := frames.Next()
		fmt.Fprintf(out, "%s(...)\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
		if !more {
			break
		}
	}
}

func isHegelFrame(fn string) bool {
	const pkg = "hegel.dev/go/hegel"
	if !strings.HasPrefix(fn, pkg) {
		return false
	}
	if len(fn) == len(pkg) {
		return true
	}
	// "hegel.dev/go/hegel.Func" or "hegel.dev/go/hegel/sub.Func" → internal.
	// "hegel.dev/go/hegel_test.Func" → external (test package), not internal.
	next := fn[len(pkg)]
	return next == '.' || next == '/'
}

func isNotHegelFrame(fn string) bool {
	return !isHegelFrame(fn)
}

func (s *testCase) setWorker(index int64) error {
	return s.tc.SetWorker(s.ctx, index)
}
