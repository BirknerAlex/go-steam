package totp

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"time"
)

var ErrInvalidSharedSecret = errors.New("invalid base64 shared secret")

// GenerateAuthCode returns a 5-character Steam authentication code.
// sharedSecret must be a valid base64-encoded string.
func GenerateAuthCode(sharedSecret string, t time.Time) (string, error) {
	key, err := base64.StdEncoding.DecodeString(sharedSecret)
	if err != nil {
		return "", ErrInvalidSharedSecret
	}

	ut := uint64(t.Unix()) / 30
	tb := make([]byte, 8)
	binary.BigEndian.PutUint64(tb, ut)

	mac := hmac.New(sha1.New, key)
	mac.Write(tb)
	hashcode := mac.Sum(nil)

	start := hashcode[19] & 0xf
	fc32 := binary.BigEndian.Uint32(hashcode[start : start+4])
	fc32 &= 1<<31 - 1
	fullcode := int(fc32)

	const chars = "23456789BCDFGHJKMNPQRTVWXY"
	const charsLen = len(chars)

	code := make([]byte, 5)
	for i := range code {
		code[i] = chars[fullcode%charsLen]
		fullcode /= charsLen
	}

	return string(code), nil
}
