package store

import (
	"os"
	"testing"
	"time"
)

func TestLocalCache_Session(t *testing.T) {
	dir := t.TempDir()
	c, err := NewLocalCache(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Initially empty.
	sess, err := c.LoadSession("alice")
	if err != nil || sess != nil {
		t.Fatalf("expected nil session, got %v %v", sess, err)
	}

	// Save and reload.
	want := &CachedSession{
		AccountName:  "alice",
		AccessToken:  "jwt.access.token",
		RefreshToken: "jwt.refresh.token",
		Expiry:       time.Now().Add(time.Hour),
	}
	if err := c.SaveSession("alice", want); err != nil {
		t.Fatal(err)
	}
	got, err := c.LoadSession("alice")
	if err != nil || got == nil {
		t.Fatalf("expected session, got %v %v", got, err)
	}
	if got.AccountName != want.AccountName || got.AccessToken != want.AccessToken {
		t.Fatalf("session mismatch: got %+v, want %+v", got, want)
	}

	// A second account doesn't overwrite the first.
	other := &CachedSession{
		AccountName:  "bob",
		AccessToken:  "jwt.bob.token",
		RefreshToken: "jwt.bob.refresh",
		Expiry:       time.Now().Add(time.Hour),
	}
	if err := c.SaveSession("bob", other); err != nil {
		t.Fatal(err)
	}
	got, err = c.LoadSession("alice")
	if err != nil || got == nil || got.AccessToken != want.AccessToken {
		t.Fatalf("alice session should be unchanged, got %v %v", got, err)
	}

	// Expired session returns nil.
	expired := &CachedSession{
		AccountName:  "alice",
		AccessToken:  "old.access.token",
		RefreshToken: "old.refresh.token",
		Expiry:       time.Now().Add(-time.Hour),
	}
	if err := c.SaveSession("alice", expired); err != nil {
		t.Fatal(err)
	}
	got, err = c.LoadSession("alice")
	if err != nil || got != nil {
		t.Fatalf("expected nil for expired session, got %v %v", got, err)
	}
}

func TestLocalCache_DepotKey(t *testing.T) {
	dir := t.TempDir()
	c, err := NewLocalCache(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Miss.
	k, err := c.LoadDepotKey(12345)
	if err != nil || k != nil {
		t.Fatalf("expected nil, got %v %v", k, err)
	}

	// Save and reload.
	key := CachedDepotKey{DepotID: 12345, Key: []byte("32bytekeyfordepot1234567890123")}
	if err := c.SaveDepotKey(key); err != nil {
		t.Fatal(err)
	}
	got, err := c.LoadDepotKey(12345)
	if err != nil || got == nil {
		t.Fatalf("expected key, got %v %v", got, err)
	}
	if string(got.Key) != string(key.Key) {
		t.Fatalf("key mismatch: got %v, want %v", got.Key, key.Key)
	}

	// A second different depot doesn't clobber the first.
	key2 := CachedDepotKey{DepotID: 99999, Key: []byte("anotherkey1234567890123456789012")}
	if err := c.SaveDepotKey(key2); err != nil {
		t.Fatal(err)
	}
	got, err = c.LoadDepotKey(12345)
	if err != nil || got == nil || string(got.Key) != string(key.Key) {
		t.Fatalf("first key should still be present, got %v %v", got, err)
	}
}

func TestLocalCache_Token(t *testing.T) {
	dir := t.TempDir()
	c, err := NewLocalCache(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Miss.
	tok, err := c.LoadToken("cdn.example.com", 100)
	if err != nil || tok != nil {
		t.Fatalf("expected nil, got %v %v", tok, err)
	}

	// Save and reload.
	want := CachedToken{
		Host:    "cdn.example.com",
		DepotID: 100,
		Token:   "auth_token_xyz",
		Expiry:  time.Now().Add(time.Hour),
	}
	if err := c.SaveToken(want); err != nil {
		t.Fatal(err)
	}
	got, err := c.LoadToken("cdn.example.com", 100)
	if err != nil || got == nil {
		t.Fatalf("expected token, got %v %v", got, err)
	}
	if got.Token != want.Token {
		t.Fatalf("token mismatch: got %q, want %q", got.Token, want.Token)
	}

	// Purge expired tokens.
	expired := CachedToken{
		Host:    "cdn.example.com",
		DepotID: 200,
		Token:   "expired_token",
		Expiry:  time.Now().Add(-time.Hour),
	}
	if err := c.SaveToken(expired); err != nil {
		t.Fatal(err)
	}
	if err := c.PurgeExpiredTokens(); err != nil {
		t.Fatal(err)
	}
	got, err = c.LoadToken("cdn.example.com", 200)
	if err != nil || got != nil {
		t.Fatalf("expected nil after purge, got %v %v", got, err)
	}
}

func TestLocalCache_TokenWithinSafetyWindowNotReturned(t *testing.T) {
	dir := t.TempDir()
	c, _ := NewLocalCache(dir)

	// A token expiring in 2 minutes is inside the 5-minute safety window and
	// should be treated as already gone.
	soon := CachedToken{Host: "h", DepotID: 1, Token: "x", Expiry: time.Now().Add(2 * time.Minute)}
	if err := c.SaveToken(soon); err != nil {
		t.Fatal(err)
	}
	got, err := c.LoadToken("h", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("token within 5-minute safety window should not be returned")
	}
}

func TestLocalCache_TokenReplacement(t *testing.T) {
	dir := t.TempDir()
	c, _ := NewLocalCache(dir)

	exp := time.Now().Add(time.Hour)
	_ = c.SaveToken(CachedToken{Host: "h", DepotID: 1, Token: "old", Expiry: exp})
	_ = c.SaveToken(CachedToken{Host: "h", DepotID: 1, Token: "new", Expiry: exp})

	got, _ := c.LoadToken("h", 1)
	if got == nil || got.Token != "new" {
		t.Errorf("expected replaced token 'new', got %v", got)
	}

	// Different depot is a separate entry.
	_ = c.SaveToken(CachedToken{Host: "h", DepotID: 2, Token: "other", Expiry: exp})
	if got, _ := c.LoadToken("h", 1); got == nil || got.Token != "new" {
		t.Error("saving depot 2 should not affect depot 1")
	}
}

func TestLocalCache_RejectedTokenCleared(t *testing.T) {
	// SaveSession then ClearSession then attempt re-auth path: cached token gone.
	dir := t.TempDir()
	c, _ := NewLocalCache(dir)
	_ = c.SaveSession("u", &CachedSession{AccountName: "u", AccessToken: "t", Expiry: time.Now().Add(time.Hour)})
	if err := c.ClearSession("u"); err != nil {
		t.Fatal(err)
	}
	got, err := c.LoadSession("u")
	if err != nil || got != nil {
		t.Errorf("session should be gone after clear: %v %v", got, err)
	}
	// Clearing a non-existent session is not an error.
	if err := c.ClearSession("nobody"); err != nil {
		t.Errorf("clearing absent session should be a no-op, got %v", err)
	}
}

func TestLocalCache_ClearSession(t *testing.T) {
	dir := t.TempDir()
	c, err := NewLocalCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.SaveSession("alice", &CachedSession{
		AccountName: "alice",
		AccessToken: "access-token",
		Expiry:      time.Now().Add(time.Hour),
	})
	if err := c.ClearSession("alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.sessionPath("alice")); !os.IsNotExist(err) {
		t.Fatal("session file should be deleted after ClearSession")
	}
}
