package storage

import (
	"testing"
	"time"

	keyring "github.com/zalando/go-keyring"
)

// sampleToken returns a representative StoredToken for round-trip tests.
func sampleToken() *StoredToken {
	return &StoredToken{
		BaseURL:      "https://jira.example.com",
		ClientID:     "client-abc",
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second),
		ObtainedAt:   time.Now().UTC().Truncate(time.Second),
		Scopes:       []string{"WRITE"},
	}
}

func assertTokenEqual(t *testing.T, got, want *StoredToken) {
	t.Helper()
	if got.AccessToken != want.AccessToken ||
		got.RefreshToken != want.RefreshToken ||
		got.BaseURL != want.BaseURL ||
		got.ClientID != want.ClientID ||
		!got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Errorf("token mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

func TestKeyringStoreRoundTrip(t *testing.T) {
	keyring.MockInit()
	s := &KeyringStore{}
	if s.Backend() != "keyring" {
		t.Errorf("Backend() = %q, want keyring", s.Backend())
	}

	key := MakeKey("https://jira.example.com", "client-abc")
	tok := sampleToken()

	if err := s.Save(key, tok); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load(key)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	assertTokenEqual(t, got, tok)

	if err := s.Delete(key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Load(key); err != ErrTokenNotFound {
		t.Errorf("Load after delete = %v, want ErrTokenNotFound", err)
	}
}

func TestKeyringNotFoundMapped(t *testing.T) {
	keyring.MockInit()
	s := &KeyringStore{}
	if _, err := s.Load("does-not-exist"); err != ErrTokenNotFound {
		t.Errorf("Load missing = %v, want ErrTokenNotFound", err)
	}
	// Deleting a missing entry is a no-op, not an error.
	if err := s.Delete("does-not-exist"); err != nil {
		t.Errorf("Delete missing = %v, want nil", err)
	}
}
