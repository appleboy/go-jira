package oauth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// NewState returns a random, URL-safe CSRF state value used to bind the
// authorize request to its callback.
func NewState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("oauth: read random state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
