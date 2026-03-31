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
const protocolVersion = "0.6"

// handshakePrefix is the prefix expected at the start of a valid handshake response.
const handshakePrefix = "Hegel/"

// handshakeRequest is the fixed bytes sent by the client to initiate a handshake.
var handshakeRequest = []byte("hegel_handshake_start")

// shutdownSentinel is placed in a channel's inbox to signal that it was closed.
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

	nextChannelID int
	channels      map[uint32]*channel
	state         connectionState

	writerMu sync.Mutex
	done     chan struct{}

	controlMu sync.Mutex
	controlCh *channel

	processExited <-chan struct{}
	crashMessage  string
}

// connectionError wraps a connection-level error that should propagate out of the test.
type connectionError struct{ msg string }

// Error implements the error interface.
func (e *connectionError) Error() string { return e.msg }

// serverCrashError returns an error indicating the server process exited unexpectedly.
func (c *connection) serverCrashError() *connectionError {
	msg := c.crashMessage
	if msg == "" {
		msg = "The hegel server process exited unexpectedly."
	}
	return &connectionError{msg: msg}
}

// newConnection creates a new multiplexed connection from separate reader and writer
// streams and registers the control channel (ID 0).
func newConnection(reader io.ReadCloser, writer io.WriteCloser, name string) *connection {
	c := &connection{
		name:          name,
		reader:        reader,
		writer:        writer,
		channels:      make(map[uint32]*channel),
		state:         stateUnresolved,
		nextChannelID: 1, // first real channel counter (matches Python's __next_channel_id = 1)
		done:          make(chan struct{}),
	}
	// channel 0 is the control channel; it is pre-registered before any handshake.
	c.controlCh = newChannel(c, 0, name)
	c.channels[0] = c.controlCh
	go c.readLoop()
	return c
}

// ControlChannel returns the channel used for handshake and control messages.
func (c *connection) ControlChannel() *channel { return c.controlCh }

// SendControlRequest sends a request on the control channel and waits for the
// response. Access is serialized so concurrent callers don't race on the
// control channel's internal state.
func (c *connection) SendControlRequest(payload []byte) (any, error) {
	c.controlMu.Lock()
	defer c.controlMu.Unlock()
	pending, err := c.controlCh.Request(payload)
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
// which closes the done channel and wakes all waiters.
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

// dispatch routes a received packet to the appropriate channel's inbox.
func (c *connection) dispatch(pkt packet) {
	c.writerMu.Lock()
	ch, ok := c.channels[pkt.ChannelID]
	c.writerMu.Unlock()

	if bytes.Equal(pkt.Payload, closeChannelPayload) && pkt.MessageID == closeChannelMessageID {
		if ok && ch != nil {
			ch.putInbox(shutdownSentinel)
		}
		c.writerMu.Lock()
		delete(c.channels, pkt.ChannelID)
		c.writerMu.Unlock()
		return
	}

	if !ok || ch == nil {
		// Message to unknown channel — send an error reply if it was a request.
		if !pkt.IsReply {
			errMsg := fmt.Sprintf("Message %d sent to non-existent channel %d",
				pkt.MessageID, pkt.ChannelID)
			errPayload, encErr := encodeCBOR(map[string]any{"error": errMsg})
			if encErr == nil {
				c.SendPacket(packet{ //nolint:errcheck
					ChannelID: pkt.ChannelID,
					MessageID: pkt.MessageID,
					IsReply:   true,
					Payload:   errPayload,
				})
			}
		}
		return
	}
	ch.putInbox(pkt)
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

	msgID, err := c.controlCh.SendRequestRaw(handshakeRequest)
	if err != nil {
		return "", err
	}
	resp, err := c.controlCh.recvResponseRaw(msgID, 10*time.Second)
	if err != nil {
		return "", err
	}
	decoded := string(resp)
	if !strings.HasPrefix(decoded, handshakePrefix) {
		return "", fmt.Errorf("bad handshake response: %q", decoded)
	}
	return strings.TrimPrefix(decoded, handshakePrefix), nil
}

// NewChannel allocates a new client-side logical channel. Panics if called before
// the handshake is complete (matching Python's ValueError).
func (c *connection) NewChannel(name string) *channel {
	c.writerMu.Lock()
	defer c.writerMu.Unlock()

	if c.state == stateUnresolved {
		panic("Cannot create a new channel before handshake has been performed")
	}

	// Client channels are odd: (counter << 1) | 1
	channelID := uint32((c.nextChannelID << 1) | 1)
	c.nextChannelID++

	ch := newChannel(c, channelID, name)
	c.channels[channelID] = ch
	return ch
}

// ConnectChannel registers an existing peer-created channel by its ID.
func (c *connection) ConnectChannel(id uint32, name string) (*channel, error) {
	c.writerMu.Lock()
	defer c.writerMu.Unlock()

	if c.state == stateUnresolved {
		return nil, fmt.Errorf("cannot create a new channel before handshake has been performed")
	}
	if _, exists := c.channels[id]; exists {
		return nil, fmt.Errorf("channel already connected as channel %d", id)
	}

	ch := newChannel(c, id, name)
	c.channels[id] = ch
	return ch, nil
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

// channel is a logical, non-thread-safe communication channel over a connection.
type channel struct {
	conn          *connection
	channelID     uint32
	inbox         chan any
	droppedOnce   sync.Once
	dropped       chan struct{} // indicates that a message was dropped at some point
	nextMessageID uint32
	responses     map[uint32][]byte
	requests      []packet
	closed        bool
	name          string
}

func newChannel(c *connection, id uint32, name string) *channel {
	return &channel{
		conn:          c,
		channelID:     id,
		inbox:         make(chan any, 64),
		dropped:       make(chan struct{}),
		nextMessageID: 1,
		name:          name,
	}
}

func (ch *channel) String() string {
	if ch.name != "" {
		return fmt.Sprintf("channel %d (%s)", ch.channelID, ch.name)
	}
	return fmt.Sprintf("channel %d", ch.channelID)
}

// ChannelID returns the numeric ID of this channel.
func (ch *channel) ChannelID() uint32 { return ch.channelID }

// putInbox delivers a packet to the channel's inbox.
func (ch *channel) putInbox(v any) {
	select {
	case ch.inbox <- v:
	default:
		// Panic if full — shouldn't happen with a generous buffer.
		ch.droppedOnce.Do(func() { close(ch.dropped) })
	}
}

// Close sends a close notification to the peer and marks the channel closed.
func (ch *channel) Close() {
	if ch.closed {
		return
	}
	ch.closed = true

	// Check if this channel is still registered (not already removed by the peer).
	ch.conn.writerMu.Lock()
	registered := ch.conn.channels[ch.channelID] == ch
	ch.conn.writerMu.Unlock()

	if registered {
		// Send asynchronously: write may block if the reader isn't consuming yet.
		go ch.conn.SendPacket(packet{ //nolint:errcheck
			ChannelID: ch.channelID,
			MessageID: closeChannelMessageID,
			IsReply:   false,
			Payload:   closeChannelPayload,
		})
	}
}

// SendRequestRaw sends raw bytes as a request and returns the message ID.
func (ch *channel) SendRequestRaw(payload []byte) (uint32, error) {
	msgID := ch.nextMessageID
	ch.nextMessageID++
	err := ch.conn.SendPacket(packet{
		ChannelID: ch.channelID,
		MessageID: msgID,
		IsReply:   false,
		Payload:   payload,
	})
	return msgID, err
}

// SendReplyRaw sends raw bytes as a reply to the given message ID.
func (ch *channel) SendReplyRaw(msgID uint32, payload []byte) error {
	return ch.conn.SendPacket(packet{
		ChannelID: ch.channelID,
		MessageID: msgID,
		IsReply:   true,
		Payload:   payload,
	})
}

// SendReplyValue sends a CBOR-encoded {"result": v} reply.
func (ch *channel) SendReplyValue(msgID uint32, v any) error {
	payload, err := encodeCBOR(map[string]any{"result": v})
	if err != nil {
		return err
	}
	return ch.SendReplyRaw(msgID, payload)
}

// SendReplyError sends a CBOR-encoded error reply with the given message and type.
func (ch *channel) SendReplyError(msgID uint32, errMsg, errType string) error {
	payload, err := encodeCBOR(map[string]any{
		"error": errMsg,
		"type":  errType,
	})
	if err != nil { //nocov
		panic(fmt.Sprintf("hegel: SendReplyError encode: %v", err)) //nocov
	}
	return ch.SendReplyRaw(msgID, payload)
}

// RecvRequestRaw waits for the next server-initiated request and returns
// (messageID, payload, error). timeout <= 0 means no timeout.
func (ch *channel) RecvRequestRaw(timeout time.Duration) (uint32, []byte, error) {
	for len(ch.requests) == 0 {
		if err := ch.processOneMessage(timeout); err != nil {
			return 0, nil, err
		}
	}
	pkt := ch.requests[0]
	ch.requests = ch.requests[1:]
	return pkt.MessageID, pkt.Payload, nil
}

// RecvRequest waits for the next server-initiated request and returns
// (messageID, CBOR-decoded payload, error).
func (ch *channel) RecvRequest(timeout time.Duration) (uint32, any, error) {
	msgID, payload, err := ch.RecvRequestRaw(timeout)
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
func (ch *channel) recvResponseRaw(msgID uint32, timeout time.Duration) ([]byte, error) {
	if ch.responses == nil {
		ch.responses = make(map[uint32][]byte)
	}
	for {
		if payload, ok := ch.responses[msgID]; ok {
			delete(ch.responses, msgID)
			return payload, nil
		}
		if err := ch.processOneMessage(timeout); err != nil {
			return nil, err
		}
	}
}

// ReceiveResponse waits for a reply to the given message ID and returns the
// CBOR-decoded result (unwrapping {"result": v} or raising requestError).
func (ch *channel) ReceiveResponse(msgID uint32, timeout time.Duration) (any, error) {
	raw, err := ch.recvResponseRaw(msgID, timeout)
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

// processOneMessage waits for a packet on the channel's inbox and routes it.
func (ch *channel) processOneMessage(timeout time.Duration) error {
	if ch.closed {
		return fmt.Errorf("%s is closed", ch)
	}

	var timeoutCh <-chan time.Time
	if timeout > 0 {
		timeoutCh = time.After(timeout)
	}

	var pkt packet
	select {
	case item := <-ch.inbox:
		if item == shutdownSentinel {
			ch.closed = true
			return fmt.Errorf("%s was closed", ch)
		}
		pkt = item.(packet)
	case <-ch.dropped:
		panic(fmt.Errorf("%s: dropped a message", ch))
	case <-ch.conn.done:
		select {
		case <-ch.conn.processExited:
			return ch.conn.serverCrashError()
		default:
		}
		return fmt.Errorf("connection closed")
	case <-timeoutCh:
		return fmt.Errorf("timed out after %v waiting for a message on %s", timeout, ch)
	}

	if pkt.IsReply {
		if ch.responses == nil {
			ch.responses = make(map[uint32][]byte)
		}
		ch.responses[pkt.MessageID] = pkt.Payload
	} else {
		ch.requests = append(ch.requests, pkt)
	}
	return nil
}

// Request sends a request and returns a pendingRequest future.
// If the write fails because the server process exited, a *connectionError is returned.
func (ch *channel) Request(payload []byte) (*pendingRequest, error) {
	msgID, err := ch.SendRequestRaw(payload)
	if err != nil {
		select {
		case <-ch.conn.processExited:
			return nil, ch.conn.serverCrashError()
		default:
		}
		return nil, err
	}
	return &pendingRequest{ch: ch, msgID: msgID}, nil
}

// pendingRequest is a future for an in-flight request.
type pendingRequest struct {
	ch    *channel
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
	v, err := p.ch.ReceiveResponse(p.msgID, 100*time.Second)
	p.value = v
	p.err = err
	p.done = true
	return v, err
}
