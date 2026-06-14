package steam

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BirknerAlex/go-steam/internal/totp"
)

// SteamGuardCallback is called when Steam requires a Guard code during login.
// The callback should return the 5-character alphanumeric code, or an error.
// It is invoked for both email-based and TOTP-based Steam Guard.
type SteamGuardCallback func() (string, error)

// InteractiveSteamGuard returns a SteamGuardCallback that prompts the user
// to type the Steam Guard code on standard input. Works for both email and
// TOTP guard when running interactively in a terminal.
func InteractiveSteamGuard() SteamGuardCallback {
	return func() (string, error) {
		fmt.Fprint(os.Stderr, "Steam Guard code: ")
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("steam guard: read input: %w", err)
			}
			return "", fmt.Errorf("steam guard: no input")
		}
		code := strings.TrimSpace(scanner.Text())
		if code == "" {
			return "", fmt.Errorf("steam guard: empty code entered")
		}
		return code, nil
	}
}

// SteamGuardCodeGenerate returns a SteamGuardCallback that automatically
// generates a Steam Guard code from a TOTP shared secret (mobile authenticator).
// sharedSecret is the base64-encoded secret shown during authenticator setup.
func SteamGuardCodeGenerate(sharedSecret string) SteamGuardCallback {
	return func() (string, error) {
		return totp.GenerateAuthCode(sharedSecret, time.Now())
	}
}

// UnknownSteamGuard returns a SteamGuardCallback that always returns an error.
// This is the default when no SteamGuardCallback is provided in Config, so that
// Steam Guard failures produce a clear message rather than hanging forever.
func UnknownSteamGuard() SteamGuardCallback {
	return func() (string, error) {
		return "", fmt.Errorf("steam guard required but no SteamGuardCallback specified in Config")
	}
}
