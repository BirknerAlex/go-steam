package cm

import (
	"crypto/aes"
	"encoding/binary"
	"encoding/hex"
	"log/slog"
	"testing"

	"github.com/BirknerAlex/go-steam/internal/proto"
)

// encryptECB mirrors Steam's SymmetricEncryptECB (AES-256-ECB + PKCS7) so we can
// build encrypted GID test vectors the same way Steam produces them.
func encryptECB(t *testing.T, key, plaintext []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	out := make([]byte, len(padded))
	for off := 0; off < len(padded); off += aes.BlockSize {
		block.Encrypt(out[off:off+aes.BlockSize], padded[off:off+aes.BlockSize])
	}
	return out
}

func TestDecryptManifestGIDRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	const want uint64 = 7280450832745492388

	plain := make([]byte, 8)
	binary.LittleEndian.PutUint64(plain, want)
	blobHex := hex.EncodeToString(encryptECB(t, key, plain))

	got, err := DecryptManifestGID(blobHex, key)
	if err != nil {
		t.Fatalf("DecryptManifestGID: %v", err)
	}
	if got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

func TestDecryptManifestGIDErrors(t *testing.T) {
	key := make([]byte, 32)

	// Not valid hex.
	if _, err := DecryptManifestGID("zzzz", key); err == nil {
		t.Error("expected error for invalid hex")
	}
	// Hex that is not a multiple of the AES block size.
	if _, err := DecryptManifestGID("aabb", key); err == nil {
		t.Error("expected error for non-block-sized ciphertext")
	}
	// Wrong key size.
	plain := make([]byte, 8)
	blobHex := hex.EncodeToString(encryptECB(t, key, plain))
	if _, err := DecryptManifestGID(blobHex, make([]byte, 16)[:5]); err == nil {
		t.Error("expected error for bad key length")
	}
	// Correct length but wrong key → PKCS7 unpad almost always fails.
	wrong := make([]byte, 32)
	for i := range wrong {
		wrong[i] = 0xFF
	}
	if _, err := DecryptManifestGID(blobHex, wrong); err == nil {
		t.Error("expected error decrypting with wrong key")
	}
}

// TestParseEncryptedManifestSection verifies the PICS appinfo parser keeps the
// hex GID blob of a password-protected branch verbatim (rather than trying to
// interpret it as a number, which silently dropped it before).
func TestParseEncryptedManifestSection(t *testing.T) {
	const buf = `"appinfo"
{
	"appid" "440"
	"depots"
	{
		"441"
		{
			"manifests"
			{
				"public" { "gid" "123456789" }
			}
			"encryptedmanifests"
			{
				"betabranch" { "gid" "deadbeefcafe0011" "size" "42" }
			}
		}
	}
}`
	info, err := parseAppInfo(slog.Default(), proto.PICSAppResult{Appid: 440, Buffer: []byte(buf)})
	if err != nil {
		t.Fatalf("parseAppInfo: %v", err)
	}
	d := info.Depots[441]
	if d == nil {
		t.Fatal("depot 441 missing")
	}
	if d.ManifestGIDs["public"] != 123456789 {
		t.Errorf("public gid = %d, want 123456789", d.ManifestGIDs["public"])
	}
	if got := d.EncryptedManifestGIDs["betabranch"]; got != "deadbeefcafe0011" {
		t.Errorf("encrypted gid = %q, want deadbeefcafe0011", got)
	}
}
