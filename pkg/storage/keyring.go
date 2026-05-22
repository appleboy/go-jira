package storage

import (
	"encoding/json"
	"errors"
	"fmt"

	keyring "github.com/zalando/go-keyring"
)

// keyringService is the service name under which entries are stored.
const keyringService = "go-jira"

// KeyringStore persists tokens in the OS keyring (Keychain, Secret Service,
// Credential Manager).
type KeyringStore struct{}

// Save serialises the token to JSON and stores it under key.
func (s *KeyringStore) Save(key string, token *StoredToken) error {
	b, err := json.Marshal(token) // #nosec G117 -- persisting the token is the point
	if err != nil {
		return fmt.Errorf("keyring marshal: %w", err)
	}
	if err := keyring.Set(keyringService, key, string(b)); err != nil {
		return fmt.Errorf("keyring set: %w", err)
	}
	return nil
}

// Load retrieves and decodes the token for key, mapping a missing entry to
// ErrTokenNotFound.
func (s *KeyringStore) Load(key string) (*StoredToken, error) {
	raw, err := keyring.Get(keyringService, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("keyring get: %w", err)
	}
	var t StoredToken
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return nil, fmt.Errorf("keyring unmarshal: %w", err)
	}
	return &t, nil
}

// Delete removes the entry for key. A missing entry is not an error.
func (s *KeyringStore) Delete(key string) error {
	if err := keyring.Delete(keyringService, key); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("keyring delete: %w", err)
	}
	return nil
}

// Backend reports "keyring".
func (s *KeyringStore) Backend() string { return backendKeyring }
