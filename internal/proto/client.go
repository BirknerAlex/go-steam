package proto

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

// CMsgClientHeartBeat is sent on a timer to keep the CM connection alive.
// Field numbers from steammessages_clientserver.proto.
type CMsgClientHeartBeat struct {
	SendReply bool // field 1
}

func (m *CMsgClientHeartBeat) Marshal() []byte {
	var b []byte
	if m.SendReply {
		b = appendTag(b, 1, protowire.VarintType)
		b = protowire.AppendVarint(b, 1)
	}
	return b
}

// CMsgClientHello must be sent as the first encrypted message after the AES
// handshake, before CMsgClientLogon.  Required by Steam servers since ~2022.
type CMsgClientHello struct {
	ProtocolVersion uint32 // field 1
}

func (m *CMsgClientHello) Marshal() []byte {
	var b []byte
	if m.ProtocolVersion != 0 {
		b = appendTag(b, 1, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(m.ProtocolVersion))
	}
	return b
}

// CMsgClientLogon carries credentials for a CM logon.
// Field numbers from steammessages_clientserver_login.proto (SteamKit2 SteamMsgClientServerLogin.cs).
type CMsgClientLogon struct {
	ProtocolVersion               uint32 // field 1
	DeprecatedObfuscatedPrivateIP uint32 // field 2 — localIP ^ 0xBAADF00D
	CellID                        uint32 // field 3
	ClientLanguage                string // field 6
	ClientOsType                  int32  // field 7 — 16 = Linux
	MachineID                     []byte // field 30
	AccountName                   string // field 50
	Password                      string // field 51
	MachineName                   string // field 96
	AccessToken                   string // field 108 — JWT refresh token for re-logon
}

func (m *CMsgClientLogon) Marshal() []byte {
	var b []byte
	if m.ProtocolVersion != 0 {
		b = appendTag(b, 1, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(m.ProtocolVersion))
	}
	if m.DeprecatedObfuscatedPrivateIP != 0 {
		b = appendTag(b, 2, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(m.DeprecatedObfuscatedPrivateIP))
	}
	if m.CellID != 0 {
		b = appendTag(b, 3, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(m.CellID))
	}
	if m.ClientLanguage != "" {
		b = appendTag(b, 6, protowire.BytesType)
		b = protowire.AppendString(b, m.ClientLanguage)
	}
	if m.ClientOsType != 0 {
		b = appendTag(b, 7, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(m.ClientOsType))
	}
	if len(m.MachineID) > 0 {
		b = appendTag(b, 30, protowire.BytesType)
		b = protowire.AppendBytes(b, m.MachineID)
	}
	if m.AccountName != "" {
		b = appendTag(b, 50, protowire.BytesType)
		b = protowire.AppendString(b, m.AccountName)
	}
	if m.Password != "" {
		b = appendTag(b, 51, protowire.BytesType)
		b = protowire.AppendString(b, m.Password)
	}
	if m.MachineName != "" {
		b = appendTag(b, 96, protowire.BytesType)
		b = protowire.AppendString(b, m.MachineName)
	}
	if m.AccessToken != "" {
		b = appendTag(b, 108, protowire.BytesType)
		b = protowire.AppendString(b, m.AccessToken)
	}
	return b
}

// CMsgClientLogonResponse carries the result of a logon attempt.
// Field numbers from SteamMsgClientServerLogin.cs (SteamKit2).
// The assigned SteamID is in the proto HEADER (pkt.Header.Steamid), not the body.
type CMsgClientLogonResponse struct {
	Eresult          int32 // field 1
	HeartbeatSeconds int32 // field 3 — heartbeat_seconds
	CellID           uint32 // field 7 — cell_id
}

func (m *CMsgClientLogonResponse) Unmarshal(data []byte) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return fmt.Errorf("proto: bad tag in CMsgClientLogonResponse")
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint")
			}
			m.Eresult = int32(v)
			data = data[n:]
		case num == 3 && typ == protowire.VarintType: // heartbeat_seconds
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint")
			}
			m.HeartbeatSeconds = int32(v)
			data = data[n:]
		case num == 7 && typ == protowire.VarintType: // cell_id
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint")
			}
			m.CellID = uint32(v)
			data = data[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, data)
			if n < 0 {
				return fmt.Errorf("proto: unknown field %d type %d", num, typ)
			}
			data = data[n:]
		}
	}
	return nil
}

// CMsgMulti wraps multiple compressed/uncompressed inner messages.
type CMsgMulti struct {
	SizeUnzipped uint32 // field 1 — 0 means not compressed
	MessageBody  []byte // field 2 — raw (possibly gzip-compressed) payload
}

func (m *CMsgMulti) Unmarshal(data []byte) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return fmt.Errorf("proto: bad tag in CMsgMulti")
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint")
			}
			m.SizeUnzipped = uint32(v)
			data = data[n:]
		case num == 2 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return fmt.Errorf("proto: bad bytes")
			}
			m.MessageBody = append([]byte(nil), v...)
			data = data[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, data)
			if n < 0 {
				return fmt.Errorf("proto: unknown field %d type %d", num, typ)
			}
			data = data[n:]
		}
	}
	return nil
}
