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
	tc             *libhegel.TestCase
	status         libhegel.Status
	origin         string
	singleTestCase bool      // set when this case runs under WithSingleTestCase
	aborted        bool      // set if test case run was short circuited
	out            io.Writer // nil for exploratory cases; set for final replay / single-case
	depth          int       // current span nesting depth
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
	err := s.tc.Target(value, label)
	if err != nil {
		panic(err)
	}
}

func (s *testCase) setStatus(status libhegel.Status) {
	s.status = status
	s.origin = ""
	if s.status == libhegel.STATUS_INTERESTING {
		s.origin = findExternalCaller()
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

func (s *testCase) generate(schema map[string]any) (any, error) {
	schemaBytes, err := encodeCBOR(schema)
	if err != nil { // coverage-ignore (fixed-shape maps always encode)
		panic(fmt.Sprintf("encode schema: %v", err))
	}
	valueBytes, err := s.tc.Generate(schemaBytes)
	if err != nil {
		return nil, err
	}
	return decodeCBOR(valueBytes)
}

func (s *testCase) isSingleTestCase() bool {
	return s.singleTestCase
}

func (s *testCase) startSpan(label libhegel.Label) error {
	err := s.tc.StartSpan(label)
	if err != nil {
		return err
	}
	s.depth++
	return nil
}

func (s *testCase) stopSpan(discard bool) error {
	err := s.tc.StopSpan(discard)
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
	id, err := s.tc.NewCollection(uint64(minSize), maxVal)
	if err != nil {
		return nil, err
	}
	return &collection{tc: s.tc, id: id}, nil
}

// --- collection protocol ---

// collection manages an engine-side collection (list/set/map) generation session.
//
// Errors from More and Reject are stashed on err. Callers iterate with
// `for coll.More(s) { ... }` and check `coll.Err()` once after the loop.
type collection struct {
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
	more, err := c.tc.CollectionMore(c.id)
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
	if err := c.tc.CollectionReject(c.id, reason); err != nil {
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

// --- Test runner options ---

// databaseState distinguishes "user has not set the database" (engine
// default), "user disabled the database", and "user supplied a path".
type databaseState int

const (
	databaseUnset databaseState = iota
	databaseDisabled
	databasePath
)

// DatabaseSetting configures example-database persistence for a test run.
// Construct one with [Database] (a path) or [DatabaseDisabled] (no
// persistence) and pass it to [WithDatabase].
type DatabaseSetting struct {
	state databaseState
	path  string
}

// Database returns a [DatabaseSetting] that persists failing examples to the
// given directory.
func Database(path string) DatabaseSetting {
	return DatabaseSetting{state: databasePath, path: path}
}

// DatabaseDisabled returns a [DatabaseSetting] that disables example-database
// persistence. No failing examples are saved or replayed.
//
// In CI environments, the database is disabled by default; this setting is
// useful when you want to disable it explicitly outside of CI.
func DatabaseDisabled() DatabaseSetting {
	return DatabaseSetting{state: databaseDisabled}
}

// runOptions holds options for property tests.
type runOptions struct {
	testCases           int
	suppressHealthCheck []HealthCheck
	database            DatabaseSetting
	derandomize         bool
	seed                *int64
	singleTestCase      bool
	// databaseKey identifies the test for example-database lookups. Set by
	// [Test] from t.Name(); nil for [Run]/[MustRun].
	databaseKey string
	// output receives note/draw-report output during single-test-case mode
	// and during the final replay of interesting cases. nil means no output.
	output io.Writer
}

// Option is a functional option for Test and Run.
type Option func(*runOptions)

// WithTestCases sets the number of test cases to run.
func WithTestCases(n int) Option {
	return func(o *runOptions) { o.testCases = n }
}

// SuppressHealthCheck suppresses the given health checks so they do not cause test failure.
func SuppressHealthCheck(checks ...HealthCheck) Option {
	return func(o *runOptions) { o.suppressHealthCheck = append(o.suppressHealthCheck, checks...) }
}

// WithDatabase configures example-database persistence for this test.
// Construct the setting with [Database] (a path) or [DatabaseDisabled].
//
// The default (when WithDatabase is not specified) is to use libhegel's default
// database location, except in CI environments where the database is
// automatically disabled.
func WithDatabase(db DatabaseSetting) Option {
	return func(o *runOptions) { o.database = db }
}

// WithDerandomize sets whether to use a fixed seed for reproducible runs.
func WithDerandomize(derandomize bool) Option {
	return func(o *runOptions) { o.derandomize = derandomize }
}

// WithSeed sets a fixed random seed for the test, making it deterministic.
func WithSeed(seed int64) Option {
	return func(o *runOptions) { o.seed = &seed }
}

// WithSingleTestCase runs exactly one test case with no shrinking, replay, or
// example database. Use it for long-running workloads or tests whose body is
// not safely re-runnable on the same inputs.
func WithSingleTestCase() Option {
	return func(o *runOptions) { o.singleTestCase = true }
}

// withDatabaseKey sets the example-database key. Unexported: only [Test]
// supplies a key, deriving it from t.Name().
func withDatabaseKey(key string) Option {
	return func(o *runOptions) { o.databaseKey = key }
}

// withOutput sets the writer that receives note and draw-report output
// during single-test-case mode and during the final replay of interesting
// cases. Unexported: [Run] sets it to [os.Stdout], [Test] to t.Output(),
// [Workload] to its stdout. Tests use it to inspect output.
func withOutput(w io.Writer) Option {
	return func(o *runOptions) { o.output = w }
}

// ciEnvVar describes a single CI-detection env var. If matchAny is true, the
// var counts as a CI signal whenever it is present in the environment.
type ciEnvVar struct {
	name     string
	expected string
	matchAny bool
}

var ciEnvVars = []ciEnvVar{
	{name: "CI", matchAny: true},
	{name: "__TOX_ENVIRONMENT_VARIABLE_ORIGINAL_CI", matchAny: true},
	{name: "TF_BUILD", expected: "true"},
	{name: "bamboo.buildKey", matchAny: true},
	{name: "BUILDKITE", expected: "true"},
	{name: "CIRCLECI", expected: "true"},
	{name: "CIRRUS_CI", expected: "true"},
	{name: "CODEBUILD_BUILD_ID", matchAny: true},
	{name: "GITHUB_ACTIONS", expected: "true"},
	{name: "GITLAB_CI", matchAny: true},
	{name: "HEROKU_TEST_RUN_ID", matchAny: true},
	{name: "TEAMCITY_VERSION", matchAny: true},
}

// isCI reports whether any well-known CI environment variable is set.
func isCI() bool {
	for _, v := range ciEnvVars {
		val, ok := os.LookupEnv(v.name)
		if !ok {
			continue
		}
		if v.matchAny || val == v.expected {
			return true
		}
	}
	return false
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
	inCI := isCI()
	o := runOptions{
		derandomize: inCI,
	}
	if inCI {
		o.database = DatabaseSetting{state: databaseDisabled}
	}
	for _, opt := range opts {
		opt(&o)
	}

	lib, _ := libhegel.GlobalHandle()
	return runWithHandle(lib, fn, o)
}

// runWithHandle is the libhegel-driving core of [runProperty], split out so
// tests can inject a stub *libhegel and exercise the engine setup/teardown
// error paths (run_start / run_result failures) without the real library.
func runWithHandle(lib *libhegel.Handle, fn testBody, opts runOptions) error {
	s := buildSettings(lib, opts)

	run, err := s.RunStart()
	if err != nil {
		return err
	}

	// Drive the engine until next_test_case returns NULL. The C ABI overloads
	// NULL to mean either "run finished" or "engine failed mid-run", with
	// hegel_last_error_message distinguishing the two. We deliberately do NOT
	// read last_error here to disambiguate: it is OS-thread-local, and this
	// loop runs on a goroutine the Go runtime may migrate between OS threads,
	// so a NULL from a normal completion can observe a stale error string left
	// by an unrelated concurrent run on the same OS thread — a spurious
	// failure. (hegel-java can check it safely because it uses 1:1 OS threads.)
	// A genuine mid-run engine failure is still surfaced: hegel_run_result
	// below returns NULL (handled) or a result whose failure list carries the
	// engine's diagnostic, which collectFailures reports.
	// In single-test-case mode libhegel returns NULL after one case.
	for {
		tc, err := run.NextTestCase()
		if err != nil {
			return err
		}
		if tc == nil {
			break
		}
		// TODO: Omit singleTestCase?
		driveOneCase(tc, fn, opts.singleTestCase, opts.output)
	}

	result, err := run.RunResult()
	if err != nil {
		return err
	}

	switch result.Status() {
	case libhegel.RUN_STATUS_PASSED:
		return nil
	case libhegel.RUN_STATUS_ERROR:
		// The run itself failed (a health check, a nondeterministic test, an
		// engine panic) and produced no verdict on the property. There are no
		// counterexamples to collect; the diagnostic lives in the run-level
		// error message.
		return fmt.Errorf("%w: %s", errPropTestFailed, result.ErrorMessage())
	default:
		return collectFailures(result)
	}
}

func buildSettings(lib *libhegel.Handle, opts runOptions) *libhegel.Settings {
	s := lib.SettingsNew()

	if opts.testCases > 0 {
		s.TestCases(uint64(opts.testCases))
	}
	s.Derandomize(opts.derandomize)

	if opts.seed != nil {
		s.Seed(uint64(*opts.seed), true)
	}

	if opts.singleTestCase {
		s.Mode(libhegel.MODE_SINGLE_TEST_CASE)
	}

	switch opts.database.state {
	case databaseUnset:
		// Don't call: libhegel uses its default.
	case databaseDisabled:
		// Empty string disables the database.
		s.Database("")
	case databasePath:
		s.Database(opts.database.path)
	default: // coverage-ignore (databaseState enum is closed)
		panic(fmt.Sprintf("unknown database state %d", opts.database.state))
	}

	if opts.database.state != databaseDisabled && opts.databaseKey != "" {
		s.DatabaseKey(opts.databaseKey)
	}

	if len(opts.suppressHealthCheck) > 0 {
		var mask HealthCheck
		for _, hc := range opts.suppressHealthCheck {
			mask |= hc
		}
		s.SuppressHealthCheck(mask)
	}

	return s
}

// driveOneCase runs one test case received from libhegel: invokes fn against a
// fresh [testCase], recovers panics into a status (VALID/INVALID/OVERRUN/
// INTERESTING) and an optional origin, and calls hegel_mark_complete.
// Failures themselves are surfaced via hegel_run_result; the caller doesn't
// need to inspect anything per-case.
func driveOneCase(tc *libhegel.TestCase, fn testBody, single bool, baseOutput io.Writer) {
	var caseOut io.Writer
	var skipUserPanic bool
	if tc.IsFinalReplay() || single {
		caseOut = baseOutput

		// Do not recover any pending panics on the final replay or when in
		// single testcase mode.
		// This ensures that debuggers can see the panics.
		skipUserPanic = true
	}

	state := &testCase{
		tc:             tc,
		singleTestCase: single,
		out:            caseOut,
	}

	func() {
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
	}()

	tc.MarkComplete(state.status, state.origin)
}

// collectFailures walks the failure list from a finished run and joins the
// failures into a single error. It is called only for a RUN_STATUS_FAILED run
// (the property has counterexamples); health-check and other run-level errors
// surface as RUN_STATUS_ERROR and are handled by the caller.
//
// panicByOrigin supplies the dynamic panic message captured by driveOneCase
// for each origin libhegel reports. The origin string itself is stable per
// call site (so libhegel can group failures for shrinking); the panic value
// is appended to the error message so users still see what the test
// panicked with.
func collectFailures(result *libhegel.Result) error {
	n := result.FailureCount()
	if n == 0 { // coverage-ignore (RUN_STATUS_FAILED implies n>=1)
		return errPropTestFailed
	}
	var errs []error
	for i := uint64(0); i < n; i++ {
		fail, err := result.Failure(i)
		if err != nil {
			return err
		}
		// TODO: Include diagnostic / panic_message once they are wired up.
		errs = append(errs, fmt.Errorf("%w: %s", errPropTestFailed, fail.Origin()))
	}
	return errors.Join(errs...)
}

// findExternalCaller describes a recovered panic for [hegel_mark_complete]'s
// origin field. The format is "<file>:<line>", where the file/line
// come from the first non-hegel frame in the stack — i.e. the user's test
// code, not internal helpers.
//
// The origin MUST be stable across every call site of the same panic so
// libhegel can group failing inputs together for shrinking.
func findExternalCaller() string {
	var pcs [32]uintptr
	n := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	file := ""
	line := 0
	for {
		f, more := frames.Next()
		if !more { // coverage-ignore
			break
		}
		if !isHegelFrame(f.Function) {
			file = f.File
			line = f.Line
			break
		}
	}
	return fmt.Sprintf("%s:%d", file, line)
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
