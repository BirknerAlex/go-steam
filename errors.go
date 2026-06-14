package steam

import "errors"

// Sentinel errors returned by Client methods.
var (
	// ErrNotOwned is returned when the authenticated account does not own the
	// requested app or depot.
	ErrNotOwned = errors.New("steam: account does not own app/depot")

	// ErrSteamGuardRequired is returned when the account has Steam Guard enabled
	// and the login cannot proceed without a guard code.
	ErrSteamGuardRequired = errors.New("steam: steam guard code required")

	// ErrInvalidCredentials is returned when username/password are rejected.
	ErrInvalidCredentials = errors.New("steam: invalid credentials")

	// ErrRateLimited is returned when Steam throttles our requests.
	ErrRateLimited = errors.New("steam: rate limited by Steam")

	// ErrSessionNotReady is returned when the CM session has not reached the
	// Ready state before the context deadline expires.
	ErrSessionNotReady = errors.New("steam: CM session not ready")

	// ErrChunkCorrupt is returned when a downloaded chunk fails SHA1 verification.
	ErrChunkCorrupt = errors.New("steam: chunk SHA1 mismatch — data corrupt")

	// ErrManifestDecrypt is returned when manifest decryption or decompression fails.
	ErrManifestDecrypt = errors.New("steam: manifest decrypt/decompress failed")

	// ErrNoCMServers is returned when the CM server list cannot be fetched.
	ErrNoCMServers = errors.New("steam: no CM servers available")

	// ErrWorkshopItemNotFound is returned when the WebAPI cannot find the
	// requested workshop item.
	ErrWorkshopItemNotFound = errors.New("steam: workshop item not found")

	// ErrDepotKeyDenied is returned when Steam denies the depot decryption key
	// request (access denied / not owned).
	ErrDepotKeyDenied = errors.New("steam: depot key access denied")

	// ErrEncryptionHandshake is returned when the CM encryption handshake fails.
	ErrEncryptionHandshake = errors.New("steam: CM encryption handshake failed")
)
