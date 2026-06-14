package cm

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/BirknerAlex/go-steam/internal/proto"
)

type handlerEntry struct {
	fn func(*proto.Packet)
}

// Dispatcher multiplexes outgoing requests and incoming responses.
//
// Request-response pairs (PICS, depot key, CDN tokens) use Send+Await with job
// ID matching.  Messages that Steam delivers as unsolicited notifications (e.g.
// CMsgClientLogOnResponse) must be handled via RegisterHandler instead.
type Dispatcher struct {
	session *Session

	mu       sync.Mutex
	pending  map[uint64]chan *proto.Packet
	handlers map[proto.EMsg][]*handlerEntry
	connLost chan struct{} // closed by cancelAll on connection drop; replaced each time
}

func newDispatcher(s *Session) *Dispatcher {
	return &Dispatcher{
		session:  s,
		pending:  make(map[uint64]chan *proto.Packet),
		handlers: make(map[proto.EMsg][]*handlerEntry),
		connLost: make(chan struct{}),
	}
}

// RegisterHandler registers fn to receive all packets with the given EMsg.
// The returned function removes the handler; call it (e.g. via defer) when done.
func (d *Dispatcher) RegisterHandler(msg proto.EMsg, fn func(*proto.Packet)) func() {
	e := &handlerEntry{fn: fn}
	d.mu.Lock()
	d.handlers[msg] = append(d.handlers[msg], e)
	d.mu.Unlock()
	return func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		entries := d.handlers[msg]
		for i, h := range entries {
			if h == e {
				d.handlers[msg] = append(entries[:i], entries[i+1:]...)
				return
			}
		}
	}
}

// ConnLost returns a channel that is closed when the connection drops.
// A fresh channel is installed on each cancelAll call.
func (d *Dispatcher) ConnLost() <-chan struct{} {
	d.mu.Lock()
	ch := d.connLost
	d.mu.Unlock()
	return ch
}

// cancelAll unblocks all Await callers and fires the connLost signal.
// Called by readLoop when the TCP connection drops.
func (d *Dispatcher) cancelAll() {
	d.mu.Lock()
	oldConnLost := d.connLost
	d.connLost = make(chan struct{})
	pending := d.pending
	d.pending = make(map[uint64]chan *proto.Packet)
	d.mu.Unlock()

	close(oldConnLost)
	for _, ch := range pending {
		close(ch)
	}
}

// Send encodes msg+body as a proto packet, assigns a random job ID, registers a
// reply channel, and sends it.  Use Await(jobID) to block for the response.
func (d *Dispatcher) Send(ctx context.Context, msg proto.EMsg, body []byte) (uint64, error) {
	jobID := randomJobID()

	hdr := proto.CMsgProtoBufHeader{JobidSource: jobID}
	d.session.mu.RLock()
	hdr.Steamid = d.session.steamID
	hdr.ClientSessionid = d.session.sessionID
	d.session.mu.RUnlock()

	// Register reply channel before sending to avoid a race where the response
	// arrives before we can register.
	ch := make(chan *proto.Packet, 1)
	d.mu.Lock()
	d.pending[jobID] = ch
	d.mu.Unlock()

	payload := proto.MarshalPacket(msg, hdr, body)
	if err := d.session.sendEncrypted(payload); err != nil {
		d.mu.Lock()
		delete(d.pending, jobID)
		d.mu.Unlock()
		return 0, fmt.Errorf("dispatch: send: %w", err)
	}
	return jobID, nil
}

// SendServiceMethod sends a Steam Unified Messages service method call.
// It sets the proto header's target_job_name to methodName and uses
// EMsgServiceMethodCallFromClient (or the non-authed variant if no SteamID).
// Returns a job ID for use with Await.
func (d *Dispatcher) SendServiceMethod(ctx context.Context, methodName string, body []byte) (uint64, error) {
	jobID := randomJobID()

	hdr := proto.CMsgProtoBufHeader{
		JobidSource:   jobID,
		TargetJobName: methodName,
	}
	d.session.mu.RLock()
	hdr.Steamid = d.session.steamID
	hdr.ClientSessionid = d.session.sessionID
	d.session.mu.RUnlock()

	eMsg := proto.EMsgServiceMethodCallFromClient
	if hdr.Steamid == 0 {
		eMsg = proto.EMsgServiceMethodCallFromClientNonAuthed
	}

	ch := make(chan *proto.Packet, 1)
	d.mu.Lock()
	d.pending[jobID] = ch
	d.mu.Unlock()

	payload := proto.MarshalPacket(eMsg, hdr, body)
	if err := d.session.sendEncrypted(payload); err != nil {
		d.mu.Lock()
		delete(d.pending, jobID)
		d.mu.Unlock()
		return 0, fmt.Errorf("dispatch: send service method: %w", err)
	}
	return jobID, nil
}

// SendNoReply sends msg+body without registering a pending reply channel.
// Use for messages whose response arrives as an unsolicited EMsg notification
// (e.g. CMsgClientLogon → CMsgClientLogOnResponse).
func (d *Dispatcher) SendNoReply(msg proto.EMsg, body []byte) error {
	hdr := proto.CMsgProtoBufHeader{JobidSource: randomJobID()}
	d.session.mu.RLock()
	hdr.Steamid = d.session.steamID
	hdr.ClientSessionid = d.session.sessionID
	d.session.mu.RUnlock()

	payload := proto.MarshalPacket(msg, hdr, body)
	if err := d.session.sendEncrypted(payload); err != nil {
		return fmt.Errorf("dispatch: send: %w", err)
	}
	return nil
}

// Await blocks until the response with the given job ID arrives, ctx expires,
// or the connection drops.
func (d *Dispatcher) Await(ctx context.Context, jobID uint64) (*proto.Packet, error) {
	d.mu.Lock()
	ch, ok := d.pending[jobID]
	connLost := d.connLost
	d.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("dispatch: unknown job ID %d", jobID)
	}
	defer func() {
		d.mu.Lock()
		delete(d.pending, jobID)
		d.mu.Unlock()
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-connLost:
		return nil, fmt.Errorf("dispatch: connection lost")
	case pkt, ok := <-ch:
		if !ok || pkt == nil {
			return nil, fmt.Errorf("dispatch: connection closed")
		}
		return pkt, nil
	}
}

// deliver routes an incoming packet to the appropriate pending channel or
// registered EMsg handlers.
func (d *Dispatcher) deliver(pkt *proto.Packet) {
	// jobid_target 0 and 0xFFFFFFFFFFFFFFFF both mean "no specific target".
	target := pkt.Header.JobidTarget
	if target != 0 && target != 0xFFFFFFFFFFFFFFFF {
		d.mu.Lock()
		ch, ok := d.pending[target]
		d.mu.Unlock()
		if ok {
			select {
			case ch <- pkt:
			default:
			}
			return
		}
	}

	// No pending job matched — dispatch to EMsg handlers.
	d.mu.Lock()
	entries := d.handlers[pkt.EMsg]
	fns := make([]func(*proto.Packet), 0, len(entries))
	for _, e := range entries {
		fns = append(fns, e.fn)
	}
	d.mu.Unlock()
	for _, fn := range fns {
		fn(pkt)
	}
}

func randomJobID() uint64 {
	var b [8]byte
	rand.Read(b[:]) //nolint:errcheck // crypto/rand never fails on POSIX
	return binary.LittleEndian.Uint64(b[:])
}
