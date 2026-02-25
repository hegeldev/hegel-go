package hegel

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"net"
	"strings"
	"testing"
	"time"
)

// crc32ChecksumIEEE is a test alias for crc32.ChecksumIEEE.
func crc32ChecksumIEEE(data []byte) uint32 { return crc32.ChecksumIEEE(data) }

// --- Helpers for constructing raw packets ---

// makeRawPacket builds a raw wire-format packet from its components.
// If checksum is 0 the real CRC32 is computed; otherwise the given value is used as-is.
func makeRawPacket(magic, checksum, channelID, messageID uint32, payload []byte, terminator byte) []byte {
	var h [20]byte
	binary.BigEndian.PutUint32(h[0:], magic)
	binary.BigEndian.PutUint32(h[4:], 0) // zeroed for CRC
	binary.BigEndian.PutUint32(h[8:], channelID)
	binary.BigEndian.PutUint32(h[12:], messageID)
	binary.BigEndian.PutUint32(h[16:], uint32(len(payload)))
	if checksum == 0 {
		checksum = crc32.ChecksumIEEE(append(h[:], payload...))
	}
	binary.BigEndian.PutUint32(h[4:], checksum)
	var buf []byte
	buf = append(buf, h[:]...)
	buf = append(buf, payload...)
	buf = append(buf, terminator)
	return buf
}

// socketPair returns a connected pair of net.Conn (using net.Pipe) and registers
// cleanup to close them.  net.Pipe is synchronous, so callers that write and then
// read must do the write in a goroutine.
func socketPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	c1, c2 := net.Pipe()
	t.Cleanup(func() { c1.Close(); c2.Close() })
	return c1, c2
}

// packetsEqual reports whether two Packets are equal, including payload bytes.
func packetsEqual(a, b packet) bool {
	return a.ChannelID == b.ChannelID &&
		a.MessageID == b.MessageID &&
		a.IsReply == b.IsReply &&
		bytes.Equal(a.Payload, b.Payload)
}

// sendRaw writes raw bytes to conn in a goroutine (required because net.Pipe is
// synchronous) and returns a channel that will receive any write error.
func sendRaw(conn net.Conn, data []byte) <-chan error {
	ch := make(chan error, 1)
	go func() {
		_, err := conn.Write(data)
		ch <- err
	}()
	return ch
}

// roundtrip writes pkt to one end of a net.Pipe and reads it back from the other.
func roundtrip(t *testing.T, pkt packet) packet {
	t.Helper()
	reader, writer := socketPair(t)
	errCh := make(chan error, 1)
	go func() {
		errCh <- writePacket(writer, pkt)
	}()
	got, err := readPacket(reader)
	if err != nil {
		t.Fatalf("readPacket: %v", err)
	}
	if werr := <-errCh; werr != nil {
		t.Fatalf("writePacket: %v", werr)
	}
	return got
}

// --- Constants ---

func TestConstants(t *testing.T) {
	if Magic != 0x4845474C {
		t.Errorf("Magic = 0x%08X, want 0x4845474C", Magic)
	}
	if ReplyBit != 1<<31 {
		t.Errorf("ReplyBit = %d, want %d", ReplyBit, 1<<31)
	}
	if Terminator != 0x0A {
		t.Errorf("Terminator = 0x%02X, want 0x0A", Terminator)
	}
	if CloseChannelMessageID != (1<<31)-1 {
		t.Errorf("CloseChannelMessageID = %d, want %d", CloseChannelMessageID, (1<<31)-1)
	}
	if len(CloseChannelPayload) != 1 || CloseChannelPayload[0] != 0xFE {
		t.Errorf("CloseChannelPayload = %v, want [0xFE]", CloseChannelPayload)
	}
}

// --- packet round-trip tests ---

func TestPacketRoundtripBasic(t *testing.T) {
	pkt := packet{ChannelID: 0, MessageID: 1, IsReply: false, Payload: []byte("hello")}
	got := roundtrip(t, pkt)
	if !packetsEqual(got, pkt) {
		t.Errorf("got %+v, want %+v", got, pkt)
	}
}

func TestPacketRoundtripEmptyPayload(t *testing.T) {
	pkt := packet{ChannelID: 0, MessageID: 1, IsReply: false, Payload: []byte{}}
	got := roundtrip(t, pkt)
	if got.ChannelID != pkt.ChannelID || got.MessageID != pkt.MessageID || got.IsReply != pkt.IsReply || len(got.Payload) != 0 {
		t.Errorf("got %+v, want %+v", got, pkt)
	}
}

func TestPacketRoundtripReply(t *testing.T) {
	pkt := packet{ChannelID: 1, MessageID: 42, IsReply: true, Payload: []byte("response")}
	got := roundtrip(t, pkt)
	if !packetsEqual(got, pkt) {
		t.Errorf("got %+v, want %+v", got, pkt)
	}
}

func TestPacketRoundtripLargeChannelID(t *testing.T) {
	pkt := packet{ChannelID: 0xFFFFFFFF, MessageID: 1, IsReply: false, Payload: []byte("data")}
	got := roundtrip(t, pkt)
	if !packetsEqual(got, pkt) {
		t.Errorf("got %+v, want %+v", got, pkt)
	}
}

func TestPacketRoundtripLargeMessageID(t *testing.T) {
	pkt := packet{ChannelID: 0, MessageID: (1 << 31) - 1, IsReply: false, Payload: []byte("data")}
	got := roundtrip(t, pkt)
	if !packetsEqual(got, pkt) {
		t.Errorf("got %+v, want %+v", got, pkt)
	}
}

func TestPacketRoundtripBinaryPayload(t *testing.T) {
	payload := make([]byte, 256)
	for i := range payload {
		payload[i] = byte(i)
	}
	pkt := packet{ChannelID: 3, MessageID: 7, IsReply: false, Payload: payload}
	got := roundtrip(t, pkt)
	if !packetsEqual(got, pkt) {
		t.Errorf("got %+v, want %+v", got, pkt)
	}
}

// --- recvExact error tests ---

func TestRecvExactConnectionClosedWithPartialData(t *testing.T) {
	reader, writer := socketPair(t)
	// Send 3 bytes then close — reader will block asking for 10.
	go func() {
		writer.Write([]byte("abc")) //nolint:errcheck
		writer.Close()
	}()
	_, err := recvExact(reader, 10)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "connection closed while reading") {
		t.Errorf("error %q does not mention 'connection closed while reading'", err.Error())
	}
}

func TestRecvExactConnectionClosedNoData(t *testing.T) {
	reader, writer := socketPair(t)
	go writer.Close()
	_, err := recvExact(reader, 10)
	if err == nil {
		t.Fatal("expected partialPacketError, got nil")
	}
	var ppe *partialPacketError
	if !isPartialPacketError(err, &ppe) {
		t.Errorf("expected partialPacketError, got %T: %v", err, err)
	}
}

func TestRecvExactZeroBytes(t *testing.T) {
	reader, _ := socketPair(t)
	data, err := recvExact(reader, 0)
	if err != nil {
		t.Fatalf("recvExact(0): %v", err)
	}
	if len(data) != 0 {
		t.Errorf("recvExact(0) = %v, want []", data)
	}
}

func TestReadPacketInvalidMagic(t *testing.T) {
	reader, writer := socketPair(t)
	raw := makeRawPacket(0xDEADBEEF, 0, 0, 1, []byte("payload"), Terminator)
	sendRaw(writer, raw)
	_, err := readPacket(reader)
	if err == nil {
		t.Fatal("expected error for invalid magic")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "invalid magic") {
		t.Errorf("error %q does not mention 'invalid magic'", err.Error())
	}
}

func TestReadPacketInvalidTerminator(t *testing.T) {
	reader, writer := socketPair(t)
	raw := makeRawPacket(Magic, 0, 0, 1, []byte("payload"), 0xFF)
	sendRaw(writer, raw)
	_, err := readPacket(reader)
	if err == nil {
		t.Fatal("expected error for invalid terminator")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "invalid terminator") {
		t.Errorf("error %q does not mention 'invalid terminator'", err.Error())
	}
}

func TestReadPacketBadChecksum(t *testing.T) {
	reader, writer := socketPair(t)
	// Build a packet with a deliberately wrong checksum.
	var badChecksum uint32 = 0x12345678
	payload := []byte("payload")
	var h [20]byte
	binary.BigEndian.PutUint32(h[0:], Magic)
	binary.BigEndian.PutUint32(h[4:], badChecksum)
	binary.BigEndian.PutUint32(h[8:], 0)
	binary.BigEndian.PutUint32(h[12:], 1)
	binary.BigEndian.PutUint32(h[16:], uint32(len(payload)))
	raw := append(h[:], payload...)
	raw = append(raw, Terminator)
	sendRaw(writer, raw)
	_, err := readPacket(reader)
	if err == nil {
		t.Fatal("expected error for bad checksum")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "checksum mismatch") {
		t.Errorf("error %q does not mention 'checksum mismatch'", err.Error())
	}
}

// --- partialPacketError ---

func TestPartialPacketErrorMessage(t *testing.T) {
	err := &partialPacketError{"test message"}
	if err.Error() != "test message" {
		t.Errorf("Error() = %q, want %q", err.Error(), "test message")
	}
}

func TestIsPartialPacketErrorNilTarget(t *testing.T) {
	err := &partialPacketError{"msg"}
	if !isPartialPacketError(err, nil) {
		t.Error("isPartialPacketError with nil target should return true for partialPacketError")
	}
}

func TestIsPartialPacketErrorFalseForOther(t *testing.T) {
	err := fmt.Errorf("some other error")
	var ppe *partialPacketError
	if isPartialPacketError(err, &ppe) {
		t.Error("isPartialPacketError should return false for non-partialPacketError")
	}
}

// errConn is a minimal net.Conn that returns a specific error on every Read.
type errConn struct{ err error }

func (c *errConn) Read([]byte) (int, error)         { return 0, c.err }
func (c *errConn) Write(b []byte) (int, error)      { return len(b), nil }
func (c *errConn) Close() error                     { return nil }
func (c *errConn) LocalAddr() net.Addr              { return nil }
func (c *errConn) RemoteAddr() net.Addr             { return nil }
func (c *errConn) SetDeadline(time.Time) error      { return nil }
func (c *errConn) SetReadDeadline(time.Time) error  { return nil }
func (c *errConn) SetWriteDeadline(time.Time) error { return nil }

// --- recvExact non-EOF error ---

func TestRecvExactNonEOFError(t *testing.T) {
	// Use a fake conn that returns a non-EOF error to hit the unhandled error branch.
	customErr := fmt.Errorf("custom network error")
	conn := &errConn{err: customErr}
	_, err := recvExact(conn, 5)
	if err == nil {
		t.Fatal("recvExact with non-EOF error: expected error")
	}
	if err != customErr {
		t.Errorf("recvExact non-EOF: got %v, want %v", err, customErr)
	}
}

// --- readPacket error paths ---

func TestReadPacketConnectionClosedDuringHeader(t *testing.T) {
	reader, writer := socketPair(t)
	// Close writer immediately — no header bytes sent.
	go writer.Close()
	_, err := readPacket(reader)
	if err == nil {
		t.Fatal("expected error when connection closes before header")
	}
}

func TestReadPacketConnectionClosedDuringPayload(t *testing.T) {
	reader, writer := socketPair(t)
	// Send a valid header claiming 10 bytes of payload, then close.
	go func() {
		var h [20]byte
		binary.BigEndian.PutUint32(h[0:], Magic)
		binary.BigEndian.PutUint32(h[4:], 0) // bad CRC but we'll close before it's checked
		binary.BigEndian.PutUint32(h[8:], 0)
		binary.BigEndian.PutUint32(h[12:], 1)
		binary.BigEndian.PutUint32(h[16:], 10) // claim 10 bytes
		writer.Write(h[:])                     //nolint:errcheck
		writer.Close()                         // close without sending payload
	}()
	_, err := readPacket(reader)
	if err == nil {
		t.Fatal("expected error when connection closes during payload")
	}
}

func TestReadPacketConnectionClosedDuringTerminator(t *testing.T) {
	reader, writer := socketPair(t)
	// Send a valid header + payload (0 bytes), then close without terminator.
	go func() {
		var h [20]byte
		binary.BigEndian.PutUint32(h[0:], Magic)
		// Compute real CRC for 0-byte payload
		headerForCRC := make([]byte, 20)
		copy(headerForCRC, h[:])
		binary.BigEndian.PutUint32(headerForCRC[8:], 0)
		binary.BigEndian.PutUint32(headerForCRC[12:], 1)
		binary.BigEndian.PutUint32(headerForCRC[16:], 0)
		checksum := crc32ChecksumIEEE(headerForCRC)
		binary.BigEndian.PutUint32(h[0:], Magic)
		binary.BigEndian.PutUint32(h[4:], checksum)
		binary.BigEndian.PutUint32(h[8:], 0)
		binary.BigEndian.PutUint32(h[12:], 1)
		binary.BigEndian.PutUint32(h[16:], 0) // zero payload
		writer.Write(h[:])                    //nolint:errcheck
		writer.Close()                        // close without terminator
	}()
	_, err := readPacket(reader)
	if err == nil {
		t.Fatal("expected error when connection closes during terminator")
	}
}

// --- CRC32 verification ---

func TestCRC32KnownVector(t *testing.T) {
	// CRC32 IEEE of empty byte slice = 0
	if c := crc32.ChecksumIEEE([]byte{}); c != 0 {
		t.Errorf("CRC32('') = 0x%08X, want 0", c)
	}
	// CRC32 IEEE of "123456789" = 0xCBF43926 (standard test vector)
	if c := crc32.ChecksumIEEE([]byte("123456789")); c != 0xCBF43926 {
		t.Errorf("CRC32('123456789') = 0x%08X, want 0xCBF43926", c)
	}
}

func TestWritePacketProducesValidCRC(t *testing.T) {
	reader, writer := socketPair(t)
	pkt := packet{ChannelID: 5, MessageID: 10, IsReply: false, Payload: []byte("test")}
	go func() { writePacket(writer, pkt) }() //nolint:errcheck
	// readPacket validates CRC internally; success means CRC was correct.
	if _, err := readPacket(reader); err != nil {
		t.Errorf("readPacket after writePacket: %v", err)
	}
}
