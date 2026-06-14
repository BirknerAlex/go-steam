package cdn

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/BirknerAlex/go-steam/internal/proto"
	"github.com/BirknerAlex/go-steam/internal/store"
)

// fakeCMSession implements the CMSession interface for token tests.
type fakeCMSession struct {
	token    string
	expiry   uint32
	eresult  int32
	calls    atomic.Int64
	awaitErr error
}

func (f *fakeCMSession) Send(ctx context.Context, msg proto.EMsg, body []byte) (uint64, error) {
	return 1, nil
}

func (f *fakeCMSession) SendServiceMethod(ctx context.Context, method string, body []byte) (uint64, error) {
	f.calls.Add(1)
	return 1, nil
}

func (f *fakeCMSession) Await(ctx context.Context, jobID uint64) (*proto.Packet, error) {
	if f.awaitErr != nil {
		return nil, f.awaitErr
	}
	var b []byte
	b = protowire.AppendTag(b, 1, protowire.BytesType)
	b = protowire.AppendString(b, f.token)
	b = protowire.AppendTag(b, 2, protowire.VarintType)
	b = protowire.AppendVarint(b, uint64(f.expiry))
	return &proto.Packet{
		Header: proto.CMsgProtoBufHeader{Eresult: f.eresult},
		Body:   b,
	}, nil
}

func newCache(t *testing.T) *store.LocalCache {
	t.Helper()
	c, err := store.NewLocalCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTokenProviderFetchAndCache(t *testing.T) {
	cache := newCache(t)
	fake := &fakeCMSession{
		token:   "tok-123",
		expiry:  uint32(time.Now().Add(time.Hour).Unix()),
		eresult: int32(proto.EResultOK),
	}
	tp := NewTokenProvider(cache, fake, 740)

	got, err := tp.GetToken(context.Background(), 1006, "cdn.example")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if got != "tok-123" {
		t.Errorf("token = %q, want tok-123", got)
	}
	if fake.calls.Load() != 1 {
		t.Errorf("CM calls = %d, want 1", fake.calls.Load())
	}

	// Second call is served from disk cache — no new CM request.
	got, err = tp.GetToken(context.Background(), 1006, "cdn.example")
	if err != nil || got != "tok-123" {
		t.Fatalf("cached GetToken: %q %v", got, err)
	}
	if fake.calls.Load() != 1 {
		t.Errorf("CM calls after cache hit = %d, want 1", fake.calls.Load())
	}
}

func TestTokenProviderNegativeCache(t *testing.T) {
	cache := newCache(t)
	// EResult != OK → fetch fails; provider treats it as "no token" and negatively caches.
	fake := &fakeCMSession{eresult: int32(proto.EResultAccessDenied)}
	tp := NewTokenProvider(cache, fake, 740)

	tok, err := tp.GetToken(context.Background(), 1, "h")
	if err != nil || tok != "" {
		t.Fatalf("expected empty token, no error; got %q %v", tok, err)
	}
	if fake.calls.Load() != 1 {
		t.Errorf("CM calls = %d, want 1", fake.calls.Load())
	}

	// Within the negative-cache window, no new CM request is made.
	if _, _ = tp.GetToken(context.Background(), 1, "h"); fake.calls.Load() != 1 {
		t.Errorf("negative cache should suppress 2nd call, calls = %d", fake.calls.Load())
	}

	// After invalidation the provider retries.
	tp.InvalidateToken(1, "h")
	if _, _ = tp.GetToken(context.Background(), 1, "h"); fake.calls.Load() != 2 {
		t.Errorf("after InvalidateToken expected a retry, calls = %d", fake.calls.Load())
	}
}

func TestTokenProviderSingleflight(t *testing.T) {
	cache := newCache(t)
	// Block Await until released so concurrent callers coalesce on one request.
	release := make(chan struct{})
	fake := &blockingCM{release: release, token: "shared", expiry: uint32(time.Now().Add(time.Hour).Unix())}
	tp := NewTokenProvider(cache, fake, 1)

	const n = 8
	var wg sync.WaitGroup
	results := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], _ = tp.GetToken(context.Background(), 5, "host")
		}(i)
	}
	time.Sleep(30 * time.Millisecond) // let all goroutines reach the in-flight wait
	close(release)
	wg.Wait()

	for i, r := range results {
		if r != "shared" {
			t.Errorf("result[%d] = %q, want shared", i, r)
		}
	}
	if fake.sends.Load() != 1 {
		t.Errorf("singleflight should collapse to 1 CM request, got %d", fake.sends.Load())
	}
}

type blockingCM struct {
	release <-chan struct{}
	token   string
	expiry  uint32
	sends   atomic.Int64
}

func (b *blockingCM) Send(ctx context.Context, msg proto.EMsg, body []byte) (uint64, error) {
	return 1, nil
}
func (b *blockingCM) SendServiceMethod(ctx context.Context, method string, body []byte) (uint64, error) {
	b.sends.Add(1)
	return 1, nil
}
func (b *blockingCM) Await(ctx context.Context, jobID uint64) (*proto.Packet, error) {
	<-b.release
	var buf []byte
	buf = protowire.AppendTag(buf, 1, protowire.BytesType)
	buf = protowire.AppendString(buf, b.token)
	buf = protowire.AppendTag(buf, 2, protowire.VarintType)
	buf = protowire.AppendVarint(buf, uint64(b.expiry))
	return &proto.Packet{Header: proto.CMsgProtoBufHeader{Eresult: int32(proto.EResultOK)}, Body: buf}, nil
}
