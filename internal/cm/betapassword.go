package cm

import (
	"context"
	"crypto/aes"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/BirknerAlex/go-steam/internal/proto"
)

// ErrBranchPasswordDenied is returned when Steam rejects the supplied branch
// (beta) password for an app.
var ErrBranchPasswordDenied = fmt.Errorf("branch password denied")

// CheckAppBetaPassword submits a beta-branch password to Steam and returns the
// per-branch AES-256 decryption keys it unlocks, keyed by branch name.
//
// The returned keys are used by DecryptManifestGID to recover the manifest GID
// of a password-protected branch from its encrypted blob in PICS app info.
// An empty/unknown password yields ErrBranchPasswordDenied.
func (s *Session) CheckAppBetaPassword(ctx context.Context, appID uint32, password string) (map[string][]byte, error) {
	req := proto.CMsgClientCheckAppBetaPassword{
		AppID:        appID,
		BetaPassword: password,
	}
	body := req.Marshal()

	jobID, err := s.dispatch.Send(ctx, proto.EMsgClientCheckAppBetaPassword, body)
	if err != nil {
		return nil, fmt.Errorf("betapassword: send: %w", err)
	}
	pkt, err := s.dispatch.Await(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("betapassword: await: %w", err)
	}

	var resp proto.CMsgClientCheckAppBetaPasswordResponse
	if err := resp.Unmarshal(pkt.Body); err != nil {
		return nil, fmt.Errorf("betapassword: unmarshal: %w", err)
	}
	if proto.EResult(resp.Eresult) != proto.EResultOK {
		return nil, fmt.Errorf("betapassword: %w (app %d, EResult %d)", ErrBranchPasswordDenied, appID, resp.Eresult)
	}

	keys := make(map[string][]byte, len(resp.BetaPasswords))
	for _, e := range resp.BetaPasswords {
		key, err := hex.DecodeString(e.BetaPassword)
		if err != nil {
			return nil, fmt.Errorf("betapassword: decode key for branch %q: %w", e.BetaName, err)
		}
		keys[e.BetaName] = key
	}
	return keys, nil
}

// DecryptManifestGID decrypts the hex-encoded encrypted manifest GID blob of a
// password-protected branch using the per-branch key from CheckAppBetaPassword.
//
// The blob is AES-256-ECB encrypted with PKCS#7 padding; the decrypted
// plaintext's first 8 bytes are the manifest GID, little-endian.  This mirrors
// SteamKit2/DepotDownloader's SymmetricDecryptECB + BitConverter.ToUInt64.
func DecryptManifestGID(encryptedHexGID string, key []byte) (uint64, error) {
	ciphertext, err := hex.DecodeString(encryptedHexGID)
	if err != nil {
		return 0, fmt.Errorf("decrypt manifest gid: bad hex: %w", err)
	}
	plaintext, err := decryptECB(key, ciphertext)
	if err != nil {
		return 0, fmt.Errorf("decrypt manifest gid: %w", err)
	}
	if len(plaintext) < 8 {
		return 0, fmt.Errorf("decrypt manifest gid: plaintext too short (%d bytes)", len(plaintext))
	}
	return binary.LittleEndian.Uint64(plaintext[:8]), nil
}

// decryptECB performs AES-ECB decryption with PKCS#7 unpadding.  Go's stdlib has
// no ECB mode, so each block is decrypted independently.
func decryptECB(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ecb: ciphertext is not a multiple of the block size (%d bytes)", len(ciphertext))
	}
	plaintext := make([]byte, len(ciphertext))
	for off := 0; off < len(ciphertext); off += aes.BlockSize {
		block.Decrypt(plaintext[off:off+aes.BlockSize], ciphertext[off:off+aes.BlockSize])
	}
	return pkcs7Unpad(plaintext)
}
