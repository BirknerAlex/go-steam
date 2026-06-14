package cdn

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1" //nolint:gosec // Steam protocol mandates SHA1 for chunk verification
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/klauspost/compress/zstd"
)

// DownloadChunk fetches, decrypts, decompresses, and verifies one content chunk.
// On a 401 the caller should refresh the token and retry; this function returns
// errUnauthorized to signal that.
// On a 5xx it returns a retriable error and the server should be penalised.
func DownloadChunk(
	ctx context.Context,
	httpClient *http.Client,
	server *Server,
	depotID uint32,
	chunk ChunkInfo,
	depotKey []byte,
	token string,
) ([]byte, error) {
	chunkID := hex.EncodeToString(chunk.SHA1)
	url := fmt.Sprintf("https://%s/depot/%d/chunk/%s", server.Host, depotID, chunkID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chunk: fetch %s: %w", chunkID[:8], err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// OK — proceed
	case http.StatusUnauthorized:
		return nil, errChunkUnauthorized
	default:
		return nil, fmt.Errorf("chunk: HTTP %d for chunk %s", resp.StatusCode, chunkID[:8])
	}

	// Limit read to compressed size + a generous margin.
	limit := int64(chunk.CompressedSize) + 1024
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("chunk: read body: %w", err)
	}

	// 1. AES-256 symmetric decrypt (Steam ECB-IV + CBC — same scheme as manifests).
	decrypted, err := aesSymmetricDecrypt(raw, depotKey)
	if err != nil {
		return nil, fmt.Errorf("chunk: decrypt %s: %w", chunkID[:8], err)
	}

	// 2. Decompress.
	// The format is detected purely from the magic bytes of the decrypted payload,
	// matching SteamKit2's DepotChunk.Process. There is no size-based "raw" case:
	// chunks whose compressed and uncompressed lengths happen to match are still
	// wrapped in one of these containers.
	//   'VS' ('VSZa') → VZip-ZSTD (Steam ZSTD wrapper)
	//   'VZ' ('VZa')  → VZip (Steam LZMA wrapper)
	//   'PK\x03\x04'  → ZIP (standard ZIP with a deflate entry)
	//   otherwise     → ZLib (RFC 1950) fallback
	var decompressed []byte
	switch {
	case len(decrypted) >= 2 && decrypted[0] == 'V' && decrypted[1] == 'S':
		decompressed, err = decompressVZipZSTD(decrypted, chunkID[:8])
		if err != nil {
			return nil, fmt.Errorf("chunk: vzip-zstd %s: %w", chunkID[:8], err)
		}
	case len(decrypted) >= 2 && decrypted[0] == 'V' && decrypted[1] == 'Z':
		decompressed, err = decompressVZip(decrypted)
		if err != nil {
			return nil, fmt.Errorf("chunk: vzip %s: %w", chunkID[:8], err)
		}
	case len(decrypted) >= 4 && decrypted[0] == 'P' && decrypted[1] == 'K' && decrypted[2] == 0x03 && decrypted[3] == 0x04:
		decompressed, err = decompressZip(decrypted)
		if err != nil {
			return nil, fmt.Errorf("chunk: zip %s: %w", chunkID[:8], err)
		}
	default:
		decompressed, err = decompressZLib(decrypted)
		if err != nil {
			return nil, fmt.Errorf("chunk: zlib %s: %w", chunkID[:8], err)
		}
	}

	// 3. Verify SHA1.
	//nolint:gosec
	h := sha1.New()
	h.Write(decompressed)
	sum := h.Sum(nil)
	if !bytes.Equal(sum, chunk.SHA1) {
		slog.Debug("chunk SHA1 mismatch", "id", chunkID[:8],
			"got", fmt.Sprintf("%x", sum[:4]),
			"want", fmt.Sprintf("%x", chunk.SHA1[:4]),
			"decompressed_len", len(decompressed),
			"uncompressed_size", chunk.UncompressedSize)
		return nil, errChunkCorrupt
	}

	return decompressed, nil
}

// errChunkUnauthorized signals a 401 so the caller can refresh the token.
var errChunkUnauthorized = fmt.Errorf("chunk: CDN token expired (401)")

// errChunkCorrupt signals that the downloaded chunk failed SHA1 verification.
// The caller should penalise the server and retry from a different one.
var errChunkCorrupt = fmt.Errorf("chunk: SHA1 mismatch (corrupt data from CDN)")

// IsUnauthorized returns true if the error is a CDN 401.
func IsUnauthorized(err error) bool {
	return err == errChunkUnauthorized
}

// IsCorrupt returns true when a chunk failed SHA1 verification.
func IsCorrupt(err error) bool {
	return err == errChunkCorrupt
}

// decompressZip extracts and returns the first entry from a ZIP archive.
// Steam CDN chunks in ZIP format contain a single deflate-compressed entry.
func decompressZip(data []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(zr.File) == 0 {
		return nil, fmt.Errorf("empty zip archive")
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// decompressZLib decompresses standard zlib (RFC 1950) data.
func decompressZLib(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

var zstdDecoder, _ = zstd.NewReader(nil)

// decompressVZipZSTD decompresses Steam's VZip-ZSTD container.
// Wire layout (after AES decryption):
//
//	[0-3]:        'V','S','Z','a' magic
//	[4-7]:        CRC32 of the uncompressed data
//	[8 .. N-15]:  a single ZSTD frame
//	footer (15B): CRC32(4) + uncompressed_size(uint64 LE, 8) + 'z','s','v'
//
// The footer must be stripped before decoding: DecodeAll treats trailing bytes
// as a second frame, and a footer CRC that happens to look like a (skippable)
// frame magic would otherwise corrupt the output. The decoded result is SHA1
// verified by the caller.
func decompressVZipZSTD(data []byte, chunkID string) ([]byte, error) {
	const headerSize = 8
	const footerSize = 15 // CRC32(4) + uncompressed_size(8) + 'z','s','v'(3)
	if len(data) < headerSize+footerSize {
		return nil, fmt.Errorf("vzip-zstd %s: too short (%d bytes)", chunkID, len(data))
	}
	footer := data[len(data)-footerSize:]
	if footer[12] != 'z' || footer[13] != 's' || footer[14] != 'v' {
		return nil, fmt.Errorf("vzip-zstd %s: bad footer magic %02x%02x%02x",
			chunkID, footer[12], footer[13], footer[14])
	}
	frame := data[headerSize : len(data)-footerSize]
	out, err := zstdDecoder.DecodeAll(frame, nil)
	if err != nil {
		return nil, fmt.Errorf("vzip-zstd %s: %w", chunkID, err)
	}
	return out, nil
}

// decompressVZip decompresses Steam's VZip format (LZMA wrapped in a Steam container).
// Wire layout:
//
//	[0-1]:   'V','Z' magic
//	[2]:     version byte (usually 'a')
//	[3-6]:   CRC32 of uncompressed data (uint32 LE)
//	[7-11]:  LZMA properties (5 bytes: lclppb + dict_size)
//	[12..N-6]: LZMA compressed stream
//	[N-6..N-2]: uncompressed size (uint32 LE) — in footer, not header
//	[N-2..N]: end marker ('z','v')
func decompressVZip(data []byte) ([]byte, error) {
	const headerSize = 2 + 1 + 4 // 7 bytes: magic + version + crc
	const propsSize = 5
	const footerSize = 4 + 2 // 6 bytes: uncompressedSize + end marker
	if len(data) < headerSize+propsSize+footerSize {
		return nil, fmt.Errorf("vzip: too short (%d bytes)", len(data))
	}
	lzmaProps := data[headerSize : headerSize+propsSize]
	lzmaStream := data[headerSize+propsSize : len(data)-footerSize]
	uncompressedSize := binary.LittleEndian.Uint32(data[len(data)-footerSize : len(data)-footerSize+4])

	// Reconstruct LZMA "alone" format: props(5) + uncompressedSize(8 LE) + stream.
	var buf bytes.Buffer
	buf.Write(lzmaProps)
	var sizeBuf [8]byte
	binary.LittleEndian.PutUint64(sizeBuf[:], uint64(uncompressedSize))
	buf.Write(sizeBuf[:])
	buf.Write(lzmaStream)

	return lzmaDecompress(buf.Bytes())
}

// aesSymmetricDecrypt decrypts Steam chunk data using the Steam symmetric scheme:
//
//	wire = encryptedIV(16) + AES-CBC(ciphertext)
//	where encryptedIV = AES-ECB(key, rawIV)
//
// This matches CryptoHelper.SymmetricDecrypt in SteamKit2.
func aesSymmetricDecrypt(data, key []byte) ([]byte, error) {
	if len(data) < aes.BlockSize {
		return nil, fmt.Errorf("chunk: data too short to decrypt (%d bytes)", len(data))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	// ECB-decrypt the first 16 bytes to recover the raw IV.
	rawIV := make([]byte, aes.BlockSize)
	block.Decrypt(rawIV, data[:aes.BlockSize])

	ciphertext := data[aes.BlockSize:]
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("chunk: ciphertext not block-aligned (%d bytes)", len(ciphertext))
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, rawIV).CryptBlocks(plaintext, ciphertext)
	// Strip PKCS#7 padding.
	if len(plaintext) > 0 {
		pad := int(plaintext[len(plaintext)-1])
		if pad >= 1 && pad <= aes.BlockSize && pad <= len(plaintext) {
			plaintext = plaintext[:len(plaintext)-pad]
		}
	}
	return plaintext, nil
}
