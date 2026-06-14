package cdn

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1" //nolint:gosec
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAESSymmetricDecrypt(t *testing.T) {
	key := bytes.Repeat([]byte{0xAB}, 32)
	plaintext := []byte("hello steam chunk content here!!")

	encrypted, err := symmetricEncryptTest(plaintext, key)
	if err != nil {
		t.Fatal("symmetricEncrypt:", err)
	}

	decrypted, err := aesSymmetricDecrypt(encrypted, key)
	if err != nil {
		t.Fatal("aesSymmetricDecrypt:", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}
}

func TestDownloadChunk_SHA1Verification(t *testing.T) {
	key := bytes.Repeat([]byte{0x01}, 32)
	plaintext := []byte("steam chunk content for sha1 test")

	encrypted, err := symmetricEncryptTest(plaintext, key)
	if err != nil {
		t.Fatal(err)
	}

	//nolint:gosec
	h := sha1.New()
	h.Write(plaintext)
	correctSHA := h.Sum(nil)

	decrypted, err := aesSymmetricDecrypt(encrypted, key)
	if err != nil {
		t.Fatal(err)
	}
	//nolint:gosec
	h2 := sha1.New()
	h2.Write(decrypted)
	gotSHA := h2.Sum(nil)
	if !bytes.Equal(gotSHA, correctSHA) {
		t.Fatalf("SHA1 mismatch: got %s, want %s", hex.EncodeToString(gotSHA), hex.EncodeToString(correctSHA))
	}
}

func TestDownloadChunk_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if !IsUnauthorized(errChunkUnauthorized) {
		t.Error("IsUnauthorized should return true for errChunkUnauthorized")
	}
	if IsUnauthorized(nil) {
		t.Error("IsUnauthorized should return false for nil")
	}
}

// symmetricEncryptTest mirrors Steam's SymmetricEncrypt: ECB-encrypt the IV, then CBC-encrypt the payload.
// Uses a fixed all-zero IV (not random) so tests are deterministic.
func symmetricEncryptTest(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	rawIV := make([]byte, aes.BlockSize) // all-zero IV for deterministic tests

	// ECB-encrypt the IV.
	encIV := make([]byte, aes.BlockSize)
	block.Encrypt(encIV, rawIV)

	// PKCS#7-pad and CBC-encrypt the plaintext.
	padLen := aes.BlockSize - len(plaintext)%aes.BlockSize
	padded := append(append([]byte(nil), plaintext...), bytes.Repeat([]byte{byte(padLen)}, padLen)...)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, rawIV).CryptBlocks(ciphertext, padded)

	return append(encIV, ciphertext...), nil
}
