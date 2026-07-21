package cm

import (
	"log/slog"
	"testing"

	"github.com/BirknerAlex/go-steam/internal/proto"
)

func TestParseTextKV_Basic(t *testing.T) {
	in := `"appinfo"
{
	"appid"  "740"
	"common"
	{
		"type"  "game"
	}
}`
	kv, err := parseTextKV([]byte(in))
	if err != nil {
		t.Fatalf("parseTextKV: %v", err)
	}
	appinfo, ok := kv["appinfo"].(map[string]any)
	if !ok {
		t.Fatalf("appinfo not a map: %T", kv["appinfo"])
	}
	if appinfo["appid"] != "740" {
		t.Errorf("appid = %v, want 740", appinfo["appid"])
	}
	common, ok := appinfo["common"].(map[string]any)
	if !ok || common["type"] != "game" {
		t.Errorf("common.type = %v, want game", appinfo["common"])
	}
}

func TestParseTextKV_EscapesAndComments(t *testing.T) {
	in := `"root"
{
	// a comment line
	"path"  "C:\\Program Files\\Steam"
	"quote" "he said \"hi\""
	"newline" "line1\nline2"
}`
	kv, err := parseTextKV([]byte(in))
	if err != nil {
		t.Fatalf("parseTextKV: %v", err)
	}
	root := kv["root"].(map[string]any)
	if root["path"] != `C:\Program Files\Steam` {
		t.Errorf("escaped backslash wrong: %q", root["path"])
	}
	if root["quote"] != `he said "hi"` {
		t.Errorf("escaped quote wrong: %q", root["quote"])
	}
	if root["newline"] != "line1\nline2" {
		t.Errorf("escaped newline wrong: %q", root["newline"])
	}
}

func TestParseTextKV_Unterminated(t *testing.T) {
	if _, err := parseTextKV([]byte(`"key" "unterminated`)); err == nil {
		t.Error("expected error for unterminated string")
	}
}

func TestExtractUint64(t *testing.T) {
	if v, ok := extractUint64("12345"); !ok || v != 12345 {
		t.Errorf("string: got %d %v", v, ok)
	}
	if v, ok := extractUint64(uint64(999)); !ok || v != 999 {
		t.Errorf("uint64: got %d %v", v, ok)
	}
	if _, ok := extractUint64("notanumber"); ok {
		t.Error("non-numeric string should fail")
	}
	if _, ok := extractUint64(3.14); ok {
		t.Error("float should fail")
	}
}

func TestParseManifestSection(t *testing.T) {
	// New format: branch → { gid → "..." }.
	gids := make(map[string]uint64)
	parseManifestSection(map[string]any{
		"public": map[string]any{"gid": "111"},
		"beta":   map[string]any{"gid": "222"},
	}, gids)
	if gids["public"] != 111 || gids["beta"] != 222 {
		t.Errorf("new format parse wrong: %v", gids)
	}

	// Old format: branch → "gid".
	gids2 := make(map[string]uint64)
	parseManifestSection(map[string]any{"public": "333"}, gids2)
	if gids2["public"] != 333 {
		t.Errorf("old format parse wrong: %v", gids2)
	}

	// Non-map input is a no-op.
	gids3 := make(map[string]uint64)
	parseManifestSection("not a map", gids3)
	if len(gids3) != 0 {
		t.Errorf("non-map should be no-op, got %v", gids3)
	}
}

func TestParseAppInfo_Full(t *testing.T) {
	vdf := `"appinfo"
{
	"appid"  "740"
	"common"
	{
		"type"  "game"
	}
	"depots"
	{
		"workshopdepot"  "741"
		"1006"
		{
			"config"
			{
				"oslist"  "linux,windows"
			}
			"manifests"
			{
				"public"
				{
					"gid"  "9999"
				}
			}
		}
		"1007"
		{
			"dlcappid"  "12345"
			"manifests"
			{
				"public"  "8888"
			}
		}
	}
}`
	info, err := parseAppInfo(slog.Default(), proto.PICSAppResult{Appid: 740, Buffer: []byte(vdf)})
	if err != nil {
		t.Fatalf("parseAppInfo: %v", err)
	}
	if info.Type != "game" {
		t.Errorf("type = %q, want game", info.Type)
	}
	if info.WorkshopDepot != 741 {
		t.Errorf("workshopdepot = %d, want 741", info.WorkshopDepot)
	}

	d1006 := info.Depots[1006]
	if d1006 == nil {
		t.Fatal("depot 1006 missing")
	}
	if !d1006.AllowAnonymous {
		t.Error("depot 1006 (no dlcappid) should allow anonymous")
	}
	if d1006.OSList != "linux,windows" {
		t.Errorf("oslist = %q", d1006.OSList)
	}
	if d1006.ManifestGIDs["public"] != 9999 {
		t.Errorf("depot 1006 public gid = %d, want 9999", d1006.ManifestGIDs["public"])
	}

	d1007 := info.Depots[1007]
	if d1007 == nil {
		t.Fatal("depot 1007 missing")
	}
	if d1007.AllowAnonymous {
		t.Error("depot 1007 (has dlcappid) should NOT allow anonymous")
	}
	if d1007.ManifestGIDs["public"] != 8888 {
		t.Errorf("depot 1007 public gid = %d, want 8888 (old format)", d1007.ManifestGIDs["public"])
	}
}

// TestParseAppInfo_LaunchEntries mirrors the real PICS "config.launch"
// shape for a Palworld-like dedicated server app: a Windows entry, a Linux
// entry (the one that matters -- its executable is a shell script that
// needs the executable bit set after download), and an unrestricted entry
// with no "config.oslist" at all.
func TestParseAppInfo_LaunchEntries(t *testing.T) {
	vdf := `"appinfo"
{
	"appid"  "2394010"
	"config"
	{
		"installdir"  "PalServer"
		"launch"
		{
			"0"
			{
				"executable"  "PalServer.exe"
				"config"
				{
					"oslist"  "windows"
				}
			}
			"1"
			{
				"executable"  "PalServer.sh"
				"config"
				{
					"oslist"  "linux"
				}
			}
			"2"
			{
				"executable"  "PalServer.exe"
			}
		}
	}
}`
	info, err := parseAppInfo(slog.Default(), proto.PICSAppResult{Appid: 2394010, Buffer: []byte(vdf)})
	if err != nil {
		t.Fatalf("parseAppInfo: %v", err)
	}
	if len(info.LaunchEntries) != 3 {
		t.Fatalf("LaunchEntries count = %d, want 3: %+v", len(info.LaunchEntries), info.LaunchEntries)
	}
	// Sorted by numeric index: windows, linux, unrestricted.
	if got := info.LaunchEntries[0]; got.Executable != "PalServer.exe" || got.OSList != "windows" {
		t.Errorf("entry 0 = %+v, want {PalServer.exe windows}", got)
	}
	if got := info.LaunchEntries[1]; got.Executable != "PalServer.sh" || got.OSList != "linux" {
		t.Errorf("entry 1 = %+v, want {PalServer.sh linux}", got)
	}
	if got := info.LaunchEntries[2]; got.Executable != "PalServer.exe" || got.OSList != "" {
		t.Errorf("entry 2 = %+v, want {PalServer.exe \"\"}", got)
	}
}

func TestParseAppInfo_EmptyBuffer(t *testing.T) {
	info, err := parseAppInfo(slog.Default(), proto.PICSAppResult{Appid: 5})
	if err != nil {
		t.Fatalf("parseAppInfo empty: %v", err)
	}
	if info.AppID != 5 || len(info.Depots) != 0 {
		t.Errorf("empty buffer should yield empty app info, got %+v", info)
	}
}

func TestAppInfoCache(t *testing.T) {
	c := &AppInfoCache{items: make(map[uint32]*AppInfo)}
	if c.get(1) != nil {
		t.Error("empty cache should miss")
	}
	info := &AppInfo{AppID: 1, Type: "game"}
	c.set(info)
	got := c.get(1)
	if got == nil || got.Type != "game" {
		t.Errorf("cache get after set failed: %v", got)
	}
}
