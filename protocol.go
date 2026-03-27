package hegel

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

// Wire protocol constants.
const (
	magic uint32 = 0x4845474C // 4-byte magic cookie ("HEGL")

	replyBit uint32 = 1 << 31 // high bit of message ID marks a reply

	terminator byte = 0x0A // appended after every packet payload

	closeChannelMessageID uint32 = (1 << 31) - 1 // special message ID for channel close
)

// closeChannelPayload is the special payload sent when closing a channel.
// It is chosen to be invalid CBOR (reserved tag byte 0xFE per RFC 8949).
var closeChannelPayload = []byte{0xFE}

// headerSize is the size of the fixed packet header in bytes (5 × uint32).
const headerSize = 20

// packet represents a single message in the Hegel wire protocol.
type packet struct {
	// ChannelID identifies the logical channel this packet belongs to.
	ChannelID uint32
	// MessageID is the per-channel message sequence number.
	MessageID uint32
	// IsReply indicates that this packet is a reply to a previous message.
	IsReply bool
	// Payload is the CBOR-encoded message body.
	Payload []byte
}

// partialPacketError is returned when the connection closes mid-packet.
type partialPacketError struct {
	msg string
}

// Error implements the error interface.
func (e *partialPacketError) Error() string { return e.msg }

// isPartialPacketError reports whether err is a *partialPacketError and, if so,
// stores it in *target.
func isPartialPacketError(err error, target **partialPacketError) bool {
	if p, ok := err.(*partialPacketError); ok {
		if target != nil {
			*target = p
		}
		return true
	}
	return false
}

// isEOFLike reports whether err indicates a connection close (EOF variant).
func isEOFLike(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.ErrClosedPipe)
}

// recvExact reads exactly n bytes from r.
// It returns a *partialPacketError if the connection closes before the first byte,
// and a plain error if it closes partway through.
func recvExact(r io.Reader, n int) ([]byte, error) {
	if n == 0 {
		return []byte{}, nil
	}
	buf := make([]byte, n)
	read := 0
	for read < n {
		nr, err := r.Read(buf[read:])
		read += nr
		if err != nil {
			if isEOFLike(err) {
				if read == 0 {
					return nil, &partialPacketError{"connection closed partway through reading packet"}
				}
				return nil, fmt.Errorf("connection closed while reading data after %d bytes", read)
			}
			return nil, err
		}
	}
	return buf, nil
}

// readPacket reads and deserializes a single packet from r.
// It validates the magic number, checksum, and terminator byte.
func readPacket(r io.Reader) (packet, error) {
	// Read the fixed 20-byte header.
	header, err := recvExact(r, headerSize)
	if err != nil {
		return packet{}, err
	}

	mgc := binary.BigEndian.Uint32(header[0:])
	checksum := binary.BigEndian.Uint32(header[4:])
	channelID := binary.BigEndian.Uint32(header[8:])
	messageID := binary.BigEndian.Uint32(header[12:])
	payloadLen := binary.BigEndian.Uint32(header[16:])

	if mgc != magic {
		return packet{}, fmt.Errorf("invalid magic number: expected 0x%08X, got 0x%08X", magic, mgc)
	}

	isReply := messageID&replyBit != 0
	if isReply {
		messageID ^= replyBit
	}

	// Read payload.
	payload, err := recvExact(r, int(payloadLen))
	if err != nil {
		return packet{}, err
	}

	// Read terminator.
	term, err := recvExact(r, 1)
	if err != nil {
		return packet{}, err
	}
	if term[0] != terminator {
		return packet{}, fmt.Errorf("invalid terminator: expected 0x%02X, got 0x%02X", terminator, term[0])
	}

	// Verify CRC32 over header-with-checksum-zeroed + payload.
	headerForCheck := make([]byte, headerSize)
	copy(headerForCheck, header)
	binary.BigEndian.PutUint32(headerForCheck[4:], 0) // zero the checksum field
	computed := crc32.ChecksumIEEE(append(headerForCheck, payload...))
	if computed != checksum {
		return packet{}, fmt.Errorf("checksum mismatch: expected 0x%08X, got 0x%08X", checksum, computed)
	}

	return packet{
		ChannelID: channelID,
		MessageID: messageID,
		IsReply:   isReply,
		Payload:   payload,
	}, nil
}

// writePacket serializes and writes a packet to w.
// It computes the CRC32 checksum and appends the terminator byte.
func writePacket(w io.Writer, pkt packet) error {
	messageID := pkt.MessageID
	if pkt.IsReply {
		messageID |= replyBit
	}

	// Build header with zeroed checksum to compute CRC.
	header := make([]byte, headerSize)
	binary.BigEndian.PutUint32(header[0:], magic)
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
	frame = append(frame, terminator)

	_, err := w.Write(frame)
	return err
}
