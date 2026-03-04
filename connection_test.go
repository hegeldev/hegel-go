package hegel

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

// --- Handshake helpers ---

// handshakePair performs a concurrent handshake between server and client connections.
func handshakePair(t *testing.T, serverConn, clientConn *connection) {
	t.Helper()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := serverConn.ReceiveHandshake(); err != nil {
			t.Errorf("ReceiveHandshake: %v", err)
		}
	}()
	if err := clientConn.SendHandshake(); err != nil {
		t.Errorf("SendHandshake: %v", err)
	}
	wg.Wait()
}

// connPair returns two connected Connections (server, client) backed by a net.Pipe.
func connPair(t *testing.T) (*connection, *connection) {
	t.Helper()
	s, c := socketPair(t)
	server := newConnection(s, "Server")
	client := newConnection(c, "Client")
	t.Cleanup(func() {
		server.Close()
		client.Close()
	})
	return server, client
}

// --- connection.live ---

func TestConnectionLive(t *testing.T) {
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	if !conn.Live() {
		t.Error("new connection should be live")
	}
	conn.Close()
	if conn.Live() {
		t.Error("closed connection should not be live")
	}
}

// --- connection.Close idempotent ---

func TestConnectionDoubleClose(t *testing.T) {
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	conn.Close()
	conn.Close() // must not panic or deadlock
}

// --- Handshake: send returns server version ---

func TestSendHandshakeReturnsVersion(t *testing.T) {
	serverConn, clientConn := connPair(t)

	done := make(chan error, 1)
	go func() { done <- serverConn.ReceiveHandshake() }()

	version, err := clientConn.SendHandshakeVersion()
	if err != nil {
		t.Fatalf("SendHandshakeVersion: %v", err)
	}
	if version != "0.1" {
		t.Errorf("version = %q, want %q", version, "0.1")
	}
	if err := <-done; err != nil {
		t.Errorf("ReceiveHandshake: %v", err)
	}
}

// --- Handshake: double send raises ---

func TestDoubleSendHandshakeRaises(t *testing.T) {
	serverConn, clientConn := connPair(t)
	handshakePair(t, serverConn, clientConn)

	err := clientConn.SendHandshake()
	if err == nil {
		t.Fatal("expected error on double SendHandshake")
	}
	mustContain(t, err.Error(), "already established")
}

// --- Handshake: double receive raises ---

func TestDoubleReceiveHandshakeRaises(t *testing.T) {
	serverConn, clientConn := connPair(t)

	done := make(chan error, 1)
	go func() {
		if err := serverConn.ReceiveHandshake(); err != nil {
			done <- err
			return
		}
		done <- serverConn.ReceiveHandshake()
	}()

	if err := clientConn.SendHandshake(); err != nil {
		t.Fatalf("SendHandshake: %v", err)
	}
	err := <-done
	if err == nil {
		t.Fatal("expected error on double ReceiveHandshake")
	}
	mustContain(t, err.Error(), "already established")
}

// --- Handshake: bad version from client ---

func TestBadHandshakeFromClient(t *testing.T) {
	serverConn, clientConn := connPair(t)

	done := make(chan error, 1)
	go func() {
		// Send a bad handshake string directly on the control channel.
		ch := clientConn.ControlChannel()
		_, err := ch.SendRequestRaw([]byte("BadVersion"))
		done <- err
	}()

	err := serverConn.ReceiveHandshake()
	if err == nil {
		t.Fatal("expected error for bad handshake")
	}
	mustContain(t, err.Error(), "bad handshake")
	<-done
}

// --- Handshake: bad response from server ---

func TestSendHandshakeBadResponse(t *testing.T) {
	serverConn, clientConn := connPair(t)

	done := make(chan error, 1)
	go func() {
		// Server receives the handshake request and sends a bad response.
		ch := serverConn.ControlChannel()
		msgID, _, err := ch.RecvRequestRaw(5 * time.Second)
		if err != nil {
			done <- err
			return
		}
		done <- ch.SendReplyRaw(msgID, []byte("NotHegel"))
	}()

	_, err := clientConn.SendHandshakeVersion()
	if err == nil {
		t.Fatal("expected error for bad handshake response")
	}
	mustContain(t, err.Error(), "bad handshake")
	<-done
}

// --- channel allocation: new_channel returns odd IDs ---

func TestNewChannelOddIDs(t *testing.T) {
	serverConn, clientConn := connPair(t)
	handshakePair(t, serverConn, clientConn)

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
	serverConn, clientConn := connPair(t)
	handshakePair(t, serverConn, clientConn)

	// channel 0 (control channel) already exists.
	_, err := clientConn.ConnectChannel(0, "dup")
	if err == nil {
		t.Fatal("expected error for duplicate channel")
	}
	mustContain(t, err.Error(), "channel already connected")
}

// --- channel close: sends close packet, idempotent ---

func TestChannelClose(t *testing.T) {
	serverConn, clientConn := connPair(t)
	handshakePair(t, serverConn, clientConn)

	ch := clientConn.NewChannel("TestClose")
	ch.Close()
	ch.Close() // idempotent
}

// --- channel close when connection not live ---

func TestChannelCloseWhenConnectionNotLive(t *testing.T) {
	serverConn, clientConn := connPair(t)
	handshakePair(t, serverConn, clientConn)

	ch := clientConn.NewChannel("TestClose")
	clientConn.Close()
	ch.Close() // must not panic
	serverConn.Close()
}

// --- channel: closed channel rejects recv ---

func TestChannelProcessMessageWhenClosed(t *testing.T) {
	serverConn, clientConn := connPair(t)
	handshakePair(t, serverConn, clientConn)

	ch := clientConn.NewChannel("TestClosed")
	ch.Close()

	_, _, err := ch.RecvRequest(100 * time.Millisecond)
	if err == nil {
		t.Fatal("expected error receiving on closed channel")
	}
	mustContain(t, err.Error(), "closed")
	serverConn.Close()
}

// --- channel timeout ---

func TestChannelTimeout(t *testing.T) {
	serverConn, clientConn := connPair(t)
	handshakePair(t, serverConn, clientConn)

	ch := clientConn.NewChannel("TestTimeout")
	defer ch.Close()

	_, _, err := ch.RecvRequest(100 * time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	mustContain(t, err.Error(), "timed out")
	serverConn.Close()
}

// --- SHUTDOWN in inbox raises ---

func TestShutdownInInboxRaises(t *testing.T) {
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	defer conn.Close()

	ch := conn.ControlChannel()
	ch.putInbox(shutdownSentinel)

	_, _, err := ch.RecvRequest(100 * time.Millisecond)
	if err == nil {
		t.Fatal("expected error for SHUTDOWN in inbox")
	}
	mustContain(t, err.Error(), "connection closed")
}

// --- Message to nonexistent channel ---

func TestMessageToNonexistentChannel(t *testing.T) {
	serverConn, clientConn := connPair(t)
	handshakePair(t, serverConn, clientConn)

	// Have the server receive a request on the control channel from the client.
	// Interleave: client sends to a nonexistent channel, then a control ping.
	// The server's runReader will dispatch: bad channel → error reply, then the ping.
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Server reads until it gets the control channel message (the ping).
		serverConn.ControlChannel().RecvRequestRaw(2 * time.Second) //nolint:errcheck
	}()

	// Client: send bad-channel packet, then a control ping (so server sees both).
	// Use goroutines because net.Pipe writes block until read.
	go func() {
		clientConn.SendPacket(packet{ //nolint:errcheck
			ChannelID: 9999,
			MessageID: 1,
			IsReply:   false,
			Payload:   mustEncode(t, map[string]any{"command": "test"}),
		})
	}()
	// The server's error reply to channel 9999 will come back to the client;
	// drain it to unblock the server write.
	go func() {
		// Force the client to read (server will write an error reply to ch 9999).
		// We discard the reply since ch 9999 doesn't exist on the client either.
		clientConn.runReader(func() bool { return !clientConn.Live() })
	}()

	// Give server a ping to ensure it processes the bad packet before blocking.
	go func() {
		clientConn.ControlChannel().SendRequestRaw([]byte("ping")) //nolint:errcheck
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

// --- Request handling (server creates channel, client connects) ---

func TestRequestHandling(t *testing.T) {
	serverConn, clientConn := connPair(t)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := serverConn.ReceiveHandshake(); err != nil {
			t.Errorf("ReceiveHandshake: %v", err)
			return
		}
		handlerCh := serverConn.NewChannel("Handler")
		handlerCh.HandleRequests(func(payload []byte) (any, error) {
			v, err := decodeCBOR(payload)
			if err != nil {
				return nil, err
			}
			m, err := extractCBORDict(v)
			if err != nil {
				return nil, err
			}
			x, err := extractCBORInt(m["x"])
			if err != nil {
				return nil, err
			}
			y, err := extractCBORInt(m["y"])
			if err != nil {
				return nil, err
			}
			return map[string]any{"sum": x + y}, nil
		}, nil)
	}()

	if err := clientConn.SendHandshake(); err != nil {
		t.Fatalf("SendHandshake: %v", err)
	}

	// Server creates channel 2 (first odd after control=0, next_id starts at 1 → 2*1|0 for server)
	// Actually server is in SERVER state so channel_id = (1<<1)|0 = 2
	sendCh, err := clientConn.ConnectChannel(2, "send")
	if err != nil {
		t.Fatalf("ConnectChannel: %v", err)
	}

	payload := mustEncode(t, map[string]any{"x": int64(2), "y": int64(3)})
	pending, err := sendCh.Request(payload)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	result, err := pending.Get()
	if err != nil {
		t.Fatalf("pending.Get: %v", err)
	}

	m, err := extractCBORDict(result)
	if err != nil {
		t.Fatalf("extractCBORDict: %v", err)
	}
	sum, err := extractCBORInt(m["sum"])
	if err != nil {
		t.Fatalf("extractCBORInt sum: %v", err)
	}
	if sum != 5 {
		t.Errorf("sum = %d, want 5", sum)
	}

	clientConn.Close()
	wg.Wait()
}

// --- pendingRequest caching ---

func TestPendingRequestCaching(t *testing.T) {
	serverConn, clientConn := connPair(t)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		serverConn.ReceiveHandshake() //nolint:errcheck
		ch := serverConn.NewChannel("PR")
		ch.HandleRequests(func(payload []byte) (any, error) {
			v, _ := decodeCBOR(payload)
			m, _ := extractCBORDict(v)
			val, _ := extractCBORInt(m["value"])
			return val * 2, nil
		}, nil)
	}()

	clientConn.SendHandshake() //nolint:errcheck
	ch, err := clientConn.ConnectChannel(2, "PR")
	if err != nil {
		t.Fatalf("ConnectChannel: %v", err)
	}

	payload := mustEncode(t, map[string]any{"value": int64(21)})
	pending, err := ch.Request(payload)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	v1, err := pending.Get()
	if err != nil {
		t.Fatalf("Get 1: %v", err)
	}
	v2, err := pending.Get()
	if err != nil {
		t.Fatalf("Get 2: %v", err)
	}

	n1, _ := extractCBORInt(v1)
	n2, _ := extractCBORInt(v2)
	if n1 != 42 || n2 != 42 {
		t.Errorf("pending caching: got %d, %d; want 42, 42", n1, n2)
	}
	clientConn.Close()
	wg.Wait()
}

// --- receive_response (channel.ReceiveResponse) ---

func TestReceiveResponse(t *testing.T) {
	serverConn, clientConn := connPair(t)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		serverConn.ReceiveHandshake() //nolint:errcheck
		ch := serverConn.NewChannel("RR")
		ch.HandleRequests(func(_ []byte) (any, error) {
			return int64(42), nil
		}, nil)
	}()

	clientConn.SendHandshake() //nolint:errcheck
	ch, err := clientConn.ConnectChannel(2, "RR")
	if err != nil {
		t.Fatalf("ConnectChannel: %v", err)
	}

	msgID, err := ch.SendRequestRaw(mustEncode(t, map[string]any{"test": true}))
	if err != nil {
		t.Fatalf("SendRequestRaw: %v", err)
	}
	result, err := ch.ReceiveResponse(msgID, 2*time.Second)
	if err != nil {
		t.Fatalf("ReceiveResponse: %v", err)
	}
	n, _ := extractCBORInt(result)
	if n != 42 {
		t.Errorf("result = %d, want 42", n)
	}
	clientConn.Close()
	wg.Wait()
}

// --- handle_requests sends error on exception ---

func TestHandleRequestsSendsErrorOnException(t *testing.T) {
	serverConn, clientConn := connPair(t)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		serverConn.ReceiveHandshake() //nolint:errcheck
		ch := serverConn.NewChannel("ErrTest")
		ch.HandleRequests(func(_ []byte) (any, error) {
			return nil, fmt.Errorf("test error")
		}, nil)
	}()

	clientConn.SendHandshake() //nolint:errcheck
	ch, err := clientConn.ConnectChannel(2, "ErrTest")
	if err != nil {
		t.Fatalf("ConnectChannel: %v", err)
	}

	pending, err := ch.Request(mustEncode(t, map[string]any{"x": true}))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	_, err = pending.Get()
	if err == nil {
		t.Fatal("expected requestError from server")
	}
	mustContain(t, err.Error(), "test error")
	clientConn.Close()
	wg.Wait()
}

// --- SendReplyWithError (explicit error kwargs) ---

func TestSendReplyErrorWithKwargs(t *testing.T) {
	serverConn, clientConn := connPair(t)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		serverConn.ReceiveHandshake() //nolint:errcheck
		ch := serverConn.NewChannel("ErrKw")
		msgID, _, err := ch.RecvRequest(2 * time.Second)
		if err != nil {
			t.Errorf("RecvRequest: %v", err)
			return
		}
		ch.SendReplyError(msgID, "custom error", "CustomType") //nolint:errcheck
	}()

	clientConn.SendHandshake() //nolint:errcheck
	ch, err := clientConn.ConnectChannel(2, "ErrKw")
	if err != nil {
		t.Fatalf("ConnectChannel: %v", err)
	}

	pending, err := ch.Request(mustEncode(t, map[string]any{"x": true}))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	_, err = pending.Get()
	if err == nil {
		t.Fatal("expected requestError")
	}
	re, ok := err.(*requestError)
	if !ok {
		t.Fatalf("expected *requestError, got %T", err)
	}
	mustContain(t, re.Error(), "custom error")
	if re.ErrorType != "CustomType" {
		t.Errorf("ErrorType = %q, want %q", re.ErrorType, "CustomType")
	}
	clientConn.Close()
	wg.Wait()
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
	s, _ := socketPair(t)
	cr := &closeReadConn{Conn: s}
	conn := newConnection(cr, "Test")
	conn.Close()
	if !cr.closed {
		t.Error("expected CloseRead to be called")
	}
}

// --- runReader: until() fires while waiting for reader lock ---

func TestRunReaderUntilFiresWhileWaitingForLock(t *testing.T) {
	s, c := socketPair(t)
	defer s.Close()
	defer c.Close()
	conn := newConnection(s, "Test")
	defer conn.Close()

	// Hold the reader lock so TryLock fails.
	conn.readerMu.Lock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// until() returns true after a brief pause, simulating data becoming available.
		i := 0
		conn.runReader(func() bool {
			i++
			return i > 2
		})
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("runReader did not exit when until() returned true while waiting for lock")
	}
	conn.readerMu.Unlock()
}

// --- dispatch: close-channel notification path ---

func TestDispatchCloseChannelNotification(t *testing.T) {
	serverConn, clientConn := connPair(t)
	handshakePair(t, serverConn, clientConn)

	// Client creates a channel and closes it — server receives the close notification.
	clientCh := clientConn.NewChannel("ClosedCh")

	// Server connects the channel so it exists in server's map.
	serverCh, err := serverConn.ConnectChannel(clientCh.ChannelID(), "ClosedCh")
	if err != nil {
		t.Fatalf("ConnectChannel: %v", err)
	}
	_ = serverCh

	// Close client channel — sends closeChannelPayload to server.
	clientCh.Close()

	// Give server time to process the close notification via runReader.
	time.Sleep(50 * time.Millisecond)

	// Force server to read the close packet by trying to recv (it'll get nothing
	// after processing close, so we just want it to dispatch the packet).
	done := make(chan struct{})
	go func() {
		defer close(done)
		serverConn.ControlChannel().RecvRequestRaw(200 * time.Millisecond) //nolint:errcheck
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
	}
}

// --- dispatch: message to unknown channel (request) → sends error reply ---
// --- dispatch: message to unknown channel (reply) → silently dropped ---

func TestDispatchUnknownChannelIsReply(t *testing.T) {
	serverConn, clientConn := connPair(t)
	handshakePair(t, serverConn, clientConn)

	// Send an IsReply=true packet to a nonexistent channel — should be silently dropped
	// (the !pkt.IsReply check prevents sending an error reply).
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Server reads: will dispatch the IsReply packet to unknown channel (drop), then ping.
		serverConn.ControlChannel().RecvRequestRaw(500 * time.Millisecond) //nolint:errcheck
	}()

	go func() {
		clientConn.SendPacket(packet{ //nolint:errcheck
			ChannelID: 9997,
			MessageID: 1,
			IsReply:   true,
			Payload:   mustEncode(t, map[string]any{"result": 1}),
		})
	}()
	go func() {
		time.Sleep(20 * time.Millisecond)
		clientConn.ControlChannel().SendRequestRaw([]byte("ping")) //nolint:errcheck
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("TestDispatchUnknownChannelIsReply timed out")
	}
}

func TestDispatchUnknownChannelRequest(t *testing.T) {
	serverConn, clientConn := connPair(t)
	handshakePair(t, serverConn, clientConn)

	// Send a request (IsReply=false) to a nonexistent server channel.
	// The server should send an error reply, which the client must drain.
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		// Server reads: dispatches bad-channel request → sends error reply, then reads ping.
		serverConn.ControlChannel().RecvRequestRaw(2 * time.Second) //nolint:errcheck
	}()

	// Client sends the bad-channel request.
	go func() {
		clientConn.SendPacket(packet{ //nolint:errcheck
			ChannelID: 9998,
			MessageID: 1,
			IsReply:   false,
			Payload:   mustEncode(t, map[string]any{"command": "test"}),
		})
	}()

	// Client drains the error reply that the server sends back.
	clientDrained := make(chan struct{})
	go func() {
		defer close(clientDrained)
		clientConn.runReader(func() bool { return !clientConn.Live() })
	}()

	// Client sends control ping so server exits its read loop.
	go func() {
		time.Sleep(30 * time.Millisecond)
		clientConn.ControlChannel().SendRequestRaw([]byte("ping")) //nolint:errcheck
	}()

	select {
	case <-serverDone:
	case <-time.After(5 * time.Second):
		t.Error("TestDispatchUnknownChannelRequest timed out")
	}
}

// --- isTimeout: nil error and non-net.Error paths ---

func TestIsTimeoutNil(t *testing.T) {
	if isTimeout(nil) {
		t.Error("nil error should not be a timeout")
	}
}

func TestIsTimeoutNonNetError(t *testing.T) {
	// A plain error is not a timeout.
	if isTimeout(fmt.Errorf("plain error")) {
		t.Error("plain error should not be a timeout")
	}
}

// --- SendHandshakeVersion: error from SendRequestRaw (closed connection) ---

func TestSendHandshakeVersionSendError(t *testing.T) {
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

// --- ReceiveHandshake: error from RecvRequestRaw (connection closed) ---

func TestReceiveHandshakeRecvError(t *testing.T) {
	s, c := net.Pipe()
	conn := newConnection(s, "Test")
	// Close peer immediately so RecvRequestRaw returns an error.
	c.Close()
	err := conn.ReceiveHandshake()
	if err == nil {
		t.Fatal("expected error from ReceiveHandshake on closed conn")
	}
	s.Close()
}

// --- newRequestError: non-string key is skipped ---

func TestNewRequestErrorNonStringKey(t *testing.T) {
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

func TestPutInboxDropsWhenFull(t *testing.T) {
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	defer conn.Close()

	ch := conn.ControlChannel()
	// Fill the inbox (capacity 64).
	for i := 0; i < 64; i++ {
		ch.inbox <- packet{ChannelID: 0, MessageID: uint32(i)}
	}
	// This call should hit the default case and silently drop.
	ch.putInbox(packet{ChannelID: 0, MessageID: 99})
	if len(ch.inbox) != 64 {
		t.Errorf("inbox length = %d, want 64 (drop did not work)", len(ch.inbox))
	}
}

// --- SendReplyValue: CBOR encode error ---

func TestSendReplyValueEncodeError(t *testing.T) {
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
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	defer conn.Close()

	ch := conn.ControlChannel()
	// Inject shutdown sentinel to make processOneMessage return an error.
	ch.inbox <- shutdownSentinel

	_, err := ch.recvResponseRaw(1, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error from recvResponseRaw when connection closed")
	}
}

// --- ReceiveResponse: CBOR decode error ---

func TestReceiveResponseDecodeCBORError(t *testing.T) {
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
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	defer conn.Close()

	ch := conn.ControlChannel()
	ch.inbox <- shutdownSentinel

	_, err := ch.ReceiveResponse(1, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error from ReceiveResponse when connection closed")
	}
}

// --- processOneMessage: reply packet routed to responses map (ch.responses nil) ---

func TestProcessOneMessageRouteReplyNilResponses(t *testing.T) {
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
	s, _ := socketPair(t)
	conn := newConnection(s, "Test")
	defer conn.Close()

	// Control channel has no name set.
	ch := conn.ControlChannel()
	name := ch.channelName()
	mustContain(t, name, "channel 0")
}

// --- Request: SendRequestRaw error path ---

func TestRequestSendError(t *testing.T) {
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

// --- HandleRequests: stopFn returns true immediately ---

func TestHandleRequestsStopFnImmediate(t *testing.T) {
	serverConn, clientConn := connPair(t)
	handshakePair(t, serverConn, clientConn)

	ch := serverConn.NewChannel("StopTest")
	// stopFn returns true on first call — HandleRequests should return immediately.
	ch.HandleRequests(func(_ []byte) (any, error) {
		return nil, nil
	}, func() bool { return true })
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
