package hegel

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// testCase holds the per-test-case context.
//
// It is compatible with most popular TestingT interfaces from assert libraries.
type testCase struct {
	stream         *stream
	singleTestCase bool // set when this case runs under WithSingleTestCase
	aborted        bool
	failed         bool         // for T.Error/Fail deferred INTERESTING
	noteFn         func(string) // nil for exploratory cases; t.Log/stdout for final replay / single-case
}

// --- Sentinel errors ---

// assumeRejected is raised by Assume(false) to reject a test case.
type assumeRejected struct{}

func (assumeRejected) Error() string { return "assume rejected" }

// dataExhausted is raised when the server sends StopTest.
type dataExhausted struct{ msg string }

func (e *dataExhausted) Error() string { return e.msg }

// flakyAbort is raised when the server detects non-deterministic data generation.
// Like dataExhausted, it aborts the test case without sending mark_complete.
// The server reports the actual flaky error in test_done results.
type flakyAbort struct{}

func (flakyAbort) Error() string { return "flaky test detected" }

// errTestCaseAborted is returned by doRequest when the test case has already
// been aborted by a prior StopTest or flaky response. Deferred or cleanup
// callers can ignore it; everyone else should propagate it normally and rely
// on the original abort sentinel having already unwound the test body.
var errTestCaseAborted = errors.New("test case aborted")

// Wire protocol error types for flaky test detection.
const (
	flakyStrategyDefinition = "FlakyStrategyDefinition"
	flakyReplay             = "FlakyReplay"
)

func (s *testCase) Assume(condition bool) {
	if !condition {
		panic(assumeRejected{})
	}
}

func (s *testCase) Note(message string) {
	if s.noteFn != nil {
		s.noteFn(message)
	}
}

func (s *testCase) Errorf(format string, args ...any) {
	s.Note(fmt.Sprintf(format, args...))
	s.failed = true
}

func (s *testCase) Fail() {
	s.failed = true
}

func (s *testCase) FailNow() {
	s.failed = true
	panic(fatalSentinel{msg: "FailNow called"})
}

func (s *testCase) Log(args ...any) {
	s.Note(fmt.Sprint(args...))
}

func (s *testCase) Target(value float64, label string) {
	if _, err := s.doRequest(map[string]any{
		"command": "target",
		"value":   value,
		"label":   label,
	}); err != nil {
		panic(err)
	}
}

// --- Internal helpers ---

// toInt64 converts a CBOR integer (int64 or uint64) to int64.
func toInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case uint64:
		return int64(x), true
	default:
		return 0, false
	}
}

// doRequest encodes msg as CBOR, sends it on s.stream, and returns the decoded
// reply. Server-side abort signals are translated to sentinel errors: StopTest
// becomes [*dataExhausted], FlakyStrategyDefinition / FlakyReplay become
// [flakyAbort]. Both also set s.aborted, after which further calls short-
// circuit with [errTestCaseAborted].
//
// Connection-level errors from the underlying stream are returned as
// [*connectionError].
//
// Panics if msg cannot be CBOR-encoded — unreachable for the fixed-shape maps
// constructed inside this package.
func (s *testCase) doRequest(msg map[string]any) (any, error) {
	if s.aborted {
		return nil, errTestCaseAborted
	}
	payload, err := encodeCBOR(msg)
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("doRequest encode: %v", err))
	}
	pending, err := s.stream.Request(payload)
	if err != nil {
		if _, ok := err.(*connectionError); ok {
			return nil, err
		}
		return nil, &connectionError{msg: err.Error()}
	}
	v, err := pending.Get()
	if err == nil {
		return v, nil
	}
	if re, ok := err.(*requestError); ok {
		switch re.ErrorType {
		case "StopTest":
			s.aborted = true
			return nil, &dataExhausted{msg: "server ran out of data"}
		case flakyStrategyDefinition, flakyReplay: // coverage-ignore
			s.aborted = true
			return nil, flakyAbort{}
		}
	}
	return nil, err
}

// generateFromSchema sends a generate command for schema and returns the
// decoded value.
func (s *testCase) generateFromSchema(schema map[string]any) (any, error) {
	return s.doRequest(map[string]any{"command": "generate", "schema": schema})
}

func (s *testCase) isSingleTestCase() bool {
	return s.singleTestCase
}

// fatalSentinel is panic'd by T.Fatal/Fatalf/FailNow to mark a test case as INTERESTING.
type fatalSentinel struct{ msg string }

func (f fatalSentinel) Error() string { return f.msg }

// testBody is the internal representation of a test function.
// It receives the [TestCase] for the current test case.
type testBody func(TestCase)

// --- Health checks ---

// HealthCheck represents a health check that can be suppressed during test execution.
//
// Health checks detect common issues with test configuration that would
// otherwise cause tests to run inefficiently or not at all.
type HealthCheck int

const (
	// FilterTooMuch indicates too many test cases are being filtered out via [TestCase.Assume].
	FilterTooMuch HealthCheck = iota
	// TooSlow indicates test execution is too slow.
	TooSlow
	// TestCasesTooLarge indicates generated test cases are too large.
	TestCasesTooLarge
	// LargeInitialTestCase indicates the smallest natural input is very large.
	LargeInitialTestCase
)

// AllHealthChecks returns all health check variants.
func AllHealthChecks() []HealthCheck {
	return []HealthCheck{FilterTooMuch, TooSlow, TestCasesTooLarge, LargeInitialTestCase}
}

// String returns the wire protocol name for this health check.
func (h HealthCheck) String() string {
	switch h {
	case FilterTooMuch:
		return "filter_too_much"
	case TooSlow:
		return "too_slow"
	case TestCasesTooLarge:
		return "test_cases_too_large"
	case LargeInitialTestCase:
		return "large_initial_test_case"
	default: // coverage-ignore
		panic("unreachable: unknown health check")
	}
}

// --- Test runner options ---

// databaseState distinguishes between "user has not set the database" (default
// server behavior applies), "user disabled the database", and "user supplied a
// path". The first state corresponds to omitting the field on the wire; the
// other two send null and a string respectively.
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
// given directory. Failing test cases are saved there and replayed on
// subsequent runs of the same test.
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
	// [Test] from t.Name(); nil for [Run]/[MustRun] (no test name available).
	databaseKey []byte
}

// Option is a functional option for Test and Run.
type Option func(*runOptions)

// WithTestCases sets the number of test cases to run.
func WithTestCases(n int) Option {
	return func(o *runOptions) { o.testCases = n }
}

// SuppressHealthCheck suppresses the given health checks so they do not cause test failure.
//
// Health checks detect common issues like excessive filtering or slow tests.
// Use this to suppress specific checks when they are expected.
func SuppressHealthCheck(checks ...HealthCheck) Option {
	return func(o *runOptions) { o.suppressHealthCheck = append(o.suppressHealthCheck, checks...) }
}

// WithDatabase configures example-database persistence for this test.
// Construct the setting with [Database] (a path) or [DatabaseDisabled].
//
// The default behavior (when WithDatabase is not specified) is to use the
// server's default database location, except in CI environments where the
// database is automatically disabled.
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
// not safely re-runnable on the same inputs — for example, code with external
// side effects, time-dependent behavior, or execution under Antithesis where
// the surrounding state evolves between iterations.
//
// In this mode [WithTestCases], [WithDatabase], [WithDerandomize], and
// [SuppressHealthCheck] are ignored. [RunStateful] loops indefinitely (until
// a rule or invariant fails, or the process is terminated externally) rather
// than capping at the usual step count.
func WithSingleTestCase() Option {
	return func(o *runOptions) { o.singleTestCase = true }
}

// withDatabaseKey sets the example-database key. Unexported: only [Test]
// supplies a key, deriving it from t.Name(). User-facing code controls the
// database via [WithDatabase] only.
func withDatabaseKey(key []byte) Option {
	return func(o *runOptions) { o.databaseKey = key }
}

// ciEnvVar describes a single CI-detection env var. If matchAny is true, the
// var counts as a CI signal whenever it is present in the environment, even
// with an empty value. Otherwise it must equal expected exactly.
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
		if v.matchAny || val == v.expected { // coverage-ignore
			return true
		}
	}
	return false
}

// Run runs a property test and returns any error.
//
// Note output goes to stdout. For use in standalone binaries and conformance tests.
func Run(fn func(TestCase), opts ...Option) error {
	return runHegel(fn, stdoutNoteFn, opts)
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
	allOpts := append(opts, withDatabaseKey([]byte(t.Name())))
	err := runHegel(body, func(msg string) { t.Helper(); t.Log(msg) }, allOpts) // coverage-ignore
	if err != nil {                                                             // coverage-ignore
		t.Fatal(err)
	}
}

// stdoutNoteFn is the noteFn for Run/MustRun: writes to stdout.
func stdoutNoteFn(msg string) {
	fmt.Println(msg)
}

// runHegel is the shared implementation for Run, MustRun, and Test.
//
// The example-database key is supplied (when applicable) by [Test];
// non-test entry points leave it nil.
func runHegel(fn testBody, noteFn func(string), opts []Option) error {
	inCI := isCI()
	o := runOptions{
		testCases:   100,
		derandomize: inCI,
	}
	if inCI {
		o.database = DatabaseSetting{state: databaseDisabled}
	}
	for _, opt := range opts {
		opt(&o)
	}

	// If HEGEL_PROTOCOL_TEST_MODE is set, use a temporary single-use session
	// so the test server gets a fresh subprocess with the right env var.
	// The test-mode server intentionally crashes for error injection, so suppress its stderr.
	if os.Getenv("HEGEL_PROTOCOL_TEST_MODE") != "" {
		s := newHegelSession()
		s.suppressStderr = true
		defer s.cleanup()
		if err := s.start(); err != nil {
			return fmt.Errorf("session start: %w", err)
		}
		return s.runTest(fn, o, noteFn)
	}

	if err := globalSession.start(); err != nil {
		return fmt.Errorf("session start: %w", err)
	}
	return globalSession.runTest(fn, o, noteFn)
}

// extractPanicOrigin extracts file/line from a recovered panic using runtime.Callers,
// skipping internal hegel frames to find the user's test code.
func extractPanicOrigin(v any) string {
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
		// Skip internal hegel frames.
		if !isHegelFrame(f.Function) {
			file = f.File
			line = f.Line
			break
		}
	}
	return fmt.Sprintf("%T at %s:%d", v, file, line)
}

func isHegelFrame(fn string) bool {
	return strings.HasPrefix(fn, "hegel.dev/go/hegel")
}

// --- Client: manages a single connection's test lifecycle ---

// client wraps a connection for running property tests.
// Multiple goroutines may call runTest concurrently — each call creates its
// own test stream and the underlying connection multiplexes via streams.
type client struct {
	conn *connection
}

func newClient(conn *connection) *client {
	return &client{conn: conn}
}

func buildSingleTestCaseMessage(streamID uint32) map[string]any {
	return map[string]any{
		"command":   "single_test_case",
		"stream_id": int64(streamID),
	}
}

func buildRunTestMessage(streamID uint32, opts runOptions) map[string]any {
	msg := map[string]any{
		"command":     "run_test",
		"test_cases":  int64(opts.testCases),
		"stream_id":   int64(streamID),
		"derandomize": opts.derandomize,
	}
	if opts.seed != nil {
		msg["seed"] = *opts.seed
	}
	if opts.database.state == databaseDisabled || opts.databaseKey == nil {
		msg["database_key"] = nil
	} else {
		msg["database_key"] = opts.databaseKey
	}
	switch opts.database.state {
	case databaseDisabled:
		msg["database"] = nil
	case databasePath:
		msg["database"] = opts.database.path
	}
	if len(opts.suppressHealthCheck) > 0 {
		names := make([]string, len(opts.suppressHealthCheck))
		for i, hc := range opts.suppressHealthCheck {
			names[i] = hc.String()
		}
		msg["suppress_health_check"] = names
	}
	return msg
}

// runTest executes one property test against the server.
//
// In single-test-case mode (opts.singleTestCase) the server runs exactly one
// case via the single_test_case command — no shrinking, replay, or example
// database. Otherwise it sends run_test and replays any interesting failures
// after the exploratory phase.
func (c *client) runTest(fn testBody, opts runOptions, noteFn func(string)) error {
	testSt := c.conn.NewStream("Test")

	cmdName := "run_test"
	var cmdMsg map[string]any
	if opts.singleTestCase {
		cmdName = "single_test_case"
		cmdMsg = buildSingleTestCaseMessage(testSt.StreamID())
	} else {
		cmdMsg = buildRunTestMessage(testSt.StreamID(), opts)
	}
	payload, err := encodeCBOR(cmdMsg)
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("runTest encode: %v", err))
	}

	if _, err := c.conn.SendControlRequest(payload); err != nil {
		return fmt.Errorf("%s send: %w", cmdName, err)
	}

	// Event loop.
	var resultData map[any]any
	var lastErr error
testCase:
	for {
		msgID, raw, err := testSt.RecvRequestRaw(30 * time.Second)
		// covered by TempGoProject
		if err != nil { // coverage-ignore
			return fmt.Errorf("test event recv: %w", err)
		}
		decoded, err := decodeCBOR(raw)
		if err != nil { // coverage-ignore
			return fmt.Errorf("test event decode: %w", err)
		}
		msg, ok := decoded.(map[any]any)
		if !ok { // coverage-ignore
			return fmt.Errorf("test event not a dict")
		}
		event, _ := msg[any("event")].(string)

		switch event {
		case "test_case":
			stIDVal := msg[any("stream_id")]
			stID, ok := toInt64(stIDVal)
			if !ok { // coverage-ignore
				return fmt.Errorf("test_case missing stream_id")
			}
			testSt.SendReplyValue(msgID, nil) //nolint:errcheck
			caseSt, err := c.conn.ConnectStream(uint32(stID), "TestCase")
			if err != nil { // coverage-ignore
				return fmt.Errorf("connect test case stream: %w", err)
			}

			var caseNoteFn func(string)
			if opts.singleTestCase {
				caseNoteFn = noteFn
			}

			lastErr = c.runTestCase(caseSt, fn, opts.singleTestCase, caseNoteFn)
			if lastErr != nil && !errors.Is(lastErr, errPropTestFailed) {
				return lastErr
			}

		case "test_done":
			testSt.SendReplyValue(msgID, true) //nolint:errcheck
			resultsVal := msg[any("results")]
			resultData, _ = resultsVal.(map[any]any)
			break testCase

		default: // coverage-ignore
			return fmt.Errorf("unrecognised event %q", event)
		}
	}

	if resultData == nil { // coverage-ignore
		panic("resultData is nil after test_done")
	}

	// Check for server-side error.
	if errMsg, ok := resultData[any("error")].(string); ok && errMsg != "" { // coverage-ignore
		return fmt.Errorf("server error: %s", errMsg)
	}

	// Check for health check failure.
	// covered by TempGoProject
	if hcMsg, ok := resultData[any("health_check_failure")].(string); ok && hcMsg != "" { // coverage-ignore
		return fmt.Errorf("health check failure:\n%s", hcMsg)
	}

	// Check for flaky test detection.
	if flakyMsg, ok := resultData[any("flaky")].(string); ok && flakyMsg != "" {
		return fmt.Errorf("flaky test detected: %s", flakyMsg)
	}

	if opts.singleTestCase {
		return lastErr
	}

	// Replay interesting (failing) test cases.
	nInterestingVal := resultData[any("interesting_test_cases")]
	nInteresting, _ := toInt64(nInterestingVal)
	var errs []error
	for i := range nInteresting {
		msgID, raw, err := testSt.RecvRequestRaw(30 * time.Second)
		if err != nil { // coverage-ignore
			return fmt.Errorf("final case recv: %w", err)
		}
		decoded, _ := decodeCBOR(raw)
		msg, _ := decoded.(map[any]any)
		stIDVal := msg[any("stream_id")]
		stID, _ := toInt64(stIDVal)
		testSt.SendReplyValue(msgID, nil) //nolint:errcheck
		caseSt, err := c.conn.ConnectStream(uint32(stID), fmt.Sprintf("FinalCase%d", i))
		if err != nil { // coverage-ignore
			return fmt.Errorf("connect final case stream: %w", err)
		}
		caseErr := c.runTestCase(caseSt, fn, false, noteFn)
		if caseErr != nil {
			errs = append(errs, caseErr)
		}
	}
	return errors.Join(errs...) // coverage-ignore
}

var errPropTestFailed = errors.New("property test failed")

// runTestCase executes one test case and sends mark_complete to the server.
//
// Returns errPropTestFailed if the test case caused an abort.
func (c *client) runTestCase(st *stream, fn testBody, singleTestCase bool, noteFn func(string)) error {
	state := &testCase{
		stream:         st,
		singleTestCase: singleTestCase,
		aborted:        false,
		noteFn:         noteFn,
	}

	sendComplete := true
	status := "VALID"
	origin := ""

	err := func() (finalErr error) {
		defer func() {
			r := recover()
			if r == nil {
				// Normal return: check the failed flag.
				if state.failed {
					status = "INTERESTING"
					origin = "test failed (via t.Error/t.Fail)"
					finalErr = fmt.Errorf("%w: test failed", errPropTestFailed)
				}
				return
			}
			switch v := r.(type) {
			case assumeRejected:
				status = "INVALID"
			case *dataExhausted:
				sendComplete = false
			case flakyAbort:
				sendComplete = false
			case *connectionError:
				// Connection is dead — sending mark_complete would just error.
				sendComplete = false
				finalErr = v
			case fatalSentinel:
				status = "INTERESTING"
				origin = extractPanicOrigin(v)
				finalErr = fmt.Errorf("%w: %s", errPropTestFailed, v.msg)
			default:
				status = "INTERESTING"
				origin = extractPanicOrigin(v)
				finalErr = fmt.Errorf("%w: %v", errPropTestFailed, v)
			}
		}()
		fn(state)
		return
	}()

	if sendComplete {
		var originVal any
		if origin != "" {
			originVal = origin
		}
		_, _ = state.doRequest(map[string]any{
			"command": "mark_complete",
			"status":  status,
			"origin":  originVal,
		})
	}
	st.Close()
	return err
}

// --- Session: manages the hegel subprocess ---

// hegelSession manages a shared hegel subprocess for the entire test suite.
// Concurrent Run() calls multiplex over a single connection via streams.
type hegelSession struct {
	mu             sync.Mutex
	conn           *connection
	cli            *client
	process        *exec.Cmd
	hegelCmd       string // overridable for testing
	suppressStderr bool   // suppress hegel subprocess stderr (used for test-mode sessions that intentionally crash)
	logFile        *os.File
	processExited  <-chan struct{} // closed when the server process exits
}

// openServerLog opens .hegel/server.{pid}.log in the project root for appending server output.
// Each process gets its own file to avoid interleaved writes from concurrent processes.
func openServerLog() *os.File {
	hegelDir := filepath.Join(getProjectRoot(), ".hegel")
	os.MkdirAll(hegelDir, 0o755) //nolint:errcheck
	logPath := filepath.Join(hegelDir, fmt.Sprintf("server.%d.log", os.Getpid()))
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("unreachable: failed to open server log: %v", err))
	}
	return f
}

// serverCrashMessageForLog returns an error message for an unexpected server exit.
// logPath is captured at setup time so this is safe to call without holding the session mutex.
func serverCrashMessageForLog(logPath string) string {
	const base = "The hegel server process exited unexpectedly."
	excerpt := serverLogExcerpt(logPath)
	if excerpt != "" {
		return base + "\n\nLast server log entries:\n" + excerpt
	}
	if logPath != "" {
		return base + "\n\n(No entries found in " + logPath + ")"
	}
	return base
}

// serverLogExcerpt reads the server log file and returns a formatted excerpt,
// or "" if the file is empty or unreadable.
func serverLogExcerpt(logPath string) string {
	if logPath == "" {
		return ""
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}
	text := string(content)
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return formatLogExcerpt(text)
}

func newHegelSession() *hegelSession {
	return &hegelSession{}
}

// serverHasExited returns true if the server process has exited.
func (s *hegelSession) serverHasExited() bool {
	if s.processExited == nil {
		return false
	}
	select {
	case <-s.processExited:
		return true
	default:
		return false
	}
}

// start starts the hegel subprocess and connects via stdio.
// If the server has exited (crash or explicit kill), it cleans up the old
// session and starts a fresh one.
func (s *hegelSession) start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil && !s.serverHasExited() {
		return nil
	}

	// Clean up stale session if the server died.
	if s.conn != nil {
		s.cleanupLocked()
	}

	// Build the hegel command.
	var cmd *exec.Cmd
	if s.hegelCmd != "" {
		cmd = exec.Command(s.hegelCmd, "--verbosity", "normal")
	} else {
		var err error
		cmd, err = hegelCommand()
		if err != nil {
			return err
		}
	}
	cmd.Dir = getProjectRoot()
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")
	logFile := openServerLog()
	if s.suppressStderr {
		cmd.Stderr = io.Discard
	} else {
		cmd.Stderr = logFile
	}
	s.logFile = logFile

	stdinPipe, err := cmd.StdinPipe()
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("unreachable: stdin pipe: %v", err))
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("unreachable: stdout pipe: %v", err))
	}

	if err := cmd.Start(); err != nil {
		stdinPipe.Close()
		stdoutPipe.Close()
		if logFile != nil {
			logFile.Close()
		}
		s.logFile = nil
		return fmt.Errorf("spawn: %w", err)
	}
	s.process = cmd

	// Start a goroutine to wait for the process to exit.
	// It captures the crash message (including log excerpt) before signaling,
	// so readers after <-processExited see the message without races.
	processExited := make(chan struct{})
	conn := newConnection(stdoutPipe, stdinPipe, "Client")
	conn.processExited = processExited
	logPath := logFile.Name()
	go func() {
		cmd.Wait() //nolint:errcheck
		conn.crashMessage = serverCrashMessageForLog(logPath)
		close(processExited)
	}()
	version, err := conn.SendHandshakeVersion()
	if err != nil {
		conn.Close()
		cmd.Process.Kill() //nolint:errcheck
		<-processExited
		s.process = nil
		if s.logFile != nil {
			s.logFile.Close()
			s.logFile = nil
		}
		return fmt.Errorf("handshake: %w", err)
	}
	_ = version // we accept any version for now

	s.conn = conn
	s.cli = newClient(conn)
	s.processExited = processExited

	return nil
}

// cleanup terminates the hegel subprocess and cleans up resources.
func (s *hegelSession) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
}

// cleanupLocked is cleanup without acquiring the mutex. Caller must hold s.mu.
func (s *hegelSession) cleanupLocked() {
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
		s.cli = nil
	}

	if s.process != nil {
		s.process.Process.Kill() //nolint:errcheck
		if s.processExited != nil {
			<-s.processExited
		}
		s.process = nil
		s.processExited = nil
	}

	if s.logFile != nil {
		s.logFile.Close() //nolint:errcheck
		s.logFile = nil
	}
}

// runTest runs a test via the session's client.
func (s *hegelSession) runTest(fn testBody, opts runOptions, noteFn func(string)) error {
	return s.cli.runTest(fn, opts, noteFn)
}

// globalSession is the package-level session, lazily started.
var globalSession = newHegelSession()

// testKillServer kills the hegel server process and waits until the connection
// detects that it has exited. Only for use in tests.
func testKillServer() {
	globalSession.mu.Lock()
	if globalSession.process == nil {
		globalSession.mu.Unlock()
		return
	}
	pid := globalSession.process.Process.Pid
	processExited := globalSession.processExited
	globalSession.mu.Unlock()

	syscall.Kill(pid, syscall.SIGTERM) //nolint:errcheck

	// Wait for the process to actually exit.
	if processExited != nil {
		<-processExited
	}
}
