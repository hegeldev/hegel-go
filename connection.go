package hegel

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// protocolVersion is the version string used in handshakes.
const protocolVersion = "0.8"

// handshakePrefix is the prefix expected at the start of a valid handshake response.
const handshakePrefix = "Hegel/"

// handshakeRequest is the fixed bytes sent by the client to initiate a handshake.
var handshakeRequest = []byte("hegel_handshake_start")

// shutdownSentinel is placed in a stream's inbox to signal that it was closed.
var shutdownSentinel = &struct{}{}

// connectionState tracks whether the connection has performed a handshake.
type connectionState int

const (
	stateUnresolved connectionState = iota
	stateClient
)

// connection manages a multiplexed stream with a dedicated reader goroutine.
// It is safe to call Close from any goroutine; all other methods must be called
// from a single goroutine per connection.
type connection struct {
	name   string
	reader io.ReadCloser
	writer io.WriteCloser

	nextStreamID int
	streams      map[uint32]*stream
	state        connectionState

	writerMu sync.Mutex
	done     chan struct{}

	controlMu sync.Mutex
	controlSt *stream

	processExited <-chan struct{}
	crashMessage  string // set by monitor goroutine before closing processExited
}

// connectionError wraps a connection-level error that should propagate out of the test.
type connectionError struct{ msg string }

// Error implements the error interface.
func (e *connectionError) Error() string { return e.msg }

// serverCrashError returns an error indicating the server process exited unexpectedly.
// crashMessage is set by the monitor goroutine before closing processExited,
// so reading it after receiving from processExited is safe without a lock.
func (c *connection) serverCrashError() *connectionError {
	msg := c.crashMessage
	if msg == "" {
		msg = "The hegel server process exited unexpectedly."
	}
	return &connectionError{msg: msg}
}

// newConnection creates a new multiplexed connection from separate reader and writer
// streams and registers the control stream (ID 0).
func newConnection(reader io.ReadCloser, writer io.WriteCloser, name string) *connection {
	c := &connection{
		name:         name,
		reader:       reader,
		writer:       writer,
		streams:      make(map[uint32]*stream),
		state:        stateUnresolved,
		nextStreamID: 1, // first real stream counter (matches Python's __next_stream_id = 1)
		done:         make(chan struct{}),
	}
	// stream 0 is the control stream; it is pre-registered before any handshake.
	c.controlSt = newStream(c, 0, name)
	c.streams[0] = c.controlSt
	go c.readLoop()
	return c
}

// ControlStream returns the stream used for handshake and control messages.
func (c *connection) ControlStream() *stream { return c.controlSt }

// SendControlRequest sends a request on the control stream and waits for the
// response. Access is serialized so concurrent callers don't race on the
// control stream's internal state.
func (c *connection) SendControlRequest(payload []byte) (any, error) {
	c.controlMu.Lock()
	defer c.controlMu.Unlock()
	pending, err := c.controlSt.Request(payload)
	if err != nil {
		return nil, err
	}
	return pending.Get()
}

// SendPacket sends a packet to the peer. It is safe to call concurrently.
func (c *connection) SendPacket(pkt packet) error {
	c.writerMu.Lock()
	defer c.writerMu.Unlock()
	return writePacket(c.writer, pkt)
}

// Close shuts down the connection. Closing the reader causes readLoop to exit,
// which closes the done stream and wakes all waiters.
func (c *connection) Close() {
	c.reader.Close() //nolint:errcheck
	<-c.done
	c.writer.Close() //nolint:errcheck
}

// readLoop continuously reads packets from the reader and dispatches them.
// It exits when readPacket returns an error (e.g. the stream was closed).
func (c *connection) readLoop() {
	defer close(c.done)
	for {
		pkt, err := readPacket(c.reader)
		if err != nil {
			return
		}
		c.dispatch(pkt)
	}
}

// dispatch routes a received packet to the appropriate stream's inbox.
func (c *connection) dispatch(pkt packet) {
	c.writerMu.Lock()
	st, ok := c.streams[pkt.StreamID]
	c.writerMu.Unlock()

	if bytes.Equal(pkt.Payload, closeStreamPayload) && pkt.MessageID == closeStreamMessageID {
		if ok && st != nil {
			st.putInbox(shutdownSentinel)
		}
		c.writerMu.Lock()
		delete(c.streams, pkt.StreamID)
		c.writerMu.Unlock()
		return
	}

	if !ok || st == nil {
		// Message to unknown stream — send an error reply if it was a request.
		if !pkt.IsReply {
			errMsg := fmt.Sprintf("Message %d sent to non-existent stream %d",
				pkt.MessageID, pkt.StreamID)
			errPayload, encErr := encodeCBOR(map[string]any{"error": errMsg})
			if encErr == nil {
				c.SendPacket(packet{ //nolint:errcheck
					StreamID:  pkt.StreamID,
					MessageID: pkt.MessageID,
					IsReply:   true,
					Payload:   errPayload,
				})
			}
		}
		return
	}
	st.putInbox(pkt)
}

// SendHandshake performs the client side of the handshake and discards the version.
func (c *connection) SendHandshake() error {
	_, err := c.SendHandshakeVersion()
	return err
}

// SendHandshakeVersion performs the client side of the handshake and returns the
// server version string (the part after "Hegel/").
func (c *connection) SendHandshakeVersion() (string, error) {
	c.writerMu.Lock()
	if c.state != stateUnresolved {
		c.writerMu.Unlock()
		return "", fmt.Errorf("handshake already established")
	}
	c.state = stateClient
	c.writerMu.Unlock()

	msgID, err := c.controlSt.SendRequestRaw(handshakeRequest)
	if err != nil {
		return "", err
	}
	resp, err := c.controlSt.recvResponseRaw(msgID, 10*time.Second)
	if err != nil {
		return "", err
	}
	decoded := string(resp)
	if !strings.HasPrefix(decoded, handshakePrefix) {
		return "", fmt.Errorf("bad handshake response: %q", decoded)
	}
	return strings.TrimPrefix(decoded, handshakePrefix), nil
}

// NewStream allocates a new client-side logical stream. Panics if called before
// the handshake is complete (matching Python's ValueError).
func (c *connection) NewStream(name string) *stream {
	c.writerMu.Lock()
	defer c.writerMu.Unlock()

	if c.state == stateUnresolved {
		panic("Cannot create a new stream before handshake has been performed")
	}

	// Client streams are odd: (counter << 1) | 1
	streamID := uint32((c.nextStreamID << 1) | 1)
	c.nextStreamID++

	st := newStream(c, streamID, name)
	c.streams[streamID] = st
	return st
}

// ConnectStream registers an existing peer-created stream by its ID.
func (c *connection) ConnectStream(id uint32, name string) (*stream, error) {
	c.writerMu.Lock()
	defer c.writerMu.Unlock()

	if c.state == stateUnresolved {
		return nil, fmt.Errorf("cannot create a new stream before handshake has been performed")
	}
	if _, exists := c.streams[id]; exists {
		return nil, fmt.Errorf("stream already connected as stream %d", id)
	}

	st := newStream(c, id, name)
	c.streams[id] = st
	return st, nil
}

// requestError is an error response received from the peer.
type requestError struct {
	msg       string
	ErrorType string
	Data      map[any]any
}

// Error implements the error interface.
func (e *requestError) Error() string { return e.msg }

// newRequestError builds a requestError from a CBOR-decoded error dict.
func newRequestError(data map[any]any) *requestError {
	msg, _ := extractCBORString(data[any("error")])
	errType, _ := extractCBORString(data[any("type")])
	rest := make(map[any]any)
	for k, v := range data {
		s, err := extractCBORString(k)
		if err != nil {
			continue
		}
		if s != "error" && s != "type" {
			rest[k] = v
		}
	}
	return &requestError{msg: msg, ErrorType: errType, Data: rest}
}

// resultOrError extracts the "result" field from a CBOR-decoded dict, or returns
// a *requestError if the dict contains an "error" field.
func resultOrError(body map[any]any) (any, error) {
	if _, hasErr := body[any("error")]; hasErr {
		return nil, newRequestError(body)
	}
	return body[any("result")], nil
}

// stream is a logical, non-thread-safe communication stream over a connection.
type stream struct {
	conn          *connection
	streamID      uint32
	inbox         chan any
	droppedOnce   sync.Once
	dropped       chan struct{} // indicates that a message was dropped at some point
	nextMessageID uint32
	responses     map[uint32][]byte
	requests      []packet
	closed        bool
	name          string
}

func newStream(c *connection, id uint32, name string) *stream {
	return &stream{
		conn:          c,
		streamID:      id,
		inbox:         make(chan any, 64),
		dropped:       make(chan struct{}),
		nextMessageID: 1,
		name:          name,
	}
}

func (st *stream) String() string {
	if st.name != "" {
		return fmt.Sprintf("stream %d (%s)", st.streamID, st.name)
	}
	return fmt.Sprintf("stream %d", st.streamID)
}

// StreamID returns the numeric ID of this stream.
func (st *stream) StreamID() uint32 { return st.streamID }

// putInbox delivers a packet to the stream's inbox.
func (st *stream) putInbox(v any) {
	select {
	case st.inbox <- v:
	default:
		// Panic if full — shouldn't happen with a generous buffer.
		st.droppedOnce.Do(func() { close(st.dropped) })
	}
}

// Close sends a close notification to the peer and marks the stream closed.
func (st *stream) Close() {
	if st.closed {
		return
	}
	st.closed = true

	// Check if this stream is still registered (not already removed by the peer).
	st.conn.writerMu.Lock()
	registered := st.conn.streams[st.streamID] == st
	st.conn.writerMu.Unlock()

	if registered {
		// Send asynchronously: write may block if the reader isn't consuming yet.
		go st.conn.SendPacket(packet{ //nolint:errcheck
			StreamID:  st.streamID,
			MessageID: closeStreamMessageID,
			IsReply:   false,
			Payload:   closeStreamPayload,
		})
	}
}

// SendRequestRaw sends raw bytes as a request and returns the message ID.
func (st *stream) SendRequestRaw(payload []byte) (uint32, error) {
	msgID := st.nextMessageID
	st.nextMessageID++
	err := st.conn.SendPacket(packet{
		StreamID:  st.streamID,
		MessageID: msgID,
		IsReply:   false,
		Payload:   payload,
	})
	return msgID, err
}

// SendReplyRaw sends raw bytes as a reply to the given message ID.
func (st *stream) SendReplyRaw(msgID uint32, payload []byte) error {
	return st.conn.SendPacket(packet{
		StreamID:  st.streamID,
		MessageID: msgID,
		IsReply:   true,
		Payload:   payload,
	})
}

// SendReplyValue sends a CBOR-encoded {"result": v} reply.
func (st *stream) SendReplyValue(msgID uint32, v any) error {
	payload, err := encodeCBOR(map[string]any{"result": v})
	if err != nil {
		return err
	}
	return st.SendReplyRaw(msgID, payload)
}

// SendReplyError sends a CBOR-encoded error reply with the given message and type.
func (st *stream) SendReplyError(msgID uint32, errMsg, errType string) error {
	payload, err := encodeCBOR(map[string]any{
		"error": errMsg,
		"type":  errType,
	})
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("hegel: SendReplyError encode: %v", err))
	}
	return st.SendReplyRaw(msgID, payload)
}

// RecvRequestRaw waits for the next server-initiated request and returns
// (messageID, payload, error). timeout <= 0 means no timeout.
func (st *stream) RecvRequestRaw(timeout time.Duration) (uint32, []byte, error) {
	for len(st.requests) == 0 {
		if err := st.processOneMessage(timeout); err != nil {
			return 0, nil, err
		}
	}
	pkt := st.requests[0]
	st.requests = st.requests[1:]
	return pkt.MessageID, pkt.Payload, nil
}

// RecvRequest waits for the next server-initiated request and returns
// (messageID, CBOR-decoded payload, error).
func (st *stream) RecvRequest(timeout time.Duration) (uint32, any, error) {
	msgID, payload, err := st.RecvRequestRaw(timeout)
	if err != nil {
		return 0, nil, err
	}
	v, err := decodeCBOR(payload)
	if err != nil {
		return 0, nil, err
	}
	return msgID, v, nil
}

// recvResponseRaw waits for a reply to the given message ID.
func (st *stream) recvResponseRaw(msgID uint32, timeout time.Duration) ([]byte, error) {
	if st.responses == nil {
		st.responses = make(map[uint32][]byte)
	}
	for {
		if payload, ok := st.responses[msgID]; ok {
			delete(st.responses, msgID)
			return payload, nil
		}
		if err := st.processOneMessage(timeout); err != nil {
			return nil, err
		}
	}
}

// ReceiveResponse waits for a reply to the given message ID and returns the
// CBOR-decoded result (unwrapping {"result": v} or raising requestError).
func (st *stream) ReceiveResponse(msgID uint32, timeout time.Duration) (any, error) {
	raw, err := st.recvResponseRaw(msgID, timeout)
	if err != nil {
		return nil, err
	}
	v, err := decodeCBOR(raw)
	if err != nil {
		return nil, err
	}
	m, err := extractCBORDict(v)
	if err != nil {
		return nil, err
	}
	return resultOrError(m)
}

// processOneMessage waits for a packet on the stream's inbox and routes it.
func (st *stream) processOneMessage(timeout time.Duration) error {
	if st.closed {
		return fmt.Errorf("%s is closed", st)
	}

	var timeoutCh <-chan time.Time
	if timeout > 0 {
		timeoutCh = time.After(timeout)
	}

	var pkt packet
	select {
	case item := <-st.inbox:
		if item == shutdownSentinel {
			st.closed = true
			return fmt.Errorf("%s was closed", st)
		}
		pkt = item.(packet)
	case <-st.dropped:
		panic(fmt.Errorf("%s: dropped a message", st))
	case <-st.conn.done:
		// When the pipe closes, the process has likely exited too. Wait
		// briefly for the monitor goroutine to confirm so we can report
		// a proper crash error instead of a generic "connection closed".
		// processExited is nil for connections without a subprocess (e.g.,
		// test connections using net.Pipe); skip the wait in that case.
		if st.conn.processExited != nil {
			select {
			case <-st.conn.processExited:
				return st.conn.serverCrashError()
			case <-time.After(100 * time.Millisecond):
			}
		}
		return fmt.Errorf("connection closed")
	case <-timeoutCh:
		return fmt.Errorf("timed out after %v waiting for a message on %s", timeout, st)
	}

	if pkt.IsReply {
		if st.responses == nil {
			st.responses = make(map[uint32][]byte)
		}
		st.responses[pkt.MessageID] = pkt.Payload
	} else {
		st.requests = append(st.requests, pkt)
	}
	return nil
}

// Request sends a request and returns a pendingRequest future.
// If the write fails because the server process exited, a *connectionError is returned.
func (st *stream) Request(payload []byte) (*pendingRequest, error) {
	msgID, err := st.SendRequestRaw(payload)
	if err != nil {
		select {
		case <-st.conn.processExited:
			return nil, st.conn.serverCrashError()
		default:
		}
		return nil, err
	}
	return &pendingRequest{st: st, msgID: msgID}, nil
}

// pendingRequest is a future for an in-flight request.
type pendingRequest struct {
	st    *stream
	msgID uint32
	value any
	done  bool
	err   error
}

// Get waits for and returns the response. Subsequent calls return the cached value.
func (p *pendingRequest) Get() (any, error) {
	if p.done {
		return p.value, p.err
	}
	v, err := p.st.ReceiveResponse(p.msgID, 100*time.Second)
	p.value = v
	p.err = err
	p.done = true
	return v, err
}
