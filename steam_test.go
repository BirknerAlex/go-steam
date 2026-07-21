package steam

import (
	"bytes"
	"crypto/aes"
	"crypto/sha1" //nolint:gosec // Steam protocol mandates SHA1
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/BirknerAlex/go-steam/internal/cdn"
	"github.com/BirknerAlex/go-steam/internal/cm"
)

func TestPhaseString(t *testing.T) {
	cases := map[Phase]string{
		PhaseResolving:   "resolving",
		PhaseManifest:    "manifest",
		PhaseDiffing:     "diffing",
		PhaseDownloading: "downloading",
		PhaseComplete:    "complete",
		Phase(999):       "unknown",
	}
	for p, want := range cases {
		if got := p.String(); got != want {
			t.Errorf("Phase(%d).String() = %q, want %q", int(p), got, want)
		}
	}
}

func TestConfigApplyDefaults(t *testing.T) {
	var c Config
	c.applyDefaults()
	if c.SteamGuardCallback == nil {
		t.Error("SteamGuardCallback should default to non-nil")
	}
	if c.MaxParallelChunks != 16 {
		t.Errorf("MaxParallelChunks default = %d, want 16", c.MaxParallelChunks)
	}
	if c.MaxParallelManifests != 4 {
		t.Errorf("MaxParallelManifests default = %d, want 4", c.MaxParallelManifests)
	}
	if c.Log == nil {
		t.Error("Log should default to non-nil")
	}
	if c.CachePath == "" {
		t.Error("CachePath should default to non-empty")
	}

	// Explicit values are preserved.
	c2 := Config{MaxParallelChunks: 3, MaxParallelManifests: 2, CachePath: "/tmp/x"}
	c2.applyDefaults()
	if c2.MaxParallelChunks != 3 || c2.MaxParallelManifests != 2 || c2.CachePath != "/tmp/x" {
		t.Errorf("applyDefaults overwrote explicit values: %+v", c2)
	}
}

func TestContainsUint32(t *testing.T) {
	s := []uint32{1, 5, 9}
	if !containsUint32(s, 5) {
		t.Error("expected to find 5")
	}
	if containsUint32(s, 7) {
		t.Error("did not expect to find 7")
	}
	if containsUint32(nil, 1) {
		t.Error("nil slice should not contain anything")
	}
}

func TestResolveManifestGID(t *testing.T) {
	d := &cm.DepotInfo{
		ManifestGIDs: map[string]uint64{"public": 111, "beta": 222},
	}
	if gid, err := resolveManifestGID(d, "beta", nil); err != nil || gid != 222 {
		t.Errorf("beta: got %d, %v; want 222, nil", gid, err)
	}
	// Unknown branch falls back to public.
	if gid, err := resolveManifestGID(d, "nope", nil); err != nil || gid != 111 {
		t.Errorf("fallback: got %d, %v; want 111, nil", gid, err)
	}
	// No public, unknown branch → error.
	d2 := &cm.DepotInfo{ManifestGIDs: map[string]uint64{"beta": 1}}
	if _, err := resolveManifestGID(d2, "nope", nil); err == nil {
		t.Error("expected error when neither branch nor public exists")
	}
}

func TestResolveManifestGIDEncryptedBranch(t *testing.T) {
	// Build an encrypted GID blob the way Steam does: AES-256-ECB(PKCS7(gid LE)).
	const wantGID uint64 = 0x1122334455667788
	key := bytes.Repeat([]byte{0xAB}, 32) // 32-byte AES-256 key
	plain := make([]byte, 8)
	binary.LittleEndian.PutUint64(plain, wantGID)
	blobHex := hex.EncodeToString(aesECBEncryptPKCS7(t, key, plain))

	d := &cm.DepotInfo{
		ManifestGIDs:          map[string]uint64{"public": 111},
		EncryptedManifestGIDs: map[string]string{"betabranch": blobHex},
	}

	// With the correct branch key, the encrypted GID is decrypted.
	keys := map[string][]byte{"betabranch": key}
	if gid, err := resolveManifestGID(d, "betabranch", keys); err != nil || gid != wantGID {
		t.Errorf("encrypted branch: got %#x, %v; want %#x, nil", gid, err, wantGID)
	}

	// Without the key (no/invalid password), it is an error — NOT a silent
	// fallback to the public branch.
	if _, err := resolveManifestGID(d, "betabranch", nil); err == nil {
		t.Error("expected error for password-protected branch without a key")
	}

	// A wrong key fails padding/length validation rather than returning garbage.
	wrong := map[string][]byte{"betabranch": bytes.Repeat([]byte{0x01}, 32)}
	if _, err := resolveManifestGID(d, "betabranch", wrong); err == nil {
		t.Error("expected error decrypting with the wrong key")
	}
}

// aesECBEncryptPKCS7 mirrors Steam's SymmetricEncryptECB for test vectors.
func aesECBEncryptPKCS7(t *testing.T, key, plaintext []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	pad := aes.BlockSize - len(plaintext)%aes.BlockSize
	padded := append(append([]byte(nil), plaintext...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	out := make([]byte, len(padded))
	for off := 0; off < len(padded); off += aes.BlockSize {
		block.Encrypt(out[off:off+aes.BlockSize], padded[off:off+aes.BlockSize])
	}
	return out
}

func TestSelectDepots(t *testing.T) {
	appInfo := &cm.AppInfo{
		Depots: map[uint32]*cm.DepotInfo{
			10: {DepotID: 10, AllowAnonymous: true, OSList: "linux"},
			11: {DepotID: 11, AllowAnonymous: true, OSList: "windows"},
			12: {DepotID: 12, AllowAnonymous: false, OSList: "linux"},
			13: {DepotID: 13, AllowAnonymous: true, OSList: ""}, // OS-agnostic
		},
	}

	// Anonymous + linux: depot 12 excluded (needs auth), 11 excluded (windows).
	got := selectDepots(appInfo, AppDownloadRequest{OS: "linux"}, false)
	ids := depotIDset(got)
	if ids[12] {
		t.Error("anon should exclude non-anonymous depot 12")
	}
	if ids[11] {
		t.Error("linux filter should exclude windows depot 11")
	}
	if !ids[10] || !ids[13] {
		t.Error("expected depot 10 (linux) and 13 (os-agnostic)")
	}

	// With auth, the non-anonymous depot is now eligible.
	got = selectDepots(appInfo, AppDownloadRequest{OS: "linux"}, true)
	if !depotIDset(got)[12] {
		t.Error("auth should include non-anonymous depot 12")
	}

	// DepotIDs restriction.
	got = selectDepots(appInfo, AppDownloadRequest{DepotIDs: []uint32{13}}, true)
	if len(got) != 1 || got[0].DepotID != 13 {
		t.Errorf("DepotIDs restriction failed: %v", depotIDset(got))
	}

	// Explicit OS filter for a platform with no matching depot: only the
	// OS-agnostic depot survives.
	got = selectDepots(appInfo, AppDownloadRequest{OS: "macos"}, true)
	if len(got) != 1 || got[0].DepotID != 13 {
		t.Errorf("macos filter should select only depot 13, got %v", depotIDset(got))
	}
}

// TestSelectDepotsDefaultsToRuntimeOS is a regression test for a bug where an
// empty OS filter downloaded depots for every platform into the same target
// dir (e.g. Windows .dll and Linux .so files mixed together), which corrupts
// mixed-platform installs. An empty req.OS must now default to the platform
// the process is running on rather than disabling OS filtering entirely.
func TestSelectDepotsDefaultsToRuntimeOS(t *testing.T) {
	appInfo := &cm.AppInfo{
		Depots: map[uint32]*cm.DepotInfo{
			10: {DepotID: 10, AllowAnonymous: true, OSList: "linux"},
			11: {DepotID: 11, AllowAnonymous: true, OSList: "windows"},
			14: {DepotID: 14, AllowAnonymous: true, OSList: "macos"},
			13: {DepotID: 13, AllowAnonymous: true, OSList: ""}, // OS-agnostic
		},
	}

	got := selectDepots(appInfo, AppDownloadRequest{}, true)
	ids := depotIDset(got)

	// The OS-agnostic depot is always selected.
	if !ids[13] {
		t.Error("expected OS-agnostic depot 13 to always be selected")
	}

	// Exactly the depot matching the current runtime platform (if any)
	// should additionally be selected -- never depots for other platforms.
	want := steamOSForGOOS(runtime.GOOS)
	platformDepots := map[uint32]string{10: "linux", 11: "windows", 14: "macos"}
	for id, os := range platformDepots {
		if os == want {
			if !ids[id] {
				t.Errorf("expected depot %d (%s) to be selected for runtime OS %q", id, os, runtime.GOOS)
			}
		} else if ids[id] {
			t.Errorf("depot %d (%s) should not be selected for runtime OS %q -- got all depots, defaulting is not filtering", id, os, runtime.GOOS)
		}
	}
}

func TestSteamOSForGOOS(t *testing.T) {
	cases := map[string]string{
		"windows": "windows",
		"darwin":  "macos",
		"linux":   "linux",
		"freebsd": "", // unknown platform: no filtering
	}
	for goos, want := range cases {
		if got := steamOSForGOOS(goos); got != want {
			t.Errorf("steamOSForGOOS(%q) = %q, want %q", goos, got, want)
		}
	}
}

func depotIDset(ds []*cm.DepotInfo) map[uint32]bool {
	m := make(map[uint32]bool)
	for _, d := range ds {
		m[d.DepotID] = true
	}
	return m
}

func TestJWTExpiry(t *testing.T) {
	makeJWT := func(payload string) string {
		seg := base64.RawURLEncoding.EncodeToString([]byte(payload))
		return "hdr." + seg + ".sig"
	}
	exp := time.Now().Add(48 * time.Hour).Unix()
	body, _ := json.Marshal(struct {
		Exp int64 `json:"exp"`
	}{Exp: exp})
	tok := makeJWT(string(body))
	got := jwtExpiry(tok)
	if got.Unix() != exp {
		t.Errorf("jwtExpiry = %d, want %d", got.Unix(), exp)
	}

	// Malformed tokens return zero time.
	if !jwtExpiry("not-a-jwt").IsZero() {
		t.Error("malformed token should return zero time")
	}
	if !jwtExpiry("a.b.c").IsZero() {
		t.Error("non-base64 payload should return zero time")
	}
	// Valid base64 but no exp claim.
	noExp := makeJWT(`{"sub":"x"}`)
	if !jwtExpiry(noExp).IsZero() {
		t.Error("token without exp should return zero time")
	}
}

func TestPreallocateAndWriteChunk(t *testing.T) {
	dir := t.TempDir()
	rel := "sub/dir/file.bin"

	preallocateFile(dir, rel, 100, 0)
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	st, err := os.Stat(abs)
	if err != nil {
		t.Fatalf("preallocate did not create file: %v", err)
	}
	if st.Size() != 100 {
		t.Errorf("preallocated size = %d, want 100", st.Size())
	}

	data := []byte("hello world chunk")
	if err := writeChunk(dir, rel, 10, data); err != nil {
		t.Fatalf("writeChunk: %v", err)
	}
	raw, _ := os.ReadFile(abs)
	if string(raw[10:10+len(data)]) != string(data) {
		t.Errorf("chunk not written at offset 10: %q", raw[10:10+len(data)])
	}

	// writeChunk on a missing file errors.
	if err := writeChunk(dir, "does/not/exist.bin", 0, data); err == nil {
		t.Error("writeChunk should error when file is absent")
	}
}

func TestPreallocateExecutableBit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits differ on windows")
	}
	dir := t.TempDir()
	preallocateFile(dir, "tool", 10, 0x20) // Executable flag
	st, err := os.Stat(filepath.Join(dir, "tool"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o111 == 0 {
		t.Errorf("executable flag should set exec bits, got %v", st.Mode().Perm())
	}
}

// TestRestoreLaunchExecutableBits is a regression test for the Palworld
// dedicated server, whose depot manifest sets EDepotFileFlag.Executable on
// none of its files -- not even PalServer.sh, the file Steam itself would
// run. PICS "config.launch" is the reliable fallback: it names PalServer.sh
// as the Linux launch executable, so it must end up +x after download even
// though the manifest gave no per-file hint.
func TestRestoreLaunchExecutableBits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix exec bit semantics don't apply on windows")
	}
	dir := t.TempDir()
	for _, name := range []string{"PalServer.sh", "PalServer.exe", "Readme.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries := []cm.LaunchEntry{
		{Executable: "PalServer.exe", OSList: "windows"},
		{Executable: "PalServer.sh", OSList: "linux"},
		{Executable: "Missing.sh", OSList: "linux"}, // not downloaded; must not error
	}

	restoreLaunchExecutableBits(dir, entries, "linux")

	sh, err := os.Stat(filepath.Join(dir, "PalServer.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if sh.Mode().Perm()&0o111 == 0 {
		t.Errorf("PalServer.sh should be executable after linux download, got %v", sh.Mode().Perm())
	}

	exe, err := os.Stat(filepath.Join(dir, "PalServer.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if exe.Mode().Perm()&0o111 != 0 {
		t.Errorf("PalServer.exe (windows-only entry) should not be touched for a linux download, got %v", exe.Mode().Perm())
	}
}

func TestRestoreLaunchExecutableBits_WindowsFilterIsNoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "PalServer.exe"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []cm.LaunchEntry{{Executable: "PalServer.exe", OSList: "windows"}}

	restoreLaunchExecutableBits(dir, entries, "windows")

	st, err := os.Stat(filepath.Join(dir, "PalServer.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o644 {
		t.Errorf("windows target should not gain exec bits, got %v", st.Mode().Perm())
	}
}

func TestEffectiveOSFilter(t *testing.T) {
	if got := effectiveOSFilter(AppDownloadRequest{OS: "macos"}); got != "macos" {
		t.Errorf("explicit OS should win: got %q", got)
	}
	if got, want := effectiveOSFilter(AppDownloadRequest{}), steamOSForGOOS(runtime.GOOS); got != want {
		t.Errorf("empty OS should default to runtime platform: got %q, want %q", got, want)
	}
}

func TestChunkOnDisk(t *testing.T) {
	dir := t.TempDir()
	rel := "data.bin"
	content := []byte("the quick brown fox jumps over the lazy dog")

	// File with the chunk content at offset 5.
	full := make([]byte, 5+len(content))
	copy(full[5:], content)
	if err := os.WriteFile(filepath.Join(dir, rel), full, 0o644); err != nil {
		t.Fatal(err)
	}

	h := sha1.New() //nolint:gosec
	h.Write(content)
	chunk := cdn.ChunkInfo{
		SHA1:             h.Sum(nil),
		Offset:           5,
		UncompressedSize: uint32(len(content)),
	}
	if !chunkOnDisk(dir, rel, chunk) {
		t.Error("chunkOnDisk should return true for matching content")
	}

	// Wrong SHA1 → false.
	bad := chunk
	bad.SHA1 = make([]byte, 20)
	if chunkOnDisk(dir, rel, bad) {
		t.Error("chunkOnDisk should return false for mismatched SHA1")
	}

	// Missing file → false.
	if chunkOnDisk(dir, "absent.bin", chunk) {
		t.Error("chunkOnDisk should return false for missing file")
	}

	// File too short → false.
	short := chunk
	short.Offset = 1000
	if chunkOnDisk(dir, rel, short) {
		t.Error("chunkOnDisk should return false when file is too short")
	}
}

func TestCreateSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privilege on windows")
	}
	dir := t.TempDir()
	createSymlink(dir, cdn.ManifestFile{Path: "link", SymlinkTarget: "target.txt"})
	got, err := os.Readlink(filepath.Join(dir, "link"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if got != "target.txt" {
		t.Errorf("symlink target = %q, want target.txt", got)
	}

	// Re-creating replaces the existing link without error.
	createSymlink(dir, cdn.ManifestFile{Path: "link", SymlinkTarget: "other.txt"})
	got, _ = os.Readlink(filepath.Join(dir, "link"))
	if got != "other.txt" {
		t.Errorf("symlink should be replaced, got %q", got)
	}
}

func TestSteamGuardCallbacks(t *testing.T) {
	// UnknownSteamGuard always errors.
	if _, err := UnknownSteamGuard()(); err == nil {
		t.Error("UnknownSteamGuard should return an error")
	}
	// SteamGuardCodeGenerate produces a 5-char code from a valid secret.
	code, err := SteamGuardCodeGenerate("cnOgv/KdpLoP6Nbh0GMkXkPXALQ=")()
	if err != nil {
		t.Fatalf("SteamGuardCodeGenerate: %v", err)
	}
	if len(code) != 5 {
		t.Errorf("guard code length = %d, want 5", len(code))
	}
}
