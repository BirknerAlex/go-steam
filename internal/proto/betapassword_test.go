package proto

import (
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestCheckAppBetaPasswordMarshal(t *testing.T) {
	m := CMsgClientCheckAppBetaPassword{AppID: 730, BetaPassword: "s3cret"}
	b := m.Marshal()

	// Decode the wire bytes back and check the two fields are present.
	var gotApp uint32
	var gotPass string
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			t.Fatal("bad tag")
		}
		b = b[n:]
		switch num {
		case 1:
			v, n := protowire.ConsumeVarint(b)
			gotApp = uint32(v)
			b = b[n:]
		case 2:
			v, n := protowire.ConsumeString(b)
			gotPass = v
			b = b[n:]
		default:
			_ = typ
			b = b[protowire.ConsumeFieldValue(num, typ, b):]
		}
	}
	if gotApp != 730 || gotPass != "s3cret" {
		t.Errorf("got app=%d pass=%q; want 730, s3cret", gotApp, gotPass)
	}
}

func TestCheckAppBetaPasswordMarshalOmitsEmpty(t *testing.T) {
	// AppID 0 and empty password must not be emitted (proto2 optional semantics).
	if b := (&CMsgClientCheckAppBetaPassword{}).Marshal(); len(b) != 0 {
		t.Errorf("empty message marshalled to %d bytes, want 0", len(b))
	}
}

func TestCheckAppBetaPasswordResponseUnmarshal(t *testing.T) {
	// Build a response: eresult=1, two betapasswords entries on field 4.
	entry := func(name, pass string) []byte {
		var e []byte
		e = appendTag(e, 1, protowire.BytesType)
		e = protowire.AppendString(e, name)
		e = appendTag(e, 2, protowire.BytesType)
		e = protowire.AppendString(e, pass)
		return e
	}
	var b []byte
	b = appendTag(b, 1, protowire.VarintType)
	b = protowire.AppendVarint(b, 1) // EResult.OK
	b = appendTag(b, 4, protowire.BytesType)
	b = protowire.AppendBytes(b, entry("beta", "aabbccdd"))
	b = appendTag(b, 4, protowire.BytesType)
	b = protowire.AppendBytes(b, entry("staging", "11223344"))

	var resp CMsgClientCheckAppBetaPasswordResponse
	if err := resp.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	if resp.Eresult != 1 {
		t.Errorf("eresult = %d, want 1", resp.Eresult)
	}
	if len(resp.BetaPasswords) != 2 {
		t.Fatalf("got %d entries, want 2", len(resp.BetaPasswords))
	}
	if resp.BetaPasswords[0].BetaName != "beta" || resp.BetaPasswords[0].BetaPassword != "aabbccdd" {
		t.Errorf("entry 0 = %+v", resp.BetaPasswords[0])
	}
	if resp.BetaPasswords[1].BetaName != "staging" || resp.BetaPasswords[1].BetaPassword != "11223344" {
		t.Errorf("entry 1 = %+v", resp.BetaPasswords[1])
	}
}
