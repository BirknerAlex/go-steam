// Package cm implements the Steam CM (Connection Manager) TCP protocol,
// including the AES encryption handshake, proto message framing, heartbeat,
// and automatic reconnect with exponential back-off.
package cm

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // required by Steam protocol
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/BirknerAlex/go-steam/internal/proto"
)

// SessionState represents the lifecycle state of a CM connection.
type SessionState int

const (
	StateDisconnected SessionState = iota
	StateConnecting
	StateEncrypting
	StateAuthenticating
	StateReady
)

func (s SessionState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateEncrypting:
		return "encrypting"
	case StateAuthenticating:
		return "authenticating"
	case StateReady:
		return "ready"
	default:
		return "unknown"
	}
}

// steamPublicKey is the well-known RSA public key used by Steam Universe 1.
// This key is used to encrypt the AES session key during the handshake.
const steamPublicKey = `-----BEGIN PUBLIC KEY-----
MIGdMA0GCSqGSIb3DQEBAQUAA4GLADCBhwKBgQDf7BrWLBBmLBc1OhSwfFkRf53T
2Ct64+AVzRkeRuh7h3SiGEYxqQMUeYKO6UWiSRKpI2hzic9pobFhRr3Bvr/WARvY
gdTckPv+T1JzZsuVcNfFjrocejN1oWI0Rrtgt4Bo+hOneoo3S57G9F1fOpn5nsQ6
6WOiu4gZKODnFMBCiQIBEQ==
-----END PUBLIC KEY-----`

const (
	netMagic      = uint32(0x31305456) // "VT01" little-endian
	protoVersion  = uint32(65581)      // MsgClientLogon.CurrentProtocol from SteamKit2
	clientOsLinux = int32(16)
	clientLang    = "english"
)

// Session is a persistent CM TCP connection.  It reconnects automatically on
// error and re-authenticates using the refresh token stored in the cache.
type Session struct {
	cfg SessionConfig
	log *slog.Logger

	mu             sync.RWMutex
	state          SessionState
	stateChans     []chan SessionState
	conn           cmConn
	useEncryption  bool // true for TCP (AES channel encrypt), false for WebSocket (TLS)
	sessionKey     []byte
	steamID        uint64
	sessionID      int32

	// dispatch layer
	dispatch *Dispatcher

	// heartbeat and shutdown
	heartbeatStop chan struct{}
	closing       chan struct{} // closed by Close() to signal intentional shutdown
	closeOnce     sync.Once
}

// SessionConfig controls Session behaviour.
type SessionConfig struct {
	// Anonymous indicates this session should log on anonymously (no credentials).
	Anonymous bool

	// AccountName is the Steam account username for authenticated sessions.
	AccountName string

	// AccessToken is a CM-issued token (the refresh token from SteamAuthViaCM)
	// used in CMsgClientLogon field 111.  Obtain it by calling SteamAuthViaCM
	// on the anonymous session BEFORE calling NewSession here.
	// Must be non-empty for authenticated (non-anonymous) sessions.
	AccessToken string

	// CMServers overrides the server list fetched from the Steam Web API.
	CMServers []string

	Log *slog.Logger
}

// NewSession creates and starts a CM session.  It blocks until the session
// reaches StateReady or ctx is cancelled.
func NewSession(ctx context.Context, cfg SessionConfig) (*Session, error) {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	s := &Session{
		cfg:           cfg,
		log:           cfg.Log,
		state:         StateDisconnected,
		heartbeatStop: make(chan struct{}),
		closing:       make(chan struct{}),
	}
	s.dispatch = newDispatcher(s)

	if err := s.connectLoop(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// State returns the current session state.
func (s *Session) State() SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// StateChange returns a channel that receives the new state every time the
// session changes state.  Multiple callers get independent channels.
func (s *Session) StateChange() <-chan SessionState {
	ch := make(chan SessionState, 4)
	s.mu.Lock()
	s.stateChans = append(s.stateChans, ch)
	s.mu.Unlock()
	return ch
}

// SteamID returns the authenticated SteamID (0 for anonymous sessions).
func (s *Session) SteamID() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.steamID
}

// Send encodes and sends a proto message, returning a job ID usable with
// Dispatch.Await.
func (s *Session) Send(ctx context.Context, msg proto.EMsg, body []byte) (uint64, error) {
	return s.dispatch.Send(ctx, msg, body)
}

// Await blocks until a reply to jobID arrives, or ctx expires.
func (s *Session) Await(ctx context.Context, jobID uint64) (*proto.Packet, error) {
	return s.dispatch.Await(ctx, jobID)
}

// SendServiceMethod sends a Steam Unified Messages service method call and
// returns a job ID usable with Await.  methodName must be in the form
// "ServiceName.MethodName#Version" (e.g. "ContentServerDirectory.GetCDNAuthToken#1").
func (s *Session) SendServiceMethod(ctx context.Context, methodName string, body []byte) (uint64, error) {
	return s.dispatch.SendServiceMethod(ctx, methodName, body)
}

// GetManifestRequestCode fetches the manifest request code required to download
// a depot manifest.  For the "public" branch, branch should be empty string.
// Returns 0 if the server does not grant a code (anonymous sessions may get 0).
func (s *Session) GetManifestRequestCode(ctx context.Context, appID, depotID uint32, manifestID uint64, branch string) (uint64, error) {
	req := proto.CContentServerDirectory_GetManifestRequestCode_Request{
		AppID:      appID,
		DepotID:    depotID,
		ManifestID: manifestID,
	}
	if branch != "public" && branch != "" {
		req.AppBranch = branch
	}
	body := req.Marshal()

	jobID, err := s.dispatch.SendServiceMethod(ctx, "ContentServerDirectory.GetManifestRequestCode#1", body)
	if err != nil {
		return 0, fmt.Errorf("manifest code: send: %w", err)
	}
	pkt, err := s.dispatch.Await(ctx, jobID)
	if err != nil {
		return 0, fmt.Errorf("manifest code: await: %w", err)
	}
	if proto.EResult(pkt.Header.Eresult) != proto.EResultOK && pkt.Header.Eresult != 0 {
		return 0, fmt.Errorf("manifest code: EResult %d", pkt.Header.Eresult)
	}
	var resp proto.CContentServerDirectory_GetManifestRequestCode_Response
	if err := resp.Unmarshal(pkt.Body); err != nil {
		return 0, fmt.Errorf("manifest code: unmarshal: %w", err)
	}
	return resp.ManifestRequestCode, nil
}

// Close shuts down the session gracefully.  Safe to call more than once.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		close(s.closing)
		close(s.heartbeatStop)
		s.mu.Lock()
		if s.conn != nil {
			s.conn.Close()
		}
		s.mu.Unlock()
	})
}

// ---- internal ---------------------------------------------------------------

func (s *Session) setState(st SessionState) {
	s.mu.Lock()
	s.state = st
	chans := make([]chan SessionState, len(s.stateChans))
	copy(chans, s.stateChans)
	s.mu.Unlock()

	for _, ch := range chans {
		select {
		case ch <- st:
		default:
		}
	}
}

func (s *Session) connectLoop(ctx context.Context) error {
	backoff := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Verify an access token is available before opening the WebSocket.
		// Token acquisition must happen before NewSession is called.
		if err := s.prepareCredentials(ctx); err != nil {
			var pe *permanentError
			if errors.As(err, &pe) {
				return pe.err
			}
			s.log.Warn("CM credential preparation failed", "err", err, "backoff", backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}

		s.setState(StateConnecting)
		if err := s.connect(ctx); err != nil {
			// Permanent errors (wrong credentials, bad guard code) must not be retried.
			var pe *permanentError
			if errors.As(err, &pe) {
				return pe.err
			}
			s.log.Warn("CM connect failed", "err", err, "backoff", backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		return nil
	}
}

func (s *Session) connect(ctx context.Context) error {
	servers, err := s.getServers(ctx)
	if err != nil {
		return err
	}
	var lastErr error
	for _, srv := range servers {
		s.log.Debug("CM connecting", "addr", srv.addr, "type", srv.typ)

		conn, err := srv.dial(ctx)
		if err != nil {
			lastErr = err
			continue
		}
		s.log.Debug("CM connected", "local", conn.LocalAddr(), "remote", conn.RemoteAddr())

		// TCP uses the AES channel-encryption handshake; WebSocket skips it
		// because TLS already provides transport-layer encryption.
		if conn.NeedsEncryption() {
			if err := s.handshake(ctx, conn); err != nil {
				conn.Close()
				lastErr = err
				continue
			}
		}
		s.mu.Lock()
		s.conn = conn
		s.useEncryption = conn.NeedsEncryption()
		s.mu.Unlock()

		// Each readLoop is bound to the conn it was started with so that
		// retries starting a new conn don't create a second reader on the
		// same TCP connection.
		go s.readLoop(conn)

		// Send CMsgClientHello immediately after connecting — matches SteamKit2's
		// CMClient.OnClientConnected().  Without this the server rejects the
		// subsequent CMsgClientLogon with EResult=7 (InvalidProtocolVer).
		hello := proto.CMsgClientHello{ProtocolVersion: protoVersion}
		helloPkt := proto.MarshalPacket(proto.EMsgClientHello, proto.CMsgProtoBufHeader{}, hello.Marshal())
		if err := s.sendEncrypted(helloPkt); err != nil {
			conn.Close()
			lastErr = fmt.Errorf("cm: send hello: %w", err)
			continue
		}

		// Authenticate.
		if err := s.authenticate(ctx); err != nil {
			// Close the conn so the bound readLoop exits cleanly rather than
			// blocking indefinitely waiting for a server that may not respond.
			conn.Close()
			return err
		}
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("cm: all servers failed: %w", lastErr)
	}
	return fmt.Errorf("cm: no servers available")
}

// cmServer is a Steam CM server endpoint with its transport type.
type cmServer struct {
	addr string // "host:port"
	typ  string // "websockets" or "tcp"
}

// dial opens a connection to the CM server.
func (s *cmServer) dial(ctx context.Context) (cmConn, error) {
	if s.typ == "websockets" {
		url := "wss://" + s.addr + "/cmsocket/"
		c, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
			Subprotocols: []string{"steamprotocol"},
			HTTPHeader: http.Header{
				"User-Agent": []string{"Valve/Steam HTTP Client 1.0"},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("cm: ws dial %s: %w", url, err)
		}
		c.SetReadLimit(1 << 23) // 8 MiB — large enough for any Steam message
		return newWSConn(c, s.addr), nil
	}
	// TCP
	nc, err := (&net.Dialer{}).DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return nil, fmt.Errorf("cm: tcp dial %s: %w", s.addr, err)
	}
	return newTCPConn(nc), nil
}

// getServers returns the CM server list, preferring the configured override.
func (s *Session) getServers(ctx context.Context) ([]cmServer, error) {
	if len(s.cfg.CMServers) > 0 {
		servers := make([]cmServer, 0, len(s.cfg.CMServers))
		for _, addr := range s.cfg.CMServers {
			servers = append(servers, cmServer{addr: addr, typ: "tcp"})
		}
		return servers, nil
	}
	return fetchCMServers(ctx)
}

type cmListForConnectResponse struct {
	Response struct {
		Serverlist []struct {
			Endpoint string `json:"endpoint"`
			Type     string `json:"type"` // "websockets" or "netfilter"
		} `json:"serverlist"`
	} `json:"response"`
}

func fetchCMServers(ctx context.Context) ([]cmServer, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.steampowered.com/ISteamDirectory/GetCMListForConnect/v1/?cellid=0&maxcount=20", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cm: fetch server list: %w", err)
	}
	defer resp.Body.Close()

	var result cmListForConnectResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("cm: decode server list: %w", err)
	}

	var servers []cmServer
	for _, e := range result.Response.Serverlist {
		switch e.Type {
		case "websockets":
			servers = append(servers, cmServer{addr: e.Endpoint, typ: "websockets"})
		// "netfilter" = legacy raw TCP — skip in favour of WebSocket; TCP servers
		// no longer respond to anonymous logon in the modern CM network.
		}
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("cm: empty server list")
	}
	return servers, nil
}

// handshake performs the AES encryption handshake before any proto messages
// can be sent.  After a successful handshake, s.sessionKey is set.
func (s *Session) handshake(_ context.Context, conn cmConn) error {
	s.setState(StateEncrypting)

	// 1. Read ChannelEncryptRequest (struct message, not proto).
	payload, err := conn.ReadPacket()
	if err != nil {
		return fmt.Errorf("cm: handshake read request: %w", err)
	}
	if s.log.Enabled(context.Background(), slog.LevelDebug) {
		s.log.Debug("handshake EncryptRequest raw", "len", len(payload))
		fmt.Fprintf(os.Stderr, "%s\n", hex.Dump(payload))
	}
	pkt, err := proto.UnmarshalPacket(payload)
	if err != nil {
		return fmt.Errorf("cm: handshake parse: %w", err)
	}
	if pkt.EMsg.Base() != proto.EMsgChannelEncryptRequest {
		return fmt.Errorf("cm: expected EncryptRequest, got %d", pkt.EMsg)
	}

	// Struct messages carry a MsgHdr before the body:
	//   [8] TargetJobID  [8] SourceJobID  [4] ProtocolVersion  [4] Universe  [16] Challenge
	// Body = payload[4:], so offsets within Body are:
	//   [0:8]  TargetJobID
	//   [8:16] SourceJobID
	//   [16:20] ProtocolVersion
	//   [20:24] Universe
	//   [24:40] Challenge (nonce)
	if len(pkt.Body) < 8+8+4+4+16 {
		return fmt.Errorf("cm: encrypt request body too short (%d bytes)", len(pkt.Body))
	}
	nonce := pkt.Body[24:40]
	if s.log.Enabled(context.Background(), slog.LevelDebug) {
		universe := binary.LittleEndian.Uint32(pkt.Body[20:24])
		protoVer := binary.LittleEndian.Uint32(pkt.Body[16:20])
		s.log.Debug("handshake EncryptRequest parsed", "proto_ver", protoVer, "universe", universe, "nonce", hex.EncodeToString(nonce))
	}

	// 2. Generate 32-byte AES session key and encrypt it with Steam's RSA public key.
	sessionKey := make([]byte, 32)
	if _, err := rand.Read(sessionKey); err != nil {
		return fmt.Errorf("cm: generate session key: %w", err)
	}

	pub, err := parseSteamPublicKey()
	if err != nil {
		return err
	}

	// Encrypt: sessionKey + nonce (matches SteamKit2 CryptoHelper.SymmetricEncryptWithIV)
	// The nonce acts as a challenge; we send it back encrypted to prove we received it.
	plaintext := append(append([]byte(nil), sessionKey...), nonce...)
	encrypted, err := rsa.EncryptOAEP(sha1.New(), rand.Reader, pub, plaintext, nil)
	if err != nil {
		return fmt.Errorf("cm: RSA encrypt session key: %w", err)
	}

	// 3. Send ChannelEncryptResponse.
	// Body: [4 protocolVersion=1][4 keyLen][keyLen encryptedKey][4 crc32][4 zero]
	crc := crc32IEEE(encrypted)
	body := make([]byte, 4+4+len(encrypted)+4+4)
	binary.LittleEndian.PutUint32(body[0:], 1) // protocol version
	binary.LittleEndian.PutUint32(body[4:], uint32(len(encrypted)))
	copy(body[8:], encrypted)
	binary.LittleEndian.PutUint32(body[8+len(encrypted):], crc)
	// last 4 bytes remain zero

	respPayload := buildStructPayload(proto.EMsgChannelEncryptResponse, body)
	if s.log.Enabled(context.Background(), slog.LevelDebug) {
		s.log.Debug("handshake EncryptResponse (payload)", "len", len(respPayload))
		fmt.Fprintf(os.Stderr, "%s\n", hex.Dump(respPayload))
	}
	if err := conn.WritePacket(respPayload); err != nil {
		return fmt.Errorf("cm: write encrypt response: %w", err)
	}

	// 4. Read ChannelEncryptResult.
	payload, err = conn.ReadPacket()
	if err != nil {
		return fmt.Errorf("cm: handshake read result: %w", err)
	}
	if s.log.Enabled(context.Background(), slog.LevelDebug) {
		s.log.Debug("handshake EncryptResult raw", "len", len(payload))
		fmt.Fprintf(os.Stderr, "%s\n", hex.Dump(payload))
	}
	pkt, err = proto.UnmarshalPacket(payload)
	if err != nil {
		return fmt.Errorf("cm: handshake parse result: %w", err)
	}
	if pkt.EMsg.Base() != proto.EMsgChannelEncryptResult {
		return fmt.Errorf("cm: expected EncryptResult, got %d", pkt.EMsg)
	}
	// EncryptResult body: [8] TargetJobID [8] SourceJobID [4] EResult
	if len(pkt.Body) < 8+8+4 {
		return fmt.Errorf("cm: encrypt result body too short (%d bytes)", len(pkt.Body))
	}
	result := binary.LittleEndian.Uint32(pkt.Body[16:])
	if result != 1 { // EResult.OK
		return fmt.Errorf("cm: encrypt result: %d", result)
	}

	s.mu.Lock()
	s.sessionKey = sessionKey
	s.mu.Unlock()
	return nil
}

// readLoop reads packets from conn and dispatches them.
// Each goroutine is bound to its own conn so that connection retries do not
// spawn a second concurrent reader on the same TCP socket.
func (s *Session) readLoop(conn cmConn) {
	for {
		s.mu.RLock()
		key := s.sessionKey
		useEnc := s.useEncryption
		s.mu.RUnlock()

		raw, err := conn.ReadPacket()
		if err != nil {
			// Suppress noise on intentional shutdown.
			select {
			case <-s.closing:
				s.dispatch.cancelAll()
				s.setState(StateDisconnected)
				return
			default:
			}

			s.log.Warn("CM read error", "err", err)
			s.dispatch.cancelAll()

			// Only spawn a background reconnect if the session was fully ready.
			// During authentication the foreground connectLoop handles the retry.
			wasReady := s.State() == StateReady
			s.setState(StateDisconnected)
			if wasReady {
				go func() {
					ctx := context.Background()
					if err := s.connectLoop(ctx); err != nil {
						s.log.Error("CM reconnect failed", "err", err)
					}
				}()
			}
			return
		}

		if s.log.Enabled(context.Background(), slog.LevelDebug) {
			s.log.Debug("CM recv raw", "len", len(raw), "encrypted", useEnc)
			fmt.Fprintf(os.Stderr, "%s\n", hex.Dump(raw))
		}

		var plain []byte
		if useEnc {
			plain, err = decryptPacket(raw, key)
			if err != nil {
				s.log.Warn("CM decrypt error", "err", err)
				continue
			}
			if s.log.Enabled(context.Background(), slog.LevelDebug) {
				s.log.Debug("CM recv decrypted", "len", len(plain))
				fmt.Fprintf(os.Stderr, "%s\n", hex.Dump(plain))
			}
		} else {
			plain = raw
		}

		pkt, err := proto.UnmarshalPacket(plain)
		if err != nil {
			s.log.Warn("CM unmarshal error", "err", err)
			continue
		}

		s.log.Debug("CM recv", "emsg", pkt.EMsg.Base(), "body_len", len(pkt.Body))

		// Unwrap CMsgMulti transparently.
		if pkt.EMsg == proto.EMsgMulti {
			s.handleMulti(pkt)
			continue
		}

		s.dispatch.deliver(pkt)
	}
}

func (s *Session) handleMulti(pkt *proto.Packet) {
	var multi proto.CMsgMulti
	if err := multi.Unmarshal(pkt.Body); err != nil {
		s.log.Warn("CMsgMulti unmarshal error", "err", err)
		return
	}
	body := multi.MessageBody
	if multi.SizeUnzipped > 0 {
		r, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			s.log.Warn("CMsgMulti gzip open error", "err", err)
			return
		}
		decompressed, err := io.ReadAll(io.LimitReader(r, int64(multi.SizeUnzipped)+1024))
		r.Close()
		if err != nil {
			s.log.Warn("CMsgMulti gzip read error", "err", err)
			return
		}
		body = decompressed
	}
	// body is a sequence of [4-byte len][payload] records.
	for len(body) >= 4 {
		sz := binary.LittleEndian.Uint32(body[:4])
		body = body[4:]
		if uint32(len(body)) < sz {
			break
		}
		pkt, err := proto.UnmarshalPacket(body[:sz])
		if err == nil {
			s.dispatch.deliver(pkt)
		}
		body = body[sz:]
	}
}

// sendEncrypted encodes and sends a payload.  For TCP connections the payload
// is AES-encrypted; for WebSocket connections it is sent as-is (TLS handles
// transport encryption).
func (s *Session) sendEncrypted(payload []byte) error {
	s.mu.RLock()
	conn := s.conn
	key := s.sessionKey
	useEnc := s.useEncryption
	s.mu.RUnlock()

	if s.log.Enabled(context.Background(), slog.LevelDebug) {
		s.log.Debug("CM send (plaintext)", "len", len(payload), "encrypted", useEnc)
		fmt.Fprintf(os.Stderr, "%s\n", hex.Dump(payload))
	}

	if !useEnc {
		return conn.WritePacket(payload)
	}

	encrypted, err := encryptPacket(payload, key)
	if err != nil {
		return fmt.Errorf("cm: encrypt: %w", err)
	}

	if s.log.Enabled(context.Background(), slog.LevelDebug) {
		s.log.Debug("CM send (AES)", "len", len(encrypted))
		fmt.Fprintf(os.Stderr, "%s\n", hex.Dump(encrypted))
	}

	return conn.WritePacket(encrypted)
}

// startHeartbeat sends periodic CMsgClientHeartBeat messages.
func (s *Session) startHeartbeat(interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-s.heartbeatStop:
				return
			case <-t.C:
				hb := proto.CMsgClientHeartBeat{}
				body := hb.Marshal()
				pkt := proto.MarshalPacket(proto.EMsgClientHeartBeat, proto.CMsgProtoBufHeader{}, body)
				if err := s.sendEncrypted(pkt); err != nil {
					s.log.Warn("heartbeat send error", "err", err)
				}
			}
		}
	}()
}

// ---- wire helpers -----------------------------------------------------------

// buildStructPayload encodes a non-proto struct message payload (used during
// the encryption handshake).  The caller passes the payload to WritePacket,
// which adds the transport-specific framing ([len][magic] for TCP,
// [magic] inside a WebSocket binary frame).
//
//	[4] EMsg | [8] TargetJobID (0xFFFFFFFFFFFFFFFF) | [8] SourceJobID (0xFFFFFFFFFFFFFFFF) | body
func buildStructPayload(msg proto.EMsg, body []byte) []byte {
	const jobIDNone = uint64(0xFFFFFFFFFFFFFFFF)
	payload := make([]byte, 4+8+8+len(body))
	binary.LittleEndian.PutUint32(payload[0:], uint32(msg))
	binary.LittleEndian.PutUint64(payload[4:], jobIDNone)
	binary.LittleEndian.PutUint64(payload[12:], jobIDNone)
	copy(payload[20:], body)
	return payload
}

// encryptPacket encrypts plaintext for the Steam CM wire format.
//
// Steam's format (matching go-steam / SteamKit2 SymmetricEncrypt):
//   1. Generate random IV (16 bytes)
//   2. AES-ECB encrypt the IV with the session key  → encryptedIV
//   3. AES-CBC encrypt plaintext with session key + raw IV  → ciphertext
//   4. Wire: encryptedIV(16) + ciphertext
//
// The HMAC variant prepends HMAC-SHA1(key[16:], encryptedIV+ciphertext)[0:3].
func encryptPacket(plaintext, key []byte) ([]byte, error) {
	rawIV := make([]byte, aes.BlockSize)
	if _, err := rand.Read(rawIV); err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// ECB-encrypt the IV.
	encIV := make([]byte, aes.BlockSize)
	block.Encrypt(encIV, rawIV)

	// CBC-encrypt the payload using the raw IV.
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, rawIV).CryptBlocks(ciphertext, padded)

	// Build wire: encryptedIV + ciphertext (plain format; HMAC prepended separately if needed).
	out := make([]byte, len(encIV)+len(ciphertext))
	copy(out, encIV)
	copy(out[len(encIV):], ciphertext)
	return out, nil
}

// decryptPacket reverses encryptPacket.
//
// Steam's wire format (plain):   encryptedIV(16) + AES-CBC(ciphertext)
//   where encryptedIV = AES-ECB(key, rawIV)
//   and ciphertext    = AES-CBC(key, rawIV, plaintext)
//
// HMAC variant: HMAC-SHA1(key[16:], encryptedIV+ciphertext)[0:3] prepended.
// We try HMAC first; fall back to plain.
func decryptPacket(data, key []byte) ([]byte, error) {
	if len(data) < aes.BlockSize {
		return nil, fmt.Errorf("cm: encrypted packet too short (%d bytes)", len(data))
	}

	// Try HMAC format: mac[3] + encryptedIV[16] + ciphertext
	if len(data) >= 3+aes.BlockSize {
		macTag := data[0:3]
		encIV := data[3 : 3+aes.BlockSize]
		ciphertext := data[3+aes.BlockSize:]
		if len(ciphertext)%aes.BlockSize == 0 {
			expectedMac := computeHMAC(key[16:], encIV, ciphertext)
			if hmac.Equal(macTag, expectedMac[:3]) {
				return symmetricDecrypt(key, encIV, ciphertext)
			}
		}
	}

	// Plain format: encryptedIV[16] + ciphertext
	encIV := data[:aes.BlockSize]
	ciphertext := data[aes.BlockSize:]
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("cm: ciphertext not block-aligned")
	}
	return symmetricDecrypt(key, encIV, ciphertext)
}

// symmetricDecrypt decrypts Steam's AES-CBC scheme where the IV is ECB-encrypted.
func symmetricDecrypt(key, encIV, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	// ECB-decrypt the IV.
	rawIV := make([]byte, aes.BlockSize)
	block.Decrypt(rawIV, encIV)
	// CBC-decrypt the ciphertext.
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, rawIV).CryptBlocks(plaintext, ciphertext)
	return pkcs7Unpad(plaintext)
}

func computeHMAC(key, iv, ciphertext []byte) []byte {
	h := hmac.New(sha1.New, key)
	h.Write(iv)
	h.Write(ciphertext)
	return h.Sum(nil)
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	padded := make([]byte, len(data)+pad)
	copy(padded, data)
	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(pad)
	}
	return padded
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("cm: empty plaintext")
	}
	pad := int(data[len(data)-1])
	if pad < 1 || pad > aes.BlockSize || pad > len(data) {
		return nil, fmt.Errorf("cm: bad PKCS7 padding byte %d", pad)
	}
	return data[:len(data)-pad], nil
}

func parseSteamPublicKey() (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(steamPublicKey))
	if block == nil {
		return nil, fmt.Errorf("cm: failed to decode Steam public key PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("cm: parse Steam public key: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("cm: Steam public key is not RSA")
	}
	return rsaPub, nil
}

// crc32IEEE computes a CRC32 (IEEE polynomial) checksum, used in the encrypt response.
func crc32IEEE(data []byte) uint32 {
	// Standard CRC32 using the IEEE polynomial.
	var crc uint32 = 0xFFFFFFFF
	for _, b := range data {
		crc ^= uint32(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xEDB88320
			} else {
				crc >>= 1
			}
		}
	}
	return ^crc
}
