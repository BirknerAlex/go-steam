package cm

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"github.com/coder/websocket"
)

// cmConn abstracts a Steam CM transport connection.
//
// TCP:       uses AES channel encryption on top of plaintext TCP.
//            Framing: [4-byte length][4-byte magic "VT01"][payload]
//
// WebSocket: TLS provides transport encryption so no channel encryption is
//            needed.  Framing: [4-byte magic "VT01"][payload] per WS frame.
type cmConn interface {
	// ReadPacket reads one framed Steam packet and returns the raw payload
	// (everything after the magic marker).
	ReadPacket() ([]byte, error)

	// WritePacket wraps payload in the appropriate framing and sends it.
	WritePacket(payload []byte) error

	// NeedsEncryption reports whether the transport requires the Steam AES
	// channel encryption handshake.  TCP connections do; WebSocket does not
	// because TLS handles the transport encryption.
	NeedsEncryption() bool

	// Close tears down the connection.
	Close() error

	// RemoteAddr returns a string representation of the remote endpoint.
	RemoteAddr() string

	// LocalAddr returns a string representation of the local endpoint.
	LocalAddr() string
}

// ---- TCP transport ----------------------------------------------------------

type tcpTransport struct{ conn net.Conn }

func newTCPConn(conn net.Conn) cmConn { return &tcpTransport{conn} }

func (t *tcpTransport) ReadPacket() ([]byte, error) {
	hdr := make([]byte, 8)
	if _, err := io.ReadFull(t.conn, hdr); err != nil {
		return nil, err
	}
	length := binary.LittleEndian.Uint32(hdr[0:])
	magic := binary.LittleEndian.Uint32(hdr[4:])
	if magic != netMagic {
		return nil, fmt.Errorf("cm: tcp bad magic 0x%08x", magic)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(t.conn, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (t *tcpTransport) WritePacket(payload []byte) error {
	framed := make([]byte, 4+4+len(payload))
	binary.LittleEndian.PutUint32(framed[0:], uint32(len(payload)))
	binary.LittleEndian.PutUint32(framed[4:], netMagic)
	copy(framed[8:], payload)
	_, err := t.conn.Write(framed)
	return err
}

func (t *tcpTransport) NeedsEncryption() bool  { return true }
func (t *tcpTransport) Close() error           { return t.conn.Close() }
func (t *tcpTransport) RemoteAddr() string     { return t.conn.RemoteAddr().String() }
func (t *tcpTransport) LocalAddr() string      { return t.conn.LocalAddr().String() }

// ---- WebSocket transport ----------------------------------------------------

type wsTransport struct {
	conn *websocket.Conn
	addr string
}

func newWSConn(conn *websocket.Conn, addr string) cmConn {
	return &wsTransport{conn: conn, addr: addr}
}

func (w *wsTransport) ReadPacket() ([]byte, error) {
	// Each WebSocket binary frame is exactly one Steam message.
	// Unlike TCP there is no length prefix or VT01 magic — the frame boundary
	// IS the message boundary (SteamKit2 WebSocketCMClient sends raw packet
	// bytes without any additional framing).
	_, msg, err := w.conn.Read(context.Background())
	if err != nil {
		return nil, err
	}
	if len(msg) < 8 {
		return nil, fmt.Errorf("cm: ws packet too short (%d bytes)", len(msg))
	}
	return msg, nil
}

func (w *wsTransport) WritePacket(payload []byte) error {
	// Send the raw Steam packet bytes directly as a binary frame.
	// No magic prefix — WebSocket framing handles message delimiting.
	return w.conn.Write(context.Background(), websocket.MessageBinary, payload)
}

func (w *wsTransport) NeedsEncryption() bool  { return false }
func (w *wsTransport) Close() error {
	return w.conn.Close(websocket.StatusNormalClosure, "")
}
func (w *wsTransport) RemoteAddr() string { return w.addr }
func (w *wsTransport) LocalAddr() string  { return "local" }
