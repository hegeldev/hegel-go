package hegel

import (
	"bytes"
	"net"
	"testing"
	"time"
)

// --- Handshake helper ---

// clientConnPair creates a client connection that has completed handshake over a net.Pipe.
// The remote end of the pipe is returned for injecting raw bytes in tests.
// A goroutine reads the handshake request and replies with "Hegel/0.4".
func clientConnPair(t *testing.T) (*connection, net.Conn) {
	t.Helper()
	s, c := socketPair(t)
	clientConn := newConnection(c, "Client")
	t.Cleanup(func() { clientConn.Close() })

	// Raw handshake responder: read one packet, reply with "Hegel/0.4".
	go func() {
		// Read the handshake request packet.
		pkt, err := readPacket(s)
		if err != nil {
			return
		}
		// Reply with version string.
		writePacket(s, packet{ //nolint:errcheck
			ChannelID: pkt.ChannelID,
			MessageID: pkt.MessageID,
			IsReply:   true,
			Payload:   []byte(handshakePrefix + protocolVersion),
		})
	}()

	if err := clientConn.SendHandshake(); err != nil {
		t.Fatalf("SendHandshake: %v", err)
	}
	return clientConn, s
}

// --- connection done channel ---

func TestConnectionDone(t *testing.T) {
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	select {
	case <-conn.done:
		t.Error("new connection should not be done")
	default:
	}
	conn.Close()
	select {
	case <-conn.done:
	default:
		t.Error("closed connection should be done")
	}
}

// --- connection.Close idempotent ---

func TestConnectionDoubleClose(t *testing.T) {
	t.Parallel()
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	conn.Close()
	conn.Close() // must not panic or deadlock
}

// --- Handshake: double send raises ---

func TestDoubleSendHandshakeRaises(t *testing.T) {
	t.Parallel()
	clientConn, _ := clientConnPair(t)

	err := clientConn.SendHandshake()
	if err == nil {
		t.Fatal("expected error on double SendHandshake")
	}
	mustContain(t, err.Error(), "already established")
}

// --- Handshake: bad response from server ---

func TestSendHandshakeBadResponse(t *testing.T) {
	t.Parallel()
	s, c := socketPair(t)
	clientConn := newConnection(c, "Client")

	go func() {
		// Read the handshake request and send a bad response.
		pkt, err := readPacket(s)
		if err != nil {
			return
		}
		writePacket(s, packet{ //nolint:errcheck
			ChannelID: pkt.ChannelID,
			MessageID: pkt.MessageID,
			IsReply:   true,
			Payload:   []byte("NotHegel"),
		})
	}()

	_, err := clientConn.SendHandshakeVersion()
	if err == nil {
		t.Fatal("expected error for bad handshake response")
	}
	mustContain(t, err.Error(), "bad handshake")
}

// --- channel allocation: new_channel returns odd IDs ---

func TestNewChannelOddIDs(t *testing.T) {
	t.Parallel()
	clientConn, _ := clientConnPair(t)

	ch1 := clientConn.NewChannel("ch1")
	ch2 := clientConn.NewChannel("ch2")
	ch3 := clientConn.NewChannel("ch3")

	if ch1.ChannelID()%2 != 1 {
		t.Errorf("ch1 ID %d is not odd", ch1.ChannelID())
	}
	if ch2.ChannelID()%2 != 1 {
		t.Errorf("ch2 ID %d is not odd", ch2.ChannelID())
	}
	if ch3.ChannelID()%2 != 1 {
		t.Errorf("ch3 ID %d is not odd", ch3.ChannelID())
	}
	if ch1.ChannelID() == ch2.ChannelID() || ch2.ChannelID() == ch3.ChannelID() {
		t.Error("channel IDs must be unique")
	}
}

// --- new_channel before handshake raises ---

func TestNewChannelBeforeHandshakeRaises(t *testing.T) {
	t.Parallel()
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	defer conn.Close()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for new_channel before handshake")
		}
	}()
	conn.NewChannel("test")
}

// --- connect_channel before handshake raises ---

func TestConnectChannelBeforeHandshakeRaises(t *testing.T) {
	t.Parallel()
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	defer conn.Close()

	_, err := conn.ConnectChannel(1, "test")
	if err == nil {
		t.Fatal("expected error for connect_channel before handshake")
	}
	mustContain(t, err.Error(), "cannot create a new channel")
}

// --- connect_channel already exists raises ---

func TestConnectChannelAlreadyExistsRaises(t *testing.T) {
	t.Parallel()
	clientConn, _ := clientConnPair(t)

	// channel 0 (control channel) already exists.
	_, err := clientConn.ConnectChannel(0, "dup")
	if err == nil {
		t.Fatal("expected error for duplicate channel")
	}
	mustContain(t, err.Error(), "channel already connected")
}

// --- channel close: sends close packet, idempotent ---

func TestChannelClose(t *testing.T) {
	t.Parallel()
	clientConn, _ := clientConnPair(t)

	ch := clientConn.NewChannel("TestClose")
	ch.Close()
	ch.Close() // idempotent
}

// --- channel close when connection not live ---

func TestChannelCloseWhenConnectionNotLive(t *testing.T) {
	t.Parallel()
	clientConn, _ := clientConnPair(t)

	ch := clientConn.NewChannel("TestClose")
	clientConn.Close()
	ch.Close() // must not panic
}

// --- channel: closed channel rejects recv ---

func TestChannelProcessMessageWhenClosed(t *testing.T) {
	t.Parallel()
	clientConn, _ := clientConnPair(t)

	ch := clientConn.NewChannel("TestClosed")
	ch.Close()

	_, _, err := ch.RecvRequest(100 * time.Millisecond)
	if err == nil {
		t.Fatal("expected error receiving on closed channel")
	}
	mustContain(t, err.Error(), "closed")
}

// --- channel timeout ---

func TestChannelTimeout(t *testing.T) {
	t.Parallel()
	clientConn, _ := clientConnPair(t)

	ch := clientConn.NewChannel("TestTimeout")
	defer ch.Close()

	_, _, err := ch.RecvRequest(100 * time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	mustContain(t, err.Error(), "timed out")
}

// --- connection closed while waiting for message ---

func TestConnectionClosedWhileWaiting(t *testing.T) {
	t.Parallel()
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")

	ch := conn.ControlChannel()
	go func() {
		time.Sleep(10 * time.Millisecond)
		conn.Close()
	}()

	_, _, err := ch.RecvRequest(2 * time.Second)
	if err == nil {
		t.Fatal("expected error when connection closes")
	}
	mustContain(t, err.Error(), "connection closed")
}

// --- Message to nonexistent channel ---

func TestMessageToNonexistentChannel(t *testing.T) {
	t.Parallel()
	clientConn, remote := clientConnPair(t)

	// Send a request from the "server" to a nonexistent channel on the client.
	// Then send a control ping so the client's reader processes both.
	done := make(chan struct{})
	go func() {
		defer close(done)
		clientConn.ControlChannel().RecvRequestRaw(2 * time.Second) //nolint:errcheck
	}()

	// Send a packet from remote to a nonexistent channel.
	go func() {
		writePacket(remote, packet{ //nolint:errcheck
			ChannelID: 9999,
			MessageID: 1,
			IsReply:   false,
			Payload:   mustEncode(t, map[string]any{"command": "test"}),
		})
	}()

	// Send a control-channel request so the client processes the bad-channel packet.
	go func() {
		time.Sleep(20 * time.Millisecond)
		writePacket(remote, packet{ //nolint:errcheck
			ChannelID: 0,
			MessageID: 100,
			IsReply:   false,
			Payload:   []byte("ping"),
		})
	}()

	// Drain the error reply the client sends back to ch 9999.
	go func() {
		readPacket(remote) //nolint:errcheck
	}()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Error("TestMessageToNonexistentChannel timed out")
	}
}

// --- requestError ---

func TestRequestError(t *testing.T) {
	t.Parallel()
	data := map[any]any{
		any("error"): any("something went wrong"),
		any("type"):  any("TestError"),
		any("extra"): any("data"),
	}
	err := newRequestError(data)
	if err.Error() != "something went wrong" {
		t.Errorf("Error() = %q, want %q", err.Error(), "something went wrong")
	}
	if err.ErrorType != "TestError" {
		t.Errorf("ErrorType = %q, want %q", err.ErrorType, "TestError")
	}
}

// --- ResultOrError ---

func TestResultOrErrorRaises(t *testing.T) {
	t.Parallel()
	body := map[any]any{
		any("error"): any("bad"),
		any("type"):  any("TestError"),
	}
	_, err := resultOrError(body)
	if err == nil {
		t.Fatal("expected error")
	}
	mustContain(t, err.Error(), "bad")
}

func TestResultOrErrorReturnsResult(t *testing.T) {
	t.Parallel()
	body := map[any]any{any("result"): any(uint64(42))}
	v, err := resultOrError(body)
	if err != nil {
		t.Fatalf("resultOrError: %v", err)
	}
	n, _ := extractCBORInt(v)
	if n != 42 {
		t.Errorf("result = %v, want 42", v)
	}
}

// --- connection.Close with CloseRead (TCP conn interface) ---

// closeReadConn wraps a net.Conn and records calls to CloseRead.
type closeReadConn struct {
	net.Conn
	closed bool
}

func (c *closeReadConn) CloseRead() error {
	c.closed = true
	return nil
}

func TestConnectionCloseCallsCloseRead(t *testing.T) {
	t.Parallel()
	s, _ := socketPair(t)
	cr := &closeReadConn{Conn: s}
	conn := newConnection(cr, "Test")
	conn.Close()
	if !cr.closed {
		t.Error("expected CloseRead to be called")
	}
}

// --- dispatch: close-channel notification path ---

func TestDispatchCloseChannelNotification(t *testing.T) {
	t.Parallel()
	clientConn, remote := clientConnPair(t)

	clientCh := clientConn.NewChannel("ClosedCh")

	// Simulate the server sending a close-channel notification.
	go func() {
		writePacket(remote, packet{ //nolint:errcheck
			ChannelID: clientCh.ChannelID(),
			MessageID: closeChannelMessageID,
			IsReply:   false,
			Payload:   closeChannelPayload,
		})
	}()

	// A subsequent recv on the closed channel should return a close error.
	_, _, err := clientCh.RecvRequestRaw(2 * time.Second)
	if err == nil {
		t.Fatal("expected error after peer close")
	}
	mustContain(t, err.Error(), "closed")
}

func TestPeerCloseWakesBlockedGoroutine(t *testing.T) {
	t.Parallel()
	clientConn, remote := clientConnPair(t)

	clientCh := clientConn.NewChannel("BlockedCh")

	// Block a goroutine on RecvRequestRaw.
	errc := make(chan error, 1)
	go func() {
		_, _, err := clientCh.RecvRequestRaw(5 * time.Second)
		errc <- err
	}()

	// Give the goroutine time to block.
	time.Sleep(20 * time.Millisecond)

	// Send a close-channel notification from the peer.
	writePacket(remote, packet{ //nolint:errcheck
		ChannelID: clientCh.ChannelID(),
		MessageID: closeChannelMessageID,
		IsReply:   false,
		Payload:   closeChannelPayload,
	})

	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("expected error from RecvRequestRaw after peer close")
		}
		mustContain(t, err.Error(), "closed")
	case <-time.After(5 * time.Second):
		t.Fatal("blocked goroutine was not woken by peer close")
	}
}

// --- dispatch: message to unknown channel (request) → sends error reply ---
// --- dispatch: message to unknown channel (reply) → silently dropped ---

func TestDispatchUnknownChannelIsReply(t *testing.T) {
	t.Parallel()
	clientConn, remote := clientConnPair(t)

	// Send an IsReply=true packet from remote to a nonexistent channel — should be silently dropped.
	done := make(chan struct{})
	go func() {
		defer close(done)
		clientConn.ControlChannel().RecvRequestRaw(500 * time.Millisecond) //nolint:errcheck
	}()

	go func() {
		writePacket(remote, packet{ //nolint:errcheck
			ChannelID: 9997,
			MessageID: 1,
			IsReply:   true,
			Payload:   mustEncode(t, map[string]any{"result": 1}),
		})
	}()
	go func() {
		time.Sleep(20 * time.Millisecond)
		// Send a control-channel request so client processes the bad packet.
		writePacket(remote, packet{ //nolint:errcheck
			ChannelID: 0,
			MessageID: 100,
			IsReply:   false,
			Payload:   []byte("ping"),
		})
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("TestDispatchUnknownChannelIsReply timed out")
	}
}

func TestDispatchUnknownChannelRequest(t *testing.T) {
	t.Parallel()
	clientConn, remote := clientConnPair(t)

	// Send a request (IsReply=false) to a nonexistent client channel.
	// The client should send an error reply.
	done := make(chan struct{})
	go func() {
		defer close(done)
		clientConn.ControlChannel().RecvRequestRaw(2 * time.Second) //nolint:errcheck
	}()

	go func() {
		writePacket(remote, packet{ //nolint:errcheck
			ChannelID: 9998,
			MessageID: 1,
			IsReply:   false,
			Payload:   mustEncode(t, map[string]any{"command": "test"}),
		})
	}()

	// Drain the error reply from client.
	go func() {
		readPacket(remote) //nolint:errcheck
	}()

	// Send control ping so client exits its read loop.
	go func() {
		time.Sleep(30 * time.Millisecond)
		writePacket(remote, packet{ //nolint:errcheck
			ChannelID: 0,
			MessageID: 100,
			IsReply:   false,
			Payload:   []byte("ping"),
		})
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("TestDispatchUnknownChannelRequest timed out")
	}
}

// --- SendHandshakeVersion: error from SendRequestRaw (closed connection) ---

func TestSendHandshakeVersionSendError(t *testing.T) {
	t.Parallel()
	s, c := net.Pipe()
	conn := newConnection(s, "Test")
	// Close both ends so SendRequestRaw fails.
	s.Close()
	c.Close()

	_, err := conn.SendHandshakeVersion()
	if err == nil {
		t.Fatal("expected error from SendHandshakeVersion on closed conn")
	}
}

// --- SendHandshakeVersion: error from recvResponseRaw (connection closed mid-handshake) ---

func TestSendHandshakeVersionRecvError(t *testing.T) {
	t.Parallel()
	s, c := net.Pipe()
	conn := newConnection(s, "Test")
	// Close the peer end immediately after accepting the write.
	go func() {
		// Drain the handshake request so SendRequestRaw unblocks, then close.
		buf := make([]byte, 64)
		c.Read(buf) //nolint:errcheck
		c.Close()
	}()
	_, err := conn.SendHandshakeVersion()
	if err == nil {
		t.Fatal("expected error when peer closes after handshake send")
	}
	s.Close()
}

// --- newRequestError: non-string key is skipped ---

func TestNewRequestErrorNonStringKey(t *testing.T) {
	t.Parallel()
	data := map[any]any{
		any("error"):   any("oops"),
		any("type"):    any("E"),
		any(uint64(1)): any("ignored"), // non-string key
	}
	re := newRequestError(data)
	if re.Error() != "oops" {
		t.Errorf("Error() = %q, want %q", re.Error(), "oops")
	}
	// The integer key should not appear in Data (it was skipped).
	if _, ok := re.Data[any(uint64(1))]; ok {
		t.Error("non-string key should be skipped in Data")
	}
}

// --- putInbox: default drop path (inbox full) ---

func TestChannelPanicsOnDroppedMessage(t *testing.T) {
	t.Parallel()
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	defer conn.Close()

	ch := conn.ControlChannel()
	for i := range cap(ch.inbox) {
		ch.inbox <- packet{ChannelID: 0, MessageID: uint32(i)}
	}
	// This call should hit the default case and enqueue a panic for the next receive.
	ch.putInbox(packet{ChannelID: 0, MessageID: 99})
	if len(ch.inbox) != cap(ch.inbox) {
		t.Errorf("inbox isn't full: %d < %d", len(ch.inbox), cap(ch.inbox))
	}

	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("Expected Recv to panic")
			}
		}()
		_, _ = ch.ReceiveResponse(999, time.Second)
	}()

}

// --- SendReplyValue: CBOR encode error ---

func TestSendReplyValueEncodeError(t *testing.T) {
	t.Parallel()
	s, c := socketPair(t)
	defer s.Close()
	defer c.Close()
	conn := newConnection(s, "Test")

	// Build a channel manually without handshake for encode error testing.
	ch := &channel{conn: conn, channelID: 0, inbox: make(chan any, 1), nextMessageID: 1}

	// func() cannot be CBOR-encoded — triggers encode error.
	err := ch.SendReplyValue(1, func() {})
	if err == nil {
		t.Fatal("expected encode error from SendReplyValue")
	}
}

// --- SendReplyError: verify happy path ---

func TestSendReplyErrorSucceeds(t *testing.T) {
	t.Parallel()
	s, c := socketPair(t)
	defer s.Close()
	defer c.Close()
	conn := newConnection(s, "Test")
	ch := &channel{conn: conn, channelID: 0, inbox: make(chan any, 1), nextMessageID: 1}
	errc := make(chan error, 1)
	go func() { errc <- ch.SendReplyError(1, "msg", "Type") }()
	// Drain from peer so write unblocks.
	buf := make([]byte, 256)
	c.Read(buf) //nolint:errcheck
	if err := <-errc; err != nil {
		t.Errorf("SendReplyError: %v", err)
	}
}

// --- RecvRequest: CBOR decode error path ---

func TestRecvRequestDecodeCBORError(t *testing.T) {
	t.Parallel()
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	defer conn.Close()

	ch := conn.ControlChannel()
	// Put a raw packet with invalid CBOR payload directly into the inbox.
	ch.inbox <- packet{ChannelID: 0, MessageID: 1, IsReply: false, Payload: []byte{0xFF}}

	_, _, err := ch.RecvRequest(100 * time.Millisecond)
	if err == nil {
		t.Fatal("expected CBOR decode error from RecvRequest")
	}
}

// --- recvResponseRaw: processOneMessage error path ---

func TestRecvResponseRawProcessError(t *testing.T) {
	t.Parallel()
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")

	ch := conn.ControlChannel()
	// Close connection so processOneMessage returns an error.
	go func() {
		time.Sleep(10 * time.Millisecond)
		conn.Close()
	}()

	_, err := ch.recvResponseRaw(1, 2*time.Second)
	if err == nil {
		t.Fatal("expected error from recvResponseRaw when connection closed")
	}
}

// --- ReceiveResponse: CBOR decode error ---

func TestReceiveResponseDecodeCBORError(t *testing.T) {
	t.Parallel()
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	defer conn.Close()

	ch := conn.ControlChannel()
	// Inject a reply with invalid CBOR into the inbox.
	ch.inbox <- packet{ChannelID: 0, MessageID: 1, IsReply: true, Payload: []byte{0xFF}}

	_, err := ch.ReceiveResponse(1, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected CBOR decode error from ReceiveResponse")
	}
}

// --- ReceiveResponse: extractCBORDict error (payload is not a map) ---

func TestReceiveResponseExtractCBORDictError(t *testing.T) {
	t.Parallel()
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	defer conn.Close()

	ch := conn.ControlChannel()
	// Inject a reply whose payload is a CBOR integer (not a dict).
	payload, _ := encodeCBOR(int64(42))
	ch.inbox <- packet{ChannelID: 0, MessageID: 1, IsReply: true, Payload: payload}

	_, err := ch.ReceiveResponse(1, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected extractCBORDict error from ReceiveResponse")
	}
}

// --- ReceiveResponse: recvResponseRaw error path ---

func TestReceiveResponseRecvError(t *testing.T) {
	t.Parallel()
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")

	ch := conn.ControlChannel()
	// Close connection so recvResponseRaw returns an error.
	go func() {
		time.Sleep(10 * time.Millisecond)
		conn.Close()
	}()

	_, err := ch.ReceiveResponse(1, 2*time.Second)
	if err == nil {
		t.Fatal("expected error from ReceiveResponse when connection closed")
	}
}

// --- processOneMessage: reply packet routed to responses map (ch.responses nil) ---

func TestProcessOneMessageRouteReplyNilResponses(t *testing.T) {
	t.Parallel()
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	defer conn.Close()

	ch := conn.ControlChannel()
	// ch.responses is nil (never initialized).
	// Put a reply packet directly in the inbox and call processOneMessage.
	payload, _ := encodeCBOR(map[string]any{"result": int64(7)})
	ch.inbox <- packet{ChannelID: 0, MessageID: 5, IsReply: true, Payload: payload}

	// processOneMessage is called with ch.responses == nil, exercising the init path.
	if err := ch.processOneMessage(100 * time.Millisecond); err != nil {
		t.Fatalf("processOneMessage: %v", err)
	}
	if ch.responses == nil {
		t.Error("ch.responses should have been initialized")
	}
	if _, ok := ch.responses[5]; !ok {
		t.Error("ch.responses[5] should contain the routed reply")
	}
}

// --- channelName: unnamed channel returns "channel N" ---

func TestChannelNameUnnamed(t *testing.T) {
	t.Parallel()
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	defer conn.Close()

	// Create a channel with an empty name to exercise the unnamed branch.
	ch := newChannel(conn, 42, "")
	name := ch.String()
	mustContain(t, name, "channel 42")
}

// --- Request: SendRequestRaw error path ---

func TestRequestSendError(t *testing.T) {
	t.Parallel()
	s, c := net.Pipe()
	conn := newConnection(s, "Test")
	// Close both ends so SendRequestRaw fails.
	s.Close()
	c.Close()

	ch := &channel{conn: conn, channelID: 0, inbox: make(chan any, 1), nextMessageID: 1}
	_, err := ch.Request([]byte("test"))
	if err == nil {
		t.Fatal("expected error from Request on closed conn")
	}
}

// --- RecvRequest: happy path ---

func TestRecvRequestHappyPath(t *testing.T) {
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	defer conn.Close()

	ch := conn.ControlChannel()
	// Inject a valid request packet with CBOR payload.
	payload, _ := encodeCBOR(map[string]any{"command": "test"})
	ch.inbox <- packet{ChannelID: 0, MessageID: 1, IsReply: false, Payload: payload}

	msgID, v, err := ch.RecvRequest(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("RecvRequest: %v", err)
	}
	if msgID != 1 {
		t.Errorf("msgID = %d, want 1", msgID)
	}
	if v == nil {
		t.Error("expected non-nil decoded value")
	}
}

// --- pendingRequest.Get: cached return ---

func TestPendingRequestGetCached(t *testing.T) {
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	defer conn.Close()

	ch := conn.ControlChannel()
	// Inject a reply so ReceiveResponse succeeds.
	replyPayload, _ := encodeCBOR(map[string]any{"result": int64(42)})
	ch.inbox <- packet{ChannelID: 0, MessageID: 1, IsReply: true, Payload: replyPayload}

	p := &pendingRequest{ch: ch, msgID: 1}
	v1, err1 := p.Get()
	if err1 != nil {
		t.Fatalf("first Get: %v", err1)
	}
	// Second call should return cached value.
	v2, err2 := p.Get()
	if err2 != nil {
		t.Fatalf("second Get: %v", err2)
	}
	if v1 != v2 {
		t.Errorf("cached Get returned different value: %v vs %v", v1, v2)
	}
}

// --- SendControlRequest: error from Request (closed connection) ---

func TestSendControlRequestSendError(t *testing.T) {
	t.Parallel()
	s, c := net.Pipe()
	conn := newConnection(s, "Test")
	// Close both ends so SendRequestRaw fails.
	s.Close()
	c.Close()

	_, err := conn.SendControlRequest([]byte("test"))
	if err == nil {
		t.Fatal("expected error from SendControlRequest on closed conn")
	}
}

// --- helpers ---

func mustContain(t *testing.T, s, sub string) {
	t.Helper()
	if !bytes.Contains([]byte(s), []byte(sub)) {
		t.Errorf("%q does not contain %q", s, sub)
	}
}

func mustEncode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := encodeCBOR(v)
	if err != nil {
		t.Fatalf("encodeCBOR(%v): %v", v, err)
	}
	return b
}
