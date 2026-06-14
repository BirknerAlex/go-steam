// Package mock provides in-memory test doubles for go-steam internals.
package mock

import (
	"sync"
	"time"

	"github.com/BirknerAlex/go-steam/internal/store"
)

// Store is an in-memory implementation of the cache interfaces — no disk I/O.
type Store struct {
	mu       sync.RWMutex
	session  *store.CachedSession
	depotKeys map[uint32]store.CachedDepotKey
	tokens   []store.CachedToken
}

// NewStore returns a fresh in-memory store.
func NewStore() *Store {
	return &Store{depotKeys: make(map[uint32]store.CachedDepotKey)}
}

// LoadSession returns the stored session if one exists and isn't expired.
func (s *Store) LoadSession() (*store.CachedSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.session == nil {
		return nil, nil
	}
	if !s.session.Expiry.IsZero() && time.Now().After(s.session.Expiry) {
		return nil, nil
	}
	cp := *s.session
	return &cp, nil
}

// SaveSession stores a session.
func (s *Store) SaveSession(sess *store.CachedSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *sess
	s.session = &cp
	return nil
}

// ClearSession removes any stored session.
func (s *Store) ClearSession() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = nil
	return nil
}

// LoadDepotKey returns the cached key for depotID, or nil.
func (s *Store) LoadDepotKey(depotID uint32) (*store.CachedDepotKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, ok := s.depotKeys[depotID]
	if !ok {
		return nil, nil
	}
	return &k, nil
}

// SaveDepotKey stores a depot key.
func (s *Store) SaveDepotKey(k store.CachedDepotKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.depotKeys[k.DepotID] = k
	return nil
}

// LoadToken returns a cached CDN token if it's not expired.
func (s *Store) LoadToken(host string, depotID uint32) (*store.CachedToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	for _, t := range s.tokens {
		if t.Host == host && t.DepotID == depotID && t.Expiry.After(now.Add(5*time.Minute)) {
			cp := t
			return &cp, nil
		}
	}
	return nil, nil
}

// SaveToken stores a CDN auth token.
func (s *Store) SaveToken(t store.CachedToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.tokens {
		if existing.Host == t.Host && existing.DepotID == t.DepotID {
			s.tokens[i] = t
			return nil
		}
	}
	s.tokens = append(s.tokens, t)
	return nil
}

// SetDepotKey is a test helper for pre-seeding a depot key.
func (s *Store) SetDepotKey(depotID uint32, key []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.depotKeys[depotID] = store.CachedDepotKey{DepotID: depotID, Key: key}
}

// SetToken is a test helper for pre-seeding a CDN token.
func (s *Store) SetToken(host string, depotID uint32, token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens = append(s.tokens, store.CachedToken{
		Host:    host,
		DepotID: depotID,
		Token:   token,
		Expiry:  time.Now().Add(24 * time.Hour),
	})
}
