package cm

import (
	"context"
	"fmt"
	"sync"

	"github.com/BirknerAlex/go-steam/internal/proto"
	"github.com/BirknerAlex/go-steam/internal/store"
)

// DepotKeyProvider fetches and caches depot AES-256 decryption keys.
// A singleflight pattern ensures that concurrent requests for the same depot
// collapse to one CM request.
type DepotKeyProvider struct {
	cache   *store.LocalCache
	session *Session

	mu      sync.Mutex
	inflight map[uint32]*depotKeyFlight
}

type depotKeyFlight struct {
	done chan struct{}
	key  []byte
	err  error
}

// NewDepotKeyProvider creates a provider backed by the given cache and session.
func NewDepotKeyProvider(cache *store.LocalCache, session *Session) *DepotKeyProvider {
	return &DepotKeyProvider{
		cache:    cache,
		session:  session,
		inflight: make(map[uint32]*depotKeyFlight),
	}
}

// GetDepotKey returns the AES-256 key for the given depot.
// Results are cached permanently on disk; concurrent calls for the same depot
// share a single inflight request.
func (p *DepotKeyProvider) GetDepotKey(ctx context.Context, appID, depotID uint32) ([]byte, error) {
	// 1. Check disk cache.
	if p.cache != nil {
		cached, err := p.cache.LoadDepotKey(depotID)
		if err == nil && cached != nil {
			return cached.Key, nil
		}
	}

	// 2. Singleflight: only one CM request per depot at a time.
	p.mu.Lock()
	if f, ok := p.inflight[depotID]; ok {
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-f.done:
			return f.key, f.err
		}
	}
	f := &depotKeyFlight{done: make(chan struct{})}
	p.inflight[depotID] = f
	p.mu.Unlock()

	// Fetch from CM.
	key, err := p.fetchDepotKey(ctx, appID, depotID)
	f.key, f.err = key, err
	close(f.done)

	p.mu.Lock()
	delete(p.inflight, depotID)
	p.mu.Unlock()

	if err != nil {
		return nil, err
	}

	// Persist to disk.
	if p.cache != nil {
		_ = p.cache.SaveDepotKey(store.CachedDepotKey{DepotID: depotID, Key: key})
	}
	return key, nil
}

func (p *DepotKeyProvider) fetchDepotKey(ctx context.Context, appID, depotID uint32) ([]byte, error) {
	req := proto.CMsgClientGetDepotDecryptionKey{
		DepotID: depotID,
		AppID:   appID,
	}
	body := req.Marshal()

	jobID, err := p.session.dispatch.Send(ctx, proto.EMsgClientGetDepotDecryptionKey, body)
	if err != nil {
		return nil, fmt.Errorf("depotkey: send: %w", err)
	}
	pkt, err := p.session.dispatch.Await(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("depotkey: await: %w", err)
	}
	var resp proto.CMsgClientGetDepotDecryptionKeyResponse
	if err := resp.Unmarshal(pkt.Body); err != nil {
		return nil, fmt.Errorf("depotkey: unmarshal: %w", err)
	}
	if proto.EResult(resp.Eresult) != proto.EResultOK {
		if proto.EResult(resp.Eresult) == proto.EResultAccessDenied {
			return nil, fmt.Errorf("depotkey: %w (depot %d)", ErrDepotKeyDenied, depotID)
		}
		return nil, fmt.Errorf("depotkey: EResult %d for depot %d", resp.Eresult, depotID)
	}
	if len(resp.DepotEncryptionKey) == 0 {
		return nil, fmt.Errorf("depotkey: empty key for depot %d", depotID)
	}
	return resp.DepotEncryptionKey, nil
}

// ErrDepotKeyDenied is returned when Steam denies the depot key request
// (the depot is not accessible with the current session credentials).
var ErrDepotKeyDenied = fmt.Errorf("depot key access denied")
