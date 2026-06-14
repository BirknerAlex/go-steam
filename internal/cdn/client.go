// Package cdn implements the Steam Content Delivery Network (CDN) client for
// downloading depot manifests and content chunks.
package cdn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Server represents one CDN server with a penalty counter for server selection.
type Server struct {
	Host    string
	Type    string // "SteamCache", "CDN", etc.
	Penalty atomic.Int64
}

// Client manages a pool of CDN servers and selects the best available server
// for each request.
type Client struct {
	http    *http.Client
	mu      sync.RWMutex
	servers []*Server
}

// NewClient creates a CDN client and fetches the server list from Steam.
func NewClient(ctx context.Context, cellID uint32) (*Client, error) {
	c := &Client{
		http: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 32,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
	if err := c.fetchServerList(ctx, cellID); err != nil {
		// Fall back to well-known CDN endpoints if the directory API is down.
		c.servers = defaultServers()
	}
	return c, nil
}

// GetServer returns the best available CDN server.  Servers with lower penalty
// scores are preferred; among equals the first in the list wins.
func (c *Client) GetServer() (*Server, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.servers) == 0 {
		return nil, fmt.Errorf("cdn: no servers available")
	}
	best := c.servers[0]
	for _, s := range c.servers[1:] {
		if s.Penalty.Load() < best.Penalty.Load() {
			best = s
		}
	}
	return best, nil
}

// Penalise increments the failure count for a server so it is deprioritised.
func (c *Client) Penalise(s *Server) {
	s.Penalty.Add(1)
}

// HTTP returns the shared http.Client for use by manifest and chunk downloaders.
func (c *Client) HTTP() *http.Client { return c.http }

// ---- server list ------------------------------------------------------------

type cdnServerListResponse struct {
	Response struct {
		Servers []struct {
			Type             string `json:"type"`
			SourceID         int    `json:"source_id"`
			CellID           int    `json:"cell_id"`
			Load             int    `json:"load"`
			WeightedLoad     float64 `json:"weighted_load"`
			NumEntriesInClientList float64 `json:"num_entries_in_client_list"`
			Host             string `json:"host"`
			VHost            string `json:"vhost"`
			HTTPS_Support    string `json:"https_support"`
		} `json:"servers"`
	} `json:"response"`
}

// contentServerDirectoryURL is the directory API endpoint used to discover CDN
// servers.  It is a package variable so tests can point it at a local server.
var contentServerDirectoryURL = "https://api.steampowered.com/IContentServerDirectoryService/GetServersForSteamPipe/v1/?cell_id=%d"

func (c *Client) fetchServerList(ctx context.Context, cellID uint32) error {
	url := fmt.Sprintf(contentServerDirectoryURL, cellID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result cdnServerListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range result.Response.Servers {
		host := s.Host
		if host == "" {
			host = s.VHost
		}
		if host != "" {
			c.servers = append(c.servers, &Server{Host: host, Type: s.Type})
		}
	}
	if len(c.servers) == 0 {
		return fmt.Errorf("cdn: empty server list")
	}
	return nil
}

func defaultServers() []*Server {
	return []*Server{
		{Host: "steamcontent.com"},
		{Host: "cdn.akamai.steamstatic.com"},
		{Host: "cdn.cloudflare.steamstatic.com"},
	}
}
