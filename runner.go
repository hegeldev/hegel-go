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
	"time"
)

// --- Context-local state (goroutine-local via sync.Map keyed by goroutine ID) ---
// Go has no goroutine-local storage, so we use a sync.Map keyed by goroutine ID.
// We obtain the goroutine ID by reading the stack trace prefix.

// goroutineID returns the current goroutine's numeric ID.
func goroutineID() int64 {
	var buf [32]byte
	n := runtime.Stack(buf[:], false)
	// Stack output starts with "goroutine N [..."
	var id int64
	fmt.Sscanf(string(buf[:n]), "goroutine %d ", &id)
	return id
}

// goroutineState holds the per-goroutine test context.
type goroutineState struct {
	channel *Channel
	isFinal bool
	aborted bool
}

var goroutineStates sync.Map // map[int64]*goroutineState

func getState() *goroutineState {
	id := goroutineID()
	if v, ok := goroutineStates.Load(id); ok {
		return v.(*goroutineState)
	}
	return nil
}

func setState(s *goroutineState) {
	id := goroutineID()
	if s == nil {
		goroutineStates.Delete(id)
	} else {
		goroutineStates.Store(id, s)
	}
}

// getCurrentIsFinal returns true if the current test case is a final (replay) run.
// Returns false if not in a test context.
func getCurrentIsFinal() bool {
	s := getState()
	if s == nil {
		return false
	}
	return s.isFinal
}

func getCurrentChannel() *Channel {
	s := getState()
	if s == nil {
		return nil
	}
	return s.channel
}

func getChannel() *Channel {
	ch := getCurrentChannel()
	if ch == nil {
		panic("hegel: not in a test context — must be called from within a test function")
	}
	return ch
}

func setAborted() {
	s := getState()
	if s != nil {
		s.aborted = true
	}
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

// --- Public test control functions ---

func generateFromSchema(schema map[string]any) (any, error) {
	ch := getChannel()
	payload, err := EncodeCBOR(map[string]any{"command": "generate", "schema": schema})
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: generateFromSchema encode: %v", err))
	}
	pending, err := ch.Request(payload)
	if err != nil {
		return nil, &connectionError{msg: err.Error()}
	}
	v, err := pending.Get()
	if err != nil {
		re, ok := err.(*RequestError)
		if ok && re.ErrorType == "StopTest" {
			setAborted()
			return nil, &dataExhausted{msg: "server ran out of data"}
		}
		return nil, err
	}
	return v, nil
}

// Assume rejects the current test case if condition is false.
// Must be called from within a test body passed to RunHegelTest.
func Assume(condition bool) {
	if !condition {
		panic(assumeRejected{})
	}
}

// Note prints message to stderr, but only during the final (replay) test case.
// Safe to call outside a test context (no-op).
func Note(message string) {
	if getCurrentIsFinal() {
		fmt.Fprintln(os.Stderr, message)
	}
}

// Target sends a target value to the Hegel server to guide test generation.
// Must be called from within a test body passed to RunHegelTest.
func Target(value float64, label string) {
	ch := getChannel()
	payload, err := EncodeCBOR(map[string]any{
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

// --- Test runner options ---

// runOptions holds options for RunHegelTest.
type runOptions struct {
	testCases int
}

// Option is a functional option for RunHegelTest.
type Option func(*runOptions)

// WithTestCases sets the number of test cases to run.
func WithTestCases(n int) Option {
	return func(o *runOptions) { o.testCases = n }
}

// --- Public API ---

// RunHegelTest runs a property test against the Hegel server.
// It panics if the test fails (for use with Go's testing.T).
// The default number of test cases is 100; override with [WithTestCases].
func RunHegelTest(name string, fn func(), opts ...Option) {
	if err := RunHegelTestE(name, fn, opts...); err != nil {
		panic(err)
	}
}

// RunHegelTestE runs a property test and returns any error instead of panicking.
func RunHegelTestE(name string, fn func(), opts ...Option) error {
	// Check for nested call.
	if getCurrentChannel() != nil {
		return fmt.Errorf("hegel: nested RunHegelTest call — cannot run %q inside a test body", name)
	}

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
		return s.runTest(name, fn, o)
	}

	if err := globalSession.start(); err != nil {
		return fmt.Errorf("hegel: session start: %w", err)
	}
	return globalSession.runTest(name, fn, o)
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

// client wraps a Connection for running property tests.
type client struct {
	conn *Connection
	mu   sync.Mutex // serializes runTest calls
}

func newClient(conn *Connection) *client {
	return &client{conn: conn}
}

// runTest executes one property test against the server.
func (c *client) runTest(name string, fn func(), opts runOptions) error {
	// Serialize the entire test run — the control channel and connection
	// are not thread-safe for concurrent access across goroutines.
	c.mu.Lock()
	defer c.mu.Unlock()

	testCh := c.conn.NewChannel("Test")

	payload, err := EncodeCBOR(map[string]any{
		"command":    "run_test",
		"name":       name,
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
		decoded, err := DecodeCBOR(raw)
		if err != nil {
			return fmt.Errorf("hegel: test event decode: %w", err)
		}
		msg, err := ExtractDict(decoded)
		if err != nil {
			return fmt.Errorf("hegel: test event not a dict: %w", err)
		}
		eventVal := msg[any("event")]
		event, _ := ExtractString(eventVal)

		switch event {
		case "test_case":
			chIDVal := msg[any("channel_id")]
			chID, err := ExtractInt(chIDVal)
			if err != nil {
				return fmt.Errorf("hegel: test_case missing channel_id: %w", err)
			}
			testCh.SendReplyValue(msgID, nil) //nolint:errcheck
			caseCh, err := c.conn.ConnectChannel(uint32(chID), "TestCase")
			if err != nil {
				return fmt.Errorf("hegel: connect test case channel: %w", err)
			}
			if err := c.runTestCase(caseCh, fn, false); err != nil {
				return err
			}

		case "test_done":
			testCh.SendReplyValue(msgID, true) //nolint:errcheck
			resultsVal := msg[any("results")]
			resultData, _ = ExtractDict(resultsVal)
			goto doneLoop

		default:
			// Unknown event: send error reply.
			errPayload, _ := EncodeCBOR(map[string]any{
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
	nInteresting, _ := ExtractInt(nInterestingVal)
	if nInteresting == 0 {
		return nil
	}

	// Replay interesting (failing) test cases.
	if nInteresting == 1 {
		msgID, raw, err := testCh.RecvRequestRaw(30 * time.Second)
		if err != nil {
			return fmt.Errorf("hegel: final case recv: %w", err)
		}
		decoded, _ := DecodeCBOR(raw)
		msg, _ := ExtractDict(decoded)
		chIDVal := msg[any("channel_id")]
		chID, _ := ExtractInt(chIDVal)
		testCh.SendReplyValue(msgID, nil) //nolint:errcheck
		caseCh, err := c.conn.ConnectChannel(uint32(chID), "FinalCase")
		if err != nil {
			return fmt.Errorf("hegel: connect final case channel: %w", err)
		}
		return c.runTestCase(caseCh, fn, true)
	}

	// Multiple interesting cases.
	var errs []error
	for i := int64(0); i < nInteresting; i++ {
		msgID, raw, err := testCh.RecvRequestRaw(30 * time.Second)
		if err != nil {
			return fmt.Errorf("hegel: final case %d recv: %w", i, err)
		}
		decoded, _ := DecodeCBOR(raw)
		msg, _ := ExtractDict(decoded)
		chIDVal := msg[any("channel_id")]
		chID, _ := ExtractInt(chIDVal)
		testCh.SendReplyValue(msgID, nil) //nolint:errcheck
		caseCh, err := c.conn.ConnectChannel(uint32(chID), fmt.Sprintf("FinalCase%d", i))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		caseErr := c.runTestCase(caseCh, fn, true)
		if caseErr != nil {
			errs = append(errs, caseErr)
		} else {
			errs = append(errs, fmt.Errorf("expected test case %d to fail but it didn't", i))
		}
	}
	return fmt.Errorf("multiple failures: %v", errs)
}

// runTestCase executes one test case and sends mark_complete to the server.
func (c *client) runTestCase(ch *Channel, fn func(), isFinal bool) (finalErr error) {
	state := &goroutineState{
		channel: ch,
		isFinal: isFinal,
		aborted: false,
	}
	setState(state)
	defer setState(nil)

	alreadyComplete := false
	status := "VALID"
	origin := ""

	func() {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			switch v := r.(type) {
			case assumeRejected:
				status = "INVALID"
			case *dataExhausted:
				alreadyComplete = true
			case *connectionError:
				finalErr = fmt.Errorf("%s", v.msg)
			default:
				status = "INTERESTING"
				origin = extractPanicOrigin(v)
				if isFinal {
					finalErr = fmt.Errorf("%v", v)
				}
			}
		}()
		fn()
	}()

	if finalErr != nil {
		// Connection error or re-raised final failure: close channel and return.
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
		encoded, err := EncodeCBOR(markPayload)
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
	conn           *Connection
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

	conn := NewConnection(sock, "SDK")
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
func (s *hegelSession) runTest(name string, fn func(), opts runOptions) error {
	return s.cli.runTest(name, fn, opts)
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
