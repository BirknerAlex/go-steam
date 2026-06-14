package totp

import (
	"reflect"
	"testing"
	"time"
)

func TestGenerateAuthCode(t *testing.T) {
	code, err := GenerateAuthCode("cnOgv/KdpLoP6Nbh0GMkXkPXALQ=", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reflect.TypeOf(code).String() != "string" {
		t.Fatalf("expected string, got %T", code)
	}
	if len(code) != 5 {
		t.Fatalf("expected 5-char code, got %d chars: %s", len(code), code)
	}
}

func TestGenerateAuthCode_InvalidSecret(t *testing.T) {
	_, err := GenerateAuthCode("not-valid-base64!!!", time.Now())
	if err != ErrInvalidSharedSecret {
		t.Fatalf("expected ErrInvalidSharedSecret, got %v", err)
	}
}

// TestGenerateAuthCode_KnownVectors locks the algorithm to fixed outputs so an
// accidental change to the TOTP derivation is caught.  Vectors were generated
// from this implementation with the well-known SteamKit test secret.
func TestGenerateAuthCode_KnownVectors(t *testing.T) {
	const secret = "cnOgv/KdpLoP6Nbh0GMkXkPXALQ="
	vectors := map[int64]string{
		0:          "W3J46",
		1700000000: "X45RP",
		1234567890: "VYNVB",
	}
	for ts, want := range vectors {
		got, err := GenerateAuthCode(secret, time.Unix(ts, 0))
		if err != nil {
			t.Fatalf("ts=%d: %v", ts, err)
		}
		if got != want {
			t.Errorf("GenerateAuthCode(ts=%d) = %q, want %q", ts, got, want)
		}
	}
}

// Codes are stable within a 30-second window and (very likely) change across them.
func TestGenerateAuthCode_WindowStability(t *testing.T) {
	const secret = "cnOgv/KdpLoP6Nbh0GMkXkPXALQ="
	// Window = floor(unix/30); 1700000000..1700000009 fall in window 56666666.
	a, _ := GenerateAuthCode(secret, time.Unix(1700000000, 0))
	b, _ := GenerateAuthCode(secret, time.Unix(1700000009, 0))
	if a != b {
		t.Errorf("codes within the same 30s window should match: %q vs %q", a, b)
	}
	// A timestamp in the next window should (with overwhelming likelihood) differ.
	c, _ := GenerateAuthCode(secret, time.Unix(1700000010, 0))
	if a == c {
		t.Errorf("codes in adjacent windows unexpectedly equal: %q", a)
	}
}
