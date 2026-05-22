// Package oauth implements the Jira Data Center OAuth 2.0 provider flow:
// Authorization Code + PKCE for interactive login, and refresh-token exchange
// for non-interactive (CI/CD) use.
//
// It is a thin wrapper around golang.org/x/oauth2: that library handles the
// token exchange, PKCE challenge derivation, automatic refresh, and RFC 6749
// error parsing, while this package adds the pieces x/oauth2 does not provide —
// the Jira DC endpoints, a local callback server and browser launch, mapping
// of well-known errors to sentinels, and refresh-token rotation write-back.
//
// Jira DC quirk: a successful refresh invalidates BOTH the old access and
// refresh tokens and returns a new refresh_token, so the caller must persist
// the rotated token immediately.
package oauth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// Jira DC OAuth 2.0 provider endpoints.
const (
	authorizePath = "/rest/oauth2/latest/authorize"
	tokenPath     = "/rest/oauth2/latest/token" // #nosec G101 -- URL path, not a credential
)

// defaultHTTPTimeout bounds token/refresh requests when no client is supplied.
const defaultHTTPTimeout = 30 * time.Second

// Config holds the settings needed to talk to a Jira DC OAuth provider.
type Config struct {
	BaseURL      string       // e.g. "https://jira.example.com"
	ClientID     string       //
	ClientSecret string       //
	RedirectURI  string       // e.g. "http://127.0.0.1:8765/callback"
	Scopes       []string     // e.g. ["WRITE"]
	HTTPClient   *http.Client // optional; defaults to a 30s-timeout client
}

// Validate checks that the fields required to start a flow are present.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("oauth: nil config")
	}
	if c.BaseURL == "" {
		return errors.New("oauth: base URL is required")
	}
	if c.ClientID == "" {
		return errors.New("oauth: client ID is required")
	}
	if c.RedirectURI == "" {
		return errors.New("oauth: redirect URI is required")
	}
	if len(c.Scopes) == 0 {
		return errors.New("oauth: at least one scope is required")
	}
	return nil
}

// oauth2Config builds the x/oauth2 configuration for this Jira instance.
//
// AuthStyleInParams puts client_id/client_secret in the request body params,
// which is what the Jira DC provider expects, and avoids x/oauth2's
// auto-detect probe request.
func (c *Config) oauth2Config() *oauth2.Config {
	// Trim a trailing slash so a base URL entered as "https://jira.example.com/"
	// does not yield a double slash ("…com//rest/…") in the endpoint URLs.
	base := strings.TrimRight(c.BaseURL, "/")
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   base + authorizePath,
			TokenURL:  base + tokenPath,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		RedirectURL: c.RedirectURI,
		Scopes:      c.Scopes,
	}
}

// ctx returns a context carrying the configured HTTP client so x/oauth2 uses
// it for all token endpoint requests, applying a default timeout otherwise.
func (c *Config) ctx(parent context.Context) context.Context {
	hc := c.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return context.WithValue(parent, oauth2.HTTPClient, hc)
}
