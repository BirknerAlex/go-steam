package cdn

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha1" //nolint:gosec // Steam mandates SHA1
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz/lzma"
)

// serveTLS starts a TLS httptest server with the given handler (DownloadChunk
// always uses https://) and returns a *Server plus the trusting http.Client.
func serveTLS(t *testing.T, h http.HandlerFunc) (*Server, *http.Client, func()) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	host := strings.TrimPrefix(srv.URL, "https://")
	return &Server{Host: host}, srv.Client(), srv.Close
}

// serveChunk serves encWire for any chunk request over TLS.
func serveChunk(t *testing.T, encWire []byte) (*Server, *http.Client, func()) {
	t.Helper()
	return serveTLS(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(encWire) //nolint:errcheck
	})
}

// roundTripChunk encrypts compressed with depotKey, serves it, and runs the full
// DownloadChunk decode pipeline, asserting the result equals wantPlain.
func roundTripChunk(t *testing.T, compressed, wantPlain, depotKey []byte, compressedSize uint32) {
	t.Helper()
	enc, err := symmetricEncryptTest(compressed, depotKey)
	if err != nil {
		t.Fatal(err)
	}
	server, client, closeFn := serveChunk(t, enc)
	defer closeFn()

	h := sha1.New() //nolint:gosec
	h.Write(wantPlain)
	chunk := ChunkInfo{
		SHA1:             h.Sum(nil),
		CompressedSize:   compressedSize,
		UncompressedSize: uint32(len(wantPlain)),
	}
	got, err := DownloadChunk(context.Background(), client, server, 100, chunk, depotKey, "")
	if err != nil {
		t.Fatalf("DownloadChunk: %v", err)
	}
	if !bytes.Equal(got, wantPlain) {
		t.Errorf("decoded chunk mismatch: got %d bytes, want %d", len(got), len(wantPlain))
	}
}

func TestDownloadChunk_ZLib(t *testing.T) {
	key := bytes.Repeat([]byte{0x08}, 32)
	plain := bytes.Repeat([]byte("zlib payload "), 50)

	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zw.Write(plain) //nolint:errcheck
	zw.Close()
	roundTripChunk(t, buf.Bytes(), plain, key, uint32(buf.Len()))
}

func TestDownloadChunk_Zip(t *testing.T) {
	key := bytes.Repeat([]byte{0x09}, 32)
	plain := bytes.Repeat([]byte("zip payload "), 40)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("chunk")
	if err != nil {
		t.Fatal(err)
	}
	f.Write(plain) //nolint:errcheck
	zw.Close()
	roundTripChunk(t, buf.Bytes(), plain, key, uint32(buf.Len()))
}

func TestDownloadChunk_VZipZSTD(t *testing.T) {
	key := bytes.Repeat([]byte{0x0A}, 32)
	plain := bytes.Repeat([]byte("zstd payload "), 60)

	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	frame := enc.EncodeAll(plain, nil)
	enc.Close()

	// Wire: header 'V','S','Z','a' + crc32(4) + ZSTD frame + footer.
	// Footer (15 bytes): CRC32(4) + uncompressed_size(uint64 LE, 8) + 'z','s','v'.
	var b bytes.Buffer
	b.Write([]byte{'V', 'S', 'Z', 'a', 0, 0, 0, 0})
	b.Write(frame)
	foot := make([]byte, 15)
	binary.LittleEndian.PutUint64(foot[4:12], uint64(len(plain)))
	foot[12], foot[13], foot[14] = 'z', 's', 'v'
	b.Write(foot)

	roundTripChunk(t, b.Bytes(), plain, key, uint32(b.Len()))
}

func TestDownloadChunk_VZipLZMA(t *testing.T) {
	key := bytes.Repeat([]byte{0x0B}, 32)
	plain := bytes.Repeat([]byte("lzma payload "), 80)

	// lzma.Writer emits the "alone" format: props(5) + uncompressedSize(8) + stream.
	var lz bytes.Buffer
	w, err := lzma.NewWriter(&lz)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	alone := lz.Bytes()
	if len(alone) < 13 {
		t.Fatalf("lzma alone output too short: %d", len(alone))
	}
	props := alone[:5]
	stream := alone[13:]

	// Wire: 'V','Z','a' + crc(4) + props(5) + stream + footer(size4 + 'z','v').
	var b bytes.Buffer
	b.Write([]byte{'V', 'Z', 'a', 0, 0, 0, 0}) // magic + version + crc
	b.Write(props)
	b.Write(stream)
	foot := make([]byte, 6)
	binary.LittleEndian.PutUint32(foot[:4], uint32(len(plain)))
	foot[4], foot[5] = 'z', 'v'
	b.Write(foot)

	roundTripChunk(t, b.Bytes(), plain, key, uint32(b.Len()))
}

// mustEncrypt AES-encrypts data with depotKey using the Steam symmetric scheme.
func mustEncrypt(t *testing.T, data, key []byte) []byte {
	t.Helper()
	enc, err := symmetricEncryptTest(data, key)
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

// zlibCompress wraps data in the zlib container DownloadChunk falls back to when
// the payload carries no VZip/VZip-ZSTD/PKZip magic.
func zlibCompress(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDownloadChunk_SHA1MismatchReturnsCorrupt(t *testing.T) {
	key := bytes.Repeat([]byte{0x0C}, 32)
	plain := []byte("content that will have a deliberately wrong sha1")
	compressed := zlibCompress(t, plain)

	server, client, closeFn := serveChunk(t, mustEncrypt(t, compressed, key))
	defer closeFn()

	chunk := ChunkInfo{
		SHA1:             make([]byte, 20), // all zeros — won't match
		CompressedSize:   uint32(len(compressed)),
		UncompressedSize: uint32(len(plain)),
	}
	_, err := DownloadChunk(context.Background(), client, server, 100, chunk, key, "")
	if !IsCorrupt(err) {
		t.Errorf("expected corrupt error, got %v", err)
	}
}

func TestDownloadChunk_Unauthorized401(t *testing.T) {
	server, client, closeFn := serveTLS(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	defer closeFn()

	chunk := ChunkInfo{SHA1: make([]byte, 20)}
	_, err := DownloadChunk(context.Background(), client, server, 1, chunk, bytes.Repeat([]byte{1}, 32), "")
	if !IsUnauthorized(err) {
		t.Errorf("expected unauthorized error, got %v", err)
	}
}

func TestDownloadChunk_ServerError(t *testing.T) {
	server, client, closeFn := serveTLS(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer closeFn()

	chunk := ChunkInfo{SHA1: make([]byte, 20)}
	_, err := DownloadChunk(context.Background(), client, server, 1, chunk, bytes.Repeat([]byte{1}, 32), "")
	if err == nil || IsUnauthorized(err) || IsCorrupt(err) {
		t.Errorf("expected a generic HTTP error, got %v", err)
	}
}

func TestDownloadChunk_BearerTokenSent(t *testing.T) {
	key := bytes.Repeat([]byte{0x0D}, 32)
	plain := []byte("token-gated chunk content")
	compressed := zlibCompress(t, plain)
	enc := mustEncrypt(t, compressed, key)

	var gotAuth string
	server, client, closeFn := serveTLS(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write(enc) //nolint:errcheck
	})
	defer closeFn()

	h := sha1.New() //nolint:gosec
	h.Write(plain)
	chunk := ChunkInfo{SHA1: h.Sum(nil), CompressedSize: uint32(len(compressed)), UncompressedSize: uint32(len(plain))}
	if _, err := DownloadChunk(context.Background(), client, server, 1, chunk, key, "mytoken"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer mytoken" {
		t.Errorf("Authorization header = %q, want Bearer mytoken", gotAuth)
	}
}

func TestDecompressVZipTooShort(t *testing.T) {
	if _, err := decompressVZip([]byte("VZ")); err == nil {
		t.Error("expected error for too-short vzip")
	}
	if _, err := decompressVZipZSTD([]byte("VS"), "test"); err == nil {
		t.Error("expected error for too-short vzip-zstd")
	}
}

func TestAESSymmetricDecrypt_Errors(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	// Too short to contain an IV.
	if _, err := aesSymmetricDecrypt([]byte{1, 2, 3}, key); err == nil {
		t.Error("expected error for data shorter than block size")
	}
	// Ciphertext not block-aligned.
	bad := make([]byte, 16+5)
	if _, err := aesSymmetricDecrypt(bad, key); err == nil {
		t.Error("expected error for non-block-aligned ciphertext")
	}
}

// sanity check that the test helper SHA1 matches hex IDs used elsewhere.
func TestChunkIDHex(t *testing.T) {
	h := sha1.New() //nolint:gosec
	h.Write([]byte("x"))
	if len(hex.EncodeToString(h.Sum(nil))) != 40 {
		t.Error("sha1 hex should be 40 chars")
	}
}
