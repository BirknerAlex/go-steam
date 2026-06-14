package cm

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/BirknerAlex/go-steam/internal/proto"
)

// TestParseAppInfo_SteamKit2_App480 runs the full PICS pipeline on the real
// captured EMsgClientPICSProductInfoResponse for app 480 (Spacewar) from
// SteamKit2's test packets: wire bytes → UnmarshalPacket → body proto →
// parseAppInfo VDF decode. It guards the text-VDF appinfo parsing against
// regressions using real Steam data rather than synthetic input.
func TestParseAppInfo_SteamKit2_App480(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "002_in_8904_k_EMsgClientPICSProductInfoResponse_app480.bin"))
	if err != nil {
		t.Fatal(err)
	}

	pkt, err := proto.UnmarshalPacket(data)
	if err != nil {
		t.Fatalf("UnmarshalPacket: %v", err)
	}
	var resp proto.CMsgClientPICSProductInfoResponse
	if err := resp.Unmarshal(pkt.Body); err != nil {
		t.Fatalf("body Unmarshal: %v", err)
	}
	if len(resp.Apps) != 1 {
		t.Fatalf("apps = %d, want 1", len(resp.Apps))
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	info, err := parseAppInfo(log, resp.Apps[0])
	if err != nil {
		t.Fatalf("parseAppInfo: %v", err)
	}

	if info.AppID != 480 {
		t.Errorf("AppID = %d, want 480", info.AppID)
	}
	if info.Type != "Game" {
		t.Errorf("Type = %q, want %q", info.Type, "Game")
	}
	if info.WorkshopDepot != 480 {
		t.Errorf("WorkshopDepot = %d, want 480", info.WorkshopDepot)
	}
	if len(info.Depots) != 2 {
		t.Fatalf("depots = %d, want 2", len(info.Depots))
	}

	d481, ok := info.Depots[481]
	if !ok {
		t.Fatal("depot 481 missing")
	}
	if !d481.AllowAnonymous {
		t.Errorf("depot 481 AllowAnonymous = false, want true")
	}
	if got := d481.ManifestGIDs["public"]; got != 3183503801510301321 {
		t.Errorf("depot 481 public manifest GID = %d, want 3183503801510301321", got)
	}
	if got := d481.ManifestGIDs["previous"]; got != 8382873932604653347 {
		t.Errorf("depot 481 previous manifest GID = %d, want 8382873932604653347", got)
	}

	d229006, ok := info.Depots[229006]
	if !ok {
		t.Fatal("depot 229006 missing")
	}
	if d229006.OSList != "windows" {
		t.Errorf("depot 229006 OSList = %q, want %q", d229006.OSList, "windows")
	}
}
