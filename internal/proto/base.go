package proto

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

// CMsgProtoBufHeader is included at the front of every protobuf CM message.
// Field numbers are from steammessages_base.proto in SteamKit2.
type CMsgProtoBufHeader struct {
	Steamid         uint64 // field 1
	ClientSessionid int32  // field 2
	JobidSource     uint64 // field 10
	JobidTarget     uint64 // field 11
	TargetJobName   string // field 12 — service method name (e.g. "ContentServerDirectory.GetCDNAuthToken#1")
	Eresult         int32  // field 13
	ErrorMessage    string // field 14
}

func (h *CMsgProtoBufHeader) Marshal() []byte {
	var b []byte
	if h.Steamid != 0 {
		// fixed64 steamid = 1  →  wire type 1 (I64), 8-byte little-endian
		b = appendTag(b, 1, protowire.Fixed64Type)
		b = protowire.AppendFixed64(b, h.Steamid)
	}
	if h.ClientSessionid != 0 {
		b = appendTag(b, 2, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(h.ClientSessionid))
	}
	if h.JobidSource != 0 {
		// fixed64 jobid_source = 10  →  wire type 1 (I64)
		b = appendTag(b, 10, protowire.Fixed64Type)
		b = protowire.AppendFixed64(b, h.JobidSource)
	}
	if h.JobidTarget != 0 {
		// fixed64 jobid_target = 11  →  wire type 1 (I64)
		b = appendTag(b, 11, protowire.Fixed64Type)
		b = protowire.AppendFixed64(b, h.JobidTarget)
	}
	if h.TargetJobName != "" {
		b = appendTag(b, 12, protowire.BytesType)
		b = protowire.AppendString(b, h.TargetJobName)
	}
	if h.Eresult != 0 {
		b = appendTag(b, 13, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(h.Eresult))
	}
	if h.ErrorMessage != "" {
		b = appendTag(b, 14, protowire.BytesType)
		b = protowire.AppendString(b, h.ErrorMessage)
	}
	return b
}

func (h *CMsgProtoBufHeader) Unmarshal(data []byte) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return fmt.Errorf("proto: bad tag in CMsgProtoBufHeader")
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(data)
			if n < 0 {
				return fmt.Errorf("proto: bad fixed64 field 1")
			}
			h.Steamid = v
			data = data[n:]
		case num == 2 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint field 2")
			}
			h.ClientSessionid = int32(v)
			data = data[n:]
		case num == 10 && typ == protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(data)
			if n < 0 {
				return fmt.Errorf("proto: bad fixed64 field 10")
			}
			h.JobidSource = v
			data = data[n:]
		case num == 11 && typ == protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(data)
			if n < 0 {
				return fmt.Errorf("proto: bad fixed64 field 11")
			}
			h.JobidTarget = v
			data = data[n:]
		case num == 12 && typ == protowire.BytesType:
			v, n := protowire.ConsumeString(data)
			if n < 0 {
				return fmt.Errorf("proto: bad string field 12")
			}
			h.TargetJobName = v
			data = data[n:]
		case num == 13 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint field 13")
			}
			h.Eresult = int32(v)
			data = data[n:]
		case num == 14 && typ == protowire.BytesType:
			v, n := protowire.ConsumeString(data)
			if n < 0 {
				return fmt.Errorf("proto: bad string field 14")
			}
			h.ErrorMessage = v
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
