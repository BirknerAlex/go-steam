package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LocalCache is a file-backed cache for Steam sessions, depot keys, and CDN
// auth tokens.  All methods are safe for concurrent use.
type LocalCache struct {
	dir  string
	mu   sync.RWMutex
}

// NewLocalCache opens (or creates) the cache directory at dir.
func NewLocalCache(dir string) (*LocalCache, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("store: create cache dir: %w", err)
	}
	return &LocalCache{dir: dir}, nil
}

func (c *LocalCache) path(name string) string {
	return filepath.Join(c.dir, name+".json")
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// sessionPath returns the path for a per-account session file.
// username is used as-is; Steam usernames are safe filenames on all platforms.
func (c *LocalCache) sessionPath(username string) string {
	return filepath.Join(c.dir, "session_"+username+".json")
}

// LoadSession returns the cached session for username, or nil if none exists or
// it has expired.
func (c *LocalCache) LoadSession(username string) (*CachedSession, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var s CachedSession
	if err := readJSON(c.sessionPath(username), &s); err != nil {
		return nil, fmt.Errorf("store: load session: %w", err)
	}
	if s.AccessToken == "" {
		return nil, nil
	}
	if !s.Expiry.IsZero() && time.Now().After(s.Expiry) {
		return nil, nil // expired
	}
	return &s, nil
}

// SaveSession persists the session for username to disk.
func (c *LocalCache) SaveSession(username string, s *CachedSession) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return writeJSON(c.sessionPath(username), s)
}

// ClearSession removes the persisted session for username (forces re-login).
func (c *LocalCache) ClearSession(username string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := os.Remove(c.sessionPath(username))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

type depotKeyStore map[uint32]CachedDepotKey

// LoadDepotKey returns the cached key for depotID, or nil if not cached.
func (c *LocalCache) LoadDepotKey(depotID uint32) (*CachedDepotKey, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var store depotKeyStore
	if err := readJSON(c.path("depotkeys"), &store); err != nil {
		return nil, fmt.Errorf("store: load depot keys: %w", err)
	}
	if store == nil {
		return nil, nil
	}
	k, ok := store[depotID]
	if !ok {
		return nil, nil
	}
	return &k, nil
}

// SaveDepotKey persists a depot key.  Depot keys never expire so no TTL is stored.
func (c *LocalCache) SaveDepotKey(k CachedDepotKey) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var store depotKeyStore
	if err := readJSON(c.path("depotkeys"), &store); err != nil {
		return fmt.Errorf("store: load depot keys for write: %w", err)
	}
	if store == nil {
		store = make(depotKeyStore)
	}
	store[k.DepotID] = k
	return writeJSON(c.path("depotkeys"), store)
}

type tokenStore []CachedToken

// LoadToken returns the cached token for the (host, depotID) pair, or nil if
// not cached or expired (within a 5-minute window).
func (c *LocalCache) LoadToken(host string, depotID uint32) (*CachedToken, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var store tokenStore
	if err := readJSON(c.path("tokens"), &store); err != nil {
		return nil, fmt.Errorf("store: load tokens: %w", err)
	}
	now := time.Now()
	for _, t := range store {
		if t.Host == host && t.DepotID == depotID {
			if t.Expiry.After(now.Add(5 * time.Minute)) {
				cp := t
				return &cp, nil
			}
		}
	}
	return nil, nil
}

// SaveToken persists a CDN auth token.
func (c *LocalCache) SaveToken(t CachedToken) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var store tokenStore
	if err := readJSON(c.path("tokens"), &store); err != nil {
		return fmt.Errorf("store: load tokens for write: %w", err)
	}
	// Replace existing entry for same host+depot.
	for i, s := range store {
		if s.Host == t.Host && s.DepotID == t.DepotID {
			store[i] = t
			return writeJSON(c.path("tokens"), store)
		}
	}
	store = append(store, t)
	return writeJSON(c.path("tokens"), store)
}

// PurgeExpiredTokens removes expired CDN tokens from disk.
func (c *LocalCache) PurgeExpiredTokens() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var store tokenStore
	if err := readJSON(c.path("tokens"), &store); err != nil {
		return fmt.Errorf("store: load tokens for purge: %w", err)
	}
	now := time.Now()
	live := store[:0]
	for _, t := range store {
		if t.Expiry.After(now) {
			live = append(live, t)
		}
	}
	return writeJSON(c.path("tokens"), live)
}
