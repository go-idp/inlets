package client

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
)

func joinHostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func hmacSHA512(message, secret string) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func hmacSHA256(message, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func compress(data string) (string, error) {
	// Currently no-op, matching TypeScript implementation
	return data, nil
}

func decompress(data string) (string, error) {
	// Currently no-op, matching TypeScript implementation
	return data, nil
}

// BinaryMessage represents a binary protocol message
type BinaryMessage struct {
	Type     uint8  // Message type
	StreamID string // Stream ID
	Sequence uint32 // Sequence number
	Flags    uint8  // Flags
	Data     []byte // Payload data
}

// buildBinaryMessage builds a binary message according to the protocol
// Format: [type(1)] [streamIdLen(1)] [streamId(variable)] [sequence(4)] [flags(1)] [dataLen(4)] [data(variable)]
func buildBinaryMessage(msg BinaryMessage) []byte {
	streamIDBytes := []byte(msg.StreamID)
	streamIDLen := len(streamIDBytes)
	if streamIDLen > 255 {
		panic("stream ID too long")
	}

	headerSize := 1 + 1 + streamIDLen + 4 + 1 + 4 // type + streamIdLen + streamId + sequence + flags + dataLen
	totalSize := headerSize + len(msg.Data)
	buffer := make([]byte, totalSize)
	offset := 0

	// Message type (1 byte)
	buffer[offset] = msg.Type
	offset++

	// Stream ID length (1 byte)
	buffer[offset] = uint8(streamIDLen)
	offset++

	// Stream ID (variable)
	copy(buffer[offset:], streamIDBytes)
	offset += streamIDLen

	// Sequence number (4 bytes, big-endian)
	binary.BigEndian.PutUint32(buffer[offset:], msg.Sequence)
	offset += 4

	// Flags (1 byte)
	buffer[offset] = msg.Flags
	offset++

	// Data length (4 bytes, big-endian)
	binary.BigEndian.PutUint32(buffer[offset:], uint32(len(msg.Data)))
	offset += 4

	// Data (variable)
	copy(buffer[offset:], msg.Data)

	return buffer
}

// parseBinaryMessage parses a binary message according to the protocol
func parseBinaryMessage(buffer []byte) (BinaryMessage, error) {
	if len(buffer) < 11 {
		return BinaryMessage{}, fmt.Errorf("message too short: %d bytes (min 11)", len(buffer))
	}

	offset := 0

	// Message type (1 byte)
	msgType := buffer[offset]
	offset++

	// Stream ID length (1 byte)
	streamIDLen := int(buffer[offset])
	offset++

	if len(buffer) < offset+streamIDLen+4+1+4 {
		return BinaryMessage{}, fmt.Errorf("message too short for stream ID")
	}

	// Stream ID (variable)
	streamID := string(buffer[offset : offset+streamIDLen])
	offset += streamIDLen

	// Sequence number (4 bytes, big-endian)
	sequence := binary.BigEndian.Uint32(buffer[offset:])
	offset += 4

	// Flags (1 byte)
	flags := buffer[offset]
	offset++

	// Data length (4 bytes, big-endian)
	dataLen := binary.BigEndian.Uint32(buffer[offset:])
	offset += 4

	if len(buffer) < offset+int(dataLen) {
		return BinaryMessage{}, fmt.Errorf("message too short for data: expected %d bytes, got %d", offset+int(dataLen), len(buffer))
	}

	// Data (variable)
	data := make([]byte, dataLen)
	copy(data, buffer[offset:offset+int(dataLen)])

	return BinaryMessage{
		Type:     msgType,
		StreamID: streamID,
		Sequence: sequence,
		Flags:    flags,
		Data:     data,
	}, nil
}
