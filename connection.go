package hegel

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// protocolVersion is the version string used in handshakes.
const protocolVersion = "0.3"

// handshakePrefix is the prefix expected at the start of a valid handshake response.
const handshakePrefix = "Hegel/"

// handshakeRequest is the fixed bytes sent by the client to initiate a handshake.
var handshakeRequest = []byte("hegel_handshake_start")

// shutdownSentinel is placed in a channel's inbox to signal connection close.
var shutdownSentinel = &struct{}{}

// connectionState tracks whether the connection has performed a handshake.
type connectionState int

const (
	stateUnresolved connectionState = iota
	stateClient
	stateServer
)

// connection manages a multiplexed socket with a demand-driven reader.
// It is safe to call Close from any goroutine; all other methods must be called
// from a single goroutine per connection (the reader lock provides ordering).
type connection struct {
	name    string
	conn    net.Conn
	running atomic.Bool

	nextChannelID int
	channels      map[uint32]*channel
	state         connectionState

	writerMu sync.Mutex
	readerMu sync.Mutex

	controlCh *channel
}

// newConnection wraps conn in a new connection and registers the control channel (ID 0).
func newConnection(conn net.Conn, name string) *connection {
	c := &connection{
		name:          name,
		conn:          conn,
		channels:      make(map[uint32]*channel),
		state:         stateUnresolved,
		nextChannelID: 1, // first real channel counter (matches Python's __next_channel_id = 1)
	}
	c.running.Store(true)
	// channel 0 is the control channel; it is pre-registered before any handshake.
	c.controlCh = &channel{
		conn:          c,
		channelID:     0,
		inbox:         make(chan any, 64),
		nextMessageID: 1,
	}
	c.channels[0] = c.controlCh
	return c
}

// Live reports whether the connection is still open.
func (c *connection) Live() bool {
	return c.running.Load()
}

// ControlChannel returns the channel used for handshake and control messages.
func (c *connection) ControlChannel() *channel { return c.controlCh }

// SendPacket sends a packet to the peer. It is safe to call concurrently.
func (c *connection) SendPacket(pkt packet) error {
	c.writerMu.Lock()
	defer c.writerMu.Unlock()
	return writePacket(c.conn, pkt)
}

// Close shuts down the connection and signals all channels.
func (c *connection) Close() {
	c.writerMu.Lock()
	if !c.running.Load() {
		c.writerMu.Unlock()
		return
	}
	c.running.Store(false)
	channels := make([]*channel, 0, len(c.channels))
	for _, ch := range c.channels {
		channels = append(channels, ch)
	}
	c.writerMu.Unlock()

	// Shut down the socket to unblock any pending reads.
	if tc, ok := c.conn.(interface{ CloseRead() error }); ok {
		tc.CloseRead() //nolint:errcheck
	}
	c.conn.Close() //nolint:errcheck

	// Signal all channel inboxes so waiters unblock.
	for _, ch := range channels {
		ch.putInbox(shutdownSentinel)
	}
}

// runReader is the demand-driven reader loop.
// It reads packets from the socket and dispatches them to the correct channel's inbox
// until the until() predicate returns true, the connection closes, or a short timeout elapses.
func (c *connection) runReader(until func() bool) {
	if until() {
		return
	}
	acquired := false
	defer func() {
		if acquired {
			c.readerMu.Unlock()
		}
	}()

	for {
		acquired = c.readerMu.TryLock()
		if acquired {
			break
		}
		if until() {
			return
		}
		time.Sleep(time.Millisecond)
	}

	for c.running.Load() && !until() {
		// Set a short deadline so we can re-check until() periodically.
		c.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)) //nolint:errcheck
		pkt, err := readPacket(c.conn)
		c.conn.SetReadDeadline(time.Time{}) //nolint:errcheck
		if err != nil {
			if isTimeout(err) {
				continue
			}
			// connection error — stop reading.
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

	if bytes.Equal(pkt.Payload, CloseChannelPayload) && pkt.MessageID == CloseChannelMessageID {
		// channel close notification — remove the channel.
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
			errPayload, encErr := EncodeCBOR(map[string]any{"error": errMsg})
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

// isTimeout reports whether err is a network timeout.
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if ne, ok := err.(net.Error); ok {
		return ne.Timeout()
	}
	return false
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

// ReceiveHandshake performs the server side of the handshake.
func (c *connection) ReceiveHandshake() error {
	c.writerMu.Lock()
	if c.state != stateUnresolved {
		c.writerMu.Unlock()
		return fmt.Errorf("handshake already established")
	}
	c.state = stateServer
	c.writerMu.Unlock()

	msgID, payload, err := c.controlCh.RecvRequestRaw(10 * time.Second)
	if err != nil {
		return err
	}
	if !bytes.Equal(payload, handshakeRequest) {
		return fmt.Errorf("bad handshake: expected %q, got %q", handshakeRequest, payload)
	}
	response := []byte(handshakePrefix + protocolVersion)
	return c.controlCh.SendReplyRaw(msgID, response)
}

// NewChannel allocates a new client-side logical channel. Panics if called before
// the handshake is complete (matching Python's ValueError).
func (c *connection) NewChannel(name string) *channel {
	c.writerMu.Lock()
	defer c.writerMu.Unlock()

	if c.state == stateUnresolved {
		panic("Cannot create a new channel before handshake has been performed")
	}

	var channelID uint32
	if c.state == stateClient {
		// Client channels are odd: (counter << 1) | 1
		channelID = uint32((c.nextChannelID << 1) | 1)
	} else {
		// Server channels are even: (counter << 1) | 0
		channelID = uint32(c.nextChannelID << 1)
	}
	c.nextChannelID++

	ch := &channel{
		conn:          c,
		channelID:     channelID,
		inbox:         make(chan any, 64),
		nextMessageID: 1,
		name:          name,
	}
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

	ch := &channel{
		conn:          c,
		channelID:     id,
		inbox:         make(chan any, 64),
		nextMessageID: 1,
		name:          name,
	}
	c.channels[id] = ch
	return ch, nil
}

// RequestError is an error response received from the peer.
type RequestError struct {
	msg       string
	ErrorType string
	Data      map[any]any
}

// Error implements the error interface.
func (e *RequestError) Error() string { return e.msg }

// newRequestError builds a RequestError from a CBOR-decoded error dict.
func newRequestError(data map[any]any) *RequestError {
	msg, _ := extractString(data[any("error")])
	errType, _ := extractString(data[any("type")])
	rest := make(map[any]any)
	for k, v := range data {
		s, err := extractString(k)
		if err != nil {
			continue
		}
		if s != "error" && s != "type" {
			rest[k] = v
		}
	}
	return &RequestError{msg: msg, ErrorType: errType, Data: rest}
}

// resultOrError extracts the "result" field from a CBOR-decoded dict, or returns
// a *RequestError if the dict contains an "error" field.
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
	nextMessageID uint32
	responses     map[uint32][]byte
	requests      []packet
	closed        bool
	name          string
}

// ChannelID returns the numeric ID of this channel.
func (ch *channel) ChannelID() uint32 { return ch.channelID }

// putInbox delivers a value (packet or shutdownSentinel) to the channel's inbox.
func (ch *channel) putInbox(v any) {
	select {
	case ch.inbox <- v:
	default:
		// Drop if full — shouldn't happen with a generous buffer.
	}
}

// Close sends a close notification to the peer and marks the channel closed.
func (ch *channel) Close() {
	if ch.closed {
		return
	}
	// Check if this channel is still registered (not already removed by the peer).
	ch.conn.writerMu.Lock()
	registered := ch.conn.channels[ch.channelID] == ch
	live := ch.conn.running.Load()
	ch.conn.writerMu.Unlock()

	ch.closed = true
	if registered && live {
		// Send asynchronously: write may block if the reader isn't consuming yet.
		go ch.conn.SendPacket(packet{ //nolint:errcheck
			ChannelID: ch.channelID,
			MessageID: CloseChannelMessageID,
			IsReply:   false,
			Payload:   CloseChannelPayload,
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
	payload, err := EncodeCBOR(map[string]any{"result": v})
	if err != nil {
		return err
	}
	return ch.SendReplyRaw(msgID, payload)
}

// SendReplyError sends a CBOR-encoded error reply with the given message and type.
func (ch *channel) SendReplyError(msgID uint32, errMsg, errType string) error {
	payload, err := EncodeCBOR(map[string]any{
		"error": errMsg,
		"type":  errType,
	})
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: SendReplyError encode: %v", err))
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
	v, err := DecodeCBOR(payload)
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
// CBOR-decoded result (unwrapping {"result": v} or raising RequestError).
func (ch *channel) ReceiveResponse(msgID uint32, timeout time.Duration) (any, error) {
	raw, err := ch.recvResponseRaw(msgID, timeout)
	if err != nil {
		return nil, err
	}
	v, err := DecodeCBOR(raw)
	if err != nil {
		return nil, err
	}
	m, err := extractDict(v)
	if err != nil {
		return nil, err
	}
	return resultOrError(m)
}

// processOneMessage calls runReader until the channel's inbox has something,
// then dequeues and routes it.
func (ch *channel) processOneMessage(timeout time.Duration) error {
	start := time.Now()
	needsMessages := func() bool {
		return ch.closed || len(ch.inbox) > 0 ||
			(timeout > 0 && time.Since(start) > timeout)
	}

	ch.conn.runReader(needsMessages)

	if ch.closed {
		return fmt.Errorf("%s is closed", ch.channelName())
	}

	select {
	case item := <-ch.inbox:
		if item == shutdownSentinel {
			return fmt.Errorf("connection closed")
		}
		pkt := item.(packet)
		if pkt.IsReply {
			if ch.responses == nil {
				ch.responses = make(map[uint32][]byte)
			}
			ch.responses[pkt.MessageID] = pkt.Payload
		} else {
			ch.requests = append(ch.requests, pkt)
		}
		return nil
	default:
		// Nothing arrived — must be timeout.
		if timeout > 0 && time.Since(start) >= timeout {
			return fmt.Errorf("timed out after %v waiting for a message on %s", timeout, ch.channelName())
		}
		return fmt.Errorf("timed out waiting for a message on %s", ch.channelName())
	}
}

func (ch *channel) channelName() string {
	if ch.name != "" {
		return ch.name
	}
	return fmt.Sprintf("channel %d", ch.channelID)
}

// Request sends a request and returns a pendingRequest future.
func (ch *channel) Request(payload []byte) (*pendingRequest, error) {
	msgID, err := ch.SendRequestRaw(payload)
	if err != nil {
		return nil, err
	}
	return &pendingRequest{ch: ch, msgID: msgID}, nil
}

// HandleRequests processes incoming requests with handler until stopFn returns true.
// The handler receives the raw CBOR request payload and returns a Go value (any) that
// will be sent as {"result": v}. On error, {"error": ..., "type": ...} is sent.
// If stopFn is nil, it runs indefinitely until the connection dies.
func (ch *channel) HandleRequests(handler func([]byte) (any, error), stopFn func() bool) {
	for {
		if stopFn != nil && stopFn() {
			return
		}
		msgID, payload, err := ch.RecvRequestRaw(0)
		if err != nil {
			return
		}
		result, handlerErr := handler(payload)
		if handlerErr != nil {
			ch.SendReplyError(msgID, handlerErr.Error(), fmt.Sprintf("%T", handlerErr)) //nolint:errcheck
		} else {
			ch.SendReplyValue(msgID, result) //nolint:errcheck
		}
	}
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
	v, err := p.ch.ReceiveResponse(p.msgID, 10*time.Second)
	p.value = v
	p.err = err
	p.done = true
	return v, err
}
