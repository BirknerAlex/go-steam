// Package proto contains Steam network protocol message types and wire encoding.
// Rather than using protoc-generated code, messages are encoded/decoded via the
// protowire package directly, keeping this library self-contained.
package proto

import (
	"encoding/binary"
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

// Packet is a fully-decoded Steam network packet ready to dispatch.
type Packet struct {
	EMsg   EMsg
	Header CMsgProtoBufHeader
	Body   []byte // raw protobuf bytes of the message body
}

// MarshalPacket encodes an EMsg + CMsgProtoBufHeader + body into a flat byte
// slice suitable for writing to the wire (does NOT include the 4-byte length
// prefix or the 4-byte magic — those are added by the transport layer).
func MarshalPacket(msg EMsg, hdr CMsgProtoBufHeader, body []byte) []byte {
	hdrBytes := hdr.Marshal()
	out := make([]byte, 4+4+len(hdrBytes)+len(body))
	binary.LittleEndian.PutUint32(out[0:], uint32(msg|ProtoFlag))
	binary.LittleEndian.PutUint32(out[4:], uint32(len(hdrBytes)))
	copy(out[8:], hdrBytes)
	copy(out[8+len(hdrBytes):], body)
	return out
}

// UnmarshalPacket decodes a raw payload (starting with EMsg uint32) into a
// Packet.  Returns an error if the payload is malformed.
func UnmarshalPacket(payload []byte) (*Packet, error) {
	if len(payload) < 8 {
		return nil, fmt.Errorf("proto: packet too short (%d bytes)", len(payload))
	}
	rawMsg := EMsg(binary.LittleEndian.Uint32(payload[0:]))
	if !rawMsg.IsProto() {
		// Struct message — only used for encryption handshake.
		return &Packet{EMsg: rawMsg.Base(), Body: payload[4:]}, nil
	}
	hdrLen := binary.LittleEndian.Uint32(payload[4:])
	if len(payload) < int(8+hdrLen) {
		return nil, fmt.Errorf("proto: truncated header (need %d, have %d)", 8+hdrLen, len(payload))
	}
	var hdr CMsgProtoBufHeader
	if err := hdr.Unmarshal(payload[8 : 8+hdrLen]); err != nil {
		return nil, fmt.Errorf("proto: header unmarshal: %w", err)
	}
	return &Packet{
		EMsg:   rawMsg.Base(),
		Header: hdr,
		Body:   payload[8+hdrLen:],
	}, nil
}

// appendTag is a convenience alias used across the proto type files.
func appendTag(b []byte, num protowire.Number, typ protowire.Type) []byte {
	return protowire.AppendTag(b, num, typ)
}
