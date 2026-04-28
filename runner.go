package hegel

import (
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

// TestCase holds the per-test-case context.
type TestCase struct {
	stream  *stream
	isFinal bool
	aborted bool
	failed  bool         // for T.Error/Fail deferred INTERESTING
	noteFn  func(string) // injected: t.Log for Case, stderr for Run
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

// Wire protocol error types for flaky test detection.
const (
	flakyStrategyDefinition = "FlakyStrategyDefinition"
	flakyReplay             = "FlakyReplay"
)

func (s *TestCase) IsFinal() bool { return s.isFinal }

// internal returns the underlying TestCase, satisfying the State interface.
func (s *TestCase) internal() *TestCase { return s }

// Assume rejects the current test case if condition is false.
func (s *TestCase) Assume(condition bool) {
	if !condition {
		panic(assumeRejected{})
	}
}

// Note prints message, but only during the final (replay) test case.
//
// Output is routed to t.Log for [Case], or stderr for [Run].
func (s *TestCase) Note(message string) {
	if s.isFinal && s.noteFn != nil {
		s.noteFn(message)
	}
}

// Target guides Hegel toward values that maximize the given metric.
func (s *TestCase) Target(value float64, label string) {
	payload, err := encodeCBOR(map[string]any{
		"command": "target",
		"value":   value,
		"label":   label,
	})
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("Target encode: %v", err))
	}
	doRequest(s, payload)
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

func generateFromSchema(gs *TestCase, schema map[string]any) (any, error) {
	st := gs.stream
	payload, err := encodeCBOR(map[string]any{"command": "generate", "schema": schema})
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("generateFromSchema encode: %v", err))
	}
	pending, err := st.Request(payload)
	if err != nil {
		// Request returns *connectionError for server crashes.
		// Other write errors are also connection-level.
		if _, ok := err.(*connectionError); ok {
			return nil, err
		}
		return nil, &connectionError{msg: err.Error()}
	}
	v, err := pending.Get()
	if err != nil {
		re, ok := err.(*requestError)
		if ok && re.ErrorType == "StopTest" {
			gs.aborted = true
			return nil, &dataExhausted{msg: "server ran out of data"}
		}
		if ok && (re.ErrorType == flakyStrategyDefinition || re.ErrorType == flakyReplay) {
			gs.aborted = true
			return nil, flakyAbort{}
		}
		return nil, err
	}
	return v, nil
}

// fatalSentinel is panic'd by T.Fatal/Fatalf/FailNow to mark a test case as INTERESTING.
type fatalSentinel struct{ msg string }

func (f fatalSentinel) Error() string { return f.msg }

// testBody is the internal representation of a test function.
// It receives the TestCase for the current test case.
type testBody func(s *TestCase)

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

// runOptions holds options for property tests.
type runOptions struct {
	testCases           int
	suppressHealthCheck []HealthCheck
}

// Option is a functional option for Case and Run.
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

// --- Public API ---

// Run runs a property test and returns any error.
//
// Note output goes to stderr. For use in standalone binaries and conformance tests.
func Run(fn func(*TestCase), opts ...Option) error {
	return runHegel(fn, stderrNoteFn, opts)
}

// MustRun runs a property test and panics if it fails.
func MustRun(fn func(*TestCase), opts ...Option) {
	if err := Run(fn, opts...); err != nil {
		panic(err)
	}
}

// Case returns a test function for use with testing.T.Run.
//
// Note output is routed to t.Log.
func Case(fn func(*T), opts ...Option) func(*testing.T) {
	return func(t *testing.T) {
		t.Helper()

		body := func(s *TestCase) {
			ht := &T{TestCase: s, T: t}
			fn(ht)
		}
		err := runHegel(body, func(msg string) { t.Log(msg) }, opts) // coverage-ignore
		if err != nil {                                              // coverage-ignore
			t.Fatal(err)
		}
	}
}

// stderrNoteFn is the noteFn for Run/MustRun: writes to stderr.
func stderrNoteFn(msg string) {
	fmt.Fprintln(os.Stderr, msg)
}

// runHegel is the shared implementation for Run, MustRun, and Case.
func runHegel(fn testBody, noteFn func(string), opts []Option) error {
	o := runOptions{testCases: 100}
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

// runTest executes one property test against the server.
func (c *client) runTest(fn testBody, opts runOptions, noteFn func(string)) error {
	testSt := c.conn.NewStream("Test")

	runTestMsg := map[string]any{
		"command":    "run_test",
		"test_cases": int64(opts.testCases),
		"stream_id":  int64(testSt.StreamID()),
	}
	if len(opts.suppressHealthCheck) > 0 {
		names := make([]string, len(opts.suppressHealthCheck))
		for i, hc := range opts.suppressHealthCheck {
			names[i] = hc.String()
		}
		runTestMsg["suppress_health_check"] = names
	}
	payload, err := encodeCBOR(runTestMsg)
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("runTest encode: %v", err))
	}

	if _, err := c.conn.SendControlRequest(payload); err != nil {
		return fmt.Errorf("run_test send: %w", err)
	}

	// Event loop.
	var resultData map[any]any
	for {
		msgID, raw, err := testSt.RecvRequestRaw(30 * time.Second)
		if err != nil {
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
			if err := c.runTestCase(caseSt, fn, false, noteFn); err != nil {
				return err
			}

		case "test_done":
			testSt.SendReplyValue(msgID, true) //nolint:errcheck
			resultsVal := msg[any("results")]
			resultData, _ = resultsVal.(map[any]any)
			goto doneLoop

		default: // coverage-ignore
			return fmt.Errorf("unrecognised event %q", event)
		}
	}

doneLoop:
	if resultData == nil { // coverage-ignore
		panic("resultData is nil after test_done")
	}

	// Check for server-side error.
	if errMsg, ok := resultData[any("error")].(string); ok && errMsg != "" { // coverage-ignore
		return fmt.Errorf("server error: %s", errMsg)
	}

	// Check for health check failure.
	if hcMsg, ok := resultData[any("health_check_failure")].(string); ok && hcMsg != "" {
		return fmt.Errorf("health check failure:\n%s", hcMsg)
	}

	// Check for flaky test detection.
	if flakyMsg, ok := resultData[any("flaky")].(string); ok && flakyMsg != "" {
		return fmt.Errorf("flaky test detected: %s", flakyMsg)
	}

	nInterestingVal := resultData[any("interesting_test_cases")]
	nInteresting, _ := toInt64(nInterestingVal)
	if nInteresting == 0 {
		return nil
	}

	// Replay interesting (failing) test cases.
	var errs []error
	for i := int64(0); i < nInteresting; i++ {
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
		caseErr := c.runTestCase(caseSt, fn, true, noteFn)
		if caseErr != nil {
			errs = append(errs, caseErr)
		}
	}
	if len(errs) == 0 { // coverage-ignore
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return fmt.Errorf("multiple failures: %v", errs) // coverage-ignore
}

// runTestCase executes one test case and sends mark_complete to the server.
func (c *client) runTestCase(st *stream, fn testBody, isFinal bool, noteFn func(string)) (finalErr error) {
	state := &TestCase{
		stream:  st,
		isFinal: isFinal,
		aborted: false,
		noteFn:  noteFn,
	}

	alreadyComplete := false
	status := "VALID"
	origin := ""

	func() {
		defer func() {
			r := recover()
			if r == nil {
				// Normal return: check the failed flag.
				if state.failed {
					status = "INTERESTING"
					origin = "test failed (via t.Error/t.Fail)"
					if isFinal {
						finalErr = fmt.Errorf("property test failed: test failed")
					}
				}
				return
			}
			switch v := r.(type) {
			case assumeRejected:
				status = "INVALID"
			case *dataExhausted:
				alreadyComplete = true
			case flakyAbort:
				alreadyComplete = true
			case *connectionError:
				finalErr = fmt.Errorf("%s", v.msg)
			case fatalSentinel:
				status = "INTERESTING"
				origin = extractPanicOrigin(v)
				if isFinal {
					finalErr = fmt.Errorf("property test failed: %s", v.msg)
				}
			default:
				status = "INTERESTING"
				origin = extractPanicOrigin(v)
				if isFinal {
					finalErr = fmt.Errorf("property test failed: %v", v)
				}
			}
		}()
		fn(state)
	}()

	if finalErr != nil {
		// connection error or re-raised final failure: close stream and return.
		st.Close()
		return finalErr
	}

	if !alreadyComplete {
		var markPayload map[string]any
		if origin != "" {
			markPayload = map[string]any{
				"command": "mark_complete",
				"status":  status,
				"origin":  origin,
			}
		} else {
			markPayload = map[string]any{
				"command": "mark_complete",
				"status":  status,
				"origin":  nil,
			}
		}
		encoded, err := encodeCBOR(markPayload)
		if err != nil { // coverage-ignore
			panic(fmt.Sprintf("mark_complete encode: %v", err))
		}
		pending, err := st.Request(encoded)
		if err == nil {
			pending.Get() //nolint:errcheck
		}
	}
	st.Close()
	return nil
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
