package cdn

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// These vectors are the real depot-chunk fixtures from SteamKit2's test suite
// (SteamKit2/Tests/Files + DepotChunkFacts.cs). Each .bin file is the raw
// AES-encrypted chunk exactly as served by the Steam CDN; DownloadChunk must
// decrypt, detect the container format from its magic bytes, decompress, and
// SHA1-verify it. The chunk ID (file name) is the expected SHA1 of the result.
//
// Covering all three real formats guards the magic-byte detection and the VZip
// (LZMA), VZip-ZSTD, and PKZip decoders against regressions — in particular the
// VZip case whose compressed/uncompressed lengths must NOT be mistaken for a
// raw, uncompressed chunk.
func TestDownloadChunk_SteamKit2Vectors(t *testing.T) {
	tests := []struct {
		name      string
		file      string
		sha1      string // also the chunk ID
		depotKey  []byte
		compLen   uint32
		uncompLen uint32
	}{
		{
			name:      "PKZip",
			file:      "depot_440_chunk_bac8e2657470b2eb70d6ddcd6c07004be8738697.bin",
			sha1:      "bac8e2657470b2eb70d6ddcd6c07004be8738697",
			compLen:   320,
			uncompLen: 544,
			depotKey: []byte{
				0x44, 0xCE, 0x5C, 0x52, 0x97, 0xA4, 0x15, 0xA1,
				0xA6, 0xF6, 0x9C, 0x85, 0x60, 0x37, 0xA5, 0xA2,
				0xFD, 0xD8, 0x2C, 0xD4, 0x74, 0xFA, 0x65, 0x9E,
				0xDF, 0xB4, 0xD5, 0x9B, 0x2A, 0xBC, 0x55, 0xFC,
			},
		},
		{
			name:      "VZip_LZMA",
			file:      "depot_232250_chunk_7b8567d9b3c09295cdbf4978c32b348d8e76c750.bin",
			sha1:      "7b8567d9b3c09295cdbf4978c32b348d8e76c750",
			compLen:   304,
			uncompLen: 798,
			depotKey: []byte{
				0xE5, 0xF6, 0xAE, 0xD5, 0x5E, 0x9E, 0xCE, 0x42,
				0x9E, 0x56, 0xB8, 0x13, 0xFB, 0xF6, 0xBF, 0xE9,
				0x24, 0xF3, 0xCF, 0x72, 0x97, 0x2F, 0xDB, 0xD0,
				0x57, 0x1F, 0xFC, 0xAD, 0x9F, 0x2F, 0x7D, 0xAA,
			},
		},
		{
			name:      "VZip_ZSTD",
			file:      "depot_3441461_chunk_9e72678e305540630a665b93e1463bc3983eb55a.bin",
			sha1:      "9e72678e305540630a665b93e1463bc3983eb55a",
			compLen:   176,
			uncompLen: 156,
			depotKey: []byte{
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10,
				0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
				0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F, 0x20,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := os.ReadFile(filepath.Join("testdata", tc.file))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			if uint32(len(wire)) != tc.compLen {
				t.Fatalf("fixture length = %d, want compressed length %d", len(wire), tc.compLen)
			}

			sha1Bytes, err := hex.DecodeString(tc.sha1)
			if err != nil {
				t.Fatal(err)
			}

			server, client, closeFn := serveChunk(t, wire)
			defer closeFn()

			chunk := ChunkInfo{
				SHA1:             sha1Bytes,
				CompressedSize:   tc.compLen,
				UncompressedSize: tc.uncompLen,
			}
			got, err := DownloadChunk(context.Background(), client, server, 1, chunk, tc.depotKey, "")
			if err != nil {
				t.Fatalf("DownloadChunk: %v", err)
			}
			// DownloadChunk already SHA1-verifies, so reaching here means the
			// decode is byte-exact; assert the length matches as a sanity check.
			if uint32(len(got)) != tc.uncompLen {
				t.Errorf("decoded length = %d, want %d", len(got), tc.uncompLen)
			}
		})
	}
}
