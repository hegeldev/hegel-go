package hegel

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
)

// Exit codes used by Hegel test binaries.
const (
	// ExitCodeTestFailure indicates a test assertion failed.
	ExitCodeTestFailure = 1
	// ExitCodeSocketError indicates a socket communication error.
	ExitCodeSocketError = 134
)

// Mode represents the execution mode for the Hegel SDK.
type Mode int

const (
	// ModeStandalone is the default mode where the test binary runs
	// and hegel is an external process that spawned it.
	ModeStandalone Mode = iota
	// ModeEmbedded is the mode where the test binary runs hegel as
	// a subprocess using the Hegel() function.
	ModeEmbedded
)

// Mode state - goroutine-local simulation using goroutine ID or explicit state.
// For simplicity, we use global state with the assumption tests run sequentially.
var (
	currentMode Mode
	isLastRun   bool
	modeMu      sync.Mutex
)

// Connection state - protected by mutex.
// Note: This assumes single-goroutine usage per test. Each test should run
// in its own goroutine without sharing connection state.
var (
	conn       net.Conn
	connMu     sync.Mutex
	spanDepth  int
	readBuffer bytes.Buffer
	requestID  uint64
	debugMode  bool
	debugOnce  sync.Once
)

func isDebug() bool {
	debugOnce.Do(func() {
		debugMode = os.Getenv("HEGEL_DEBUG") != ""
	})
	return debugMode
}

func isConnected() bool {
	connMu.Lock()
	defer connMu.Unlock()
	return conn != nil
}

func openConnection() {
	connMu.Lock()
	defer connMu.Unlock()

	if conn != nil {
		panic("hegel: openConnection called while already connected")
	}

	socketPath := os.Getenv("HEGEL_SOCKET")
	if socketPath == "" {
		fmt.Fprintln(os.Stderr, "hegel: HEGEL_SOCKET environment variable not set")
		os.Exit(ExitCodeSocketError)
	}

	var err error
	conn, err = net.Dial("unix", socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hegel: failed to connect to socket %s: %v\n", socketPath, err)
		os.Exit(ExitCodeSocketError)
	}

	readBuffer.Reset()
}

func closeConnection() {
	connMu.Lock()
	defer connMu.Unlock()

	if conn == nil {
		panic("hegel: closeConnection called while not connected")
	}
	if spanDepth != 0 {
		panic(fmt.Sprintf("hegel: closeConnection called with %d unclosed span(s)", spanDepth))
	}

	conn.Close()
	conn = nil
}

func getSpanDepth() int {
	connMu.Lock()
	defer connMu.Unlock()
	return spanDepth
}

func incrementSpanDepth() {
	connMu.Lock()
	defer connMu.Unlock()
	spanDepth++
}

func decrementSpanDepth() {
	connMu.Lock()
	defer connMu.Unlock()
	if spanDepth <= 0 {
		panic("hegel: decrementSpanDepth called with no open spans")
	}
	spanDepth--
}

// sendRequest sends a command to the Hegel server and returns the result.
func sendRequest(command string, payload any) json.RawMessage {
	connMu.Lock()
	defer connMu.Unlock()

	if conn == nil {
		panic("hegel: sendRequest called without active connection")
	}

	id := atomic.AddUint64(&requestID, 1)

	request := map[string]any{
		"id":      id,
		"command": command,
		"payload": payload,
	}

	data, err := json.Marshal(request)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hegel: failed to marshal request: %v\n", err)
		os.Exit(ExitCodeSocketError)
	}
	data = append(data, '\n')

	if isDebug() {
		fmt.Fprintf(os.Stderr, "REQUEST: %s", data)
	}

	if _, err := conn.Write(data); err != nil {
		fmt.Fprintf(os.Stderr, "hegel: write error: %v\n", err)
		os.Exit(ExitCodeSocketError)
	}

	// Read response - may need to read from buffer first
	reader := bufio.NewReader(&combinedReader{buffer: &readBuffer, conn: conn})
	line, err := reader.ReadBytes('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "hegel: read error: %v\n", err)
		os.Exit(ExitCodeSocketError)
	}

	// Store any extra buffered data back for next read
	// bufio.Reader may have read more than one line into its buffer
	if reader.Buffered() > 0 {
		extra := make([]byte, reader.Buffered())
		n, _ := reader.Read(extra)
		readBuffer.Write(extra[:n])
	}

	if isDebug() {
		fmt.Fprintf(os.Stderr, "RESPONSE: %s", line)
	}

	var response struct {
		ID     uint64          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *string         `json:"error"`
	}

	err = json.Unmarshal(line, &response)
	if err != nil {
		panic(fmt.Sprintf("hegel: failed to parse server response as JSON: %v\nResponse: %s", err, line))
	}
	if response.ID != id {
		panic(fmt.Sprintf("hegel: response ID mismatch: expected %d, got %d", id, response.ID))
	}
	if response.Error != nil {
		panic(fmt.Sprintf("hegel: server returned error: %s", *response.Error))
	}

	return response.Result
}

// combinedReader reads from buffer first, then from connection.
type combinedReader struct {
	buffer *bytes.Buffer
	conn   net.Conn
}

func (r *combinedReader) Read(p []byte) (n int, err error) {
	if r.buffer.Len() > 0 {
		return r.buffer.Read(p)
	}
	return r.conn.Read(p)
}

// generateFromSchema generates a value of type T from a JSON schema.
func generateFromSchema[T any](schema map[string]any) T {
	needConnection := !isConnected()
	if needConnection {
		openConnection()
	}

	result := sendRequest("generate", schema)

	if needConnection {
		closeConnection()
	}

	var value T
	err := json.Unmarshal(result, &value)
	if err != nil {
		panic(fmt.Sprintf("hegel: failed to deserialize server response: %v\nValue: %s", err, result))
	}

	// Auto-log generated value during final replay (counterexample)
	if IsLastRun() {
		fmt.Fprintf(os.Stderr, "Generated: %s\n", result)
	}

	return value
}

// CurrentMode returns the current execution mode.
func CurrentMode() Mode {
	modeMu.Lock()
	defer modeMu.Unlock()
	return currentMode
}

// IsLastRun returns true if this is the last run (during shrinking).
// In embedded mode, this indicates when Note() output should be printed.
func IsLastRun() bool {
	modeMu.Lock()
	defer modeMu.Unlock()
	return isLastRun
}

// Note prints a message to stderr.
// In standalone mode, always prints.
// In embedded mode, only prints on the last run.
func Note(message string) {
	modeMu.Lock()
	mode := currentMode
	last := isLastRun
	modeMu.Unlock()

	if mode == ModeStandalone {
		fmt.Fprintln(os.Stderr, message)
	} else if last {
		fmt.Fprintln(os.Stderr, message)
	}
	// In embedded mode on non-last runs: silently ignore
}

// AssumeFailedError is a sentinel error used to signal assume(false) in embedded mode.
type AssumeFailedError struct{}

func (e *AssumeFailedError) Error() string {
	return "assume failed"
}

// Assume checks a condition and rejects the test input if false.
// This tells Hegel to try different input rather than treating it as a failure.
//
// In standalone mode, this function exits the process when condition is false.
// In embedded mode, this function panics with an AssumeFailedError when condition is false.
func Assume(condition bool) {
	if !condition {
		modeMu.Lock()
		mode := currentMode
		modeMu.Unlock()

		if mode == ModeEmbedded {
			panic(&AssumeFailedError{})
		}

		code := getRejectCode()
		os.Exit(code)
	}
}

func getRejectCode() int {
	codeStr := os.Getenv("HEGEL_REJECT_CODE")
	if codeStr == "" {
		fmt.Fprintln(os.Stderr, "hegel: HEGEL_REJECT_CODE environment variable not set")
		os.Exit(ExitCodeSocketError)
	}

	code, err := strconv.Atoi(codeStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hegel: HEGEL_REJECT_CODE is not a valid integer: %s\n", codeStr)
		os.Exit(ExitCodeSocketError)
	}

	return code
}
