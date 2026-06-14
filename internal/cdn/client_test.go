package cdn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetServerPicksLowestPenalty(t *testing.T) {
	c := &Client{servers: []*Server{
		{Host: "a"},
		{Host: "b"},
		{Host: "c"},
	}}
	// Penalise a and c; b should win.
	c.Penalise(c.servers[0])
	c.Penalise(c.servers[0])
	c.Penalise(c.servers[2])
	best, err := c.GetServer()
	if err != nil {
		t.Fatal(err)
	}
	if best.Host != "b" {
		t.Errorf("GetServer = %q, want b (lowest penalty)", best.Host)
	}
}

func TestGetServerEmpty(t *testing.T) {
	c := &Client{}
	if _, err := c.GetServer(); err == nil {
		t.Error("expected error when no servers available")
	}
}

func TestDefaultServers(t *testing.T) {
	s := defaultServers()
	if len(s) == 0 {
		t.Fatal("defaultServers should be non-empty")
	}
	for _, srv := range s {
		if srv.Host == "" {
			t.Error("default server has empty host")
		}
	}
}

func TestFetchServerListFallback(t *testing.T) {
	// When the directory API is unreachable, NewClient falls back to defaults.
	c, err := NewClient(context.Background(), 0)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if len(c.servers) == 0 {
		t.Error("expected non-empty server list (real or fallback)")
	}
	if c.HTTP() == nil {
		t.Error("HTTP client should be set")
	}
}

func TestFetchServerListParsing(t *testing.T) {
	// Drive fetchServerList against a local server returning a known JSON body.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"response":{"servers":[
			{"type":"SteamCache","host":"cache1.example","vhost":"vh1"},
			{"type":"CDN","host":"","vhost":"vh2"},
			{"type":"CDN","host":"","vhost":""}
		]}}`)) //nolint:errcheck
	}))
	defer srv.Close()

	orig := contentServerDirectoryURL
	contentServerDirectoryURL = srv.URL + "/?cell_id=%d"
	defer func() { contentServerDirectoryURL = orig }()

	c := &Client{http: srv.Client()}
	if err := c.fetchServerList(context.Background(), 0); err != nil {
		t.Fatalf("fetchServerList: %v", err)
	}
	// Three entries: host set, vhost fallback, and one with neither (dropped).
	if len(c.servers) != 2 {
		t.Fatalf("expected 2 usable servers, got %d", len(c.servers))
	}
	if c.servers[0].Host != "cache1.example" {
		t.Errorf("server[0].Host = %q, want cache1.example", c.servers[0].Host)
	}
	if c.servers[1].Host != "vh2" {
		t.Errorf("server[1].Host = %q, want vh2 (vhost fallback)", c.servers[1].Host)
	}
}
