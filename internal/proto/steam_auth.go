package proto

import (
	"encoding/binary"
	"fmt"
	"math"

	"google.golang.org/protobuf/encoding/protowire"
)

// ---- CAuthentication_DeviceDetails -----------------------------------------
// Nested in CAuthentication_BeginAuthSessionViaCredentials_Request at field 9.

type CAuthentication_DeviceDetails struct {
	DeviceFriendlyName string // field 1
	PlatformType       int32  // field 2 — EAuthTokenPlatformType; 1 = SteamClient
}

func (m *CAuthentication_DeviceDetails) marshal() []byte {
	var b []byte
	if m.DeviceFriendlyName != "" {
		b = appendTag(b, 1, protowire.BytesType)
		b = protowire.AppendString(b, m.DeviceFriendlyName)
	}
	if m.PlatformType != 0 {
		b = appendTag(b, 2, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(m.PlatformType))
	}
	return b
}

// ---- CAuthentication_BeginAuthSessionViaCredentials_Request ----------------
// Actual Steam proto (steammessages_auth.steamclient.proto):
//   field 1  device_friendly_name  string  (not used — set via device_details instead)
//   field 2  account_name          string
//   field 3  encrypted_password    string  (base64 of PKCS1v15-encrypted password)
//   field 4  encryption_timestamp  uint64
//   field 5  remember_login        bool
//   field 6  platform_type         EAuthTokenPlatformType  (not set at top level)
//   field 7  persistence           ESessionPersistence
//   field 8  website_id            string  (default "Unknown"; SteamClient uses "Client")
//   field 9  device_details        CAuthentication_DeviceDetails
//   field 10 guard_data            string

type CAuthentication_BeginAuthSessionViaCredentials_Request struct {
	AccountName         string                        // field 2
	EncryptedPassword   string                        // field 3 — base64(PKCS1v15(password))
	EncryptionTimestamp uint64                        // field 4 — from GetPasswordRSAPublicKey
	Persistence         int32                         // field 7 — ESessionPersistence; 1 = persistent
	WebsiteID           string                        // field 8 — "Client" for SteamClient
	DeviceDetails       CAuthentication_DeviceDetails // field 9
	GuardData           string                        // field 10
}

func (m *CAuthentication_BeginAuthSessionViaCredentials_Request) Marshal() []byte {
	var b []byte
	if m.AccountName != "" {
		b = appendTag(b, 2, protowire.BytesType)
		b = protowire.AppendString(b, m.AccountName)
	}
	if m.EncryptedPassword != "" {
		b = appendTag(b, 3, protowire.BytesType)
		b = protowire.AppendString(b, m.EncryptedPassword)
	}
	if m.EncryptionTimestamp != 0 {
		b = appendTag(b, 4, protowire.VarintType)
		b = protowire.AppendVarint(b, m.EncryptionTimestamp)
	}
	if m.Persistence != 0 {
		b = appendTag(b, 7, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(m.Persistence))
	}
	if m.WebsiteID != "" {
		b = appendTag(b, 8, protowire.BytesType)
		b = protowire.AppendString(b, m.WebsiteID)
	}
	if dd := m.DeviceDetails.marshal(); len(dd) > 0 {
		b = appendTag(b, 9, protowire.BytesType)
		b = protowire.AppendBytes(b, dd)
	}
	if m.GuardData != "" {
		b = appendTag(b, 10, protowire.BytesType)
		b = protowire.AppendString(b, m.GuardData)
	}
	return b
}

// ---- CAuthentication_BeginAuthSessionViaCredentials_Response ---------------

type CAuthentication_AllowedConfirmation struct {
	ConfirmationType int32 // field 1 — EAuthSessionGuardType
}

type CAuthentication_BeginAuthSessionViaCredentials_Response struct {
	ClientID              uint64                                // field 1
	RequestID             []byte                               // field 2
	Interval              float32                              // field 3
	AllowedConfirmations  []CAuthentication_AllowedConfirmation // field 4
	SteamID               uint64                               // field 5
}

func (m *CAuthentication_BeginAuthSessionViaCredentials_Response) Unmarshal(data []byte) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return fmt.Errorf("proto: bad tag")
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint")
			}
			m.ClientID = v
			data = data[n:]
		case num == 2 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return fmt.Errorf("proto: bad bytes")
			}
			m.RequestID = append([]byte(nil), v...)
			data = data[n:]
		case num == 3 && typ == protowire.Fixed32Type:
			if len(data) < 4 {
				return fmt.Errorf("proto: short float")
			}
			m.Interval = math.Float32frombits(binary.LittleEndian.Uint32(data[:4]))
			data = data[4:]
		case num == 4 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return fmt.Errorf("proto: bad bytes")
			}
			var ac CAuthentication_AllowedConfirmation
			if err := ac.unmarshal(v); err != nil {
				return err
			}
			m.AllowedConfirmations = append(m.AllowedConfirmations, ac)
			data = data[n:]
		case num == 5 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint")
			}
			m.SteamID = v
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

func (a *CAuthentication_AllowedConfirmation) unmarshal(data []byte) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return fmt.Errorf("proto: bad tag in AllowedConfirmation")
		}
		data = data[n:]
		if num == 1 && typ == protowire.VarintType {
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint")
			}
			a.ConfirmationType = int32(v)
			data = data[n:]
		} else {
			n := protowire.ConsumeFieldValue(num, typ, data)
			if n < 0 {
				return fmt.Errorf("proto: unknown field")
			}
			data = data[n:]
		}
	}
	return nil
}

// ---- CAuthentication_UpdateAuthSessionWithSteamGuardCode_Request -----------

type CAuthentication_UpdateAuthSessionWithSteamGuardCode_Request struct {
	ClientID uint64 // field 1
	SteamID  uint64 // field 2
	Code     string // field 3
	CodeType int32  // field 4 — EAuthSessionGuardType: 2=email, 3=device(TOTP)
}

func (m *CAuthentication_UpdateAuthSessionWithSteamGuardCode_Request) Marshal() []byte {
	var b []byte
	if m.ClientID != 0 {
		b = appendTag(b, 1, protowire.VarintType)
		b = protowire.AppendVarint(b, m.ClientID)
	}
	if m.SteamID != 0 {
		// steamid is fixed64 in the proto (wire type 1), not uint64/varint.
		b = appendTag(b, 2, protowire.Fixed64Type)
		b = protowire.AppendFixed64(b, m.SteamID)
	}
	if m.Code != "" {
		b = appendTag(b, 3, protowire.BytesType)
		b = protowire.AppendString(b, m.Code)
	}
	if m.CodeType != 0 {
		b = appendTag(b, 4, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(m.CodeType))
	}
	return b
}

// ---- CAuthentication_PollAuthSessionStatus_Request -------------------------

type CAuthentication_PollAuthSessionStatus_Request struct {
	ClientID  uint64 // field 1
	RequestID []byte // field 2
}

func (m *CAuthentication_PollAuthSessionStatus_Request) Marshal() []byte {
	var b []byte
	if m.ClientID != 0 {
		b = appendTag(b, 1, protowire.VarintType)
		b = protowire.AppendVarint(b, m.ClientID)
	}
	if len(m.RequestID) > 0 {
		b = appendTag(b, 2, protowire.BytesType)
		b = protowire.AppendBytes(b, m.RequestID)
	}
	return b
}

// ---- CAuthentication_PollAuthSessionStatus_Response -----------------------

type CAuthentication_PollAuthSessionStatus_Response struct {
	RefreshToken string // field 3 — long-lived token (aud: ["client","renew","derive"])
	AccessToken  string // field 4 — short-lived token for immediate use
	AccountName  string // field 6
}

func (m *CAuthentication_PollAuthSessionStatus_Response) Unmarshal(data []byte) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return fmt.Errorf("proto: bad tag")
		}
		data = data[n:]
		switch {
		case num == 3 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return fmt.Errorf("proto: bad bytes")
			}
			m.RefreshToken = string(v)
			data = data[n:]
		case num == 4 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return fmt.Errorf("proto: bad bytes")
			}
			m.AccessToken = string(v)
			data = data[n:]
		case num == 6 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return fmt.Errorf("proto: bad bytes")
			}
			m.AccountName = string(v)
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
