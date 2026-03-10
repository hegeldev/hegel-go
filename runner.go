package hegel

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestCase holds the per-test-case context.
type TestCase struct {
	channel *channel
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

// connectionError wraps a connection-level error that should propagate out of the test.
type connectionError struct{ msg string }

func (e *connectionError) Error() string { return e.msg }

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
	ch := s.channel
	payload, err := encodeCBOR(map[string]any{
		"command": "target",
		"value":   value,
		"label":   label,
	})
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: Target encode: %v", err))
	}
	pending, err := ch.Request(payload)
	if err != nil {
		panic(fmt.Sprintf("hegel: Target send: %v", err))
	}
	if _, err := pending.Get(); err != nil {
		panic(fmt.Sprintf("hegel: Target response: %v", err))
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

func generateFromSchema(gs *TestCase, schema map[string]any) (any, error) {
	ch := gs.channel
	payload, err := encodeCBOR(map[string]any{"command": "generate", "schema": schema})
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: generateFromSchema encode: %v", err))
	}
	pending, err := ch.Request(payload)
	if err != nil {
		return nil, &connectionError{msg: err.Error()}
	}
	v, err := pending.Get()
	if err != nil {
		re, ok := err.(*requestError)
		if ok && re.ErrorType == "StopTest" {
			gs.aborted = true
			return nil, &dataExhausted{msg: "server ran out of data"}
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

// --- Test runner options ---

// runOptions holds options for property tests.
type runOptions struct {
	testCases int
}

// Option is a functional option for Case and Run.
type Option func(*runOptions)

// WithTestCases sets the number of test cases to run.
func WithTestCases(n int) Option {
	return func(o *runOptions) { o.testCases = n }
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
		err := runHegel(body, func(msg string) { t.Log(msg) }, opts)
		if err != nil {
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
			return fmt.Errorf("hegel: session start: %w", err)
		}
		return s.runTest(fn, o, noteFn)
	}

	if err := globalSession.start(); err != nil {
		return fmt.Errorf("hegel: session start: %w", err)
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
		if !more {
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
	return strings.HasPrefix(fn, "github.com/antithesishq/hegel-go")
}

// --- Client: manages a single connection's test lifecycle ---

// client wraps a connection for running property tests.
type client struct {
	conn *connection
	mu   sync.Mutex // serializes runTest calls
}

func newClient(conn *connection) *client {
	return &client{conn: conn}
}

// runTest executes one property test against the server.
func (c *client) runTest(fn testBody, opts runOptions, noteFn func(string)) error {
	// Serialize the entire test run — the control channel and connection
	// are not thread-safe for concurrent access across goroutines.
	c.mu.Lock()
	defer c.mu.Unlock()

	testCh := c.conn.NewChannel("Test")

	payload, err := encodeCBOR(map[string]any{
		"command":    "run_test",
		"test_cases": int64(opts.testCases),
		"channel_id": int64(testCh.ChannelID()),
	})
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: runTest encode: %v", err))
	}

	ctrl := c.conn.ControlChannel()
	pending, err := ctrl.Request(payload)
	if err != nil {
		return fmt.Errorf("hegel: run_test send: %w", err)
	}
	if _, err := pending.Get(); err != nil {
		return fmt.Errorf("hegel: run_test ack: %w", err)
	}

	// Event loop.
	var resultData map[any]any
	for {
		msgID, raw, err := testCh.RecvRequestRaw(30 * time.Second)
		if err != nil {
			return fmt.Errorf("hegel: test event recv: %w", err)
		}
		decoded, err := decodeCBOR(raw)
		if err != nil {
			return fmt.Errorf("hegel: test event decode: %w", err)
		}
		msg, ok := decoded.(map[any]any)
		if !ok {
			return fmt.Errorf("hegel: test event not a dict")
		}
		event, _ := msg[any("event")].(string)

		switch event {
		case "test_case":
			chIDVal := msg[any("channel_id")]
			chID, ok := toInt64(chIDVal)
			if !ok {
				return fmt.Errorf("hegel: test_case missing channel_id")
			}
			testCh.SendReplyValue(msgID, nil) //nolint:errcheck
			caseCh, err := c.conn.ConnectChannel(uint32(chID), "TestCase")
			if err != nil {
				return fmt.Errorf("hegel: connect test case channel: %w", err)
			}
			if err := c.runTestCase(caseCh, fn, false, noteFn); err != nil {
				return err
			}

		case "test_done":
			testCh.SendReplyValue(msgID, true) //nolint:errcheck
			resultsVal := msg[any("results")]
			resultData, _ = resultsVal.(map[any]any)
			goto doneLoop

		default:
			// Unknown event: send error reply.
			errPayload, _ := encodeCBOR(map[string]any{
				"error": fmt.Sprintf("unrecognised event %q", event),
				"type":  "InvalidMessage",
			})
			testCh.SendReplyRaw(msgID, errPayload) //nolint:errcheck
		}
	}

doneLoop:
	if resultData == nil {
		panic("hegel: unreachable: resultData is nil after test_done")
	}

	nInterestingVal := resultData[any("interesting_test_cases")]
	nInteresting, _ := toInt64(nInterestingVal)
	if nInteresting == 0 {
		return nil
	}

	// Replay interesting (failing) test cases.
	if nInteresting == 1 {
		msgID, raw, err := testCh.RecvRequestRaw(30 * time.Second)
		if err != nil {
			return fmt.Errorf("hegel: final case recv: %w", err)
		}
		decoded, _ := decodeCBOR(raw)
		msg, _ := decoded.(map[any]any)
		chIDVal := msg[any("channel_id")]
		chID, _ := toInt64(chIDVal)
		testCh.SendReplyValue(msgID, nil) //nolint:errcheck
		caseCh, err := c.conn.ConnectChannel(uint32(chID), "FinalCase")
		if err != nil {
			return fmt.Errorf("hegel: connect final case channel: %w", err)
		}
		return c.runTestCase(caseCh, fn, true, noteFn)
	}

	// Multiple interesting cases.
	var errs []error
	for i := int64(0); i < nInteresting; i++ {
		msgID, raw, err := testCh.RecvRequestRaw(30 * time.Second)
		if err != nil {
			return fmt.Errorf("hegel: final case %d recv: %w", i, err)
		}
		decoded, _ := decodeCBOR(raw)
		msg, _ := decoded.(map[any]any)
		chIDVal := msg[any("channel_id")]
		chID, _ := toInt64(chIDVal)
		testCh.SendReplyValue(msgID, nil) //nolint:errcheck
		caseCh, err := c.conn.ConnectChannel(uint32(chID), fmt.Sprintf("FinalCase%d", i))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		caseErr := c.runTestCase(caseCh, fn, true, noteFn)
		if caseErr != nil {
			errs = append(errs, caseErr)
		} else {
			errs = append(errs, fmt.Errorf("expected test case %d to fail but it didn't", i))
		}
	}
	return fmt.Errorf("multiple failures: %v", errs)
}

// runTestCase executes one test case and sends mark_complete to the server.
func (c *client) runTestCase(ch *channel, fn testBody, isFinal bool, noteFn func(string)) (finalErr error) {
	state := &TestCase{
		channel: ch,
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
					if isFinal {
						finalErr = fmt.Errorf("test failed")
					}
				}
				return
			}
			switch v := r.(type) {
			case assumeRejected:
				status = "INVALID"
			case *dataExhausted:
				alreadyComplete = true
			case *connectionError:
				finalErr = fmt.Errorf("%s", v.msg)
			case fatalSentinel:
				status = "INTERESTING"
				origin = extractPanicOrigin(v)
				if isFinal {
					finalErr = fmt.Errorf("%s", v.msg)
				}
			default:
				status = "INTERESTING"
				origin = extractPanicOrigin(v)
				if isFinal {
					finalErr = fmt.Errorf("%v", v)
				}
			}
		}()
		fn(state)
	}()

	if finalErr != nil {
		// connection error or re-raised final failure: close channel and return.
		ch.Close()
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
		if err != nil {
			panic(fmt.Sprintf("hegel: unreachable: mark_complete encode: %v", err))
		}
		pending, err := ch.Request(encoded)
		if err == nil {
			pending.Get() //nolint:errcheck
		}
	}
	ch.Close()
	return nil
}

// --- Session: manages the hegel subprocess ---

// hegelSession manages a shared hegel subprocess for the entire test suite.
type hegelSession struct {
	mu             sync.Mutex
	conn           *connection
	cli            *client
	process        *exec.Cmd
	tempDir        string
	socketPath     string
	hegelCmd       string // overridable for testing
	suppressStderr bool   // suppress hegel subprocess stderr (used for test-mode sessions that intentionally crash)
}

// mkdirTempFn is the function used to create temp directories.
// Overridable in tests to simulate failures.
var mkdirTempFn = os.MkdirTemp

func newHegelSession() *hegelSession {
	return &hegelSession{}
}

func (s *hegelSession) hasWorkingClient() bool {
	return s.cli != nil && s.conn != nil && s.conn.Live()
}

// start starts the hegel subprocess and connects to it (idempotent).
func (s *hegelSession) start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hasWorkingClient() {
		return nil
	}

	// Find hegel binary.
	hegelBin := s.hegelCmd
	if hegelBin == "" {
		hegelBin = findHegel()
	}

	// Create temp dir for socket.
	tmp, err := mkdirTempFn("", "hegel-")
	if err != nil {
		return fmt.Errorf("hegel: mktemp: %w", err)
	}
	s.tempDir = tmp
	sockPath := filepath.Join(tmp, "hegel.sock")
	s.socketPath = sockPath

	// Spawn hegel process.
	cmd := exec.Command(hegelBin, sockPath)
	cmd.Stdout = os.Stderr
	if s.suppressStderr {
		cmd.Stderr = io.Discard
	} else {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tmp) //nolint:errcheck
		return fmt.Errorf("hegel: spawn: %w", err)
	}
	s.process = cmd

	// Wait for socket to appear and connect.
	var sock net.Conn
	for i := 0; i < 50; i++ {
		if _, statErr := os.Stat(sockPath); statErr == nil {
			c, connErr := net.Dial("unix", sockPath)
			if connErr == nil {
				sock = c
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if sock == nil {
		cmd.Process.Kill() //nolint:errcheck
		os.RemoveAll(tmp)  //nolint:errcheck
		return fmt.Errorf("hegel: timeout waiting for hegel to start")
	}

	conn := newConnection(sock, "SDK")
	version, err := conn.SendHandshakeVersion()
	if err != nil {
		sock.Close()       //nolint:errcheck
		cmd.Process.Kill() //nolint:errcheck
		os.RemoveAll(tmp)  //nolint:errcheck
		return fmt.Errorf("hegel: handshake: %w", err)
	}
	_ = version // we accept any version for now

	s.conn = conn
	s.cli = newClient(conn)

	// Register cleanup on first successful start.
	// (atexit equivalent: use a finalizer or just let the OS clean up on exit.
	// For test suites, this is sufficient.)
	return nil
}

// cleanup terminates the hegel subprocess and cleans up resources.
func (s *hegelSession) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil {
		func() {
			defer func() { recover() }() //nolint:errcheck
			s.conn.Close()
		}()
		s.conn = nil
		s.cli = nil
	}

	if s.process != nil {
		func() {
			defer func() { recover() }()           //nolint:errcheck
			s.process.Process.Signal(os.Interrupt) //nolint:errcheck
			s.process.Wait()                       //nolint:errcheck
		}()
		s.process = nil
	}

	if s.tempDir != "" {
		func() {
			defer func() { recover() }() //nolint:errcheck
			os.RemoveAll(s.tempDir)      //nolint:errcheck
		}()
		s.tempDir = ""
	}
}

// runTest runs a test via the session's client.
func (s *hegelSession) runTest(fn testBody, opts runOptions, noteFn func(string)) error {
	return s.cli.runTest(fn, opts, noteFn)
}

// findHegel locates the hegel binary.
func findHegel() string {
	// Check venv in current working directory.
	cwd, err := os.Getwd()
	if err == nil {
		if p := findHegelInDir(filepath.Join(cwd, ".venv")); p != "" {
			return p
		}
	}
	// Check PATH.
	if p, err := exec.LookPath("hegel"); err == nil {
		return p
	}
	// Fallback.
	return "hegel"
}

// findHegelInDir looks for bin/hegel inside dir.
func findHegelInDir(dir string) string {
	p := filepath.Join(dir, "bin", "hegel")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

// globalSession is the package-level session, lazily started.
var globalSession = newHegelSession()
