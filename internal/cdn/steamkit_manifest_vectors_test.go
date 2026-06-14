package cdn

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// depot440Key is the depot 440 manifest decryption key from SteamKit2's
// DepotManifestFacts.cs (identical to the PKZip chunk vector key).
var depot440Key = []byte{
	0x44, 0xCE, 0x5C, 0x52, 0x97, 0xA4, 0x15, 0xA1,
	0xA6, 0xF6, 0x9C, 0x85, 0x60, 0x37, 0xA5, 0xA2,
	0xFD, 0xD8, 0x2C, 0xD4, 0x74, 0xFA, 0x65, 0x9E,
	0xDF, 0xB4, 0xD5, 0x9B, 0x2A, 0xBC, 0x55, 0xFC,
}

const depot440GID = 1118032470228587934

// zipWrapManifest reconstructs the real Steam CDN wire format around the raw
// magic-prefixed manifest sections in SteamKit2's fixtures: the CDN serves the
// sections wrapped in a single-entry ZIP, which is what decodeManifest's ZIP
// branch consumes.
func zipWrapManifest(t *testing.T, sections []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("z")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(sections); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// assertDepot440Manifest replicates SteamKit2 DepotManifestFacts.TestDecryptedManifest
// against our parsed Manifest (limited to the fields our Manifest type exposes).
func assertDepot440Manifest(t *testing.T, m *Manifest) {
	t.Helper()

	if m.DepotID != 440 {
		t.Errorf("DepotID = %d, want 440", m.DepotID)
	}
	if m.ManifestGID != depot440GID {
		t.Errorf("ManifestGID = %d, want %d", m.ManifestGID, uint64(depot440GID))
	}
	wantTime := time.Date(2013, 4, 17, 20, 39, 24, 0, time.UTC)
	if !m.CreatedAt.UTC().Equal(wantTime) {
		t.Errorf("CreatedAt = %v, want %v", m.CreatedAt.UTC(), wantTime)
	}

	wantNames := []string{
		"bin/dxsupport.cfg",
		"bin/dxsupport.csv",
		"bin/dxsupport_episodic.cfg",
		"bin/dxsupport_sp.cfg",
		"bin/vidcfg.bin",
		"hl2/media/startupvids.txt",
		"tf/media/startupvids.txt",
	}
	if len(m.Files) != len(wantNames) {
		t.Fatalf("file count = %d, want %d", len(m.Files), len(wantNames))
	}
	for i, want := range wantNames {
		if m.Files[i].Path != want {
			t.Errorf("Files[%d].Path = %q, want %q", i, m.Files[i].Path, want)
		}
		if len(m.Files[i].Chunks) != 1 {
			t.Errorf("Files[%d] has %d chunks, want 1", i, len(m.Files[i].Chunks))
		}
	}

	if m.Files[0].Flags != 0 {
		t.Errorf("Files[0].Flags = %d, want 0", m.Files[0].Flags)
	}
	if m.Files[0].Size != 398709 {
		t.Errorf("Files[0].Size = %d, want 398709", m.Files[0].Size)
	}
	wantHash, _ := hex.DecodeString("bac8e2657470b2eb70d6ddcd6c07004be8738697")
	if !bytes.Equal(m.Files[2].SHA1, wantHash) {
		t.Errorf("Files[2].SHA1 = %x, want %x", m.Files[2].SHA1, wantHash)
	}

	// Files[6] = tf/media/startupvids.txt, single chunk.
	chunk := m.Files[6].Chunks[0]
	if chunk.CompressedSize != 144 {
		t.Errorf("Files[6].Chunks[0].CompressedSize = %d, want 144", chunk.CompressedSize)
	}
	if chunk.UncompressedSize != 17 {
		t.Errorf("Files[6].Chunks[0].UncompressedSize = %d, want 17", chunk.UncompressedSize)
	}
	if chunk.Offset != 0 {
		t.Errorf("Files[6].Chunks[0].Offset = %d, want 0", chunk.Offset)
	}
	wantChunkID, _ := hex.DecodeString("94020bde145a521edec9a9424e7a90fd042481e9")
	if !bytes.Equal(chunk.SHA1, wantChunkID) {
		t.Errorf("Files[6].Chunks[0].SHA1 (ChunkID) = %x, want %x", chunk.SHA1, wantChunkID)
	}
}

// TestDecodeManifest_SteamKit2_ParsesAndDecrypts mirrors SteamKit2's
// ParsesAndDecryptsManifest: an encrypted-filenames manifest is parsed and its
// filenames decrypted with the depot key.
func TestDecodeManifest_SteamKit2_ParsesAndDecrypts(t *testing.T) {
	sections, err := os.ReadFile(filepath.Join("testdata", "depot_440_1118032470228587934.manifest"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := decodeManifest(zipWrapManifest(t, sections), depot440Key, 440, depot440GID)
	if err != nil {
		t.Fatalf("decodeManifest: %v", err)
	}
	assertDepot440Manifest(t, m)
}

// TestDecodeManifest_SteamKit2_ParsesDecrypted mirrors SteamKit2's
// ParsesDecryptedManifest: a manifest whose filenames are already in the clear
// is parsed without a depot key.
func TestDecodeManifest_SteamKit2_ParsesDecrypted(t *testing.T) {
	sections, err := os.ReadFile(filepath.Join("testdata", "depot_440_1118032470228587934_decrypted.manifest"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := decodeManifest(zipWrapManifest(t, sections), nil, 440, depot440GID)
	if err != nil {
		t.Fatalf("decodeManifest: %v", err)
	}
	assertDepot440Manifest(t, m)
}

// TestDecodeManifest_SteamKit2_ParsesV4 mirrors SteamKit2's
// ParsesAndDecryptsManifestVersion4: the Steam3 binary ("version 4") manifest is
// parsed and its encrypted filenames decrypted with the depot key. Unlike the
// protobuf fixtures this format is consumed verbatim (no ZIP wrapper).
func TestDecodeManifest_SteamKit2_ParsesV4(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "depot_440_1118032470228587934_v4.manifest"))
	if err != nil {
		t.Fatal(err)
	}

	// The raw header carries metadata our Manifest type doesn't expose; assert it
	// directly, matching SteamKit2's FilenamesEncrypted / EncryptedCRC checks.
	_, meta, err := parseSteam3Manifest(raw)
	if err != nil {
		t.Fatalf("parseSteam3Manifest: %v", err)
	}
	if !meta.FilenamesEncrypted {
		t.Error("FilenamesEncrypted = false, want true")
	}
	if meta.CrcEncrypted != 1195249848 {
		t.Errorf("EncryptedCRC = %d, want 1195249848", meta.CrcEncrypted)
	}
	if meta.CbDiskOriginal != 825745 {
		t.Errorf("TotalUncompressedSize = %d, want 825745", meta.CbDiskOriginal)
	}
	if meta.CbDiskCompressed != 43168 {
		t.Errorf("TotalCompressedSize = %d, want 43168", meta.CbDiskCompressed)
	}

	m, err := decodeManifest(raw, depot440Key, 440, depot440GID)
	if err != nil {
		t.Fatalf("decodeManifest: %v", err)
	}
	assertDepot440Manifest(t, m)
}
