package proto

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

// CContentServerDirectory_GetCDNAuthToken_Request is the modern service-method
// request for a CDN auth token.  Used with EMsg ServiceMethodCallFromClient and
// target_job_name "ContentServerDirectory.GetCDNAuthToken#1".
// Field numbers from SteamTracking/Protobufs steam/steammessages_contentsystem.steamclient.proto.
type CContentServerDirectory_GetCDNAuthToken_Request struct {
	DepotID  uint32 // field 1
	HostName string // field 2
	AppID    uint32 // field 3
}

func (m *CContentServerDirectory_GetCDNAuthToken_Request) Marshal() []byte {
	var b []byte
	if m.DepotID != 0 {
		b = appendTag(b, 1, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(m.DepotID))
	}
	if m.HostName != "" {
		b = appendTag(b, 2, protowire.BytesType)
		b = protowire.AppendString(b, m.HostName)
	}
	if m.AppID != 0 {
		b = appendTag(b, 3, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(m.AppID))
	}
	return b
}

// CContentServerDirectory_GetCDNAuthToken_Response carries the CDN token.
// Field numbers from SteamTracking/Protobufs steam/steammessages_contentsystem.steamclient.proto.
type CContentServerDirectory_GetCDNAuthToken_Response struct {
	Token          string // field 1
	ExpirationTime uint32 // field 2 — Unix timestamp
}

func (m *CContentServerDirectory_GetCDNAuthToken_Response) Unmarshal(data []byte) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return fmt.Errorf("proto: bad tag in CContentServerDirectory_GetCDNAuthToken_Response")
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.BytesType:
			v, n := protowire.ConsumeString(data)
			if n < 0 {
				return fmt.Errorf("proto: bad string")
			}
			m.Token = v
			data = data[n:]
		case num == 2 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint")
			}
			m.ExpirationTime = uint32(v)
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

// CContentServerDirectory_GetManifestRequestCode_Request requests a manifest
// download code from the CM.  Used with service method
// "ContentServerDirectory.GetManifestRequestCode#1".
// Field numbers from SteamTracking/Protobufs steam/steammessages_contentsystem.steamclient.proto.
type CContentServerDirectory_GetManifestRequestCode_Request struct {
	AppID    uint32 // field 1
	DepotID  uint32 // field 2
	ManifestID uint64 // field 3
	AppBranch string // field 4 (empty for "public")
}

func (m *CContentServerDirectory_GetManifestRequestCode_Request) Marshal() []byte {
	var b []byte
	if m.AppID != 0 {
		b = appendTag(b, 1, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(m.AppID))
	}
	if m.DepotID != 0 {
		b = appendTag(b, 2, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(m.DepotID))
	}
	if m.ManifestID != 0 {
		b = appendTag(b, 3, protowire.VarintType)
		b = protowire.AppendVarint(b, m.ManifestID)
	}
	if m.AppBranch != "" {
		b = appendTag(b, 4, protowire.BytesType)
		b = protowire.AppendString(b, m.AppBranch)
	}
	return b
}

// CContentServerDirectory_GetManifestRequestCode_Response carries the manifest request code.
type CContentServerDirectory_GetManifestRequestCode_Response struct {
	ManifestRequestCode uint64 // field 1
}

func (m *CContentServerDirectory_GetManifestRequestCode_Response) Unmarshal(data []byte) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return fmt.Errorf("proto: bad tag in CContentServerDirectory_GetManifestRequestCode_Response")
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint")
			}
			m.ManifestRequestCode = v
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

// CMsgClientGetDepotDecryptionKey requests the AES key for a depot.
type CMsgClientGetDepotDecryptionKey struct {
	DepotID uint32 // field 1
	AppID   uint32 // field 2
}

func (m *CMsgClientGetDepotDecryptionKey) Marshal() []byte {
	var b []byte
	if m.DepotID != 0 {
		b = appendTag(b, 1, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(m.DepotID))
	}
	if m.AppID != 0 {
		b = appendTag(b, 2, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(m.AppID))
	}
	return b
}

// CMsgClientGetDepotDecryptionKeyResponse carries the depot AES key.
type CMsgClientGetDepotDecryptionKeyResponse struct {
	Eresult       int32  // field 1
	DepotID       uint32 // field 2
	DepotEncryptionKey []byte // field 3
}

func (m *CMsgClientGetDepotDecryptionKeyResponse) Unmarshal(data []byte) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return fmt.Errorf("proto: bad tag in CMsgClientGetDepotDecryptionKeyResponse")
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
		case num == 2 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint")
			}
			m.DepotID = uint32(v)
			data = data[n:]
		case num == 3 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return fmt.Errorf("proto: bad bytes")
			}
			m.DepotEncryptionKey = append([]byte(nil), v...)
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
