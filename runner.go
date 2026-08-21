package hegel

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
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
	out         io.Writer // nil when output is deferred; otherwise emitted live
	depth       int       // current span nesting depth
	panicPolicy panicPolicy
	abortFn     func(error)
}

func newTestCase(ctx *libhegel.Context, tc *libhegel.TestCase, out io.Writer, policy panicPolicy) *testCase {
	return &testCase{
		ctx:         ctx,
		tc:          tc,
		out:         out,
		panicPolicy: policy,
	}
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
	if s.out != nil {
		fmt.Fprintln(s.out, message)
	}
}

func (s *testCase) log(format string, args ...any) {
	if s.out != nil {
		fmt.Fprintln(s.out, fmt.Sprintf(format, args...))
	}
}

func (s *testCase) output() io.Writer {
	return s.out
}

func (s *testCase) setOutput(out io.Writer) {
	s.out = out
}

func (s *testCase) reportDraw(skip int, value any) {
	if s.out == nil {
		return
	}
	msg := formatDrawReport(skip+1, value)
	fmt.Fprintln(s.out, msg)
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
	if s.out != nil {
		if _, ok := s.out.(*lockedWriter); !ok {
			s.out = &lockedWriter{w: s.out}
		}
	}
	tc, err := s.tc.Clone(s.ctx)
	if err != nil {
		return nil, err
	}
	return newTestCase(s.ctx.Clone(), tc, s.out, s.panicPolicy), nil
}

func (s *testCase) stateMachineNew(ruleNames []string, ruleGroups []int64, invariantNames []string, maxConcurrency int) (*libhegel.StateMachine, int64, error) {
	machine, concurrency, err := s.tc.NewStateMachine(s.ctx, ruleNames, ruleGroups, invariantNames, 1, int64(maxConcurrency))
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

func (s *testCase) startSpan(label libhegel.Label) error {
	err := s.tc.StartSpan(s.ctx, label)
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
	// BackendAuto chooses automatically (the default): urandom under
	// Antithesis, otherwise the default seeded PRNG.
	BackendAuto = libhegel.BACKEND_AUTO
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
// Settings-backed options are recorded as appliers rather than inert fields, so
// "not set" is simply "no applier" — the engine default applies and there is no
// set/unset bookkeeping. Only options the runner itself reads (beyond
// configuring libhegel) keep dedicated fields.
type runOptions struct {
	settingsAppliers []settingApplier
	singleTestCase   bool
	// output receives note/draw-report output during single-test-case mode
	// and during the final replay of interesting cases. nil means no output.
	output io.Writer
}

// addSetting appends a libhegel settings mutation applied at build time.
func (o *runOptions) addSetting(apply settingApplier) {
	o.settingsAppliers = append(o.settingsAppliers, apply)
}

// Option is a functional option for Test and Run.
type Option func(*runOptions)

// WithTestCases sets the number of test cases to run.
func WithTestCases(n int) Option {
	return func(o *runOptions) {
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.TestCases(ctx, uint64(n))
		})
	}
}

// WithStatefulStepCount sets the target number of rule steps to run per
// stateful test case. n must be at least 1; the default is 50.
func WithStatefulStepCount(n int) Option {
	return func(o *runOptions) {
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.StatefulStepCount(ctx, int64(n))
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
// default is [BackendAuto].
func WithBackend(b Backend) Option {
	return func(o *runOptions) {
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.Backend(ctx, b)
		})
	}
}

// WithVerbosity sets how much the engine logs during a run. See [Verbosity].
// The default is [VerbosityNormal].
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

// WithPhases restricts the run to the given test phases. See [Phase] and
// [AllPhases]. When WithPhases is not specified the engine runs all phases.
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

// WithSingleTestCase runs exactly one test case with no shrinking, replay, or
// example database. Use it for long-running workloads or tests whose body is
// not safely re-runnable on the same inputs.
func WithSingleTestCase() Option {
	return func(o *runOptions) {
		o.singleTestCase = true
		o.addSetting(func(ctx *libhegel.Context, s *libhegel.Settings) error {
			return s.Mode(ctx, libhegel.MODE_SINGLE_TEST_CASE)
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

// withOutput sets the writer that receives note and draw-report output
// during single-test-case mode and during the final replay of interesting
// cases. Unexported: [Run] sets it to [os.Stdout], [Test] to t.Output(),
// [Workload] to its stdout. Tests use it to inspect output.
func withOutput(w io.Writer) Option {
	return func(o *runOptions) { o.output = w }
}

// Run runs a property test and returns any error.
//
// Note output goes to stdout. For use in standalone binaries and conformance tests.
func Run(fn func(TestCase), opts ...Option) error {
	return run(fn, append(opts, withOutput(os.Stdout))...)
}

// MustRun runs a property test and panics if it fails.
func MustRun(fn func(TestCase), opts ...Option) {
	if err := Run(fn, opts...); err != nil {
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

	if err := run(body, allOpts...); err != nil { // coverage-ignore (run's error is covered via Run; this only delegates to stdlib testing.T)
		if errors.Is(err, errPropTestFailed) {
			t.Fail()
		} else {
			t.Fatal(err)
		}
	}
}

// run is the shared implementation for Run, MustRun, and Test.
//
// The example-database key is supplied (when applicable) by [Test]; non-test
// entry points leave it nil. Note/draw-report output is routed via
// [withOutput]; absent that option no output is produced.
func run(fn testBody, opts ...Option) error {
	var o runOptions
	for _, opt := range opts {
		opt(&o)
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
		} else if opts.singleTestCase {
			// We can't replay and output is unbounded. Just output it directly
			// and propagate user panics.
			out = opts.output
			policy = propagateUserPanics
		}

		state := newTestCase(ctx, tc, out, policy)

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
	s := ctx.SettingsNew()

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
	result := s.invoke(fn)
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
		formatInvocationResult(s.out, result)
		status = outcome.status
		origin = findCallerInPCs(outcome.pcs, isNotHegelFrame)
	}

	return true, s.tc.MarkComplete(s.ctx, status, origin)

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
		tc, err := s.TestCaseFromBlob(ctx, blob, opts.output)
		if err != nil {
			return err
		}
		state := newTestCase(ctx, tc, opts.output, propagateUserPanics)
		if _, err := state.run(fn); err != nil {
			return err
		}
		origins = append(origins, fail.Origin(ctx))
	}
	return fmt.Errorf("%w: %d failures %v", errPropTestFailed, len(origins), origins)
}

// findCallerInPCs returns the first matching frame as "<file>:<line> (<pc>)".
// The result is used as libhegel's stable shrink-grouping key.
func findCallerInPCs(pcs []uintptr, filter func(string) bool) string {
	frames := runtime.CallersFrames(pcs)
	for {
		frame, more := frames.Next()
		if filter(frame.Function) {
			return fmt.Sprintf("%s:%d (%#x)", frame.File, frame.Line, frame.PC)
		}
		if !more {
			break
		}
	}
	return "<unknown>:0 (0x0)"
}

func formatInvocationResult(out io.Writer, err error) {
	var outcome *invocationError
	if !errors.As(err, &outcome) || out == nil {
		return
	}
	header := outcome.kind
	var worker *workerError
	if errors.As(err, &worker) {
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

// lockedWriter serializes output shared by the engine callback and concurrent
// state-machine workers.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}
