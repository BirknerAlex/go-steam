package cm

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/BirknerAlex/go-steam/internal/proto"
)

// fakeConn is a no-op cmConn that records written payloads.
type fakeConn struct {
	mu      sync.Mutex
	written [][]byte
}

func (f *fakeConn) ReadPacket() ([]byte, error) { select {} } // never returns
func (f *fakeConn) WritePacket(p []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.written = append(f.written, append([]byte(nil), p...))
	return nil
}
func (f *fakeConn) NeedsEncryption() bool { return false }
func (f *fakeConn) Close() error          { return nil }
func (f *fakeConn) RemoteAddr() string    { return "fake" }
func (f *fakeConn) LocalAddr() string     { return "fake" }

func newTestSession() (*Session, *fakeConn) {
	s := &Session{
		log:           slog.Default(),
		state:         StateReady,
		heartbeatStop: make(chan struct{}),
		closing:       make(chan struct{}),
	}
	s.dispatch = newDispatcher(s)
	fc := &fakeConn{}
	s.conn = fc
	s.useEncryption = false
	return s, fc
}

func TestDispatchSendAwait(t *testing.T) {
	s, fc := newTestSession()
	ctx := context.Background()

	jobID, err := s.dispatch.Send(ctx, proto.EMsgClientPICSProductInfoRequest, []byte{0x01})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(fc.written) != 1 {
		t.Fatalf("expected 1 written packet, got %d", len(fc.written))
	}

	// Simulate the response arriving with JobidTarget == jobID.
	reply := &proto.Packet{
		EMsg:   proto.EMsgClientPICSProductInfoResponse,
		Header: proto.CMsgProtoBufHeader{JobidTarget: jobID},
		Body:   []byte{0x02},
	}
	go s.dispatch.deliver(reply)

	got, err := s.dispatch.Await(ctx, jobID)
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	if got.Header.JobidTarget != jobID {
		t.Errorf("got wrong packet: %+v", got.Header)
	}
}

func TestDispatchAwaitUnknownJob(t *testing.T) {
	s, _ := newTestSession()
	if _, err := s.dispatch.Await(context.Background(), 12345); err == nil {
		t.Error("Await on unknown job ID should error")
	}
}

func TestDispatchAwaitContextCancel(t *testing.T) {
	s, _ := newTestSession()
	jobID, _ := s.dispatch.Send(context.Background(), proto.EMsgClientHeartBeat, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.dispatch.Await(ctx, jobID); err == nil {
		t.Error("Await should return cancellation error")
	}
}

func TestDispatchCancelAllUnblocksAwait(t *testing.T) {
	s, _ := newTestSession()
	jobID, _ := s.dispatch.Send(context.Background(), proto.EMsgClientHeartBeat, nil)

	done := make(chan error, 1)
	go func() {
		_, err := s.dispatch.Await(context.Background(), jobID)
		done <- err
	}()
	// Give the goroutine a moment to start awaiting, then drop the connection.
	time.Sleep(20 * time.Millisecond)
	s.dispatch.cancelAll()

	select {
	case err := <-done:
		if err == nil {
			t.Error("Await should error after cancelAll")
		}
	case <-time.After(time.Second):
		t.Fatal("Await did not unblock after cancelAll")
	}
}

func TestDispatchConnLostReplacedAfterCancelAll(t *testing.T) {
	s, _ := newTestSession()
	first := s.dispatch.ConnLost()
	s.dispatch.cancelAll()
	select {
	case <-first:
		// expected: old channel closed
	default:
		t.Error("old ConnLost channel should be closed after cancelAll")
	}
	second := s.dispatch.ConnLost()
	select {
	case <-second:
		t.Error("new ConnLost channel should be open")
	default:
	}
}

func TestDispatchHandlers(t *testing.T) {
	s, _ := newTestSession()
	var mu sync.Mutex
	calls := 0
	remove := s.dispatch.RegisterHandler(proto.EMsgClientLogOnResponse, func(*proto.Packet) {
		mu.Lock()
		calls++
		mu.Unlock()
	})

	// An unsolicited packet (no matching pending job) routes to the handler.
	s.dispatch.deliver(&proto.Packet{EMsg: proto.EMsgClientLogOnResponse})
	mu.Lock()
	if calls != 1 {
		t.Errorf("handler call count = %d, want 1", calls)
	}
	mu.Unlock()

	// After removal the handler is no longer invoked.
	remove()
	s.dispatch.deliver(&proto.Packet{EMsg: proto.EMsgClientLogOnResponse})
	mu.Lock()
	if calls != 1 {
		t.Errorf("handler should not fire after removal, count = %d", calls)
	}
	mu.Unlock()
}

func TestDispatchServiceMethodSetsJobName(t *testing.T) {
	s, fc := newTestSession()
	jobID, err := s.dispatch.SendServiceMethod(context.Background(), "Foo.Bar#1", []byte{0x09})
	if err != nil {
		t.Fatalf("SendServiceMethod: %v", err)
	}
	if jobID == 0 {
		t.Error("expected non-zero job ID")
	}
	// Decode the written packet and confirm the target job name is set.
	pkt, err := proto.UnmarshalPacket(fc.written[0])
	if err != nil {
		t.Fatalf("decode written packet: %v", err)
	}
	if pkt.Header.TargetJobName != "Foo.Bar#1" {
		t.Errorf("TargetJobName = %q, want Foo.Bar#1", pkt.Header.TargetJobName)
	}
	// Anonymous (steamID 0) uses the non-authed service method EMsg.
	if pkt.EMsg != proto.EMsgServiceMethodCallFromClientNonAuthed {
		t.Errorf("EMsg = %d, want non-authed service method %d", pkt.EMsg, proto.EMsgServiceMethodCallFromClientNonAuthed)
	}
}
