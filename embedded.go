package hegel

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// HegelOptions configures the embedded mode execution.
type HegelOptions struct {
	// TestCases is the number of test cases to run. Default: 100.
	TestCases int
	// Debug enables debug output from hegel.
	Debug bool
	// HegelPath is the path to the hegel binary. Default: "hegel".
	HegelPath string
}

// Hegel runs property-based tests using Hegel in embedded mode.
//
// This function:
// 1. Creates a Unix socket server
// 2. Spawns the hegel CLI as a subprocess
// 3. Accepts connections from hegel (one per test case)
// 4. Runs the test function for each test case
// 5. Reports results back to hegel
// 6. Panics if any test case fails
//
// Example:
//
//	hegel.Hegel(func() {
//	    x := hegel.Integers[int]().Generate()
//	    y := hegel.Integers[int]().Generate()
//	    hegel.Note(fmt.Sprintf("Testing %d + %d", x, y))
//	    if x+y != y+x {
//	        panic("commutativity violated")
//	    }
//	}, hegel.HegelOptions{})
func Hegel(testFn func(), options HegelOptions) {
	// Set defaults
	hegelPath := options.HegelPath
	if hegelPath == "" {
		var err error
		hegelPath, err = ensureHegel()
		if err != nil {
			panic(fmt.Sprintf("Failed to ensure hegel is installed: %v", err))
		}
	}
	testCases := options.TestCases
	if testCases <= 0 {
		testCases = 100
	}

	// Create temp directory with socket
	tempDir, err := os.MkdirTemp("", "hegel_*")
	if err != nil {
		panic(fmt.Sprintf("Failed to create temp directory: %v", err))
	}
	defer os.RemoveAll(tempDir)

	socketPath := filepath.Join(tempDir, "hegel.sock")

	// Create Unix socket server
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to create socket: %v", err))
	}
	defer listener.Close()

	// Set accept timeout
	listener.(*net.UnixListener).SetDeadline(time.Time{})

	// Build hegel command
	args := []string{"--client-mode", socketPath, "--no-tui", "--test-cases", fmt.Sprint(testCases)}
	if options.Debug {
		args = append(args, "--debug")
	}

	cmd := exec.Command(hegelPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start hegel
	if err := cmd.Start(); err != nil {
		panic(fmt.Sprintf("Failed to start hegel: %v", err))
	}

	// Channel to signal hegel exit
	hegelDone := make(chan error, 1)
	go func() {
		hegelDone <- cmd.Wait()
	}()

	// Accept connections until hegel exits
	for {
		// Try to accept with timeout
		listener.(*net.UnixListener).SetDeadline(time.Now().Add(100 * time.Millisecond))
		conn, err := listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Check if hegel exited
				select {
				case exitErr := <-hegelDone:
					if exitErr != nil {
						if exitError, ok := exitErr.(*exec.ExitError); ok && exitError.ExitCode() != 0 {
							panic(fmt.Sprintf("Hegel test failed (exit code %d)", exitError.ExitCode()))
						}
					}
					return
				default:
					// Hegel still running, continue
					continue
				}
			}
			panic(fmt.Sprintf("Accept failed: %v", err))
		}

		// Handle connection
		handleGoConnection(conn, testFn, options.Debug)
	}
}

// handleGoConnection handles a single connection from hegel (one test case).
func handleGoConnection(conn net.Conn, testFn func(), debug bool) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Read handshake
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return
	}

	var handshake struct {
		Type      string `json:"type"`
		IsLastRun bool   `json:"is_last_run"`
	}
	if err := json.Unmarshal(line, &handshake); err != nil {
		return
	}

	if debug {
		fmt.Fprintf(os.Stderr, "Handshake received: is_last_run=%v\n", handshake.IsLastRun)
	}

	// Set mode state
	modeMu.Lock()
	currentMode = ModeEmbedded
	isLastRun = handshake.IsLastRun
	modeMu.Unlock()

	// Set up embedded connection for generate() calls
	setEmbeddedConnection(conn, reader)

	// Send handshake_ack
	if _, err := conn.Write([]byte(`{"type": "handshake_ack"}` + "\n")); err != nil {
		clearEmbeddedConnection()
		setModeStandalone()
		return
	}

	// Run test with panic recovery
	resultType := "pass"
	var errorMessage string

	func() {
		defer func() {
			if r := recover(); r != nil {
				if rejectErr, ok := r.(*RejectError); ok {
					resultType = "reject"
					errorMessage = rejectErr.Message
				} else {
					resultType = "fail"
					errorMessage = fmt.Sprintf("%v", r)
				}
			}
		}()
		testFn()
	}()

	// Clear embedded connection
	clearEmbeddedConnection()

	// Send result
	result := map[string]string{
		"type":   "test_result",
		"result": resultType,
	}
	if errorMessage != "" {
		result["message"] = errorMessage
	}

	resultJSON, _ := json.Marshal(result)

	if debug {
		fmt.Fprintf(os.Stderr, "Sending result: %s\n", resultJSON)
	}

	conn.Write(append(resultJSON, '\n'))

	// Reset mode
	setModeStandalone()
}

// setEmbeddedConnection sets the main connection for embedded mode.
// This allows generateFromSchema() to use the connection established by Hegel().
func setEmbeddedConnection(c net.Conn, reader *bufio.Reader) {
	connMu.Lock()
	defer connMu.Unlock()
	conn = c
	readBuffer.Reset()
}

// clearEmbeddedConnection clears the connection without closing it.
// The caller (handleGoConnection) is responsible for closing.
func clearEmbeddedConnection() {
	connMu.Lock()
	defer connMu.Unlock()
	conn = nil
}

func setModeStandalone() {
	modeMu.Lock()
	defer modeMu.Unlock()
	currentMode = ModeStandalone
	isLastRun = false
}
