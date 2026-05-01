package hegel

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- RunHegelTest: basic passing test ---

func TestRunHegelTestPasses(t *testing.T) {

	called := false
	Test(t, func(ht *T) {
		called = true
		b := Draw[bool](ht, Booleans())
		// A valid assertion: b is either true or false.
		if b != true && b != false {
			t.Errorf("expected bool, got %v", b)
		}
	}, WithTestCases(5))
	if !called {
		t.Error("test function was never called")
	}
}

// --- RunHegelTest: failing test raises error ---

func TestRunHegelTestFails(t *testing.T) {
	t.Parallel()

	newTempGoProject(t).
		testBody(`x := hegel.Draw[int](ht, hegel.Integers[int](0, 100))
if x >= 0 {
	panic(fmt.Sprintf("assertion failed: %d >= 0", x))
}`, "hegel.WithTestCases(10)").
		expectFailure(`assertion failed`).
		goTest()
}

// --- RunHegelTest: assume(false) -> INVALID, test continues ---

func TestRunHegelTestAllInvalid(t *testing.T) {

	// A test that always calls Assume(false) should pass (all cases rejected).
	Test(t, func(ht *T) {
		ht.Assume(false)
	}, WithTestCases(5))
}

// --- RunHegelTest: assume(true) -> no effect ---

func TestAssumeTrue(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		ht.Assume(true)
		b := Draw[bool](ht, Booleans())
		_ = b // use the value
		if b != true && b != false {
			ht.Fatal("expected bool")
		}
	}, WithTestCases(5))
}

// --- note(): not printed when not final ---

func TestNoteNotFinal(t *testing.T) {
	t.Parallel()

	// note() should not panic or error when called outside final run
	Test(t, func(ht *T) {
		ht.Note("should not appear")
		_ = Draw[bool](ht, Booleans())
	}, WithTestCases(3))
}

// --- target(): sends target command ---

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

// --- HEGEL_PROTOCOL_TEST_MODE=stop_test_on_generate ---

func TestStopTestOnGenerate(t *testing.T) {

	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_generate")
	// Should complete without error: client handles StopTest cleanly.
	Test(t, func(ht *T) {
		Draw[bool](ht, Booleans())
	}, WithTestCases(5))
}

// --- HEGEL_PROTOCOL_TEST_MODE=stop_test_on_mark_complete ---

func TestStopTestOnMarkComplete(t *testing.T) {

	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_mark_complete")
	Test(t, func(ht *T) {
		Draw[bool](ht, Booleans())
	}, WithTestCases(5))
}

// --- HEGEL_PROTOCOL_TEST_MODE=empty_test ---

func TestEmptyTest(t *testing.T) {

	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "empty_test")
	Test(t, func(_ *T) {
		panic("should not be called")
	}, WithTestCases(5))
}

// --- HEGEL_PROTOCOL_TEST_MODE=error_response ---

func TestErrorResponse(t *testing.T) {

	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "error_response")
	// The server sends a requestError on generate; the test body should
	// see a panic (INTERESTING) and RunHegelTestE should return an error.
	var gotErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				gotErr = fmt.Errorf("%v", r)
			}
		}()
		gotErr = runHegel(func(s *TestCase) {
			Draw[bool](s, Booleans()) // server sends error_response here
		}, stdoutNoteFn, []Option{WithTestCases(3)})
	}()
	// The error from the server causes INTERESTING status -> re-raised on final run.
	// Either a panic or a non-nil error is acceptable.
	_ = gotErr // we just verify it doesn't deadlock or hang
}

// --- Draw outside context: calling Draw with nil-stream state panics ---

func TestDrawWithNilStreamState(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic when Draw called with nil-stream state")
		}
	}()
	s := &TestCase{} // stream is nil -> will panic
	Draw[bool](s, Booleans())
}

// --- Assume outside context raises ---

func TestAssumeOutsideContext(t *testing.T) {
	t.Parallel()
	// Assume(false) on a nil *TestCase should panic.
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic from Assume outside test context")
		}
	}()
	var s *TestCase
	s.Assume(false)
}

// --- Note outside context is no-op (isFinal defaults false) ---

func TestNoteOutsideContext(t *testing.T) {
	t.Parallel()
	// Note() on a zero-value *TestCase should not panic (isFinal=false).
	s := &TestCase{}
	s.Note("outside context -- safe")
}

// --- Target outside context raises ---

func TestTargetOutsideContext(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic from Target outside test context")
		}
	}()
	s := &TestCase{} // stream is nil -> panic
	s.Target(1.0, "x")
}

// --- hegelSession: start and cleanup ---

func TestHegelSessionStartAndCleanup(t *testing.T) {
	t.Parallel()

	s := newHegelSession()
	if err := s.start(); err != nil {
		t.Fatalf("session.start: %v", err)
	}
	// Double start should be a no-op.
	if err := s.start(); err != nil {
		t.Fatalf("double session.start: %v", err)
	}
	s.cleanup()
	// Double cleanup should not panic.
	s.cleanup()
}

// --- hegelSession: cleanup with nil fields is safe ---

func TestHegelSessionCleanupEmpty(t *testing.T) {
	t.Parallel()
	s := newHegelSession()
	s.cleanup() // Should not panic when nothing started.
}

// --- hegelSession: start fails when hegel exits immediately ---

func TestHegelSessionStartExitsImmediately(t *testing.T) {
	t.Parallel()
	// Use `false` (exits immediately) so stdio pipes close immediately.
	falseBin, err := exec.LookPath("false")
	if err != nil {
		t.Skip("false binary not available")
	}
	s := newHegelSession()
	s.hegelCmd = falseBin // exits immediately, pipes close
	startErr := s.start()
	if startErr == nil {
		s.cleanup()
		t.Fatal("expected handshake error")
	}
	mustContainStr(t, startErr.Error(), "handshake")
}

// --- hegelSession: concurrent starts (double-checked locking) ---

func TestHegelSessionConcurrentStart(t *testing.T) {
	t.Parallel()

	s := newHegelSession()
	defer s.cleanup()

	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = s.start()
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent start %d: %v", i, err)
		}
	}
}

// --- RunHegelTest with real test cases=1 ---

func TestRunHegelTestSingleCase(t *testing.T) {

	count := 0
	Test(t, func(ht *T) {
		count++
		b := Draw[bool](ht, Booleans())
		if b != true && b != false {
			ht.Fatal("not a bool")
		}
	}, WithTestCases(1))
	if count == 0 {
		t.Error("expected at least one test case to run")
	}
}

// --- showcase: concurrent RunHegelTest calls from different goroutines ---

func TestConcurrentRunHegelTest(t *testing.T) {

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			Test(t, func(ht *T) {
				b := Draw[bool](ht, Booleans())
				if b != true && b != false {
					ht.Fatal("not a bool")
				}
			}, WithTestCases(3))
		}(i)
	}
	wg.Wait()
}

// --- RunHegelTestE returns nil on success ---

func TestRunHegelTestESuccess(t *testing.T) {

	Test(t, func(ht *T) {
		_ = Draw[bool](ht, Booleans())
	}, WithTestCases(3))
}

// --- WithTestCases option ---

func TestWithTestCasesOption(t *testing.T) {
	t.Parallel()

	count := 0
	Test(t, func(ht *T) {
		count++
		Draw[bool](ht, Booleans())
	}, WithTestCases(10))
	// count should be >= 10 (at least the requested cases)
	if count < 1 {
		t.Error("expected test cases to run")
	}
}

// --- HEGEL_PROTOCOL_TEST_MODE=stop_test_on_collection_more ---

func TestStopTestOnCollectionMore(t *testing.T) {

	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_collection_more")
	err := runHegel(func(s *TestCase) {
		max := 10
		coll := newCollection(s, 0, &max)
		_ = coll.More(s)
	}, stdoutNoteFn, nil)
	_ = err // StopTest causes abort, not necessarily an error return
}

// --- HEGEL_PROTOCOL_TEST_MODE=stop_test_on_new_collection ---

func TestStopTestOnNewCollection(t *testing.T) {

	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_new_collection")
	err := runHegel(func(s *TestCase) {
		max := 10
		coll := newCollection(s, 0, &max)
		_ = coll.More(s)
	}, stdoutNoteFn, nil)
	_ = err // StopTest causes abort, not necessarily an error return
}

// --- runTest: connection error in test function is re-raised ---

func TestConnectionErrorInTestFunction(t *testing.T) {
	t.Parallel()

	err := runHegel(func(_ *TestCase) {
		panic(&connectionError{msg: "test connection lost"})
	}, stdoutNoteFn, []Option{WithTestCases(1)})
	if err == nil {
		t.Fatal("expected error to be raised for connection error")
	}
	mustContainStr(t, err.Error(), "connection lost")
}

// --- Unit tests for error/recovery paths ---

// --- assumeRejected.Error() ---

func TestAssumeRejectedError(t *testing.T) {
	t.Parallel()
	e := assumeRejected{}
	if e.Error() != "assume rejected" {
		t.Errorf("assumeRejected.Error() = %q", e.Error())
	}
}

// --- dataExhausted.Error() ---

func TestDataExhaustedError(t *testing.T) {
	t.Parallel()
	e := &dataExhausted{msg: "exhausted"}
	if e.Error() != "exhausted" {
		t.Errorf("dataExhausted.Error() = %q", e.Error())
	}
}

// --- flakyAbort.Error() ---

func TestFlakyAbortError(t *testing.T) {
	t.Parallel()
	e := flakyAbort{}
	if e.Error() != "flaky test detected" {
		t.Errorf("flakyAbort.Error() = %q", e.Error())
	}
}

// --- connectionError.Error() ---

func TestConnectionErrorError(t *testing.T) {
	t.Parallel()
	e := &connectionError{msg: "conn lost"}
	if e.Error() != "conn lost" {
		t.Errorf("connectionError.Error() = %q", e.Error())
	}
}

// --- serverCrashMessageForLog: fallback when logPath is empty ---

func TestServerCrashMessageNoLogFile(t *testing.T) {
	t.Parallel()
	msg := serverCrashMessageForLog("")
	mustContainStr(t, msg, "server process exited unexpectedly")
}

// --- aborted flag: set directly on state ---

func TestAbortedFlagDirect(t *testing.T) {
	t.Parallel()
	state := &TestCase{}
	state.aborted = true
	if !state.aborted {
		t.Error("expected aborted to be true after direct assignment")
	}
}

// --- generateFromSchema: connection error (Request fails) ---

func TestGenerateFromSchemaConnectionError(t *testing.T) {
	t.Parallel()
	s, c := socketPair(t)
	conn := newConnection(s, s, "C")
	c.Close()
	// We need state=client so NewStream works.
	conn.state = stateClient
	st := &stream{conn: conn, streamID: 1, inbox: make(chan any, 1), nextMessageID: 1}
	conn.streams[1] = st

	// Close the underlying conn so SendPacket fails.
	s.Close()

	state := &TestCase{stream: st}

	var caught any
	func() {
		defer func() { caught = recover() }()
		Draw[bool](state, Booleans())
	}()
	if caught == nil {
		t.Fatal("expected panic from Draw on connection error")
	}
	_, isConnErr := caught.(*connectionError)
	if !isConnErr {
		t.Errorf("expected *connectionError, got %T: %v", caught, caught)
	}
}

// --- Target: error path when Request fails ---

func TestTargetConnectionError(t *testing.T) {
	t.Parallel()
	s, _ := socketPair(t)
	conn := newConnection(s, s, "C")
	conn.state = stateClient
	st := &stream{conn: conn, streamID: 1, inbox: make(chan any, 1), nextMessageID: 1}
	conn.streams[1] = st
	s.Close()

	state := &TestCase{stream: st}

	var caught any
	func() {
		defer func() { caught = recover() }()
		state.Target(1.0, "x")
	}()
	if caught == nil {
		t.Fatal("expected panic from Target on connection error")
	}
}

// --- isHegelFrame ---

func TestIsHegelFrame(t *testing.T) {
	if !isHegelFrame("hegel.dev/go/hegel.someFunc") {
		t.Error("expected isHegelFrame to return true for hegel frame")
	}
	if isHegelFrame("testing.tRunner") {
		t.Error("expected isHegelFrame to return false for non-hegel frame")
	}
	// Short name (less than module path length).
	if isHegelFrame("short") {
		t.Error("expected isHegelFrame to return false for short frame")
	}
}

// --- extractPanicOrigin: non-error value ---

func TestExtractPanicOriginNonError(t *testing.T) {
	t.Parallel()
	origin := extractPanicOrigin("just a string")
	// Should include the type (string) and file info.
	if origin == "" {
		t.Error("expected non-empty origin from extractPanicOrigin")
	}
}

// --- extractPanicOrigin: error value ---

func TestExtractPanicOriginError(t *testing.T) {
	t.Parallel()
	origin := extractPanicOrigin(errors.New("test"))
	if origin == "" {
		t.Error("expected non-empty origin from extractPanicOrigin with error")
	}
}

// --- Note: isFinal=true prints to stderr ---

func TestNoteIsFinalTrue(t *testing.T) {
	t.Parallel()
	state := &TestCase{isFinal: true, noteFn: stdoutNoteFn}
	// Should not panic.
	state.Note("test note on final")
}

// --- hegelSession: start with spawn error ---

func TestHegelSessionSpawnError(t *testing.T) {
	t.Parallel()
	s := newHegelSession()
	s.hegelCmd = "/nonexistent/binary/that/does/not/exist"
	err := s.start()
	if err == nil {
		s.cleanup()
		t.Fatal("expected error from session with bad binary")
	}
}

// --- hegelSession: start fails when hegelCommand() errors ---

func TestHegelSessionStartHegelCommandError(t *testing.T) {
	resetProjectRoot(t)
	t.Setenv(hegelServerCommandEnv, "")

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// No uv on PATH → hegelCommand() should fail.
	t.Setenv("PATH", "/nonexistent")
	t.Setenv("HOME", "/nonexistent")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))

	s := newHegelSession()
	// hegelCmd is empty, so start() calls hegelCommand() which should fail.
	err := s.start()
	if err == nil {
		s.cleanup()
		t.Fatal("expected error when hegelCommand fails")
	}
	mustContainStr(t, err.Error(), "hegel")
}

// --- hegelSession: cleanup with erroring close ---

func TestHegelSessionCleanupWithErrors(t *testing.T) {
	t.Parallel()
	s := newHegelSession()
	// Set conn to a closed connection so Close() might error.
	sc, cc := socketPair(t)
	sc.Close()
	cc.Close()
	s.conn = newConnection(sc, sc, "closed")
	s.conn.Close() // pre-close

	// This should not panic.
	s.cleanup()
}

// --- RunHegelTestE: session start error ---

func TestRunHegelTestESessionError(t *testing.T) {
	// Use an internal session with a bad cmd to force start() failure.
	old := globalSession
	defer func() { globalSession = old }()
	globalSession = newHegelSession()
	globalSession.hegelCmd = "/nonexistent/hegel"

	err := runHegel(func(_ *TestCase) {}, stdoutNoteFn, []Option{WithTestCases(1)})
	if err == nil {
		t.Error("expected error when session cannot start")
	}
	mustContainStr(t, err.Error(), "session start")
}

// --- RunHegelTest: panic path (test fails) ---

func TestRunHegelTestPanicsOnFailure(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected RunHegelTest to panic on test failure")
		}
	}()

	// Simulate failure by using a session with a fake bad-test server.
	// We swap globalSession temporarily.
	old := globalSession
	defer func() { globalSession = old }()

	// Use a session that always returns an error.
	fake := newHegelSession()
	fake.hegelCmd = "/nonexistent/hegel"
	globalSession = fake

	if _err := runHegel(func(_ *TestCase) {}, stdoutNoteFn, []Option{WithTestCases(1)}); _err != nil {
		panic(_err)
	}
}

// --- RunHegelTestE: calls session.runTest ---

func TestRunHegelTestECallsRunTest(t *testing.T) {

	called := false
	Test(t, func(ht *T) {
		called = true
		Draw[bool](ht, Booleans())
	}, WithTestCases(1))
	if !called {
		t.Error("test body was never called")
	}
}

// --- hegelSession.runTest: covered via integration ---

func TestHegelSessionRunTest(t *testing.T) {
	t.Parallel()

	s := newHegelSession()
	defer s.cleanup()
	if err := s.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	err := s.runTest(func(st *TestCase) {
		Draw[bool](st, Booleans())
	}, runOptions{testCases: 2}, stdoutNoteFn)
	if err != nil {
		t.Errorf("session.runTest: %v", err)
	}
}

// --- hegelCommand: basic non-error check ---

func TestHegelCommandReturnsNonNil(t *testing.T) {
	t.Parallel()
	cmd, err := hegelCommand()
	if err != nil {
		t.Skipf("hegelCommand: %v (uv not available)", err)
	}
	if cmd == nil {
		t.Error("hegelCommand returned nil cmd")
	}
}

// --- hegelSession.start: double-checked locking (inner check) ---

func TestHegelSessionStartInnerCheck(t *testing.T) {
	t.Parallel()

	s := newHegelSession()
	defer s.cleanup()

	// Start it once.
	if err := s.start(); err != nil {
		t.Fatalf("first start: %v", err)
	}
	// Start again -- should hit outer hasWorkingClient check.
	if err := s.start(); err != nil {
		t.Errorf("second start: %v", err)
	}
}

// --- hegelSession.cleanup: conn/process/tempDir paths via integration ---

func TestHegelSessionCleanupAllPaths(t *testing.T) {
	t.Parallel()

	s := newHegelSession()
	if err := s.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Cleanup should close conn and kill process.
	s.cleanup()
	if s.conn != nil {
		t.Error("conn should be nil after cleanup")
	}
	if s.process != nil {
		t.Error("process should be nil after cleanup")
	}
}

// --- runTest: multi-interesting, single error (len(errs)==1 branch) ---

func TestRunTestMultiInterestingSingleError(t *testing.T) {
	t.Parallel()
	t.Skip("len(errs)==1 in multi-interesting is unreachable when nInteresting>1")
}

// --- extractPanicOrigin: all frames are hegel frames ---

func TestExtractPanicOriginAllHegelFrames(t *testing.T) {
	t.Parallel()
	origin := extractPanicOrigin("test panic")
	if origin == "" {
		t.Error("expected non-empty origin")
	}
}

// --- RunHegelTestE: HEGEL_PROTOCOL_TEST_MODE path, session start error ---

func TestRunHegelTestEProtocolModeStartError(t *testing.T) {
	resetProjectRoot(t)
	t.Setenv(hegelServerCommandEnv, "")
	// Set HEGEL_PROTOCOL_TEST_MODE so RunHegelTestE uses a temp session.
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "empty_test")

	tmp := t.TempDir()
	t.Chdir(tmp)

	// Block all paths to finding hegel/uv: no PATH, no cached uv.
	t.Setenv("PATH", "/nonexistent")
	t.Setenv("HOME", "/nonexistent")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))

	err := runHegel(func(_ *TestCase) {}, stdoutNoteFn, []Option{WithTestCases(1)})
	if err == nil {
		t.Error("expected error when session cannot start in protocol test mode")
	}
	mustContainStr(t, err.Error(), "session start")
}

// --- hegelSession.start: handshake error ---

func TestHegelSessionStartHandshakeError(t *testing.T) {
	// Write a fake hegel binary that writes garbage to stdout and exits.
	// This causes SendHandshakeVersion to fail because the data isn't a valid packet.
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "fake_hegel.sh")
	script := "#!/bin/sh\nprintf 'bad_data\\n'\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	s := newHegelSession()
	s.hegelCmd = scriptPath
	err := s.start()
	if err == nil {
		s.cleanup()
		t.Fatal("expected handshake error")
	}
	mustContainStr(t, err.Error(), "handshake")
}

// --- hegelCommand tests are in installer_test.go ---

// =============================================================================
// fatalSentinel.Error()
// =============================================================================

func TestFatalSentinelError(t *testing.T) {
	t.Parallel()
	f := fatalSentinel{msg: "test fatal"}
	if f.Error() != "test fatal" {
		t.Errorf("got %q", f.Error())
	}
}

// =============================================================================
// toInt64: uint64 branch and invalid type branch
// =============================================================================

func TestToInt64Int64(t *testing.T) {
	t.Parallel()
	v, ok := toInt64(int64(-7))
	if !ok || v != -7 {
		t.Errorf("got %d, %v", v, ok)
	}
}

func TestToInt64Uint64(t *testing.T) {
	t.Parallel()
	v, ok := toInt64(uint64(42))
	if !ok || v != 42 {
		t.Errorf("got %d, %v", v, ok)
	}
}

func TestToInt64Invalid(t *testing.T) {
	t.Parallel()
	_, ok := toInt64("not a number")
	if ok {
		t.Error("expected false for invalid type")
	}
}

// =============================================================================
// Public API: MustRun — panics on error
// =============================================================================

func TestMustRunPanicsOnError(t *testing.T) {
	old := globalSession
	defer func() { globalSession = old }()
	globalSession = newHegelSession()
	globalSession.hegelCmd = "/nonexistent"

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustRun to panic on error")
		}
	}()
	MustRun(func(*TestCase) {}, WithTestCases(1))
}

// =============================================================================
// Public API: Run — via real binary
// =============================================================================

func TestRunPublicAPI(t *testing.T) {

	err := Run(func(s *TestCase) {
		_ = Draw[bool](s, Booleans())
	}, WithTestCases(1))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// =============================================================================
// Public API: MustRun — success via real binary
// =============================================================================

func TestMustRunSuccess(t *testing.T) {

	MustRun(func(s *TestCase) {
		_ = Draw[bool](s, Booleans())
	}, WithTestCases(1))
}

// =============================================================================
// Public API: Test — via real binary
// =============================================================================

func TestTestSuccess(t *testing.T) {

	Test(t, func(ht *T) {
		_ = Draw[bool](ht, Booleans())
		ht.Note("test note via Test")
	}, WithTestCases(1))
}

// =============================================================================
// state.failed triggers INTERESTING on final replay
// =============================================================================

func TestStateFailedPath(t *testing.T) {

	err := runHegel(func(s *TestCase) {
		_ = Draw[bool](s, Booleans())
		s.failed = true // simulates T.Error/T.Fail
	}, stdoutNoteFn, []Option{WithTestCases(1)})
	if err == nil {
		t.Error("expected error when state.failed is true")
	}
}

// =============================================================================
// fatalSentinel triggers INTERESTING on final replay
// =============================================================================

func TestFatalSentinelPath(t *testing.T) {

	err := runHegel(func(s *TestCase) {
		_ = Draw[bool](s, Booleans())
		panic(fatalSentinel{msg: "test fatal"})
	}, stdoutNoteFn, []Option{WithTestCases(1)})
	if err == nil {
		t.Error("expected error when fatalSentinel is raised")
	}
}

// =============================================================================
// Test: notes printed on final replay reach the process's real stdout
// =============================================================================

// Verifies the full path ht.Note -> noteFn (t.Log) -> go test output by
// spawning a real `go test` subprocess that calls hegel.Test. The body
// emits a note on final replay; we assert it shows up in the captured
// test output.
func TestNoteFnOnFinal(t *testing.T) {
	t.Parallel()

	newTempGoProject(t).
		testBody(`_ = hegel.Draw[bool](ht, hegel.Booleans())
ht.Note("note for final")
panic("always fail for final replay")`, "hegel.WithTestCases(1)").
		expectFailure(`note for final`).
		goTest()
}

// --- hegelCommand: covered in installer_test.go ---

// --- runTest: SendControlRequest error (closed connection) ---

func TestRunTestSendControlRequestError(t *testing.T) {
	t.Parallel()
	conn, remote := clientConnPair(t)
	remote.Close()

	cl := newClient(conn)
	err := cl.runTest(func(_ *TestCase) {}, runOptions{testCases: 1}, stdoutNoteFn)
	if err == nil {
		t.Fatal("expected error from runTest on closed conn")
	}
	mustContainStr(t, err.Error(), "run_test send")
}

// =============================================================================
// HealthCheck.String()
// =============================================================================

func TestHealthCheckString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		hc   HealthCheck
		want string
	}{
		{FilterTooMuch, "filter_too_much"},
		{TooSlow, "too_slow"},
		{TestCasesTooLarge, "test_cases_too_large"},
		{LargeInitialTestCase, "large_initial_test_case"},
	}
	for _, tt := range tests {
		if got := tt.hc.String(); got != tt.want {
			t.Errorf("HealthCheck(%d).String() = %q, want %q", tt.hc, got, tt.want)
		}
	}
}

// =============================================================================
// AllHealthChecks()
// =============================================================================

func TestAllHealthChecks(t *testing.T) {
	t.Parallel()
	all := AllHealthChecks()
	if len(all) != 4 {
		t.Errorf("AllHealthChecks() has %d elements, want 4", len(all))
	}
}

// =============================================================================
// SuppressHealthCheck option
// =============================================================================

func TestSuppressHealthCheckOption(t *testing.T) {
	t.Parallel()
	o := runOptions{testCases: 100}
	SuppressHealthCheck(FilterTooMuch, TooSlow)(&o)
	if len(o.suppressHealthCheck) != 2 {
		t.Errorf("suppressHealthCheck has %d elements, want 2", len(o.suppressHealthCheck))
	}
	if o.suppressHealthCheck[0] != FilterTooMuch {
		t.Errorf("suppressHealthCheck[0] = %v, want FilterTooMuch", o.suppressHealthCheck[0])
	}
}

// =============================================================================
// SuppressHealthCheck: integration test with real server
// =============================================================================

func TestSuppressHealthCheckIntegration(t *testing.T) {

	// Exercise the suppress_health_check protocol path.
	Test(t, func(ht *T) {
		n := Draw[int](ht, Integers[int](0, 100))
		ht.Assume(n < 90)
	}, SuppressHealthCheck(FilterTooMuch, TooSlow), WithTestCases(5))
}

func TestSuppressAllHealthChecksIntegration(t *testing.T) {

	Test(t, func(ht *T) {
		n := Draw[int](ht, Integers[int](0, 100))
		ht.Assume(n < 90)
	}, SuppressHealthCheck(AllHealthChecks()...), WithTestCases(5))
}

// =============================================================================
// Server crash detection: processExited stream
// =============================================================================

func TestProcessExitedStream(t *testing.T) {
	t.Parallel()
	s, c := socketPair(t)
	conn := newConnection(s, s, "C")
	defer conn.Close()
	c.Close()

	exited := make(chan struct{})
	conn.processExited = exited

	// Not exited yet.
	select {
	case <-conn.processExited:
		t.Error("processExited should not be closed initially")
	default:
	}

	// Mark exited.
	close(exited)
	select {
	case <-conn.processExited:
	default:
		t.Error("processExited should be closed after close()")
	}
}

// =============================================================================
// Session start timeout: process alive but no socket
// =============================================================================

func TestHegelSessionStartTimeout(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "fake_hegel.sh")
	os.WriteFile(script, []byte("#!/bin/sh\nsleep 60\n"), 0o755) //nolint:errcheck
	s := newHegelSession()
	s.hegelCmd = script
	startErr := s.start()
	s.cleanup()
	if startErr == nil {
		t.Fatal("expected timeout error")
	}
	mustContainStr(t, startErr.Error(), "timed out")
}

// =============================================================================
// Health check failure: filter too aggressively without suppressing
// =============================================================================

func TestHealthCheckFailureFilterTooMuch(t *testing.T) {
	t.Parallel()

	// Filtering based on a tiny range triggers FilterTooMuch: the server sees
	// almost all test cases rejected and raises a health check failure.
	newTempGoProject(t).
		testBody(`n := hegel.Draw[int](ht, hegel.Integers[int](0, 1000))
ht.Assume(n == 0)`, "hegel.WithTestCases(200)").
		expectFailure(`health check failure`).
		goTest()
}

// =============================================================================
// Flaky global state detection
// =============================================================================

var flakyCounter atomic.Int64

func TestFlakyGlobalState(t *testing.T) {

	flakyCounter.Store(0)
	err := runHegel(func(s *TestCase) {
		min := int(flakyCounter.Load())
		_ = Draw[int](s, Integers[int](min, min+100))
		flakyCounter.Add(1)
	}, stdoutNoteFn, nil)
	if err == nil {
		t.Fatal("expected error for flaky test")
	}
	mustContainStr(t, err.Error(), "flaky")
}

// =============================================================================
// Server crash on Request error (closed socket + ServerHasExited)
// =============================================================================

func TestGenerateServerCrashOnRequest(t *testing.T) {
	t.Parallel()
	s, c := socketPair(t)
	conn := newConnection(s, s, "C")
	c.Close()
	conn.state = stateClient
	st := &stream{conn: conn, streamID: 1, inbox: make(chan any, 1), dropped: make(chan struct{}), nextMessageID: 1}
	conn.writerMu.Lock()
	conn.streams[1] = st
	conn.writerMu.Unlock()

	exited := make(chan struct{})
	close(exited)
	conn.processExited = exited

	s.Close()

	state := &TestCase{stream: st}
	var caught any
	func() {
		defer func() { caught = recover() }()
		Draw[bool](state, Booleans())
	}()
	if caught == nil {
		t.Fatal("expected panic from Draw on server crash")
	}
	connErr, ok := caught.(*connectionError)
	if !ok {
		t.Fatalf("expected *connectionError, got %T: %v", caught, caught)
	}
	mustContainStr(t, connErr.msg, "server process exited unexpectedly")
}

// =============================================================================
// Server crash on Get error (socket closed mid-request + ServerHasExited)
// =============================================================================

func TestGenerateServerCrashOnGet(t *testing.T) {
	t.Parallel()
	s, c := socketPair(t)
	conn := newConnection(s, s, "C")
	conn.state = stateClient
	st := &stream{conn: conn, streamID: 1, inbox: make(chan any, 1), dropped: make(chan struct{}), nextMessageID: 1}
	conn.writerMu.Lock()
	conn.streams[1] = st
	conn.writerMu.Unlock()

	exited := make(chan struct{})
	conn.processExited = exited

	// Read the request on peer side, then simulate crash
	go func() {
		readPacket(c) //nolint:errcheck
		close(exited)
		c.Close()
	}()

	state := &TestCase{stream: st}
	var caught any
	func() {
		defer func() { caught = recover() }()
		Draw[bool](state, Booleans())
	}()
	if caught == nil {
		t.Fatal("expected panic from Draw on server crash")
	}
	connErr, ok := caught.(*connectionError)
	if !ok {
		t.Fatalf("expected *connectionError, got %T: %v", caught, caught)
	}
	mustContainStr(t, connErr.msg, "server process exited unexpectedly")
}

// =============================================================================
// ServerHasExited in processOneMessage
// =============================================================================

func TestProcessOneMessageServerCrash(t *testing.T) {
	t.Parallel()
	s, c := socketPair(t)
	conn := newConnection(s, s, "C")
	conn.state = stateClient
	st := conn.NewStream("Test")

	exited := make(chan struct{})
	close(exited)
	conn.processExited = exited
	c.Close()

	// Wait for readLoop to notice the close
	<-conn.done

	_, _, err := st.RecvRequestRaw(1 * time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
	mustContainStr(t, err.Error(), "server process exited unexpectedly")
}

// processExited is non-nil but never closes — the wait times out and we fall
// through to the generic "connection closed" error.
func TestProcessOneMessageConnectionClosedTimeout(t *testing.T) {
	t.Parallel()
	s, c := socketPair(t)
	conn := newConnection(s, s, "C")
	conn.state = stateClient
	st := conn.NewStream("Test")

	conn.processExited = make(chan struct{})
	c.Close()

	<-conn.done

	_, _, err := st.RecvRequestRaw(1 * time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
	mustContainStr(t, err.Error(), "connection closed")
}

// =============================================================================
// buildRunTestMessage: wire-format base fields
// =============================================================================

func TestBuildRunTestMessageBaseFields(t *testing.T) {
	t.Parallel()
	msg := buildRunTestMessage(7, runOptions{testCases: 42, derandomize: true})
	if msg["command"] != "run_test" {
		t.Errorf("command = %v", msg["command"])
	}
	if msg["test_cases"] != int64(42) {
		t.Errorf("test_cases = %v", msg["test_cases"])
	}
	if msg["stream_id"] != int64(7) {
		t.Errorf("stream_id = %v", msg["stream_id"])
	}
	if msg["derandomize"] != true {
		t.Errorf("derandomize = %v", msg["derandomize"])
	}
}

func TestDatabaseDisabledSetting(t *testing.T) {
	t.Parallel()
	s := DatabaseDisabled()
	if s.state != databaseDisabled {
		t.Errorf("state = %v, want databaseDisabled", s.state)
	}
}

func TestBuildRunTestMessageDatabaseDisabled(t *testing.T) {
	t.Parallel()
	msg := buildRunTestMessage(1, runOptions{
		testCases:   1,
		database:    DatabaseDisabled(),
		databaseKey: []byte("k"),
	})
	v, ok := msg["database"]
	if !ok {
		t.Fatal("expected database field to be present")
	}
	if v != nil {
		t.Errorf("database = %v, want nil", v)
	}
	if msg["database_key"] != nil {
		t.Errorf("database_key = %v, want nil when disabled", msg["database_key"])
	}
}

// In CI, runHegel disables the database regardless of the user's option.
func TestRunHegelDisablesDatabaseInCI(t *testing.T) {
	clearCIEnv(t)
	t.Setenv("CI", "true")
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "empty_test")
	err := runHegel(func(_ *TestCase) {}, stdoutNoteFn, nil)
	if err != nil {
		t.Fatalf("runHegel: %v", err)
	}
}

func TestWithDerandomizeIntegration(t *testing.T) {
	clearCIEnv(t)
	// Just exercise the wire path — full determinism is enforced server-side.
	Test(t, func(ht *T) {
		_ = Draw[bool](ht, Booleans())
	}, WithDerandomize(true), WithTestCases(3))
}

// --- helpers ---

func clearCIEnv(t *testing.T) {
	t.Helper()
	for _, v := range ciEnvVars {
		if old, ok := os.LookupEnv(v.name); ok {
			os.Unsetenv(v.name)
			t.Cleanup(func() { os.Setenv(v.name, old) })
		}
	}
}

func mustContainStr(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("%q does not contain %q", s, sub)
	}
}
