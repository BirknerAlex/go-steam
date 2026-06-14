package workshop

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withURL(t *testing.T, u string) {
	t.Helper()
	orig := publishedFileDetailsURL
	publishedFileDetailsURL = u
	t.Cleanup(func() { publishedFileDetailsURL = orig })
}

func TestGetItemInfo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Write([]byte(`{"response":{"result":1,"resultcount":1,"publishedfiledetails":[
			{"result":1,"publishedfileid":"3625223587","consumer_app_id":294100,
			 "hcontent_file":"123456789","title":"My Mod","file_size":"2048","time_updated":1700000000}
		]}}`)) //nolint:errcheck
	}))
	defer srv.Close()
	withURL(t, srv.URL)

	item, err := GetItemInfo(context.Background(), 3625223587)
	if err != nil {
		t.Fatalf("GetItemInfo: %v", err)
	}
	if item.ItemID != 3625223587 {
		t.Errorf("ItemID = %d", item.ItemID)
	}
	if item.AppID != 294100 {
		t.Errorf("AppID = %d, want 294100", item.AppID)
	}
	if item.ManifestGID != 123456789 {
		t.Errorf("ManifestGID = %d", item.ManifestGID)
	}
	if item.DepotID != 294100 {
		t.Errorf("DepotID = %d, want appid by convention", item.DepotID)
	}
	if item.Title != "My Mod" || item.FileSize != 2048 {
		t.Errorf("title/size = %q/%d", item.Title, item.FileSize)
	}
}

func TestGetItemInfo_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"response":{"result":1,"resultcount":1,"publishedfiledetails":[{"result":9}]}}`)) //nolint:errcheck
	}))
	defer srv.Close()
	withURL(t, srv.URL)

	if _, err := GetItemInfo(context.Background(), 1); err == nil {
		t.Error("expected error for item with result != 1")
	}
}

func TestGetItemInfo_EmptyDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"response":{"result":1,"resultcount":0,"publishedfiledetails":[]}}`)) //nolint:errcheck
	}))
	defer srv.Close()
	withURL(t, srv.URL)

	if _, err := GetItemInfo(context.Background(), 1); err == nil {
		t.Error("expected error when no details returned")
	}
}
