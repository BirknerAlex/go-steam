package steam

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/BirknerAlex/go-steam/internal/cdn"
	"github.com/BirknerAlex/go-steam/internal/cm"
	"github.com/BirknerAlex/go-steam/internal/store"
)

// Config controls Client behaviour.
type Config struct {
	// Username and Password are required for authenticated downloads (apps that
	// require ownership, or workshop items).
	// Leave empty to use an anonymous session (public free-to-play games only).
	Username string
	Password string

	// CachePath is the directory used to persist sessions, depot keys, and CDN
	// tokens between runs.  Defaults to the OS temp directory if empty.
	CachePath string

	// CellID is the Steam CDN cell used for server selection.  0 selects automatically.
	CellID uint32

	// MaxParallelChunks is the maximum number of concurrent chunk downloads.
	// Defaults to 16 if zero.
	MaxParallelChunks int64

	// MaxParallelManifests is the maximum number of manifests downloaded concurrently.
	// Defaults to 4 if zero.
	MaxParallelManifests int64

	// SteamGuardCallback is called when Steam requires a Guard code during login.
	// Use InteractiveSteamGuard(), SteamGuardCodeGenerate(secret), or a custom
	// func — for example one that fetches and parses the guard email automatically.
	// Defaults to UnknownSteamGuard() which returns an error, so Steam Guard
	// failures are surfaced immediately rather than hanging.
	SteamGuardCallback SteamGuardCallback

	// CMServers overrides the CM server list (useful for testing).
	CMServers []string

	// Log is the logger used by the client.  Defaults to slog.Default().
	Log *slog.Logger
}

func (c *Config) applyDefaults() {
	if c.SteamGuardCallback == nil {
		c.SteamGuardCallback = UnknownSteamGuard()
	}
	if c.MaxParallelChunks <= 0 {
		c.MaxParallelChunks = 16
	}
	if c.MaxParallelManifests <= 0 {
		c.MaxParallelManifests = 4
	}
	if c.Log == nil {
		c.Log = slog.Default()
	}
	if c.CachePath == "" {
		c.CachePath = defaultCachePath()
	}
}

// Client is the entry point for Steam content downloads.
// Create one with New(); use DownloadApp or DownloadWorkshopItem to fetch content.
type Client struct {
	cfg          Config
	cache        *store.LocalCache
	anonSession  *cm.Session
	authSession  *cm.Session
	cdnClient    *cdn.Client
	chunkSem     *semaphore.Weighted
	manifestSem  *semaphore.Weighted
	log          *slog.Logger
}

// New creates a Client and establishes CM connections.
// It blocks until both sessions are ready or ctx is cancelled.
func New(ctx context.Context, cfg Config) (*Client, error) {
	cfg.applyDefaults()

	cache, err := store.NewLocalCache(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("steam: open cache: %w", err)
	}

	c := &Client{
		cfg:         cfg,
		cache:       cache,
		chunkSem:    semaphore.NewWeighted(cfg.MaxParallelChunks),
		manifestSem: semaphore.NewWeighted(cfg.MaxParallelManifests),
		log:         cfg.Log,
	}

	// Anonymous session — always required (used for public app info, public depots).
	anonCfg := cm.SessionConfig{
		Anonymous: true,
		CMServers: cfg.CMServers,
		Log:       cfg.Log,
	}
	anonSess, err := cm.NewSession(ctx, anonCfg)
	if err != nil {
		return nil, fmt.Errorf("steam: anonymous CM session: %w", err)
	}
	c.anonSession = anonSess

	// Authenticated session — only if credentials are provided.
	if cfg.Username != "" {
		// Try a cached access token first; if absent/expired, authenticate via the
		// anonymous session (SteamAuthViaCM issues tokens that the CM accepts in
		// CMsgClientLogon field 108).
		var accessToken string
		usingCache := false
		if sess, err := cache.LoadSession(cfg.Username); err == nil && sess != nil {
			accessToken = sess.AccessToken
			usingCache = true
		}

		var authSess *cm.Session
		if accessToken != "" {
			authCfg := cm.SessionConfig{
				AccountName: cfg.Username,
				AccessToken: accessToken,
				CMServers:   cfg.CMServers,
				Log:         cfg.Log,
			}
			var sessErr error
			authSess, sessErr = cm.NewSession(ctx, authCfg)
			if sessErr != nil {
				if usingCache && cfg.Password != "" {
					// Cached token was rejected — clear it and fall through to re-auth.
					_ = cache.ClearSession(cfg.Username)
					authSess = nil
					accessToken = ""
				} else {
					return nil, fmt.Errorf("steam: auth CM session: %w", sessErr)
				}
			}
		}

		if authSess == nil {
			if cfg.Password == "" {
				return nil, fmt.Errorf("steam: no cached session and no password for %q", cfg.Username)
			}
			tokens, err := cm.SteamAuthViaCM(ctx, anonSess, cfg.Username, cfg.Password,
				func() (string, error) { return cfg.SteamGuardCallback() })
			if err != nil {
				return nil, fmt.Errorf("steam: auth: %w", err)
			}
			expiry := jwtExpiry(tokens.RefreshToken)
			_ = cache.SaveSession(cfg.Username, &store.CachedSession{
				AccountName:  cfg.Username,
				AccessToken:  tokens.AccessToken,
				RefreshToken: tokens.RefreshToken,
				Expiry:       expiry,
			})
			authCfg := cm.SessionConfig{
				AccountName: cfg.Username,
				AccessToken: tokens.AccessToken,
				CMServers:   cfg.CMServers,
				Log:         cfg.Log,
			}
			authSess, err = cm.NewSession(ctx, authCfg)
			if err != nil {
				return nil, fmt.Errorf("steam: auth CM session: %w", err)
			}
		}
		c.authSession = authSess
	}

	// CDN client.
	cdnClient, err := cdn.NewClient(ctx, cfg.CellID)
	if err != nil {
		return nil, fmt.Errorf("steam: CDN client: %w", err)
	}
	c.cdnClient = cdnClient

	return c, nil
}

// Close shuts down all CM sessions gracefully.
func (c *Client) Close() {
	if c.anonSession != nil {
		c.anonSession.Close()
	}
	if c.authSession != nil {
		c.authSession.Close()
	}
}

// sessionForDepots returns the appropriate CM session for downloading depots.
// The auth session is preferred whenever it exists — it can access everything
// the anon session can, plus depots that require ownership.
func (c *Client) sessionForDepots(infos []*cm.DepotInfo) (*cm.Session, error) {
	if c.authSession != nil {
		return c.authSession, nil
	}
	for _, d := range infos {
		if !d.AllowAnonymous {
			return nil, fmt.Errorf("steam: depot %d requires authentication", d.DepotID)
		}
	}
	return c.anonSession, nil
}

// jwtExpiry parses the "exp" Unix timestamp from a JWT and returns it as
// time.Time.  Returns zero time if the token cannot be parsed.
func jwtExpiry(token string) time.Time {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		if payload, err = base64.StdEncoding.DecodeString(parts[1]); err != nil {
			return time.Time{}
		}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}
