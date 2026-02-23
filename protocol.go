package hegel

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"net"
)

// Wire protocol constants.
const (
	// Magic is the 4-byte magic cookie at the start of every packet header ("HEGL").
	Magic uint32 = 0x4845474C

	// ReplyBit is the high bit of the message ID field; when set the packet is a reply.
	ReplyBit uint32 = 1 << 31

	// Terminator is the single byte appended after every packet payload.
	Terminator byte = 0x0A

	// CloseChannelMessageID is the special message ID used to close a channel.
	CloseChannelMessageID uint32 = (1 << 31) - 1
)

// CloseChannelPayload is the special payload sent when closing a channel.
// It is chosen to be invalid CBOR (reserved tag byte 0xFE per RFC 8949).
var CloseChannelPayload = []byte{0xFE}

// headerSize is the size of the fixed packet header in bytes (5 × uint32).
const headerSize = 20

// Packet represents a single message in the Hegel wire protocol.
type Packet struct {
	// ChannelID identifies the logical channel this packet belongs to.
	ChannelID uint32
	// MessageID is the per-channel message sequence number.
	MessageID uint32
	// IsReply indicates that this packet is a reply to a previous message.
	IsReply bool
	// Payload is the CBOR-encoded message body.
	Payload []byte
}

// PartialPacketError is returned when the connection closes mid-packet.
type PartialPacketError struct {
	msg string
}

// Error implements the error interface.
func (e *PartialPacketError) Error() string { return e.msg }

// isPartialPacketError reports whether err is a *PartialPacketError and, if so,
// stores it in *target.
func isPartialPacketError(err error, target **PartialPacketError) bool {
	if p, ok := err.(*PartialPacketError); ok {
		if target != nil {
			*target = p
		}
		return true
	}
	return false
}

// isEOFLike reports whether err indicates a connection close (EOF variant).
func isEOFLike(err error) bool {
	return err == io.EOF || err == io.ErrUnexpectedEOF || err == io.ErrClosedPipe
}

// recvExact reads exactly n bytes from conn.
// It returns a *PartialPacketError if the connection closes before the first byte,
// and a plain error if it closes partway through.
func recvExact(conn net.Conn, n int) ([]byte, error) {
	if n == 0 {
		return []byte{}, nil
	}
	buf := make([]byte, n)
	read := 0
	for read < n {
		nr, err := conn.Read(buf[read:])
		read += nr
		if err != nil {
			if isEOFLike(err) {
				if read == 0 {
					return nil, &PartialPacketError{"connection closed partway through reading packet"}
				}
				return nil, fmt.Errorf("connection closed while reading data after %d bytes", read)
			}
			return nil, err
		}
	}
	return buf, nil
}

// ReadPacket reads and deserializes a single packet from conn.
// It validates the magic number, checksum, and terminator byte.
func ReadPacket(conn net.Conn) (Packet, error) {
	// Read the fixed 20-byte header.
	header, err := recvExact(conn, headerSize)
	if err != nil {
		return Packet{}, err
	}

	magic := binary.BigEndian.Uint32(header[0:])
	checksum := binary.BigEndian.Uint32(header[4:])
	channelID := binary.BigEndian.Uint32(header[8:])
	messageID := binary.BigEndian.Uint32(header[12:])
	payloadLen := binary.BigEndian.Uint32(header[16:])

	if magic != Magic {
		return Packet{}, fmt.Errorf("invalid magic number: expected 0x%08X, got 0x%08X", Magic, magic)
	}

	isReply := messageID&ReplyBit != 0
	if isReply {
		messageID ^= ReplyBit
	}

	// Read payload.
	payload, err := recvExact(conn, int(payloadLen))
	if err != nil {
		return Packet{}, err
	}

	// Read terminator.
	term, err := recvExact(conn, 1)
	if err != nil {
		return Packet{}, err
	}
	if term[0] != Terminator {
		return Packet{}, fmt.Errorf("invalid terminator: expected 0x%02X, got 0x%02X", Terminator, term[0])
	}

	// Verify CRC32 over header-with-checksum-zeroed + payload.
	headerForCheck := make([]byte, headerSize)
	copy(headerForCheck, header)
	binary.BigEndian.PutUint32(headerForCheck[4:], 0) // zero the checksum field
	computed := crc32.ChecksumIEEE(append(headerForCheck, payload...))
	if computed != checksum {
		return Packet{}, fmt.Errorf("checksum mismatch: expected 0x%08X, got 0x%08X", checksum, computed)
	}

	return Packet{
		ChannelID: channelID,
		MessageID: messageID,
		IsReply:   isReply,
		Payload:   payload,
	}, nil
}

// WritePacket serializes and writes a packet to conn.
// It computes the CRC32 checksum and appends the terminator byte.
func WritePacket(conn net.Conn, pkt Packet) error {
	messageID := pkt.MessageID
	if pkt.IsReply {
		messageID |= ReplyBit
	}

	// Build header with zeroed checksum to compute CRC.
	header := make([]byte, headerSize)
	binary.BigEndian.PutUint32(header[0:], Magic)
	binary.BigEndian.PutUint32(header[4:], 0) // zeroed for CRC computation
	binary.BigEndian.PutUint32(header[8:], pkt.ChannelID)
	binary.BigEndian.PutUint32(header[12:], messageID)
	binary.BigEndian.PutUint32(header[16:], uint32(len(pkt.Payload)))

	checksum := crc32.ChecksumIEEE(append(header, pkt.Payload...))
	binary.BigEndian.PutUint32(header[4:], checksum)

	// Write header + payload + terminator as a single call.
	frame := make([]byte, 0, headerSize+len(pkt.Payload)+1)
	frame = append(frame, header...)
	frame = append(frame, pkt.Payload...)
	frame = append(frame, Terminator)

	_, err := conn.Write(frame)
	return err
}
