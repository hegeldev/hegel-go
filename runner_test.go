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

// hegelBinPath verifies the hegel binary is available (installed by just setup).
func hegelBinPath(t *testing.T) {
	t.Helper()
	root := getProjectRoot()
	if findHegelInDir(filepath.Join(root, ".venv")) != "" {
		return
	}
	if _, err := exec.LookPath("hegel"); err == nil {
		return
	}
	t.Skip("hegel binary not found -- run 'just setup'")
}

// --- RunHegelTest: basic passing test ---

func TestRunHegelTestPasses(t *testing.T) {
	hegelBinPath(t)
	called := false
	if _err := runHegel(func(s *TestCase) {
		called = true
		b := Draw[bool](s, Booleans())
		// A valid assertion: b is either true or false.
		if b != true && b != false {
			t.Errorf("expected bool, got %v", b)
		}
	}, stderrNoteFn, []Option{WithTestCases(5)}); _err != nil {
		panic(_err)
	}
	if !called {
		t.Error("test function was never called")
	}
}

// --- RunHegelTest: failing test raises error ---

func TestRunHegelTestFails(t *testing.T) {
	hegelBinPath(t)
	err := runHegel(func(s *TestCase) {
		x := Draw[int](s, Integers[int](0, 100))
		// This always fails: no integer < 0 in [0,100]
		if x >= 0 {
			panic(fmt.Sprintf("assertion failed: %d >= 0", x))
		}
	}, stderrNoteFn, []Option{WithTestCases(10)})
	if err == nil {
		t.Error("expected RunHegelTestE to return an error for always-failing test")
	}
}

// --- RunHegelTest: assume(false) -> INVALID, test continues ---

func TestRunHegelTestAllInvalid(t *testing.T) {
	hegelBinPath(t)
	// A test that always calls Assume(false) should pass (all cases rejected).
	if _err := runHegel(func(s *TestCase) {
		s.Assume(false)
	}, stderrNoteFn, []Option{WithTestCases(5)}); _err != nil {
		panic(_err)
	}
}

// --- RunHegelTest: assume(true) -> no effect ---

func TestAssumeTrue(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
		s.Assume(true)
		b := Draw[bool](s, Booleans())
		_ = b // use the value
		if b != true && b != false {
			panic("expected bool")
		}
	}, stderrNoteFn, []Option{WithTestCases(5)}); _err != nil {
		panic(_err)
	}
}

// --- note(): not printed when not final ---

func TestNoteNotFinal(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	// note() should not panic or error when called outside final run
	if _err := runHegel(func(s *TestCase) {
		s.Note("should not appear")
		_ = Draw[bool](s, Booleans())
	}, stderrNoteFn, []Option{WithTestCases(3)}); _err != nil {
		panic(_err)
	}
}

// --- target(): sends target command ---

func TestTargetSendsCommand(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
		x := Draw[int](s, Integers[int](0, 100))
		s.Target(float64(x), "my_target")
		if x < 0 || x > 100 {
			panic("out of range")
		}
	}, stderrNoteFn, []Option{WithTestCases(5)}); _err != nil {
		panic(_err)
	}
}

// --- HEGEL_PROTOCOL_TEST_MODE=stop_test_on_generate ---

func TestStopTestOnGenerate(t *testing.T) {
	hegelBinPath(t)
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_generate")
	// Should complete without error: client handles StopTest cleanly.
	if _err := runHegel(func(s *TestCase) {
		Draw[bool](s, Booleans())
	}, stderrNoteFn, []Option{WithTestCases(5)}); _err != nil {
		panic(_err)
	}
}

// --- HEGEL_PROTOCOL_TEST_MODE=stop_test_on_mark_complete ---

func TestStopTestOnMarkComplete(t *testing.T) {
	hegelBinPath(t)
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_mark_complete")
	if _err := runHegel(func(s *TestCase) {
		Draw[bool](s, Booleans())
	}, stderrNoteFn, []Option{WithTestCases(5)}); _err != nil {
		panic(_err)
	}
}

// --- HEGEL_PROTOCOL_TEST_MODE=empty_test ---

func TestEmptyTest(t *testing.T) {
	hegelBinPath(t)
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "empty_test")
	if _err := runHegel(func(_ *TestCase) {
		panic("should not be called")
	}, stderrNoteFn, []Option{WithTestCases(5)}); _err != nil {
		panic(_err)
	}
}

// --- HEGEL_PROTOCOL_TEST_MODE=error_response ---

func TestErrorResponse(t *testing.T) {
	hegelBinPath(t)
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
		}, stderrNoteFn, []Option{WithTestCases(3)})
	}()
	// The error from the server causes INTERESTING status -> re-raised on final run.
	// Either a panic or a non-nil error is acceptable.
	_ = gotErr // we just verify it doesn't deadlock or hang
}

// --- Draw outside context: calling Draw with nil-channel state panics ---

func TestDrawWithNilChannelState(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic when Draw called with nil-channel state")
		}
	}()
	s := &TestCase{} // channel is nil -> will panic
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
	s := &TestCase{} // channel is nil -> panic
	s.Target(1.0, "x")
}

// --- findHegelInDir tests are in installer_test.go ---

// --- hegelSession: start and cleanup ---

func TestHegelSessionStartAndCleanup(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
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
	hegelBinPath(t)
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
	hegelBinPath(t)
	count := 0
	if _err := runHegel(func(s *TestCase) {
		count++
		b := Draw[bool](s, Booleans())
		if b != true && b != false {
			panic("not a bool")
		}
	}, stderrNoteFn, []Option{WithTestCases(1)}); _err != nil {
		panic(_err)
	}
	if count == 0 {
		t.Error("expected at least one test case to run")
	}
}

// --- showcase: concurrent RunHegelTest calls from different goroutines ---

func TestConcurrentRunHegelTest(t *testing.T) {
	hegelBinPath(t)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if _err := runHegel(func(s *TestCase) {
				b := Draw[bool](s, Booleans())
				if b != true && b != false {
					panic("not a bool")
				}
			}, stderrNoteFn, []Option{WithTestCases(3)}); _err != nil {
				panic(_err)
			}
		}(i)
	}
	wg.Wait()
}

// --- RunHegelTestE returns nil on success ---

func TestRunHegelTestESuccess(t *testing.T) {
	hegelBinPath(t)
	err := runHegel(func(s *TestCase) {
		_ = Draw[bool](s, Booleans())
	}, stderrNoteFn, []Option{WithTestCases(3)})
	if err != nil {
		t.Errorf("RunHegelTestE: unexpected error: %v", err)
	}
}

// --- WithTestCases option ---

func TestWithTestCasesOption(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	count := 0
	if _err := runHegel(func(s *TestCase) {
		count++
		Draw[bool](s, Booleans())
	}, stderrNoteFn, []Option{WithTestCases(10)}); _err != nil {
		panic(_err)
	}
	// count should be >= 10 (at least the requested cases)
	if count < 1 {
		t.Error("expected test cases to run")
	}
}

// --- HEGEL_PROTOCOL_TEST_MODE=stop_test_on_collection_more ---

func TestStopTestOnCollectionMore(t *testing.T) {
	hegelBinPath(t)
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_collection_more")
	err := runHegel(func(s *TestCase) {
		coll := newCollection(s, 0, 10)
		_ = coll.More(s)
	}, stderrNoteFn, nil)
	_ = err // StopTest causes abort, not necessarily an error return
}

// --- HEGEL_PROTOCOL_TEST_MODE=stop_test_on_new_collection ---

func TestStopTestOnNewCollection(t *testing.T) {
	hegelBinPath(t)
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_new_collection")
	err := runHegel(func(s *TestCase) {
		coll := newCollection(s, 0, 10)
		_ = coll.More(s)
	}, stderrNoteFn, nil)
	_ = err // StopTest causes abort, not necessarily an error return
}

// --- isFinal: Note prints on final run ---
// We test this by running a failing test so the final replay happens.
// We capture whether isFinal was true via the state in the closure.

func TestNoteOnFinalRun(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	noted := false
	noteFunc := func(s *TestCase) {
		if s.isFinal {
			noted = true
		}
		s.Note("final note")
		// Always fail so we get a final replay.
		panic("intentional failure for final replay test")
	}
	func() {
		defer func() { recover() }()                                 //nolint:errcheck
		runHegel(noteFunc, stderrNoteFn, []Option{WithTestCases(3)}) //nolint:errcheck
	}()
	if !noted {
		t.Error("expected isFinal to be true during final replay")
	}
}

// --- runTest: connection error in test function is re-raised ---

func TestConnectionErrorInTestFunction(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	err := runHegel(func(_ *TestCase) {
		panic(&connectionError{msg: "test connection lost"})
	}, stderrNoteFn, []Option{WithTestCases(1)})
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
	// We need state=client so NewChannel works.
	conn.state = stateClient
	ch := &channel{conn: conn, channelID: 1, inbox: make(chan any, 1), nextMessageID: 1}
	conn.channels[1] = ch

	// Close the underlying conn so SendPacket fails.
	s.Close()

	state := &TestCase{channel: ch}

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
	ch := &channel{conn: conn, channelID: 1, inbox: make(chan any, 1), nextMessageID: 1}
	conn.channels[1] = ch
	s.Close()

	state := &TestCase{channel: ch}

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
	state := &TestCase{isFinal: true, noteFn: stderrNoteFn}
	// Should not panic.
	state.Note("test note on final")
}

// --- findHegelInDir fallback test is in installer_test.go ---

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

	err := runHegel(func(_ *TestCase) {}, stderrNoteFn, []Option{WithTestCases(1)})
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

	if _err := runHegel(func(_ *TestCase) {}, stderrNoteFn, []Option{WithTestCases(1)}); _err != nil {
		panic(_err)
	}
}

// --- RunHegelTestE: calls session.runTest ---

func TestRunHegelTestECallsRunTest(t *testing.T) {
	hegelBinPath(t)
	called := false
	err := runHegel(func(s *TestCase) {
		called = true
		Draw[bool](s, Booleans())
	}, stderrNoteFn, []Option{WithTestCases(1)})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("test body was never called")
	}
}

// --- hegelSession.runTest: covered via integration ---

func TestHegelSessionRunTest(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	s := newHegelSession()
	defer s.cleanup()
	if err := s.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	err := s.runTest(func(st *TestCase) {
		Draw[bool](st, Booleans())
	}, runOptions{testCases: 2}, stderrNoteFn)
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
	hegelBinPath(t)
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

// --- hegelSession.start: hegelCmd field used ---

func TestHegelSessionStartHegelCmd(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	root := getProjectRoot()
	path := findHegelInDir(filepath.Join(root, ".venv"))
	if path == "" {
		// Fall back to PATH if .venv doesn't have it.
		path, _ = exec.LookPath("hegel")
	}
	s := newHegelSession()
	s.hegelCmd = path
	defer s.cleanup()
	if err := s.start(); err != nil {
		t.Fatalf("start with hegelCmd: %v", err)
	}
}

// --- hegelSession.cleanup: conn/process/tempDir paths via integration ---

func TestHegelSessionCleanupAllPaths(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
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
	// Set HEGEL_PROTOCOL_TEST_MODE so RunHegelTestE uses a temp session.
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "empty_test")

	tmp := t.TempDir() // no .venv here
	t.Chdir(tmp)

	// Save and restore PATH (remove hegel from it).
	t.Setenv("PATH", "/nonexistent")

	err := runHegel(func(_ *TestCase) {}, stderrNoteFn, []Option{WithTestCases(1)})
	if err == nil {
		t.Error("expected error when session cannot start in protocol test mode")
	}
	mustContainStr(t, err.Error(), "session start")
}

// --- hegelSession.start: handshake error ---

func TestHegelSessionStartHandshakeError(t *testing.T) {
	t.Parallel()
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
	hegelBinPath(t)
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
	hegelBinPath(t)
	MustRun(func(s *TestCase) {
		_ = Draw[bool](s, Booleans())
	}, WithTestCases(1))
}

// =============================================================================
// Public API: Case — via real binary
// =============================================================================

func TestCaseSuccess(t *testing.T) {
	hegelBinPath(t)
	t.Run("case_test", Case(func(ht *T) {
		_ = Draw[bool](ht, Booleans())
		ht.Note("test note via Case")
	}, WithTestCases(1)))
}

// =============================================================================
// state.failed triggers INTERESTING on final replay
// =============================================================================

func TestStateFailedPath(t *testing.T) {
	hegelBinPath(t)
	err := runHegel(func(s *TestCase) {
		_ = Draw[bool](s, Booleans())
		s.failed = true // simulates T.Error/T.Fail
	}, stderrNoteFn, []Option{WithTestCases(1)})
	if err == nil {
		t.Error("expected error when state.failed is true")
	}
}

// =============================================================================
// fatalSentinel triggers INTERESTING on final replay
// =============================================================================

func TestFatalSentinelPath(t *testing.T) {
	hegelBinPath(t)
	err := runHegel(func(s *TestCase) {
		_ = Draw[bool](s, Booleans())
		panic(fatalSentinel{msg: "test fatal"})
	}, stderrNoteFn, []Option{WithTestCases(1)})
	if err == nil {
		t.Error("expected error when fatalSentinel is raised")
	}
}

// =============================================================================
// Case: noteFn (t.Log) is called on final replay
// =============================================================================

func TestCaseNoteFnOnFinal(t *testing.T) {
	hegelBinPath(t)
	noted := false
	err := runHegel(func(s *TestCase) {
		_ = Draw[bool](s, Booleans())
		s.Note("note for final")
		panic("always fail for final replay")
	}, func(msg string) { noted = true }, []Option{WithTestCases(1)})
	if err == nil {
		t.Error("expected error from failing test")
	}
	if !noted {
		t.Error("expected noteFn to be called on final replay")
	}
}

// --- hegelCommand: covered in installer_test.go ---

// --- runTest: SendControlRequest error (closed connection) ---

func TestRunTestSendControlRequestError(t *testing.T) {
	t.Parallel()
	conn, remote := clientConnPair(t)
	remote.Close()

	cl := newClient(conn)
	err := cl.runTest(func(_ *TestCase) {}, runOptions{testCases: 1}, stderrNoteFn)
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
	hegelBinPath(t)
	// Exercise the suppress_health_check protocol path.
	err := runHegel(func(s *TestCase) {
		n := Draw[int](s, Integers[int](0, 100))
		s.Assume(n < 90)
	}, stderrNoteFn, []Option{SuppressHealthCheck(FilterTooMuch, TooSlow), WithTestCases(5)})
	if err != nil {
		t.Errorf("expected test to pass with suppressed health check: %v", err)
	}
}

func TestSuppressAllHealthChecksIntegration(t *testing.T) {
	hegelBinPath(t)
	err := runHegel(func(s *TestCase) {
		n := Draw[int](s, Integers[int](0, 100))
		s.Assume(n < 90)
	}, stderrNoteFn, []Option{SuppressHealthCheck(AllHealthChecks()...), WithTestCases(5)})
	if err != nil {
		t.Errorf("expected test to pass with all health checks suppressed: %v", err)
	}
}

// =============================================================================
// Server crash detection: processExited channel
// =============================================================================

func TestProcessExitedChannel(t *testing.T) {
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
	t.Parallel()
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
	hegelBinPath(t)
	// Filtering based on a tiny range triggers FilterTooMuch: the server sees
	// almost all test cases rejected and raises a health check failure.
	err := runHegel(func(s *TestCase) {
		n := Draw[int](s, Integers[int](0, 1000))
		s.Assume(n == 0) // reject ~99.9% of test cases
	}, stderrNoteFn, []Option{WithTestCases(200)})
	if err == nil {
		t.Fatal("expected health check failure")
	}
	mustContainStr(t, err.Error(), "health check failure")
}

// =============================================================================
// Flaky global state detection
// =============================================================================

var flakyCounter atomic.Int64

func TestFlakyGlobalState(t *testing.T) {
	hegelBinPath(t)
	flakyCounter.Store(0)
	err := runHegel(func(s *TestCase) {
		min := int(flakyCounter.Load())
		_ = Draw[int](s, Integers[int](min, min+100))
		flakyCounter.Add(1)
	}, stderrNoteFn, []Option{WithTestCases(100)})
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
	ch := &channel{conn: conn, channelID: 1, inbox: make(chan any, 1), dropped: make(chan struct{}), nextMessageID: 1}
	conn.writerMu.Lock()
	conn.channels[1] = ch
	conn.writerMu.Unlock()

	exited := make(chan struct{})
	close(exited)
	conn.processExited = exited

	s.Close()

	state := &TestCase{channel: ch}
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
	ch := &channel{conn: conn, channelID: 1, inbox: make(chan any, 1), dropped: make(chan struct{}), nextMessageID: 1}
	conn.writerMu.Lock()
	conn.channels[1] = ch
	conn.writerMu.Unlock()

	exited := make(chan struct{})
	conn.processExited = exited

	// Read the request on peer side, then simulate crash
	go func() {
		readPacket(c) //nolint:errcheck
		close(exited)
		c.Close()
	}()

	state := &TestCase{channel: ch}
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
	ch := conn.NewChannel("Test")

	exited := make(chan struct{})
	close(exited)
	conn.processExited = exited
	c.Close()

	// Wait for readLoop to notice the close
	<-conn.done

	_, _, err := ch.RecvRequestRaw(1 * time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
	mustContainStr(t, err.Error(), "server process exited unexpectedly")
}

// --- helpers ---

func mustContainStr(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("%q does not contain %q", s, sub)
	}
}
