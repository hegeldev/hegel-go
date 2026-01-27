package hegel

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
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

// State - protected by mutex.
// For simplicity, we use global state with the assumption tests run sequentially.
var (
	isLastRun bool
	modeMu    sync.Mutex
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

// convertSpecialValues converts special object wrappers from the server to native Go values.
// Handles:
// - {"$float": "nan"} -> math.NaN()
// - {"$float": "inf"} -> math.Inf(1)
// - {"$float": "-inf"} -> math.Inf(-1)
// - {"$integer": "..."} -> json.Number (preserves precision)
// - json.Number -> kept as-is (will be converted when unmarshaling to target type)
func convertSpecialValues(value any) any {
	switch v := value.(type) {
	case []any:
		result := make([]any, len(v))
		for i, elem := range v {
			result[i] = convertSpecialValues(elem)
		}
		return result
	case json.Number:
		// Keep json.Number as-is - it will be properly converted when
		// unmarshaling to the target type
		return v
	case map[string]any:
		// Check for special single-key objects
		if len(v) == 1 {
			if floatVal, ok := v["$float"].(string); ok {
				switch floatVal {
				case "nan":
					return math.NaN()
				case "inf":
					return math.Inf(1)
				case "-inf":
					return math.Inf(-1)
				}
			}
			if intVal, ok := v["$integer"].(string); ok {
				// Return as json.Number to preserve precision
				return json.Number(intVal)
			}
		}
		// Recursively convert map values
		result := make(map[string]any, len(v))
		for key, val := range v {
			result[key] = convertSpecialValues(val)
		}
		return result
	default:
		return value
	}
}

// generateFromSchema generates a value of type T from a JSON schema.
func generateFromSchema[T any](schema map[string]any) T {
	result := sendRequest("generate", schema)

	// Use json.Decoder with UseNumber() to preserve numeric precision.
	// Without this, large integers (near MaxInt64) lose precision when
	// decoded as float64.
	decoder := json.NewDecoder(bytes.NewReader(result))
	decoder.UseNumber()

	var raw any
	if err := decoder.Decode(&raw); err != nil {
		panic(fmt.Sprintf("hegel: failed to parse server response: %v\nValue: %s", err, result))
	}

	// Convert special object wrappers (NaN, Inf, $integer)
	converted := convertSpecialValues(raw)

	// Re-marshal and unmarshal to target type
	convertedBytes, err := json.Marshal(converted)
	if err != nil {
		panic(fmt.Sprintf("hegel: failed to re-marshal converted value: %v", err))
	}

	var value T
	err = json.Unmarshal(convertedBytes, &value)
	if err != nil {
		panic(fmt.Sprintf("hegel: failed to deserialize server response: %v\nValue: %s", err, result))
	}

	// Auto-log generated value during final replay (counterexample)
	if IsLastRun() {
		fmt.Fprintf(os.Stderr, "Generated: %s\n", result)
	}

	return value
}

// IsLastRun returns true if this is the last run (during shrinking).
// This indicates when Note() output should be printed.
func IsLastRun() bool {
	modeMu.Lock()
	defer modeMu.Unlock()
	return isLastRun
}

// Note prints a message to stderr.
// Only prints on the last run (final replay for counterexample output).
func Note(message string) {
	modeMu.Lock()
	last := isLastRun
	modeMu.Unlock()

	if last {
		fmt.Fprintln(os.Stderr, message)
	}
}

// AssumeFailedError is a sentinel error used to signal assume(false).
type AssumeFailedError struct{}

func (e *AssumeFailedError) Error() string {
	return "assume failed"
}

// Assume checks a condition and rejects the test input if false.
// This tells Hegel to try different input rather than treating it as a failure.
func Assume(condition bool) {
	if !condition {
		panic(&AssumeFailedError{})
	}
}
