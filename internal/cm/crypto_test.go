package cm

import (
	"bytes"
	"crypto/aes"
	"hash/crc32"
	"testing"

	"github.com/BirknerAlex/go-steam/internal/proto"
)

func TestEncryptDecryptPacketRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	for _, plain := range [][]byte{
		[]byte("hello"),
		[]byte(""),
		bytes.Repeat([]byte{0xAA}, 16), // exactly one block
		bytes.Repeat([]byte{0xBB}, 33), // spans blocks
	} {
		enc, err := encryptPacket(plain, key)
		if err != nil {
			t.Fatalf("encryptPacket(%d bytes): %v", len(plain), err)
		}
		// Wire is encryptedIV(16) + ciphertext, ciphertext block-aligned.
		if len(enc) < aes.BlockSize || (len(enc)-aes.BlockSize)%aes.BlockSize != 0 {
			t.Fatalf("ciphertext not block-aligned: total %d", len(enc))
		}
		dec, err := decryptPacket(enc, key)
		if err != nil {
			t.Fatalf("decryptPacket: %v", err)
		}
		if !bytes.Equal(dec, plain) {
			t.Errorf("round trip mismatch: got %q, want %q", dec, plain)
		}
	}
}

func TestDecryptPacketHMACVariant(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, 32)
	plain := []byte("hmac-protected payload")

	enc, err := encryptPacket(plain, key)
	if err != nil {
		t.Fatal(err)
	}
	// Build the HMAC variant: mac[0:3] + encryptedIV + ciphertext.
	encIV := enc[:aes.BlockSize]
	ciphertext := enc[aes.BlockSize:]
	mac := computeHMAC(key[16:], encIV, ciphertext)
	hmacWire := append(append([]byte(nil), mac[:3]...), enc...)

	dec, err := decryptPacket(hmacWire, key)
	if err != nil {
		t.Fatalf("decrypt HMAC variant: %v", err)
	}
	if !bytes.Equal(dec, plain) {
		t.Errorf("HMAC round trip mismatch: got %q, want %q", dec, plain)
	}
}

func TestDecryptPacketTooShort(t *testing.T) {
	if _, err := decryptPacket([]byte{1, 2, 3}, bytes.Repeat([]byte{0}, 32)); err == nil {
		t.Error("expected error for too-short packet")
	}
}

func TestPKCS7PadUnpad(t *testing.T) {
	for _, in := range [][]byte{nil, []byte("a"), bytes.Repeat([]byte{1}, 16), bytes.Repeat([]byte{2}, 17)} {
		padded := pkcs7Pad(in, aes.BlockSize)
		if len(padded)%aes.BlockSize != 0 {
			t.Fatalf("padded length %d not a multiple of block size", len(padded))
		}
		out, err := pkcs7Unpad(padded)
		if err != nil {
			t.Fatalf("unpad: %v", err)
		}
		if !bytes.Equal(out, in) {
			t.Errorf("pad/unpad mismatch: got %v, want %v", out, in)
		}
	}
}

func TestPKCS7UnpadInvalid(t *testing.T) {
	if _, err := pkcs7Unpad(nil); err == nil {
		t.Error("empty input should error")
	}
	// Padding byte larger than block size.
	bad := bytes.Repeat([]byte{0xFF}, 16)
	if _, err := pkcs7Unpad(bad); err == nil {
		t.Error("padding byte 0xFF should be rejected")
	}
	// Padding byte 0 is invalid.
	zero := make([]byte, 16)
	if _, err := pkcs7Unpad(zero); err == nil {
		t.Error("padding byte 0 should be rejected")
	}
}

func TestCRC32IEEE(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("steam"), bytes.Repeat([]byte{0x5A}, 256)} {
		got := crc32IEEE(data)
		want := crc32.ChecksumIEEE(data)
		if got != want {
			t.Errorf("crc32IEEE(%q) = %08x, want %08x", data, got, want)
		}
	}
}

func TestBuildStructPayloadRoundTrip(t *testing.T) {
	body := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	payload := buildStructPayload(proto.EMsgChannelEncryptResponse, body)
	pkt, err := proto.UnmarshalPacket(payload)
	if err != nil {
		t.Fatalf("UnmarshalPacket: %v", err)
	}
	if pkt.EMsg != proto.EMsgChannelEncryptResponse {
		t.Errorf("EMsg = %d, want %d", pkt.EMsg, proto.EMsgChannelEncryptResponse)
	}
	// Body retains the two job IDs (8+8) followed by the payload.
	if len(pkt.Body) != 16+len(body) {
		t.Fatalf("body length = %d, want %d", len(pkt.Body), 16+len(body))
	}
	if !bytes.Equal(pkt.Body[16:], body) {
		t.Errorf("payload body = %v, want %v", pkt.Body[16:], body)
	}
}

func TestParseSteamPublicKey(t *testing.T) {
	pub, err := parseSteamPublicKey()
	if err != nil {
		t.Fatalf("parseSteamPublicKey: %v", err)
	}
	if pub.N == nil || pub.E == 0 {
		t.Error("parsed RSA key looks empty")
	}
}

func TestSessionStateString(t *testing.T) {
	cases := map[SessionState]string{
		StateDisconnected:   "disconnected",
		StateConnecting:     "connecting",
		StateEncrypting:     "encrypting",
		StateAuthenticating: "authenticating",
		StateReady:          "ready",
		SessionState(99):    "unknown",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("SessionState(%d).String() = %q, want %q", int(s), got, want)
		}
	}
}
