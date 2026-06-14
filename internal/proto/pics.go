package proto

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

// CMsgClientPICSProductInfoRequest requests app/package metadata from PICS.
// Field numbers from steammessages_clientserver_2.proto.
type CMsgClientPICSProductInfoRequest struct {
	Packages []PICSPackageInfo // field 1
	Apps     []PICSAppInfo     // field 2
	MetaDataOnly bool          // field 3
}

// PICSAppInfo is an embedded message in CMsgClientPICSProductInfoRequest.
type PICSAppInfo struct {
	Appid       uint32 // field 1
	AccessToken uint64 // field 2
	OnlyPublic  bool   // field 3
}

// PICSPackageInfo is an embedded message in CMsgClientPICSProductInfoRequest.
type PICSPackageInfo struct {
	Packageid   uint32 // field 1
	AccessToken uint64 // field 2
}

func marshalPICSApp(a PICSAppInfo) []byte {
	var b []byte
	if a.Appid != 0 {
		b = appendTag(b, 1, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(a.Appid))
	}
	if a.AccessToken != 0 {
		b = appendTag(b, 2, protowire.VarintType)
		b = protowire.AppendVarint(b, a.AccessToken)
	}
	if a.OnlyPublic {
		b = appendTag(b, 3, protowire.VarintType)
		b = protowire.AppendVarint(b, 1)
	}
	return b
}

func (m *CMsgClientPICSProductInfoRequest) Marshal() []byte {
	var b []byte
	for _, app := range m.Apps {
		inner := marshalPICSApp(app)
		b = appendTag(b, 2, protowire.BytesType)
		b = protowire.AppendBytes(b, inner)
	}
	if m.MetaDataOnly {
		b = appendTag(b, 3, protowire.VarintType)
		b = protowire.AppendVarint(b, 1)
	}
	return b
}

// PICSAppResult is one app entry in the PICS product info response.
type PICSAppResult struct {
	Appid        uint32 // field 1
	ChangeNumber uint32 // field 2
	MissingToken bool   // field 3 — true when no PICS access token was provided for a non-public app
	Sha          []byte // field 4
	Buffer       []byte // field 5 — VDF-encoded app info (empty when MissingToken=true)
	OnlyPublic   bool   // field 6
	Size         uint32 // field 7
}

// CMsgClientPICSProductInfoResponse carries app/package info back from PICS.
type CMsgClientPICSProductInfoResponse struct {
	Apps             []PICSAppResult // field 1
	Packages         []interface{}   // field 2 (unused in our flow)
	UnknownPackage   []uint32        // field 3
	MetaDataOnly     bool            // field 4
	ResponsePending  bool            // field 5
	HttpMinSize      uint32          // field 6
	HttpHost         string          // field 7
}

func unmarshalPICSApp(data []byte) (PICSAppResult, error) {
	var r PICSAppResult
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return r, fmt.Errorf("proto: bad tag")
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return r, fmt.Errorf("proto: bad varint")
			}
			r.Appid = uint32(v)
			data = data[n:]
		case num == 2 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return r, fmt.Errorf("proto: bad varint")
			}
			r.ChangeNumber = uint32(v)
			data = data[n:]
		case num == 3 && typ == protowire.VarintType: // missing_token
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return r, fmt.Errorf("proto: bad varint")
			}
			r.MissingToken = v != 0
			data = data[n:]
		case num == 4 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return r, fmt.Errorf("proto: bad bytes")
			}
			r.Sha = append([]byte(nil), v...)
			data = data[n:]
		case num == 5 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return r, fmt.Errorf("proto: bad bytes")
			}
			r.Buffer = append([]byte(nil), v...)
			data = data[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, data)
			if n < 0 {
				return r, fmt.Errorf("proto: unknown field")
			}
			data = data[n:]
		}
	}
	return r, nil
}

func (m *CMsgClientPICSProductInfoResponse) Unmarshal(data []byte) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return fmt.Errorf("proto: bad tag in CMsgClientPICSProductInfoResponse")
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return fmt.Errorf("proto: bad bytes")
			}
			app, err := unmarshalPICSApp(v)
			if err != nil {
				return err
			}
			m.Apps = append(m.Apps, app)
			data = data[n:]
		case num == 6 && typ == protowire.VarintType: // response_pending (field 6; field 5 = meta_data_only)
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint")
			}
			m.ResponsePending = v != 0
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

// CMsgClientPICSAccessTokenRequest asks for access tokens for apps.
// Field numbers from SteamMsgClientServerAppInfo.cs:
//
//	field 1 = packageids (repeated uint32)
//	field 2 = appids     (repeated uint32)
type CMsgClientPICSAccessTokenRequest struct {
	AppIDs []uint32 // field 2 (appids)
}

func (m *CMsgClientPICSAccessTokenRequest) Marshal() []byte {
	var b []byte
	for _, id := range m.AppIDs {
		b = appendTag(b, 2, protowire.VarintType) // field 2 = appids
		b = protowire.AppendVarint(b, uint64(id))
	}
	return b
}

// CMsgClientPICSAccessTokenResponse carries per-app access tokens.
// Field numbers from SteamMsgClientServerAppInfo.cs:
//
//	field 3 = app_access_tokens (repeated AppToken)
//	field 4 = app_denied_tokens (repeated uint32)
type CMsgClientPICSAccessTokenResponse struct {
	AppAccessTokens []AppAccessToken // field 3
	AppDeniedTokens []uint32         // field 4
}

// AppAccessToken is an app/token pair in the access token response.
type AppAccessToken struct {
	AppID       uint32 // field 1
	AccessToken uint64 // field 2
}

func unmarshalAppToken(data []byte) (AppAccessToken, error) {
	var t AppAccessToken
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return t, fmt.Errorf("proto: bad tag")
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return t, fmt.Errorf("proto: bad varint")
			}
			t.AppID = uint32(v)
			data = data[n:]
		case num == 2 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return t, fmt.Errorf("proto: bad varint")
			}
			t.AccessToken = v
			data = data[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, data)
			if n < 0 {
				return t, fmt.Errorf("proto: unknown field")
			}
			data = data[n:]
		}
	}
	return t, nil
}

func (m *CMsgClientPICSAccessTokenResponse) Unmarshal(data []byte) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return fmt.Errorf("proto: bad tag in CMsgClientPICSAccessTokenResponse")
		}
		data = data[n:]
		switch {
		case num == 3 && typ == protowire.BytesType: // app_access_tokens
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return fmt.Errorf("proto: bad bytes")
			}
			tok, err := unmarshalAppToken(v)
			if err != nil {
				return err
			}
			m.AppAccessTokens = append(m.AppAccessTokens, tok)
			data = data[n:]
		case num == 4 && typ == protowire.VarintType: // app_denied_tokens
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint")
			}
			m.AppDeniedTokens = append(m.AppDeniedTokens, uint32(v))
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
