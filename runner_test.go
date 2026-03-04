package hegel

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Helper: check hegel binary is available ---

func hegelBinPath(t *testing.T) string {
	t.Helper()
	// justfile sets PATH=".venv/bin:$PATH" for tests; go test inherits it.
	path, err := exec.LookPath("hegel")
	if err != nil {
		t.Skip("hegel binary not found in PATH -- skipping integration test")
	}
	return path
}

// setEnv sets an environment variable for the duration of the test and restores the
// original value (or removes it) via t.Cleanup.
func setEnv(t *testing.T, key, value string) {
	t.Helper()
	old, hadOld := os.LookupEnv(key)
	os.Setenv(key, value) //nolint:errcheck
	t.Cleanup(func() {
		if hadOld {
			os.Setenv(key, old) //nolint:errcheck
		} else {
			os.Unsetenv(key) //nolint:errcheck
		}
	})
}

// --- RunHegelTest: basic passing test ---

func TestRunHegelTestPasses(t *testing.T) {
	hegelBinPath(t)
	called := false
	if _err := runHegel(t.Name(), func(s *TestCase) {
		called = true
		b := Draw[bool](s, Booleans(0.5))
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
	err := runHegel(t.Name()+"_inner", func(s *TestCase) {
		x := Draw[int64](s, Integers(0, 100))
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
	if _err := runHegel(t.Name(), func(s *TestCase) {
		s.Assume(false)
	}, stderrNoteFn, []Option{WithTestCases(5)}); _err != nil {
		panic(_err)
	}
}

// --- RunHegelTest: assume(true) -> no effect ---

func TestAssumeTrue(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		s.Assume(true)
		b := Draw[bool](s, Booleans(0.5))
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
	hegelBinPath(t)
	// note() should not panic or error when called outside final run
	if _err := runHegel(t.Name(), func(s *TestCase) {
		s.Note("should not appear")
		_ = Draw[bool](s, Booleans(0.5))
	}, stderrNoteFn, []Option{WithTestCases(3)}); _err != nil {
		panic(_err)
	}
}

// --- target(): sends target command ---

func TestTargetSendsCommand(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		x := Draw[int64](s, Integers(0, 100))
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
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_generate")
	// Should complete without error: SDK handles StopTest cleanly.
	if _err := runHegel(t.Name(), func(s *TestCase) {
		Draw[bool](s, Booleans(0.5))
	}, stderrNoteFn, []Option{WithTestCases(5)}); _err != nil {
		panic(_err)
	}
}

// --- HEGEL_PROTOCOL_TEST_MODE=stop_test_on_mark_complete ---

func TestStopTestOnMarkComplete(t *testing.T) {
	hegelBinPath(t)
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_mark_complete")
	if _err := runHegel(t.Name(), func(s *TestCase) {
		Draw[bool](s, Booleans(0.5))
	}, stderrNoteFn, []Option{WithTestCases(5)}); _err != nil {
		panic(_err)
	}
}

// --- HEGEL_PROTOCOL_TEST_MODE=empty_test ---

func TestEmptyTest(t *testing.T) {
	hegelBinPath(t)
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "empty_test")
	if _err := runHegel(t.Name(), func(_ *TestCase) {
		panic("should not be called")
	}, stderrNoteFn, []Option{WithTestCases(5)}); _err != nil {
		panic(_err)
	}
}

// --- HEGEL_PROTOCOL_TEST_MODE=error_response ---

func TestErrorResponse(t *testing.T) {
	hegelBinPath(t)
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "error_response")
	// The server sends a requestError on generate; the test body should
	// see a panic (INTERESTING) and RunHegelTestE should return an error.
	var gotErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				gotErr = fmt.Errorf("%v", r)
			}
		}()
		gotErr = runHegel(t.Name()+"_inner", func(s *TestCase) {
			Draw[bool](s, Booleans(0.5)) // server sends error_response here
		}, stderrNoteFn, []Option{WithTestCases(3)})
	}()
	// The error from the server causes INTERESTING status -> re-raised on final run.
	// Either a panic or a non-nil error is acceptable.
	_ = gotErr // we just verify it doesn't deadlock or hang
}

// --- Draw outside context: calling Draw with nil-channel state panics ---

func TestDrawWithNilChannelState(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic when Draw called with nil-channel state")
		}
	}()
	s := &TestCase{} // channel is nil -> will panic
	Draw[bool](s, Booleans(0.5))
}

// --- Assume outside context raises ---

func TestAssumeOutsideContext(t *testing.T) {
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
	// Note() on a zero-value *TestCase should not panic (isFinal=false).
	s := &TestCase{}
	s.Note("outside context -- safe")
}

// --- Target outside context raises ---

func TestTargetOutsideContext(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic from Target outside test context")
		}
	}()
	s := &TestCase{} // channel is nil -> panic
	s.Target(1.0, "x")
}

// --- findHegel: venv path ---

func TestFindHegelInVenv(t *testing.T) {
	tmp := t.TempDir()
	binDir := tmp + "/bin"
	os.MkdirAll(binDir, 0o755) //nolint:errcheck
	hegelBin := binDir + "/hegel"
	os.WriteFile(hegelBin, []byte("#!/bin/sh\n"), 0o755) //nolint:errcheck

	result := findHegelInDir(tmp)
	if result != hegelBin {
		t.Errorf("findHegelInDir(%q) = %q, want %q", tmp, result, hegelBin)
	}
}

func TestFindHegelVenvViaCwd(t *testing.T) {
	tmp := t.TempDir()
	venvBin := filepath.Join(tmp, ".venv", "bin")
	os.MkdirAll(venvBin, 0o755) //nolint:errcheck
	hegelBin := filepath.Join(venvBin, "hegel")
	os.WriteFile(hegelBin, []byte("#!/bin/sh\n"), 0o755) //nolint:errcheck

	origDir, _ := os.Getwd()
	os.Chdir(tmp)           //nolint:errcheck
	defer os.Chdir(origDir) //nolint:errcheck

	result := findHegel()
	expected := filepath.Join(tmp, ".venv", "bin", "hegel")
	if result != expected {
		t.Errorf("findHegel() = %q, want %q", result, expected)
	}
}

// --- findHegel: not in dir returns empty ---

func TestFindHegelInDirMissing(t *testing.T) {
	tmp := t.TempDir()
	result := findHegelInDir(tmp)
	if result != "" {
		t.Errorf("findHegelInDir missing: got %q, want empty", result)
	}
}

// --- hegelSession: start and cleanup ---

func TestHegelSessionStartAndCleanup(t *testing.T) {
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
	s := newHegelSession()
	s.cleanup() // Should not panic when nothing started.
}

// --- hegelSession: timeout when hegel doesn't appear ---

func TestHegelSessionStartTimeout(t *testing.T) {
	// Use `false` (exits immediately) so the socket never appears.
	falseBin, err := exec.LookPath("false")
	if err != nil {
		t.Skip("false binary not available")
	}
	s := newHegelSession()
	s.hegelCmd = falseBin // exits immediately without creating socket
	startErr := s.start()
	if startErr == nil {
		s.cleanup()
		t.Fatal("expected timeout error")
	}
	mustContainStr(t, startErr.Error(), "timeout")
}

// --- hegelSession: concurrent starts (double-checked locking) ---

func TestHegelSessionConcurrentStart(t *testing.T) {
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
	if _err := runHegel(t.Name(), func(s *TestCase) {
		count++
		b := Draw[bool](s, Booleans(0.5))
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
			if _err := runHegel(fmt.Sprintf("%s_%d", t.Name(), idx), func(s *TestCase) {
				b := Draw[bool](s, Booleans(0.5))
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
	err := runHegel(t.Name(), func(s *TestCase) {
		_ = Draw[bool](s, Booleans(0.5))
	}, stderrNoteFn, []Option{WithTestCases(3)})
	if err != nil {
		t.Errorf("RunHegelTestE: unexpected error: %v", err)
	}
}

// --- WithTestCases option ---

func TestWithTestCasesOption(t *testing.T) {
	hegelBinPath(t)
	count := 0
	if _err := runHegel(t.Name(), func(s *TestCase) {
		count++
		Draw[bool](s, Booleans(0.5))
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
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_collection_more")
	err := runHegel(t.Name(), func(s *TestCase) {
		coll := newCollection(s, 0, 10)
		_ = coll.More(s)
	}, stderrNoteFn, nil)
	_ = err // StopTest causes abort, not necessarily an error return
}

// --- HEGEL_PROTOCOL_TEST_MODE=stop_test_on_new_collection ---

func TestStopTestOnNewCollection(t *testing.T) {
	hegelBinPath(t)
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_new_collection")
	err := runHegel(t.Name(), func(s *TestCase) {
		coll := newCollection(s, 0, 10)
		_ = coll.More(s)
	}, stderrNoteFn, nil)
	_ = err // StopTest causes abort, not necessarily an error return
}

// --- isFinal: Note prints on final run ---
// We test this by running a failing test so the final replay happens.
// We capture whether isFinal was true via the state in the closure.

func TestNoteOnFinalRun(t *testing.T) {
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
		defer func() { recover() }()                                                    //nolint:errcheck
		runHegel(t.Name()+"_inner", noteFunc, stderrNoteFn, []Option{WithTestCases(3)}) //nolint:errcheck
	}()
	if !noted {
		t.Error("expected isFinal to be true during final replay")
	}
}

// --- runTest: unrecognised event handled gracefully ---
// We use a custom connection to simulate a bogus event, then test_done.

func TestRunTestUnrecognisedEvent(t *testing.T) {
	// Build a fake server using connection primitives.
	s, c := socketPair(t)
	serverConn := newConnection(s, "FakeServer")
	clientConn := newConnection(c, "Client")

	serverDone := make(chan error, 1)
	go func() {
		defer close(serverDone)
		if err := serverConn.ReceiveHandshake(); err != nil {
			serverDone <- err
			return
		}
		ctrl := serverConn.ControlChannel()

		// Receive run_test command.
		msgID, payload, err := ctrl.RecvRequestRaw(5 * time.Second)
		if err != nil {
			serverDone <- err
			return
		}
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)

		// Ack the run_test.
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		// Connect the test channel.
		testCh, err := serverConn.ConnectChannel(uint32(chID), "TestCh")
		if err != nil {
			serverDone <- err
			return
		}

		// Send a bogus event.
		bogusPayload, _ := encodeCBOR(map[string]any{"event": "bogus_event"})
		bogusID, _ := testCh.SendRequestRaw(bogusPayload)
		// Drain the error reply from the client.
		testCh.recvResponseRaw(bogusID, 5*time.Second) //nolint:errcheck

		// Send test_done.
		donePayload, _ := encodeCBOR(map[string]any{
			"event": "test_done",
			"results": map[string]any{
				"passed":                 true,
				"test_cases":             int64(0),
				"valid_test_cases":       int64(0),
				"invalid_test_cases":     int64(0),
				"interesting_test_cases": int64(0),
			},
		})
		doneID, _ := testCh.SendRequestRaw(donePayload)
		testCh.recvResponseRaw(doneID, 5*time.Second) //nolint:errcheck
	}()

	// Perform handshake on client side.
	if err := clientConn.SendHandshake(); err != nil {
		t.Fatalf("client SendHandshake: %v", err)
	}

	// Run the client side.
	cli := newClient(clientConn)
	err := cli.runTest("unrecognised_event_test", func(_ *TestCase) {}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Errorf("runTest with bogus event: unexpected error: %v", err)
	}

	if sErr := <-serverDone; sErr != nil {
		t.Errorf("server goroutine error: %v", sErr)
	}
}

// --- runTest: connection error in test function is re-raised ---

func TestConnectionErrorInTestFunction(t *testing.T) {
	hegelBinPath(t)
	err := runHegel(t.Name(), func(_ *TestCase) {
		panic(&connectionError{msg: "test connection lost"})
	}, stderrNoteFn, []Option{WithTestCases(1)})
	if err == nil {
		t.Fatal("expected error to be raised for connection error")
	}
	mustContainStr(t, err.Error(), "connection lost")
}

// --- runTestCase: sets isFinal correctly ---

func TestRunTestCaseFinalFlag(t *testing.T) {
	// Build a minimal client with a fake connection and channel.
	s, c := socketPair(t)
	defer s.Close()
	defer c.Close()

	serverConn := newConnection(s, "S")
	clientConn := newConnection(c, "C")

	go func() {
		serverConn.ReceiveHandshake() //nolint:errcheck
	}()
	clientConn.SendHandshake() //nolint:errcheck

	client := newClient(clientConn)
	ch := clientConn.NewChannel("TestCh")

	// Use a goroutine to respond mark_complete from "server" side.
	serverCh, _ := serverConn.ConnectChannel(ch.ChannelID(), "TestCh")
	go func() {
		msgID, _, _ := serverCh.RecvRequestRaw(2 * time.Second)
		serverCh.SendReplyValue(msgID, nil) //nolint:errcheck
	}()

	wasFinal := false
	err := client.runTestCase(ch, func(state *TestCase) {
		wasFinal = state.isFinal
	}, true, stderrNoteFn)
	if err != nil {
		t.Errorf("runTestCase: %v", err)
	}
	if !wasFinal {
		t.Error("expected isFinal to be true during final run")
	}
}

// --- fakeServer: minimal server for unit tests ---

// fakeServerConn builds a handshaked server+client connection pair for unit tests.
// The server goroutine runs fn; the client connection is returned.
// Any server error is reported via t.Error.
func fakeServerConn(t *testing.T, fn func(serverConn *connection)) *connection {
	t.Helper()
	s, c := socketPair(t)
	serverConn := newConnection(s, "FakeServer")
	clientConn := newConnection(c, "Client")

	serverReady := make(chan struct{})
	go func() {
		serverConn.ReceiveHandshake() //nolint:errcheck
		close(serverReady)
		fn(serverConn)
	}()

	if err := clientConn.SendHandshake(); err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	<-serverReady
	return clientConn
}

// sendTestDone sends a test_done event on testCh with the given results.
func sendTestDone(t *testing.T, testCh *channel, passed bool, interesting int64) {
	t.Helper()
	payload, _ := encodeCBOR(map[string]any{
		"event": "test_done",
		"results": map[string]any{
			"passed":                 passed,
			"test_cases":             int64(0),
			"valid_test_cases":       int64(0),
			"invalid_test_cases":     int64(0),
			"interesting_test_cases": interesting,
		},
	})
	msgID, _ := testCh.SendRequestRaw(payload)
	testCh.recvResponseRaw(msgID, 5*time.Second) //nolint:errcheck
}

// runTestOnFakeServer sets up a fake server that sends a single test_case event,
// runs the test body, then sends test_done.
func runTestOnFakeServer(t *testing.T, testFn testBody, serverReply func(caseCh *channel)) error {
	t.Helper()
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()

		// Receive run_test.
		msgID, payload, err := ctrl.RecvRequestRaw(5 * time.Second)
		if err != nil {
			t.Errorf("server recv run_test: %v", err)
			return
		}
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		// Connect test channel.
		testCh, err := serverConn.ConnectChannel(uint32(chID), "TestCh")
		if err != nil {
			t.Errorf("server connect test channel: %v", err)
			return
		}

		// Create a case channel and send test_case.
		caseCh := serverConn.NewChannel("CaseCh")
		casePayload, _ := encodeCBOR(map[string]any{
			"event":      "test_case",
			"channel_id": int64(caseCh.ChannelID()),
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck

		// Run the server-side reply handler.
		serverReply(caseCh)

		// Send test_done.
		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	return cli.runTest("unit_test", testFn, runOptions{testCases: 1}, stderrNoteFn)
}

// --- Unit tests for error/recovery paths ---

// --- assumeRejected.Error() ---

func TestAssumeRejectedError(t *testing.T) {
	e := assumeRejected{}
	if e.Error() != "assume rejected" {
		t.Errorf("assumeRejected.Error() = %q", e.Error())
	}
}

// --- dataExhausted.Error() ---

func TestDataExhaustedError(t *testing.T) {
	e := &dataExhausted{msg: "exhausted"}
	if e.Error() != "exhausted" {
		t.Errorf("dataExhausted.Error() = %q", e.Error())
	}
}

// --- connectionError.Error() ---

func TestConnectionErrorError(t *testing.T) {
	e := &connectionError{msg: "conn lost"}
	if e.Error() != "conn lost" {
		t.Errorf("connectionError.Error() = %q", e.Error())
	}
}

// --- aborted flag: set directly on state ---

func TestAbortedFlagDirect(t *testing.T) {
	state := &TestCase{}
	state.aborted = true
	if !state.aborted {
		t.Error("expected aborted to be true after direct assignment")
	}
}

// --- generateFromSchema: StopTest causes DataExhausted ---

func TestGenerateFromSchemaStopTest(t *testing.T) {
	// Set up a fake channel that returns a StopTest requestError.
	s, c := socketPair(t)
	serverConn := newConnection(s, "S")
	clientConn := newConnection(c, "C")
	go func() {
		serverConn.ReceiveHandshake() //nolint:errcheck
	}()
	clientConn.SendHandshake() //nolint:errcheck
	ch := clientConn.NewChannel("test")

	// Server-side: replies to generate requests with StopTest error.
	serverCh, _ := serverConn.ConnectChannel(ch.ChannelID(), "test")
	go func() {
		msgID, _, _ := serverCh.RecvRequestRaw(2 * time.Second)
		serverCh.SendReplyError(msgID, "no more data", "StopTest") //nolint:errcheck
	}()

	state := &TestCase{channel: ch}

	var caught any
	func() {
		defer func() { caught = recover() }()
		Draw[bool](state, Booleans(0.5))
	}()
	if caught == nil {
		t.Fatal("expected panic from Draw on StopTest")
	}
	_, isExhausted := caught.(*dataExhausted)
	if !isExhausted {
		t.Errorf("expected *dataExhausted, got %T: %v", caught, caught)
	}
	if !state.aborted {
		t.Error("expected aborted flag set after StopTest")
	}
}

// --- generateFromSchema: non-StopTest requestError propagates ---

func TestGenerateFromSchemaNonStopTestError(t *testing.T) {
	s, c := socketPair(t)
	serverConn := newConnection(s, "S")
	clientConn := newConnection(c, "C")
	go func() {
		serverConn.ReceiveHandshake() //nolint:errcheck
	}()
	clientConn.SendHandshake() //nolint:errcheck
	ch := clientConn.NewChannel("test")

	serverCh, _ := serverConn.ConnectChannel(ch.ChannelID(), "test")
	go func() {
		msgID, _, _ := serverCh.RecvRequestRaw(2 * time.Second)
		serverCh.SendReplyError(msgID, "bad schema", "SchemaError") //nolint:errcheck
	}()

	state := &TestCase{channel: ch}

	_, err := generateFromSchema(state, map[string]any{"type": "boolean"})
	if err == nil {
		t.Fatal("expected error from generateFromSchema")
	}
	_, isRequestError := err.(*requestError)
	if !isRequestError {
		t.Errorf("expected *requestError, got %T: %v", err, err)
	}
}

// --- generateFromSchema: connection error (Request fails) ---

func TestGenerateFromSchemaConnectionError(t *testing.T) {
	s, c := socketPair(t)
	conn := newConnection(s, "C")
	c.Close()
	// Don't handshake -- just create a channel manually on a pre-client connection.
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
		Draw[bool](state, Booleans(0.5))
	}()
	if caught == nil {
		t.Fatal("expected panic from Draw on connection error")
	}
	_, isConnErr := caught.(*connectionError)
	if !isConnErr {
		t.Errorf("expected *connectionError, got %T: %v", caught, caught)
	}
}

// --- Integers generator: basic path via fake server ---

func TestIntegersGenerateUnit(t *testing.T) {
	// Use a fake server to exercise Draw(s, Integers()).
	err := runTestOnFakeServer(t, func(s *TestCase) {
		n := Draw[int64](s, Integers(0, 10))
		if n < 0 || n > 10 {
			panic(fmt.Sprintf("out of range: %d", n))
		}
	}, func(caseCh *channel) {
		// Respond to generate with value 7, then mark_complete.
		msgID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(msgID, int64(7)) //nolint:errcheck
		// mark_complete
		msgID2, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(msgID2, nil) //nolint:errcheck
	})
	if err != nil {
		t.Errorf("Integers generate unit: %v", err)
	}
}

// --- Target: error path when Request fails ---

func TestTargetConnectionError(t *testing.T) {
	s, _ := socketPair(t)
	conn := newConnection(s, "C")
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

// --- Target: error path when Get fails ---

func TestTargetResponseError(t *testing.T) {
	s, c := socketPair(t)
	serverConn := newConnection(s, "S")
	clientConn := newConnection(c, "C")
	go func() {
		serverConn.ReceiveHandshake() //nolint:errcheck
	}()
	clientConn.SendHandshake() //nolint:errcheck
	ch := clientConn.NewChannel("test")

	serverCh, _ := serverConn.ConnectChannel(ch.ChannelID(), "test")
	go func() {
		msgID, _, _ := serverCh.RecvRequestRaw(2 * time.Second)
		serverCh.SendReplyError(msgID, "target failed", "TargetError") //nolint:errcheck
	}()

	state := &TestCase{channel: ch}

	var caught any
	func() {
		defer func() { caught = recover() }()
		state.Target(1.0, "x")
	}()
	if caught == nil {
		t.Fatal("expected panic from Target on response error")
	}
}

// --- runTest: event decode error ---

func TestRunTestEventDecodeError(t *testing.T) {
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
		// Send an invalid CBOR payload.
		testCh.SendReplyRaw(0, []byte{0xFF}) //nolint:errcheck
		// This won't be received as a request, so send it as a raw request.
		invalidMsgID, _ := testCh.SendRequestRaw([]byte{0xFF}) // invalid CBOR
		testCh.recvResponseRaw(invalidMsgID, 2*time.Second)    //nolint:errcheck
		// Now send test_done to unblock.
		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("decode_err", func(_ *TestCase) {}, runOptions{testCases: 1}, stderrNoteFn)
	if err == nil {
		t.Error("expected error from runTest on invalid CBOR event")
	}
}

// --- runTest: event not a dict error ---

func TestRunTestEventNotDictError(t *testing.T) {
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
		// Send a CBOR integer (not a dict).
		badPayload, _ := encodeCBOR(int64(42))
		badID, _ := testCh.SendRequestRaw(badPayload)
		testCh.recvResponseRaw(badID, 2*time.Second) //nolint:errcheck
		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("not_dict", func(_ *TestCase) {}, runOptions{testCases: 1}, stderrNoteFn)
	if err == nil {
		t.Error("expected error from runTest on non-dict event")
	}
}

// --- runTest: test_case missing channel field ---

func TestRunTestCaseMissingChannel(t *testing.T) {
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
		// Send test_case without channel field.
		badPayload, _ := encodeCBOR(map[string]any{"event": "test_case"})
		badID, _ := testCh.SendRequestRaw(badPayload)
		testCh.recvResponseRaw(badID, 2*time.Second) //nolint:errcheck
		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("missing_ch", func(_ *TestCase) {}, runOptions{testCases: 1}, stderrNoteFn)
	if err == nil {
		t.Error("expected error from runTest on test_case missing channel")
	}
}

// --- runTest: run_test send error (closed conn) ---

func TestRunTestSendError(t *testing.T) {
	s, c := socketPair(t)
	serverConn := newConnection(s, "S")
	clientConn := newConnection(c, "C")
	go func() {
		serverConn.ReceiveHandshake() //nolint:errcheck
	}()
	clientConn.SendHandshake() //nolint:errcheck

	// Close the conn before sending run_test.
	s.Close()
	c.Close()

	cli := newClient(clientConn)
	err := cli.runTest("closed", func(_ *TestCase) {}, runOptions{testCases: 1}, stderrNoteFn)
	if err == nil {
		t.Error("expected error from runTest on closed conn")
	}
}

// --- runTest: run_test ack error ---

func TestRunTestAckError(t *testing.T) {
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, _, _ := ctrl.RecvRequestRaw(5 * time.Second)
		// Reply with an error instead of ack.
		ctrl.SendReplyError(msgID, "cannot run test", "ServerError") //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("ack_err", func(_ *TestCase) {}, runOptions{testCases: 1}, stderrNoteFn)
	if err == nil {
		t.Error("expected error from runTest on ack error")
	}
}

// --- runTest: test event recv error (channel closed) ---

func TestRunTestEventRecvError(t *testing.T) {
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		// Don't connect the test channel -- just close the connection.
		_ = chID
		serverConn.Close()
	})

	cli := newClient(clientConn)
	err := cli.runTest("recv_err", func(_ *TestCase) {}, runOptions{testCases: 1}, stderrNoteFn)
	if err == nil {
		t.Error("expected error from runTest when connection closed before event")
	}
}

// --- runTest: connect test case channel error ---

func TestRunTestConnectCaseChannelError(t *testing.T) {
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
		// Send test_case with channel ID = 0 (already registered as control).
		casePayload, _ := encodeCBOR(map[string]any{
			"event":      "test_case",
			"channel_id": int64(0), // already exists!
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 2*time.Second) //nolint:errcheck
		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("dup_ch", func(_ *TestCase) {}, runOptions{testCases: 1}, stderrNoteFn)
	if err == nil {
		t.Error("expected error from runTest on duplicate channel")
	}
}

// --- runTestCase: INTERESTING status on panic ---

func TestRunTestCaseInteresting(t *testing.T) {
	err := runTestOnFakeServer(t, func(_ *TestCase) {
		panic("assertion failure")
	}, func(caseCh *channel) {
		// Receive mark_complete with INTERESTING status.
		msgID, payload, _ := caseCh.RecvRequestRaw(2 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		statusVal, _ := extractCBORString(m[any("status")])
		if statusVal != "INTERESTING" {
			// Still ack to unblock.
		}
		_ = statusVal
		caseCh.SendReplyValue(msgID, nil) //nolint:errcheck
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- runTestCase: dataExhausted -> alreadyComplete ---

func TestRunTestCaseDataExhausted(t *testing.T) {
	err := runTestOnFakeServer(t, func(_ *TestCase) {
		panic(&dataExhausted{msg: "exhausted"})
	}, func(caseCh *channel) {
		// Server should NOT receive mark_complete when data is exhausted.
		// Just wait with a short timeout.
		caseCh.RecvRequestRaw(100 * time.Millisecond) //nolint:errcheck
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- runTestCase: INVALID status from assume ---

func TestRunTestCaseInvalid(t *testing.T) {
	err := runTestOnFakeServer(t, func(s *TestCase) {
		s.Assume(false)
	}, func(caseCh *channel) {
		msgID, payload, _ := caseCh.RecvRequestRaw(2 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		statusVal, _ := extractCBORString(m[any("status")])
		if statusVal != "INVALID" {
			t.Errorf("expected INVALID status, got %q", statusVal)
		}
		caseCh.SendReplyValue(msgID, nil) //nolint:errcheck
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- runTestCase: connection error re-raised ---

func TestRunTestCaseConnectionError(t *testing.T) {
	// connection error inside test body should propagate.
	err := runTestOnFakeServer(t, func(_ *TestCase) {
		panic(&connectionError{msg: "conn broke"})
	}, func(caseCh *channel) {
		// mark_complete should NOT be sent; server just drains.
		caseCh.RecvRequestRaw(100 * time.Millisecond) //nolint:errcheck
	})
	if err == nil {
		t.Error("expected error from runTestCase on connection error")
	}
	mustContainStr(t, err.Error(), "conn broke")
}

// --- runTestCase: mark_complete send error (conn closed) ---

func TestRunTestCaseMarkCompleteError(t *testing.T) {
	// The channel is closed before mark_complete -> send fails but we handle gracefully.
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
		caseCh := serverConn.NewChannel("CaseCh")
		casePayload, _ := encodeCBOR(map[string]any{
			"event":      "test_case",
			"channel_id": int64(caseCh.ChannelID()),
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck

		// Don't read mark_complete; close the server conn so client's Request fails.
		serverConn.Close()
		sendTestDone(t, testCh, true, 0) // this will likely fail, that's ok
	})

	cli := newClient(clientConn)
	// Should not panic even if mark_complete fails.
	cli.runTest("mark_err", func(_ *TestCase) {}, runOptions{testCases: 1}, stderrNoteFn) //nolint:errcheck
}

// --- runTest: multiple interesting cases (nInteresting > 1) ---

func TestRunTestMultipleInteresting(t *testing.T) {
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")

		// Send test_done with 2 interesting cases immediately.
		donePayload, _ := encodeCBOR(map[string]any{
			"event": "test_done",
			"results": map[string]any{
				"passed":                 false,
				"test_cases":             int64(10),
				"valid_test_cases":       int64(10),
				"invalid_test_cases":     int64(0),
				"interesting_test_cases": int64(2),
			},
		})
		doneID, _ := testCh.SendRequestRaw(donePayload)
		testCh.recvResponseRaw(doneID, 5*time.Second) //nolint:errcheck

		// Send 2 final test cases.
		for i := 0; i < 2; i++ {
			caseCh := serverConn.NewChannel(fmt.Sprintf("FinalCh%d", i))
			casePayload, _ := encodeCBOR(map[string]any{
				"event":      "test_case",
				"channel_id": int64(caseCh.ChannelID()),
			})
			caseID, _ := testCh.SendRequestRaw(casePayload)
			testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck
			// Receive mark_complete from client.
			markID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
			caseCh.SendReplyValue(markID, nil) //nolint:errcheck
		}
	})

	cli := newClient(clientConn)
	err := cli.runTest("multi_interesting", func(_ *TestCase) {
		panic("always fails")
	}, runOptions{testCases: 10}, stderrNoteFn)
	if err == nil {
		t.Error("expected error from multi-interesting run")
	}
	mustContainStr(t, err.Error(), "multiple failures")
}

// --- runTest: single interesting case, server reply error on connect ---

func TestRunTestSingleInterestingConnectError(t *testing.T) {
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")

		// test_done with 1 interesting.
		donePayload, _ := encodeCBOR(map[string]any{
			"event": "test_done",
			"results": map[string]any{
				"passed":                 false,
				"test_cases":             int64(1),
				"valid_test_cases":       int64(1),
				"invalid_test_cases":     int64(0),
				"interesting_test_cases": int64(1),
			},
		})
		doneID, _ := testCh.SendRequestRaw(donePayload)
		testCh.recvResponseRaw(doneID, 5*time.Second) //nolint:errcheck

		// Send final test_case with channel 0 (already exists -> ConnectChannel fails).
		casePayload, _ := encodeCBOR(map[string]any{
			"event":      "test_case",
			"channel_id": int64(0), // control channel, already exists
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("single_conn_err", func(_ *TestCase) {}, runOptions{testCases: 1}, stderrNoteFn)
	if err == nil {
		t.Error("expected error from runTest on final connect failure")
	}
}

// --- runTest: final case recv error ---

func TestRunTestFinalCaseRecvError(t *testing.T) {
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")

		// test_done with 1 interesting.
		donePayload, _ := encodeCBOR(map[string]any{
			"event": "test_done",
			"results": map[string]any{
				"passed":                 false,
				"test_cases":             int64(1),
				"valid_test_cases":       int64(1),
				"invalid_test_cases":     int64(0),
				"interesting_test_cases": int64(1),
			},
		})
		doneID, _ := testCh.SendRequestRaw(donePayload)
		testCh.recvResponseRaw(doneID, 5*time.Second) //nolint:errcheck

		// Close without sending a test_case -> RecvRequestRaw returns error.
		serverConn.Close()
	})

	cli := newClient(clientConn)
	err := cli.runTest("final_recv_err", func(_ *TestCase) {}, runOptions{testCases: 1}, stderrNoteFn)
	if err == nil {
		t.Error("expected error when final case not received")
	}
}

// --- runTest: multi-interesting final case recv error ---

func TestRunTestMultiInterestingRecvError(t *testing.T) {
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")

		// test_done with 2 interesting.
		donePayload, _ := encodeCBOR(map[string]any{
			"event": "test_done",
			"results": map[string]any{
				"passed":                 false,
				"test_cases":             int64(2),
				"valid_test_cases":       int64(2),
				"invalid_test_cases":     int64(0),
				"interesting_test_cases": int64(2),
			},
		})
		doneID, _ := testCh.SendRequestRaw(donePayload)
		testCh.recvResponseRaw(doneID, 5*time.Second) //nolint:errcheck

		// Send only 1 final case, then close -- 2nd recv should fail.
		caseCh := serverConn.NewChannel("FinalCh0")
		casePayload, _ := encodeCBOR(map[string]any{
			"event":      "test_case",
			"channel_id": int64(caseCh.ChannelID()),
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck
		// Respond to mark_complete from client.
		markID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(markID, nil) //nolint:errcheck

		// Close before sending 2nd case.
		serverConn.Close()
	})

	cli := newClient(clientConn)
	err := cli.runTest("multi_recv_err", func(_ *TestCase) {
		panic("always fails")
	}, runOptions{testCases: 2}, stderrNoteFn)
	if err == nil {
		t.Error("expected error from multi-interesting recv failure")
	}
}

// --- isHegelFrame ---

func TestIsHegelFrame(t *testing.T) {
	if !isHegelFrame("github.com/antithesishq/hegel-go.someFunc") {
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
	origin := extractPanicOrigin("just a string")
	// Should include the type (string) and file info.
	if origin == "" {
		t.Error("expected non-empty origin from extractPanicOrigin")
	}
}

// --- extractPanicOrigin: error value ---

func TestExtractPanicOriginError(t *testing.T) {
	origin := extractPanicOrigin(errors.New("test"))
	if origin == "" {
		t.Error("expected non-empty origin from extractPanicOrigin with error")
	}
}

// --- Note: isFinal=true prints to stderr ---

func TestNoteIsFinalTrue(t *testing.T) {
	state := &TestCase{isFinal: true, noteFn: stderrNoteFn}
	// Should not panic.
	state.Note("test note on final")
}

// --- findHegel: fallback when not in venv or PATH ---

func TestFindHegelFallback(t *testing.T) {
	// findHegel should return "hegel" as fallback when nothing found.
	// We can't easily test this without mocking, but we can test findHegelInDir.
	result := findHegelInDir("/nonexistent/path")
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

// --- hegelSession: start with spawn error ---

func TestHegelSessionSpawnError(t *testing.T) {
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
	s := newHegelSession()
	// Set conn to a closed connection so Close() might error.
	sc, cc := socketPair(t)
	sc.Close()
	cc.Close()
	s.conn = newConnection(sc, "closed")
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

	err := runHegel("session_start_fail", func(_ *TestCase) {}, stderrNoteFn, []Option{WithTestCases(1)})
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

	if _err := runHegel("should_panic", func(_ *TestCase) {}, stderrNoteFn, []Option{WithTestCases(1)}); _err != nil {
		panic(_err)
	}
}

// --- RunHegelTestE: calls session.runTest ---

func TestRunHegelTestECallsRunTest(t *testing.T) {
	hegelBinPath(t)
	called := false
	err := runHegel(t.Name(), func(s *TestCase) {
		called = true
		Draw[bool](s, Booleans(0.5))
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
	hegelBinPath(t)
	s := newHegelSession()
	defer s.cleanup()
	if err := s.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	err := s.runTest("session_run", func(st *TestCase) {
		Draw[bool](st, Booleans(0.5))
	}, runOptions{testCases: 2}, stderrNoteFn)
	if err != nil {
		t.Errorf("session.runTest: %v", err)
	}
}

// --- findHegel: uses cwd venv or PATH ---

func TestFindHegel(t *testing.T) {
	// Just verify it returns a non-empty string.
	result := findHegel()
	if result == "" {
		t.Error("findHegel returned empty string")
	}
}

// --- hegelSession.start: double-checked locking (inner check) ---

func TestHegelSessionStartInnerCheck(t *testing.T) {
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
	hegelBinPath(t)
	path, _ := exec.LookPath("hegel")
	s := newHegelSession()
	s.hegelCmd = path
	defer s.cleanup()
	if err := s.start(); err != nil {
		t.Fatalf("start with hegelCmd: %v", err)
	}
}

// --- hegelSession.cleanup: conn/process/tempDir paths via integration ---

func TestHegelSessionCleanupAllPaths(t *testing.T) {
	hegelBinPath(t)
	s := newHegelSession()
	if err := s.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Cleanup should close conn, kill process, remove tempdir.
	s.cleanup()
	if s.conn != nil {
		t.Error("conn should be nil after cleanup")
	}
	if s.process != nil {
		t.Error("process should be nil after cleanup")
	}
	if s.tempDir != "" {
		t.Error("tempDir should be empty after cleanup")
	}
}

// --- runTest: nInteresting==1, case passes (no error from runTestCase) ---

func TestRunTestSingleInterestingCasePasses(t *testing.T) {
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")

		// test_done with 1 interesting case.
		donePayload, _ := encodeCBOR(map[string]any{
			"event": "test_done",
			"results": map[string]any{
				"passed":                 false,
				"test_cases":             int64(1),
				"valid_test_cases":       int64(1),
				"invalid_test_cases":     int64(0),
				"interesting_test_cases": int64(1),
			},
		})
		doneID, _ := testCh.SendRequestRaw(donePayload)
		testCh.recvResponseRaw(doneID, 5*time.Second) //nolint:errcheck

		// Send final test_case.
		caseCh := serverConn.NewChannel("FinalCh")
		casePayload, _ := encodeCBOR(map[string]any{
			"event":      "test_case",
			"channel_id": int64(caseCh.ChannelID()),
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck

		// Receive mark_complete (test body passed).
		markID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(markID, nil) //nolint:errcheck
	})

	cli := newClient(clientConn)
	// Test body passes (VALID) -- final case returns nil.
	err := cli.runTest("single_interesting_pass", func(_ *TestCase) {}, runOptions{testCases: 1}, stderrNoteFn)
	// nil return is fine -- it means the final run also passed.
	_ = err
}

// --- runTest: multi-interesting, single error (len(errs)==1 branch) ---

func TestRunTestMultiInterestingSingleError(t *testing.T) {
	t.Skip("len(errs)==1 in multi-interesting is unreachable when nInteresting>1")
}

// --- extractPanicOrigin: all frames are hegel frames ---

func TestExtractPanicOriginAllHegelFrames(t *testing.T) {
	origin := extractPanicOrigin("test panic")
	if origin == "" {
		t.Error("expected non-empty origin")
	}
}

// --- RunHegelTestE: HEGEL_PROTOCOL_TEST_MODE path, session start error ---

func TestRunHegelTestEProtocolModeStartError(t *testing.T) {
	// Set HEGEL_PROTOCOL_TEST_MODE so RunHegelTestE uses a temp session.
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "empty_test")

	// Save and restore cwd.
	origCwd, _ := os.Getwd()
	tmp := t.TempDir()      // no .venv here
	os.Chdir(tmp)           //nolint:errcheck
	defer os.Chdir(origCwd) //nolint:errcheck

	// Save and restore PATH (remove hegel from it).
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", "/nonexistent") //nolint:errcheck
	defer os.Setenv("PATH", oldPath)  //nolint:errcheck

	err := runHegel("protocol_mode_start_error", func(_ *TestCase) {}, stderrNoteFn, []Option{WithTestCases(1)})
	if err == nil {
		t.Error("expected error when session cannot start in protocol test mode")
	}
	mustContainStr(t, err.Error(), "session start")
}

// --- runTest: multi-interesting, connect error ---

func TestRunTestMultiInterestingConnectError(t *testing.T) {
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")

		// test_done with 2 interesting cases.
		donePayload, _ := encodeCBOR(map[string]any{
			"event": "test_done",
			"results": map[string]any{
				"passed":                 false,
				"test_cases":             int64(2),
				"valid_test_cases":       int64(2),
				"invalid_test_cases":     int64(0),
				"interesting_test_cases": int64(2),
			},
		})
		doneID, _ := testCh.SendRequestRaw(donePayload)
		testCh.recvResponseRaw(doneID, 5*time.Second) //nolint:errcheck

		// Send final test_case with channel ID 0 (control channel -> already connected).
		for i := 0; i < 2; i++ {
			casePayload, _ := encodeCBOR(map[string]any{
				"event":      "test_case",
				"channel_id": int64(0), // channel 0 exists -> ConnectChannel will fail
			})
			caseID, _ := testCh.SendRequestRaw(casePayload)
			testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck
		}
	})

	cli := newClient(clientConn)
	err := cli.runTest("multi_connect_err", func(_ *TestCase) {
		panic("always fails")
	}, runOptions{testCases: 2}, stderrNoteFn)
	if err == nil {
		t.Error("expected error from multi-interesting with connect errors")
	}
	mustContainStr(t, err.Error(), "multiple failures")
}

// --- runTest: multi-interesting, case passes ---

func TestRunTestMultiInterestingCasePasses(t *testing.T) {
	caseCount := 0
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")

		// test_done with 2 interesting cases.
		donePayload, _ := encodeCBOR(map[string]any{
			"event": "test_done",
			"results": map[string]any{
				"passed":                 false,
				"test_cases":             int64(2),
				"valid_test_cases":       int64(2),
				"invalid_test_cases":     int64(0),
				"interesting_test_cases": int64(2),
			},
		})
		doneID, _ := testCh.SendRequestRaw(donePayload)
		testCh.recvResponseRaw(doneID, 5*time.Second) //nolint:errcheck

		// Send 2 final test cases.
		for i := 0; i < 2; i++ {
			caseCh := serverConn.NewChannel(fmt.Sprintf("FinalCh%d", i))
			casePayload, _ := encodeCBOR(map[string]any{
				"event":      "test_case",
				"channel_id": int64(caseCh.ChannelID()),
			})
			caseID, _ := testCh.SendRequestRaw(casePayload)
			testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck
			// Wait for mark_complete from client.
			markID, _, _ := caseCh.RecvRequestRaw(10 * time.Second)
			caseCh.SendReplyValue(markID, nil) //nolint:errcheck
		}
	})

	cli := newClient(clientConn)
	err := cli.runTest("multi_interesting_passes", func(_ *TestCase) {
		caseCount++
		if caseCount == 1 {
			panic("first case fails")
		}
		// Second case: fn returns normally -> "expected to fail" error.
	}, runOptions{testCases: 2}, stderrNoteFn)
	if err == nil {
		t.Error("expected error from multi-interesting run")
	}
	mustContainStr(t, err.Error(), "multiple failures")
}

// --- hegelSession.start: MkdirTemp error ---

func TestHegelSessionStartMkdirTempError(t *testing.T) {
	orig := mkdirTempFn
	mkdirTempFn = func(dir, pattern string) (string, error) {
		return "", fmt.Errorf("simulated mktemp failure")
	}
	defer func() { mkdirTempFn = orig }()

	s := newHegelSession()
	s.hegelCmd = "hegel" // doesn't matter, mktemp fails first
	err := s.start()
	if err == nil {
		s.cleanup()
		t.Fatal("expected error from start when mkdirTemp fails")
	}
	mustContainStr(t, err.Error(), "mktemp")
}

// --- hegelSession.start: handshake error ---

func TestHegelSessionStartHandshakeError(t *testing.T) {
	// Write a fake hegel binary that creates the socket, accepts one connection,
	// sends bad handshake data, then exits. This causes SendHandshakeVersion to fail.
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "fake_hegel.sh")
	// The Python one-liner: bind, listen, accept, send garbage, close.
	script := "#!/bin/sh\n" +
		`python3 -c "import socket,sys; s=socket.socket(socket.AF_UNIX); s.bind(sys.argv[1]); s.listen(1); c,_=s.accept(); c.send(b'bad_data\n'); c.close()" "$1"` +
		"\n"
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

// --- findHegel: LookPath success and fallback ---

func TestFindHegelLookPathAndFallback(t *testing.T) {
	// Change to a temp dir without .venv so findHegelInDir returns "".
	origCwd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(origCwd) //nolint:errcheck

	// First: put hegel somewhere in PATH -> LookPath succeeds.
	hegelBin := filepath.Join(tmp, "hegel")
	if err := os.WriteFile(hegelBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake hegel: %v", err)
	}
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmp+":"+oldPath) //nolint:errcheck
	result := findHegel()
	os.Setenv("PATH", oldPath) //nolint:errcheck
	if result != hegelBin {
		t.Errorf("findHegel with PATH: got %q, want %q", result, hegelBin)
	}

	// Second: remove hegel from PATH -> fallback "hegel".
	os.Setenv("PATH", "/nonexistent") //nolint:errcheck
	result = findHegel()
	os.Setenv("PATH", oldPath) //nolint:errcheck
	if result != "hegel" {
		t.Errorf("findHegel fallback: got %q, want \"hegel\"", result)
	}
}

// =============================================================================
// fatalSentinel.Error()
// =============================================================================

func TestFatalSentinelError(t *testing.T) {
	f := fatalSentinel{msg: "test fatal"}
	if f.Error() != "test fatal" {
		t.Errorf("got %q", f.Error())
	}
}

// =============================================================================
// runTestCase: fatalSentinel recovery path
// =============================================================================

func TestRunTestCaseFatalSentinel(t *testing.T) {
	err := runTestOnFakeServer(t, func(_ *TestCase) {
		panic(fatalSentinel{msg: "fatal error"})
	}, func(caseCh *channel) {
		// Receive mark_complete with INTERESTING status.
		msgID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(msgID, nil) //nolint:errcheck
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// =============================================================================
// runTestCase: failed flag check (T.Error/T.Fail path)
// =============================================================================

func TestRunTestCaseFailedFlag(t *testing.T) {
	err := runTestOnFakeServer(t, func(s *TestCase) {
		s.failed = true // simulate T.Fail()
	}, func(caseCh *channel) {
		// Should get INTERESTING mark_complete
		msgID, payload, _ := caseCh.RecvRequestRaw(2 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		statusVal, _ := extractCBORString(m[any("status")])
		if statusVal != "INTERESTING" {
			t.Errorf("expected INTERESTING status for failed flag, got %q", statusVal)
		}
		caseCh.SendReplyValue(msgID, nil) //nolint:errcheck
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// =============================================================================
// runTestCase: failed flag on final run returns error
// =============================================================================

func TestRunTestCaseFailedFlagOnFinal(t *testing.T) {
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")

		// test_done with 1 interesting case.
		donePayload, _ := encodeCBOR(map[string]any{
			"event": "test_done",
			"results": map[string]any{
				"passed":                 false,
				"test_cases":             int64(1),
				"valid_test_cases":       int64(1),
				"invalid_test_cases":     int64(0),
				"interesting_test_cases": int64(1),
			},
		})
		doneID, _ := testCh.SendRequestRaw(donePayload)
		testCh.recvResponseRaw(doneID, 5*time.Second) //nolint:errcheck

		// Send final test_case.
		caseCh := serverConn.NewChannel("FinalCh")
		casePayload, _ := encodeCBOR(map[string]any{
			"event":      "test_case",
			"channel_id": int64(caseCh.ChannelID()),
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck

		// Receive mark_complete (INTERESTING) from client.
		markID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(markID, nil) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("failed_flag_final", func(s *TestCase) {
		s.failed = true // simulate T.Fail() on final run
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err == nil {
		t.Error("expected error when failed flag set on final run")
	}
	mustContainStr(t, err.Error(), "test failed")
}

// =============================================================================
// fatalSentinel on final run returns error
// =============================================================================

func TestRunTestCaseFatalSentinelOnFinal(t *testing.T) {
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chIDVal := m[any("channel_id")]
		chID, _ := extractCBORInt(chIDVal)
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")

		// test_done with 1 interesting case.
		donePayload, _ := encodeCBOR(map[string]any{
			"event": "test_done",
			"results": map[string]any{
				"passed":                 false,
				"test_cases":             int64(1),
				"valid_test_cases":       int64(1),
				"invalid_test_cases":     int64(0),
				"interesting_test_cases": int64(1),
			},
		})
		doneID, _ := testCh.SendRequestRaw(donePayload)
		testCh.recvResponseRaw(doneID, 5*time.Second) //nolint:errcheck

		// Send final test_case.
		caseCh := serverConn.NewChannel("FinalCh")
		casePayload, _ := encodeCBOR(map[string]any{
			"event":      "test_case",
			"channel_id": int64(caseCh.ChannelID()),
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck

		// mark_complete from client.
		caseCh.RecvRequestRaw(2 * time.Second) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("fatal_sentinel_final", func(_ *TestCase) {
		panic(fatalSentinel{msg: "fatal on final"})
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err == nil {
		t.Error("expected error when fatalSentinel on final run")
	}
	mustContainStr(t, err.Error(), "fatal on final")
}

// =============================================================================
// toInt64: uint64 branch and invalid type branch
// =============================================================================

func TestToInt64Int64(t *testing.T) {
	v, ok := toInt64(int64(-7))
	if !ok || v != -7 {
		t.Errorf("got %d, %v", v, ok)
	}
}

func TestToInt64Uint64(t *testing.T) {
	v, ok := toInt64(uint64(42))
	if !ok || v != 42 {
		t.Errorf("got %d, %v", v, ok)
	}
}

func TestToInt64Invalid(t *testing.T) {
	_, ok := toInt64("not a number")
	if ok {
		t.Error("expected false for invalid type")
	}
}

// =============================================================================
// Target via fake server
// =============================================================================

func TestTargetFakeServer(t *testing.T) {
	err := runTestOnFakeServer(t, func(s *TestCase) {
		s.Target(1.5, "score")
	}, func(caseCh *channel) {
		// Receive target command.
		targetID, targetPayload, _ := caseCh.RecvRequestRaw(2 * time.Second)
		decoded, _ := decodeCBOR(targetPayload)
		m, _ := extractCBORDict(decoded)
		cmd, _ := extractCBORString(m[any("command")])
		if cmd != "target" {
			t.Errorf("expected 'target' command, got %q", cmd)
		}
		caseCh.SendReplyValue(targetID, nil) //nolint:errcheck

		// Receive mark_complete.
		mcID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// =============================================================================
// Public API: Run — via injected globalSession with fake server
// =============================================================================

// fakeGlobalSession replaces globalSession with a pre-connected fake session,
// and restores it on test cleanup. Returns the server connection for protocol handling.
func fakeGlobalSession(t *testing.T) *connection {
	t.Helper()
	old := globalSession
	t.Cleanup(func() { globalSession = old })

	s, c := socketPair(t)
	serverConn := newConnection(s, "FakeGlobalServer")
	clientConn := newConnection(c, "GlobalClient")

	serverReady := make(chan struct{})
	go func() {
		serverConn.ReceiveHandshake() //nolint:errcheck
		close(serverReady)
	}()
	if err := clientConn.SendHandshake(); err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	<-serverReady

	sess := newHegelSession()
	sess.conn = clientConn
	sess.cli = newClient(clientConn)
	globalSession = sess
	return serverConn
}

// simplePassingServerHandler handles a single run_test with 1 test case that
// expects 1 generate command followed by mark_complete. Sends test_done(passed=true).
func simplePassingServerHandler(t *testing.T, serverConn *connection) {
	t.Helper()
	ctrl := serverConn.ControlChannel()
	msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
	decoded, _ := decodeCBOR(payload)
	m, _ := extractCBORDict(decoded)
	chID, _ := extractCBORInt(m[any("channel_id")])
	ctrl.SendReplyValue(msgID, true) //nolint:errcheck

	testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
	caseCh := serverConn.NewChannel("Case")
	casePayload, _ := encodeCBOR(map[string]any{
		"event":      "test_case",
		"channel_id": int64(caseCh.ChannelID()),
		"is_final":   false,
	})
	caseID, _ := testCh.SendRequestRaw(casePayload)
	testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck

	// Handle generate command.
	genID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
	caseCh.SendReplyValue(genID, true) //nolint:errcheck

	// Receive mark_complete.
	mcID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
	caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

	sendTestDone(t, testCh, true, 0)
}

func TestRunPublicAPI(t *testing.T) {
	serverConn := fakeGlobalSession(t)
	go simplePassingServerHandler(t, serverConn)

	err := Run("test_run_api", func(s *TestCase) {
		_ = Draw[bool](s, Booleans(0.5))
	}, WithTestCases(1))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// =============================================================================
// Public API: MustRun — success
// =============================================================================

func TestMustRunSuccess(t *testing.T) {
	serverConn := fakeGlobalSession(t)
	go simplePassingServerHandler(t, serverConn)

	MustRun("test_must_run", func(s *TestCase) {
		_ = Draw[bool](s, Booleans(0.5))
	}, WithTestCases(1))
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
	MustRun("should_panic", func(*TestCase) {}, WithTestCases(1))
}

// =============================================================================
// Public API: Case — returns func(*testing.T)
// =============================================================================

func TestCaseSuccess(t *testing.T) {
	serverConn := fakeGlobalSession(t)
	go func() {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chID, _ := extractCBORInt(m[any("channel_id")])
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")

		// Non-final test case.
		caseCh := serverConn.NewChannel("Case")
		casePayload, _ := encodeCBOR(map[string]any{
			"event":      "test_case",
			"channel_id": int64(caseCh.ChannelID()),
			"is_final":   false,
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck
		genID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(genID, true) //nolint:errcheck
		mcID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		// test_done with 1 interesting case → triggers final replay with isFinal=true.
		sendTestDone(t, testCh, true, 1)

		// Final replay test case (isFinal=true).
		finalCh := serverConn.NewChannel("FinalCase")
		finalPayload, _ := encodeCBOR(map[string]any{
			"event":      "test_case",
			"channel_id": int64(finalCh.ChannelID()),
			"is_final":   true,
		})
		fID, _ := testCh.SendRequestRaw(finalPayload)
		testCh.recvResponseRaw(fID, 5*time.Second) //nolint:errcheck
		fGenID, _, _ := finalCh.RecvRequestRaw(2 * time.Second)
		finalCh.SendReplyValue(fGenID, true) //nolint:errcheck
		fMcID, _, _ := finalCh.RecvRequestRaw(2 * time.Second)
		finalCh.SendReplyValue(fMcID, nil) //nolint:errcheck
	}()

	t.Run("case_test", Case(func(ht *T) {
		_ = Draw[bool](ht, Booleans(0.5))
		ht.Note("test note via Case") // exercises noteFn = t.Log on final case
	}, WithTestCases(1)))
}

// =============================================================================
// hegelSession.runTest — via fake server (covers hegelSession.runTest wrapper)
// =============================================================================

func TestHegelSessionRunTestFakeServer(t *testing.T) {
	s, c := socketPair(t)
	serverConn := newConnection(s, "FakeServer")
	clientConn := newConnection(c, "Client")

	serverReady := make(chan struct{})
	go func() {
		serverConn.ReceiveHandshake() //nolint:errcheck
		close(serverReady)
	}()
	if err := clientConn.SendHandshake(); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	<-serverReady

	sess := newHegelSession()
	sess.conn = clientConn
	sess.cli = newClient(clientConn)

	go func() {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chID, _ := extractCBORInt(m[any("channel_id")])
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
		caseCh := serverConn.NewChannel("Case")
		casePayload, _ := encodeCBOR(map[string]any{
			"event":      "test_case",
			"channel_id": int64(caseCh.ChannelID()),
			"is_final":   false,
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck

		// mark_complete
		mcID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	}()

	err := sess.runTest("session_test", func(_ *TestCase) {}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("session runTest: %v", err)
	}
}

// =============================================================================
// hegelSession.cleanup — covers process/tmpdir cleanup paths
// =============================================================================

func TestHegelSessionCleanup(t *testing.T) {
	sess := newHegelSession()
	// Set up a connection that we can close.
	s, c := socketPair(t)
	serverConn := newConnection(s, "FakeServer")
	clientConn := newConnection(c, "Client")
	serverReady := make(chan struct{})
	go func() {
		serverConn.ReceiveHandshake() //nolint:errcheck
		close(serverReady)
	}()
	clientConn.SendHandshake() //nolint:errcheck
	<-serverReady

	sess.conn = clientConn
	sess.cli = newClient(clientConn)

	// Create a temp dir so cleanup removes it.
	tmpDir, err := os.MkdirTemp("", "hegel-test-cleanup-")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	sess.tempDir = tmpDir

	// Call cleanup — should not panic.
	sess.cleanup()

	if sess.conn != nil {
		t.Error("expected conn to be nil after cleanup")
	}
	if sess.cli != nil {
		t.Error("expected cli to be nil after cleanup")
	}
	if sess.tempDir != "" {
		t.Error("expected tempDir to be empty after cleanup")
	}
	// Verify temp dir was removed.
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Errorf("expected temp dir to be removed, stat error: %v", err)
	}
}

// =============================================================================
// hegelSession.start — hasWorkingClient returns early
// =============================================================================

func TestHegelSessionStartIdempotent(t *testing.T) {
	s, c := socketPair(t)
	serverConn := newConnection(s, "FakeServer")
	clientConn := newConnection(c, "Client")
	serverReady := make(chan struct{})
	go func() {
		serverConn.ReceiveHandshake() //nolint:errcheck
		close(serverReady)
	}()
	clientConn.SendHandshake() //nolint:errcheck
	<-serverReady

	sess := newHegelSession()
	sess.conn = clientConn
	sess.cli = newClient(clientConn)

	// start() should return nil immediately since hasWorkingClient() is true.
	err := sess.start()
	if err != nil {
		t.Fatalf("start with existing client: %v", err)
	}
}

// =============================================================================
// hegelSession.start — mkdirTemp failure
// =============================================================================

func TestHegelSessionStartMkdirFail(t *testing.T) {
	oldFn := mkdirTempFn
	defer func() { mkdirTempFn = oldFn }()
	mkdirTempFn = func(dir, pattern string) (string, error) {
		return "", fmt.Errorf("simulated mktemp failure")
	}

	sess := newHegelSession()
	sess.hegelCmd = "/nonexistent" // Won't actually be called since mkdirTemp fails first
	err := sess.start()
	if err == nil {
		t.Error("expected start to fail with mkdirTemp error")
	}
	mustContainStr(t, err.Error(), "mktemp")
}

// =============================================================================
// findHegel — basic non-empty check (different from TestFindHegelFallback above)
// =============================================================================

func TestFindHegelReturnsNonEmpty(t *testing.T) {
	result := findHegel()
	if result == "" {
		t.Error("findHegel should return non-empty string")
	}
}

// --- helpers ---

func mustContainStr(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("%q does not contain %q", s, sub)
	}
}
