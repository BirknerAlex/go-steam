package mock

import (
	"crypto/aes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// CDNServer is an httptest-based mock CDN that serves pre-registered manifests
// and chunks.  It exercises the full AES+LZMA pipeline.
type CDNServer struct {
	server    *httptest.Server
	mu        sync.RWMutex
	manifests map[string][]byte // key: "depotID/manifestGID/5"
	chunks    map[string][]byte // key: "depotID/chunkHex"
	tokens    map[string]bool   // valid tokens
	// SimulateErrors counts how many requests fail before succeeding.
	SimulateErrors int
	errCount       int
}

// NewCDNServer creates and starts a mock CDN HTTP server.
func NewCDNServer() *CDNServer {
	c := &CDNServer{
		manifests: make(map[string][]byte),
		chunks:    make(map[string][]byte),
		tokens:    make(map[string]bool),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/depot/", c.handleDepot)
	c.server = httptest.NewServer(mux)
	return c
}

// Host returns the mock server host (without scheme).
func (c *CDNServer) Host() string {
	return strings.TrimPrefix(c.server.URL, "http://")
}

// URL returns the full mock server base URL.
func (c *CDNServer) URL() string { return c.server.URL }

// Close shuts down the mock server.
func (c *CDNServer) Close() { c.server.Close() }

// AddToken registers an auth token as valid.
func (c *CDNServer) AddToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens[token] = true
}

// AddManifest registers a pre-encoded manifest response.
func (c *CDNServer) AddManifest(depotID uint32, manifestGID uint64, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := fmt.Sprintf("%d/%d/5", depotID, manifestGID)
	c.manifests[key] = data
}

// AddChunk registers a pre-encoded (AES-ECB encrypted) chunk response.
// Use EncodeChunk to prepare the bytes.
func (c *CDNServer) AddChunk(depotID uint32, chunkHex string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := fmt.Sprintf("%d/%s", depotID, chunkHex)
	c.chunks[key] = data
}

// EncodeChunk AES-ECB-encrypts plaintext with depotKey for use with AddChunk.
func EncodeChunk(plaintext, depotKey []byte) ([]byte, error) {
	block, err := aes.NewCipher(depotKey)
	if err != nil {
		return nil, err
	}
	// PKCS#7 pad to block size.  Copy into a fresh buffer rather than
	// appending to the caller's slice, which could share backing storage.
	padLen := aes.BlockSize - len(plaintext)%aes.BlockSize
	padded := make([]byte, len(plaintext)+padLen)
	copy(padded, plaintext)
	for i := len(plaintext); i < len(padded); i++ {
		padded[i] = byte(padLen)
	}
	out := make([]byte, len(padded))
	for i := 0; i < len(padded); i += aes.BlockSize {
		block.Encrypt(out[i:i+aes.BlockSize], padded[i:i+aes.BlockSize])
	}
	return out, nil
}

// EncodeManifest builds a minimal mock manifest payload with a single
// length-prefixed section (for use with AddManifest).
func EncodeManifest(protoBytes []byte) []byte {
	out := make([]byte, 4+len(protoBytes))
	binary.LittleEndian.PutUint32(out, uint32(len(protoBytes)))
	copy(out[4:], protoBytes)
	return out
}

func (c *CDNServer) handleDepot(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	if c.SimulateErrors > 0 && c.errCount < c.SimulateErrors {
		c.errCount++
		c.mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	c.mu.Unlock()

	// Validate Bearer token if any tokens are registered.
	c.mu.RLock()
	hasTokens := len(c.tokens) > 0
	c.mu.RUnlock()
	if hasTokens {
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		c.mu.RLock()
		valid := c.tokens[token]
		c.mu.RUnlock()
		if !valid {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	// Path format: /depot/<depotID>/(manifest|chunk)/<id>[/5]
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/depot/"), "/")
	if len(parts) < 3 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	depotID := parts[0]
	resourceType := parts[1]
	resourceID := parts[2]

	switch resourceType {
	case "manifest":
		suffix := ""
		if len(parts) >= 4 {
			suffix = "/" + parts[3]
		}
		key := depotID + "/" + resourceID + suffix
		c.mu.RLock()
		data, ok := c.manifests[key]
		c.mu.RUnlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "")
		w.Write(data) //nolint:errcheck
	case "chunk":
		key := depotID + "/" + resourceID
		c.mu.RLock()
		data, ok := c.chunks[key]
		c.mu.RUnlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(data) //nolint:errcheck
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}
