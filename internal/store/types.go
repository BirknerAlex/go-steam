package store

import "time"

// CachedSession holds the persistent Steam session credentials so the client
// can reconnect without prompting for a password again.
type CachedSession struct {
	// AccountName is the Steam account username.
	AccountName string `json:"account_name"`
	// AccessToken is the short-lived JWT token (aud: ["client"]) passed
	// directly to CMsgClientLogon.  Expires in ~24 h.
	AccessToken string `json:"access_token"`
	// RefreshToken is the long-lived JWT token (aud: ["web","renew","derive"]).
	// Kept for informational purposes; not used for CM logon directly.
	RefreshToken string `json:"refresh_token"`
	// Expiry is the wall-clock expiry of the access token.
	// LoadSession returns nil after this time to trigger re-auth.
	Expiry time.Time `json:"expiry"`
}

// CachedDepotKey holds a depot's AES-256 decryption key. Depot keys never
// expire for a given build — they are permanent.
type CachedDepotKey struct {
	// DepotID is the Steam depot identifier.
	DepotID uint32 `json:"depot_id"`
	// Key is the 32-byte AES-256 key used to decrypt manifests and chunks.
	Key []byte `json:"key"`
}

// CachedToken holds a CDN auth token for a specific (host, depotID) pair.
type CachedToken struct {
	// Host is the CDN hostname this token is valid for.
	Host string `json:"host"`
	// DepotID is the depot this token authorises.
	DepotID uint32 `json:"depot_id"`
	// Token is the opaque auth token string included in CDN requests.
	Token string `json:"token"`
	// Expiry is the time after which the token must be refreshed.
	Expiry time.Time `json:"expiry"`
}
