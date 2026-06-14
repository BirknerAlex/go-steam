package proto

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// These packet fixtures are the captured CM responses from SteamKit2's test
// suite (SteamKit2/Tests/Packets, exercised by PacketFacts.cs). Each file holds
// a raw, on-the-wire EMsgClientPICSProductInfoResponse: a protobuf message that
// our UnmarshalPacket (the equivalent of SteamKit2 CMClient.GetPacketMsg) must
// recognise, frame, and hand off to the body decoder.
//
// File name convention: <seq>_<direction>_<emsg>_k_<EMsgName>_<target>.bin
// PacketFacts derives the expected EMsg from the third underscore-delimited
// field, so we do the same here.
func TestUnmarshalPacket_SteamKit2_PICSProductInfo(t *testing.T) {
	tests := []struct {
		file      string
		wantApps  int
		wantAppID uint32 // 0 = don't check (e.g. package responses)
		wantBuf   bool   // expect a non-empty VDF buffer on the first app
	}{
		{
			file:      "001_in_8904_k_EMsgClientPICSProductInfoResponse_app480_metadata.bin",
			wantApps:  1,
			wantAppID: 480,
			wantBuf:   false, // metadata-only: sha present, buffer empty
		},
		{
			file:      "002_in_8904_k_EMsgClientPICSProductInfoResponse_app480.bin",
			wantApps:  1,
			wantAppID: 480,
			wantBuf:   true, // full app info: text-VDF buffer
		},
		{
			file:     "003_in_8904_k_EMsgClientPICSProductInfoResponse_sub0.bin",
			wantApps: 0, // package (sub) response — carries no app entries
		},
	}

	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tc.file))
			if err != nil {
				t.Fatal(err)
			}

			// Replicate PacketFacts.GetPacket: the EMsg is encoded in the file name.
			parts := strings.Split(strings.TrimSuffix(tc.file, ".bin"), "_")
			if len(parts) <= 3 {
				t.Fatalf("unexpected fixture name %q", tc.file)
			}
			if parts[1] != "in" {
				t.Skip("not an inbound packet")
			}
			emsgFromName, err := strconv.ParseUint(parts[2], 10, 32)
			if err != nil {
				t.Fatal(err)
			}

			// The raw message must carry the protobuf flag.
			rawMsg := EMsg(binary.LittleEndian.Uint32(data[:4]))
			if !rawMsg.IsProto() {
				t.Fatalf("raw message is not a protobuf packet (msg=%#x)", uint32(rawMsg))
			}

			pkt, err := UnmarshalPacket(data)
			if err != nil {
				t.Fatalf("UnmarshalPacket: %v", err)
			}
			if uint64(pkt.EMsg) != emsgFromName {
				t.Errorf("EMsg = %d, want %d (from file name)", pkt.EMsg, emsgFromName)
			}
			if pkt.EMsg != EMsgClientPICSProductInfoResponse {
				t.Errorf("EMsg = %d, want EMsgClientPICSProductInfoResponse (%d)",
					pkt.EMsg, EMsgClientPICSProductInfoResponse)
			}
			// The protobuf header must have deserialized (job source is set on real responses).
			if pkt.Header.JobidTarget == 0 {
				t.Errorf("header JobidTarget = 0, expected a non-zero target job id")
			}

			var resp CMsgClientPICSProductInfoResponse
			if err := resp.Unmarshal(pkt.Body); err != nil {
				t.Fatalf("body Unmarshal: %v", err)
			}
			if len(resp.Apps) != tc.wantApps {
				t.Fatalf("apps = %d, want %d", len(resp.Apps), tc.wantApps)
			}
			if tc.wantApps == 0 {
				return
			}

			app := resp.Apps[0]
			if tc.wantAppID != 0 && app.Appid != tc.wantAppID {
				t.Errorf("app[0].Appid = %d, want %d", app.Appid, tc.wantAppID)
			}
			if app.MissingToken {
				t.Errorf("app[0].MissingToken = true, want false")
			}
			if len(app.Sha) != 20 {
				t.Errorf("app[0].Sha length = %d, want 20", len(app.Sha))
			}
			if tc.wantBuf {
				if len(app.Buffer) == 0 {
					t.Errorf("app[0].Buffer is empty, expected app-info VDF")
				} else if !strings.HasPrefix(string(app.Buffer), "\"appinfo\"") {
					t.Errorf("app[0].Buffer does not start with the appinfo VDF header: %q",
						string(app.Buffer[:min(16, len(app.Buffer))]))
				}
			} else if len(app.Buffer) != 0 {
				t.Errorf("app[0].Buffer = %d bytes, want empty (metadata-only response)", len(app.Buffer))
			}
		})
	}
}
