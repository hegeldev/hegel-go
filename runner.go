package hegel

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"strings"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

// testCase holds the per-test-case context.
//
// It is compatible with most popular TestingT interfaces from assert libraries.
type testCase struct {
	ctx     *libhegel.Context
	tc      *libhegel.TestCase
	status  libhegel.Status
	origin  string
	aborted bool      // set if test case run was short circuited
	out     io.Writer // nil for exploratory cases; set for final replay / single-case
	depth   int       // current span nesting depth
}

// --- Sentinel errors ---

// errTestCaseAborted is panic'd by T.Fatal/Fatalf/FailNow.
var errTestCaseAborted = errors.New("test case aborted")

// errPropTestFailed is the marker error for "property failed during this run".
// driveOneCase wraps it for failing cases; runProperty collects across cases.
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

func (s *testCase) reportDraw(skip int, value any) {
	if s.out == nil {
		return
	}
	loc, msg := formatDrawReport(skip+1, value)
	fmt.Fprintf(s.out, "%s: %s\n", loc, msg)
}

func (s *testCase) Errorf(format string, args ...any) {
	s.Note(fmt.Sprintf(format, args...))
	s.Fail()
}

func (s *testCase) Fail() {
	s.setStatus(libhegel.STATUS_INTERESTING)
}

func (s *testCase) FailNow() {
	s.abort(errTestCaseAborted)
}

func (s *testCase) Log(args ...any) {
	s.Note(fmt.Sprint(args...))
}

func (s *testCase) Target(value float64, label string) {
	err := s.tc.Target(s.ctx, value, label)
	if err != nil {
		panic(err)
	}
}

func (s *testCase) clone() (TestCase, error) {
	clone, err := s.tc.Clone(s.ctx)
	if err != nil {
		return nil, err
	}

	return &testCase{ctx: s.ctx, tc: clone, out: s.out}, nil
}

func (s *testCase) setStatus(status libhegel.Status) {
	s.status = status
	s.origin = ""
	if s.status == libhegel.STATUS_INTERESTING {
		s.origin = findCaller(2, isNotHegelFrame)
	}
}

func (s *testCase) getStatus() libhegel.Status {
	return s.status
}

func (s *testCase) abort(err error) {
	var status libhegel.Status
	switch {
	case err == nil:
		status = s.status
	case errors.Is(err, libhegel.E_ASSUME):
		status = libhegel.STATUS_INVALID
	case errors.Is(err, libhegel.E_STOP_TEST):
		status = libhegel.STATUS_OVERRUN
	case errors.Is(err, errTestCaseAborted):
		status = libhegel.STATUS_INTERESTING
	default:
		// Unrecognized error: we panic instead of aborting.
		panic(err)
	}

	if status < s.status {
		// Ensure we never override STATUS_INTERESTING during an abort.
		return
	}

	s.setStatus(status)
	s.aborted = true
	panic(err)
}

func (s *testCase) recoverAbort() {
	if s.aborted {
		s.aborted = false
		_ = recover()
	}
}

func (s *testCase) engine() (*libhegel.Context, *libhegel.TestCase) {
	return s.ctx, s.tc
}

func (s *testCase) stateMachineNew(ruleNames, invariantNames []string) (libhegel.StateMachine, int64, error) {
	// hegel-go drives state machines sequentially for now: one concurrency
	// group holding every rule, and the concurrency level fixed at 1 (which
	// consumes no entropy). The drawn concurrency is therefore always 1 and is
	// discarded.
	machine, concurrency, err := s.tc.NewStateMachine(s.ctx, 1, ruleNames, make([]int64, len(ruleNames)), invariantNames, 1, int64(runtime.GOMAXPROCS(-1)))
	return machine, concurrency, err
}

func (s *testCase) stateMachineNextGroup(machine libhegel.StateMachine) (libhegel.StateMachineGroup, error) {
	return s.tc.StateMachineNextGroup(s.ctx, machine)
}

func (s *testCase) stateMachineNextRule(machine libhegel.StateMachine, worker int64) (int64, error) {
	return s.tc.StateMachineNextRule(s.ctx, machine, worker)
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
	id       libhegel.Collection
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
	// singleTestCase is read by the runner and replay logic, not just passed to
	// libhegel as a mode.
	singleTestCase bool
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

	if err := run(body, allOpts...); err != nil { // coverage-ignore (run's error is covered via Run; this only delegates to stdlib t.Fatal)
		t.Fatal(err)
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

	var output io.Writer
	var skipUserPanic bool
	if opts.singleTestCase {
		output = opts.output
		skipUserPanic = true
	}

	run, err := s.RunStart(ctx, output)
	if err != nil {
		return err
	}

	for {
		tc, err := run.NextTestCase(ctx)
		if err != nil {
			return err
		}
		if tc == nil {
			break
		}

		origin, status := driveOneCase(ctx, tc, fn, output, skipUserPanic)
		if err := tc.MarkComplete(ctx, status, origin); err != nil {
			return err
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
	default:
		if opts.singleTestCase {
			// There is never a need to replay a single test case.
			return errPropTestFailed
		}

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

func driveOneCase(ctx *libhegel.Context, tc *libhegel.TestCase, fn testBody, output io.Writer, skipUserPanic bool) (origin string, status libhegel.Status) {
	state := &testCase{
		ctx: ctx,
		tc:  tc,
		out: output,
	}
	// Registered first so it runs last: publish the case's final status and
	// origin into the named returns. When fn panics (FailNow/abort/user panic)
	// the normal return below is skipped, so without this the named returns
	// would keep their zero values (a VALID status) and the failure would be
	// silently dropped by the caller's mark_complete.
	defer func() {
		origin, status = state.origin, state.status
	}()
	defer func() {
		if skipUserPanic {
			return
		}

		r := recover()
		if r == nil {
			return
		}

		// The panic is recovered into an INTERESTING status. On the final
		// replay (or in single-case mode) we never get here: skipUserPanic
		// lets the panic propagate so the Go runtime prints it for debuggers.
		state.setStatus(libhegel.STATUS_INTERESTING)
	}()
	defer state.recoverAbort()
	fn(state)
	return
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
		tc, err := s.TestCaseFromBlob(ctx, fail.ReproductionBlob(ctx), opts.output)
		if err != nil {
			return err
		}
		driveOneCase(ctx, tc, fn, opts.output, true)
		origins = append(origins, fail.Origin(ctx))
	}
	return fmt.Errorf("%w: %d failures %v", errPropTestFailed, len(origins), origins)
}

// findCaller describes a recovered panic for [hegel_mark_complete]'s
// origin field. The format is "<file>:<line> (<pc>)", where the file/line
// and program counter come from the first frame matched by filter.
// skip has the same meaning as runtime.Callers.
//
// The origin MUST be stable for the same call site and distinct between
// different call sites so libhegel can group failing inputs together for
// shrinking without conflating separate failures.
//
//go:noinline
func findCaller(skip int, filter func(string) bool) string {
	var pcs [32]uintptr
	n := runtime.Callers(skip+1, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	file := ""
	line := 0
	var pc uintptr
	for {
		f, more := frames.Next()
		if !more { // coverage-ignore
			break
		}
		if filter(f.Function) {
			file = f.File
			line = f.Line
			pc = f.PC
			break
		}
	}
	return fmt.Sprintf("%s:%d (%#x)", file, line, pc)
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
