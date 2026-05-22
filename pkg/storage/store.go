// Package storage persists OAuth tokens for the local (interactive) login
// flow. It offers two backends behind a single Store interface: the OS
// keyring (preferred) and an AES-256-GCM encrypted file (fallback when no
// keyring is available, e.g. a Linux CI box without D-Bus).
//
// Tokens are keyed by MakeKey(baseURL, clientID) so multiple Jira sites or
// OAuth clients can coexist without clobbering each other.
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// ErrTokenNotFound is returned by Load when no token exists for the key.
var ErrTokenNotFound = errors.New("storage: token not found")

// Backend identifiers reported by Store.Backend.
const (
	backendKeyring = "keyring"
	backendFile    = "file"
)

// StoredToken is the persisted form of an OAuth token plus the metadata needed
// to refresh it without re-running login.
type StoredToken struct {
	BaseURL      string    `json:"base_url"`
	ClientID     string    `json:"client_id"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	ObtainedAt   time.Time `json:"obtained_at"`
	Scopes       []string  `json:"scopes"`
}

// Store persists and retrieves tokens by key.
type Store interface {
	Save(key string, token *StoredToken) error
	Load(key string) (*StoredToken, error)
	Delete(key string) error
	Backend() string // "keyring" | "file"
}

// MakeKey derives a stable, opaque storage key from the Jira base URL and
// OAuth client ID, so different sites/clients never share an entry.
func MakeKey(baseURL, clientID string) string {
	// Normalize both inputs so cosmetically different but equivalent values
	// (e.g. a trailing slash, or surrounding whitespace from an env var / flag)
	// map to the same key and a stored token isn't seen as "missing" depending
	// on how it was entered.
	normBase := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	normClient := strings.TrimSpace(clientID)
	sum := sha256.Sum256([]byte(normBase + ":" + normClient))
	return hex.EncodeToString(sum[:])
}

// NewStoredToken builds a StoredToken from a freshly issued or refreshed
// oauth2 token. When the provider omits a new refresh token it carries
// prevRefresh forward, so a rotated entry never loses the ability to refresh.
func NewStoredToken(
	baseURL, clientID string,
	tok *oauth2.Token,
	prevRefresh string,
	scopes []string,
) *StoredToken {
	refresh := tok.RefreshToken
	if refresh == "" {
		refresh = prevRefresh
	}
	return &StoredToken{
		BaseURL:      baseURL,
		ClientID:     clientID,
		AccessToken:  tok.AccessToken,
		RefreshToken: refresh,
		ExpiresAt:    tok.Expiry,
		ObtainedAt:   time.Now().UTC(),
		Scopes:       scopes,
	}
}
