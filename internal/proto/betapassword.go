package proto

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

// CMsgClientCheckAppBetaPassword submits a beta-branch password to Steam in
// exchange for the per-branch decryption keys.  Used with EMsg
// ClientCheckAppBetaPassword (5450).
//
// Field numbers from steammessages_clientserver_2.proto:
//
//	field 1 = app_id       (uint32)
//	field 2 = betapassword (string) — the user-supplied branch password
//	field 3 = language     (int32, optional)
type CMsgClientCheckAppBetaPassword struct {
	AppID        uint32 // field 1
	BetaPassword string // field 2
}

func (m *CMsgClientCheckAppBetaPassword) Marshal() []byte {
	var b []byte
	if m.AppID != 0 {
		b = appendTag(b, 1, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(m.AppID))
	}
	if m.BetaPassword != "" {
		b = appendTag(b, 2, protowire.BytesType)
		b = protowire.AppendString(b, m.BetaPassword)
	}
	return b
}

// CMsgClientCheckAppBetaPasswordResponse carries the per-branch decryption keys
// for every branch the supplied password unlocks.
//
// Field numbers from steammessages_clientserver_2.proto:
//
//	field 1 = eresult       (int32)
//	field 4 = betapasswords (repeated BetaPassword)
type CMsgClientCheckAppBetaPasswordResponse struct {
	Eresult       int32              // field 1
	BetaPasswords []BetaPasswordEntry // field 4
}

// BetaPasswordEntry is one branch → key pair in the beta password response.
//
//	field 1 = betaname     (string) — branch name
//	field 2 = betapassword (string) — hex-encoded AES-256 key for the branch
type BetaPasswordEntry struct {
	BetaName     string // field 1
	BetaPassword string // field 2 — hex-encoded key
}

func unmarshalBetaPasswordEntry(data []byte) (BetaPasswordEntry, error) {
	var e BetaPasswordEntry
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return e, fmt.Errorf("proto: bad tag in BetaPasswordEntry")
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.BytesType:
			v, n := protowire.ConsumeString(data)
			if n < 0 {
				return e, fmt.Errorf("proto: bad string")
			}
			e.BetaName = v
			data = data[n:]
		case num == 2 && typ == protowire.BytesType:
			v, n := protowire.ConsumeString(data)
			if n < 0 {
				return e, fmt.Errorf("proto: bad string")
			}
			e.BetaPassword = v
			data = data[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, data)
			if n < 0 {
				return e, fmt.Errorf("proto: unknown field")
			}
			data = data[n:]
		}
	}
	return e, nil
}

func (m *CMsgClientCheckAppBetaPasswordResponse) Unmarshal(data []byte) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return fmt.Errorf("proto: bad tag in CMsgClientCheckAppBetaPasswordResponse")
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
		case num == 4 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return fmt.Errorf("proto: bad bytes")
			}
			entry, err := unmarshalBetaPasswordEntry(v)
			if err != nil {
				return err
			}
			m.BetaPasswords = append(m.BetaPasswords, entry)
			data = data[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, data)
			if n < 0 {
				return fmt.Errorf("proto: unknown field")
			}
			data = data[n:]
		}
	}
	return nil
}
