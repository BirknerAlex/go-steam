package proto

import (
	"bytes"
	"encoding/binary"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

// --- helpers ----------------------------------------------------------------

func appendVarintField(b []byte, num protowire.Number, v uint64) []byte {
	b = appendTag(b, num, protowire.VarintType)
	return protowire.AppendVarint(b, v)
}

func appendBytesField(b []byte, num protowire.Number, v []byte) []byte {
	b = appendTag(b, num, protowire.BytesType)
	return protowire.AppendBytes(b, v)
}

// --- response decoders ------------------------------------------------------

func TestGetCDNAuthTokenResponseUnmarshal(t *testing.T) {
	var b []byte
	b = appendBytesField(b, 1, []byte("the-token"))
	b = appendVarintField(b, 2, 1700000000)

	var resp CContentServerDirectory_GetCDNAuthToken_Response
	if err := resp.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	if resp.Token != "the-token" || resp.ExpirationTime != 1700000000 {
		t.Errorf("got %+v", resp)
	}
}

func TestGetManifestRequestCodeResponseUnmarshal(t *testing.T) {
	b := appendVarintField(nil, 1, 987654321)
	var resp CContentServerDirectory_GetManifestRequestCode_Response
	if err := resp.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	if resp.ManifestRequestCode != 987654321 {
		t.Errorf("code = %d", resp.ManifestRequestCode)
	}
}

func TestDepotDecryptionKeyResponseUnmarshal(t *testing.T) {
	key := bytes.Repeat([]byte{0xAB}, 32)
	var b []byte
	b = appendVarintField(b, 1, uint64(EResultOK))
	b = appendVarintField(b, 2, 1006)
	b = appendBytesField(b, 3, key)

	var resp CMsgClientGetDepotDecryptionKeyResponse
	if err := resp.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	if resp.Eresult != int32(EResultOK) || resp.DepotID != 1006 || !bytes.Equal(resp.DepotEncryptionKey, key) {
		t.Errorf("got %+v", resp)
	}
}

func TestPICSAccessTokenResponseUnmarshal(t *testing.T) {
	// app_access_tokens (field 3) is a nested message: appid(1), token(2).
	var inner []byte
	inner = appendVarintField(inner, 1, 740)
	inner = appendVarintField(inner, 2, 0xDEADBEEF)

	var b []byte
	b = appendBytesField(b, 3, inner)
	b = appendVarintField(b, 4, 999) // app_denied_tokens

	var resp CMsgClientPICSAccessTokenResponse
	if err := resp.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	if len(resp.AppAccessTokens) != 1 || resp.AppAccessTokens[0].AppID != 740 ||
		resp.AppAccessTokens[0].AccessToken != 0xDEADBEEF {
		t.Errorf("access tokens = %+v", resp.AppAccessTokens)
	}
	if len(resp.AppDeniedTokens) != 1 || resp.AppDeniedTokens[0] != 999 {
		t.Errorf("denied = %+v", resp.AppDeniedTokens)
	}
}

func TestPICSProductInfoResponseUnmarshal(t *testing.T) {
	// app (field 1): appid(1), missing_token(3), buffer(5).
	var app []byte
	app = appendVarintField(app, 1, 740)
	app = appendVarintField(app, 3, 1) // missing_token
	app = appendBytesField(app, 5, []byte("vdfbytes"))

	var b []byte
	b = appendBytesField(b, 1, app)
	b = appendVarintField(b, 6, 1) // response_pending

	var resp CMsgClientPICSProductInfoResponse
	if err := resp.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	if len(resp.Apps) != 1 {
		t.Fatalf("apps = %d", len(resp.Apps))
	}
	a := resp.Apps[0]
	if a.Appid != 740 || !a.MissingToken || string(a.Buffer) != "vdfbytes" {
		t.Errorf("app = %+v", a)
	}
	if !resp.ResponsePending {
		t.Error("response_pending should be true")
	}
}

func TestLogonResponseUnmarshal(t *testing.T) {
	var b []byte
	b = appendVarintField(b, 1, uint64(EResultOK))
	b = appendVarintField(b, 3, 270) // heartbeat_seconds
	b = appendVarintField(b, 7, 42)  // cell_id

	var resp CMsgClientLogonResponse
	if err := resp.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	if resp.Eresult != int32(EResultOK) || resp.HeartbeatSeconds != 270 || resp.CellID != 42 {
		t.Errorf("got %+v", resp)
	}
}

func TestPollAuthStatusResponseUnmarshal(t *testing.T) {
	var b []byte
	b = appendBytesField(b, 3, []byte("refresh-tok"))
	b = appendBytesField(b, 4, []byte("access-tok"))
	b = appendBytesField(b, 6, []byte("accountname"))

	var resp CAuthentication_PollAuthSessionStatus_Response
	if err := resp.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	if resp.RefreshToken != "refresh-tok" || resp.AccessToken != "access-tok" || resp.AccountName != "accountname" {
		t.Errorf("got %+v", resp)
	}
}

func TestBeginAuthSessionResponseUnmarshal(t *testing.T) {
	// allowed_confirmations (field 4) nested: confirmation_type(1).
	conf := appendVarintField(nil, 1, 3) // device code (TOTP)

	var b []byte
	b = appendVarintField(b, 1, 12345)             // client_id
	b = appendBytesField(b, 2, []byte{1, 2, 3})    // request_id
	b = appendTag(b, 3, protowire.Fixed32Type)     // interval (float32)
	b = protowire.AppendFixed32(b, 0x40A00000)     // 5.0f
	b = appendBytesField(b, 4, conf)               // allowed_confirmations
	b = appendVarintField(b, 5, 76561190000000000) // steamid

	var resp CAuthentication_BeginAuthSessionViaCredentials_Response
	if err := resp.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	if resp.ClientID != 12345 || !bytes.Equal(resp.RequestID, []byte{1, 2, 3}) {
		t.Errorf("client/request wrong: %+v", resp)
	}
	if resp.Interval != 5.0 {
		t.Errorf("interval = %v, want 5.0", resp.Interval)
	}
	if len(resp.AllowedConfirmations) != 1 || resp.AllowedConfirmations[0].ConfirmationType != 3 {
		t.Errorf("confirmations = %+v", resp.AllowedConfirmations)
	}
	if resp.SteamID != 76561190000000000 {
		t.Errorf("steamid = %d", resp.SteamID)
	}
}

func TestMultiUnmarshal(t *testing.T) {
	var b []byte
	b = appendVarintField(b, 1, 0) // size_unzipped = 0 (uncompressed)
	b = appendBytesField(b, 2, []byte("raw multi body"))

	var m CMsgMulti
	if err := m.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	if m.SizeUnzipped != 0 || string(m.MessageBody) != "raw multi body" {
		t.Errorf("got %+v", m)
	}
}

// --- request encoders (decode back with protowire) --------------------------

// fieldMap decodes a flat proto message into number → first value seen.
func decodeFields(t *testing.T, data []byte) map[protowire.Number]any {
	t.Helper()
	out := make(map[protowire.Number]any)
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			t.Fatalf("bad tag")
		}
		data = data[n:]
		switch typ {
		case protowire.VarintType:
			v, m := protowire.ConsumeVarint(data)
			out[num] = v
			data = data[m:]
		case protowire.BytesType:
			v, m := protowire.ConsumeBytes(data)
			if _, ok := out[num]; !ok {
				out[num] = append([]byte(nil), v...)
			}
			data = data[m:]
		case protowire.Fixed64Type:
			v, m := protowire.ConsumeFixed64(data)
			out[num] = v
			data = data[m:]
		case protowire.Fixed32Type:
			v, m := protowire.ConsumeFixed32(data)
			out[num] = v
			data = data[m:]
		default:
			t.Fatalf("unexpected wire type %d", typ)
		}
	}
	return out
}

func TestCDNAuthTokenRequestMarshal(t *testing.T) {
	req := CContentServerDirectory_GetCDNAuthToken_Request{DepotID: 1006, HostName: "cdn.h", AppID: 740}
	f := decodeFields(t, req.Marshal())
	if f[1] != uint64(1006) || string(f[2].([]byte)) != "cdn.h" || f[3] != uint64(740) {
		t.Errorf("fields = %v", f)
	}
}

func TestManifestRequestCodeRequestMarshal(t *testing.T) {
	req := CContentServerDirectory_GetManifestRequestCode_Request{AppID: 1, DepotID: 2, ManifestID: 3, AppBranch: "beta"}
	f := decodeFields(t, req.Marshal())
	if f[1] != uint64(1) || f[2] != uint64(2) || f[3] != uint64(3) || string(f[4].([]byte)) != "beta" {
		t.Errorf("fields = %v", f)
	}
}

func TestDepotKeyRequestMarshal(t *testing.T) {
	req := CMsgClientGetDepotDecryptionKey{DepotID: 1006, AppID: 740}
	f := decodeFields(t, req.Marshal())
	if f[1] != uint64(1006) || f[2] != uint64(740) {
		t.Errorf("fields = %v", f)
	}
}

func TestClientLogonMarshalFields(t *testing.T) {
	msg := CMsgClientLogon{
		ProtocolVersion: 65581,
		AccountName:     "user",
		AccessToken:     "jwt",
		ClientOsType:    16,
		ClientLanguage:  "english",
	}
	f := decodeFields(t, msg.Marshal())
	if f[1] != uint64(65581) {
		t.Errorf("protocol = %v", f[1])
	}
	if string(f[50].([]byte)) != "user" {
		t.Errorf("account = %v", f[50])
	}
	if string(f[108].([]byte)) != "jwt" {
		t.Errorf("access token (field 108) = %v", f[108])
	}
	if f[7] != uint64(16) {
		t.Errorf("os type = %v", f[7])
	}
}

func TestUpdateGuardCodeRequestSteamIDIsFixed64(t *testing.T) {
	req := CAuthentication_UpdateAuthSessionWithSteamGuardCode_Request{
		ClientID: 1, SteamID: 76561190000000000, Code: "ABCDE", CodeType: 3,
	}
	f := decodeFields(t, req.Marshal())
	// steamid (field 2) MUST be fixed64, not varint.
	if v, ok := f[2].(uint64); !ok || v != 76561190000000000 {
		t.Errorf("steamid field 2 = %v (expected fixed64 %d)", f[2], uint64(76561190000000000))
	}
	if string(f[3].([]byte)) != "ABCDE" || f[4] != uint64(3) {
		t.Errorf("code/codetype = %v %v", f[3], f[4])
	}
}

func TestBeginAuthRequestMarshalNestedDeviceDetails(t *testing.T) {
	req := CAuthentication_BeginAuthSessionViaCredentials_Request{
		AccountName:       "user",
		EncryptedPassword: "enc",
		Persistence:       1,
		WebsiteID:         "Client",
		DeviceDetails:     CAuthentication_DeviceDetails{DeviceFriendlyName: "steam-go", PlatformType: 1},
	}
	f := decodeFields(t, req.Marshal())
	if string(f[2].([]byte)) != "user" || string(f[3].([]byte)) != "enc" {
		t.Errorf("account/pw = %v %v", f[2], f[3])
	}
	if string(f[8].([]byte)) != "Client" {
		t.Errorf("website id = %v", f[8])
	}
	// device_details (field 9) is a nested message.
	dd, ok := f[9].([]byte)
	if !ok {
		t.Fatalf("device_details not present")
	}
	inner := decodeFields(t, dd)
	if string(inner[1].([]byte)) != "steam-go" || inner[2] != uint64(1) {
		t.Errorf("device details inner = %v", inner)
	}
}

func TestPICSAccessTokenRequestUsesField2(t *testing.T) {
	req := CMsgClientPICSAccessTokenRequest{AppIDs: []uint32{740, 741}}
	// appids are field 2 (repeated). Verify both encode under tag 2.
	data := req.Marshal()
	count := 0
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		data = data[n:]
		if num == 2 && typ == protowire.VarintType {
			count++
		}
		_, m := protowire.ConsumeVarint(data)
		data = data[m:]
	}
	if count != 2 {
		t.Errorf("expected 2 appids under field 2, got %d", count)
	}
}

func TestEResultOK(t *testing.T) {
	if !EResultOK.OK() {
		t.Error("EResultOK.OK() should be true")
	}
	if EResultFail.OK() {
		t.Error("EResultFail.OK() should be false")
	}
}

func TestUnmarshalPacketTooShort(t *testing.T) {
	if _, err := UnmarshalPacket([]byte{1, 2, 3}); err == nil {
		t.Error("expected error for short packet")
	}
}

func TestUnmarshalPacketStructMessage(t *testing.T) {
	// Non-proto (struct) message: EMsg without proto flag.
	payload := make([]byte, 8)
	binary.LittleEndian.PutUint32(payload[0:], uint32(EMsgChannelEncryptRequest))
	pkt, err := UnmarshalPacket(payload)
	if err != nil {
		t.Fatal(err)
	}
	if pkt.EMsg != EMsgChannelEncryptRequest {
		t.Errorf("EMsg = %d, want %d", pkt.EMsg, EMsgChannelEncryptRequest)
	}
}
