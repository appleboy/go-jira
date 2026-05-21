package auth

import (
	"context"
	"errors"
	"fmt"
	"github/appleboy/go-jira/pkg/oauth"
	"github/appleboy/go-jira/pkg/storage"
	"log/slog"
)

// Config carries everything Resolve needs to choose an Authenticator.
type Config struct {
	// Basic / Bearer
	Username string
	Password string
	Token    string

	// OAuth env-injection mode (CI/CD)
	OAuthRefreshToken string

	// OAuth common
	OAuthClientID     string
	OAuthClientSecret string
	OAuthBaseURL      string
	OAuthRedirectURI  string
	OAuthScopes       []string

	// Storage backend for the local (oauth-storage) flow; nil disables it.
	Store storage.Store

	// OnRotate, if set, is invoked with each rotated token (the persistence
	// policy — e.g. writing a CI output file — belongs to the caller, not here).
	OnRotate func(*storage.StoredToken) error
}

// Resolve picks the right Authenticator based on cfg, in priority order:
//
//  1. oauth-env     (JIRA_OAUTH_REFRESH_TOKEN present)
//  2. oauth-storage (a token for this base URL/client exists in storage)
//  3. bearer        (token / --token)
//  4. basic         (username + password)
func Resolve(ctx context.Context, cfg Config) (Authenticator, error) {
	if cfg.OAuthRefreshToken != "" {
		return resolveOAuthEnv(ctx, cfg)
	}
	if cfg.Store != nil && cfg.OAuthClientID != "" && cfg.OAuthBaseURL != "" {
		a, ok, err := tryResolveOAuthStorage(cfg)
		if err != nil {
			return nil, err
		}
		if ok {
			return a, nil
		}
	}
	if cfg.Token != "" {
		return &BearerAuth{Token: cfg.Token}, nil
	}
	if cfg.Username != "" && cfg.Password != "" {
		return &BasicAuth{Username: cfg.Username, Password: cfg.Password}, nil
	}
	return nil, errors.New("no authentication configured: run `go-jira login`, " +
		"set JIRA_TOKEN, or set JIRA_USERNAME/JIRA_PASSWORD")
}

// oauthConfig builds the protocol-layer config shared by both OAuth modes.
func oauthConfig(cfg Config) *oauth.Config {
	return &oauth.Config{
		BaseURL:      cfg.OAuthBaseURL,
		ClientID:     cfg.OAuthClientID,
		ClientSecret: cfg.OAuthClientSecret,
		RedirectURI:  cfg.OAuthRedirectURI,
		Scopes:       cfg.OAuthScopes,
	}
}

// tryResolveOAuthStorage looks up a stored token for the current base URL and
// client. ok is false (with nil error) when no token is stored, so the caller
// can fall through to the next auth method.
func tryResolveOAuthStorage(cfg Config) (Authenticator, bool, error) {
	key := storage.MakeKey(cfg.OAuthBaseURL, cfg.OAuthClientID)
	tok, err := cfg.Store.Load(key)
	if err != nil {
		if errors.Is(err, storage.ErrTokenNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("oauth-storage: load token: %w", err)
	}
	a := &OAuthAuthenticator{
		cfg:      oauthConfig(cfg),
		store:    cfg.Store,
		storeKey: key,
		mode:     ModeOAuthStorage,
		cached:   tok,
		OnRotate: cfg.OnRotate,
	}
	return a, true, nil
}

// resolveOAuthEnv builds an authenticator from an injected refresh token. It
// immediately exchanges the refresh token for an access token (which also
// rotates the refresh token) and, if configured, writes the rotated token to
// OAuthRefreshTokenOutput.
func resolveOAuthEnv(ctx context.Context, cfg Config) (Authenticator, error) {
	if cfg.OAuthClientID == "" {
		return nil, errors.New("oauth-env: JIRA_OAUTH_CLIENT_ID is required")
	}
	if cfg.OAuthClientSecret == "" {
		return nil, errors.New("oauth-env: JIRA_OAUTH_CLIENT_SECRET is required")
	}

	oc := oauthConfig(cfg)
	tok, err := oc.Refresh(ctx, cfg.OAuthRefreshToken)
	if err != nil {
		return nil, fmt.Errorf("oauth-env: initial refresh failed: %w", err)
	}

	cached := storage.NewStoredToken(
		cfg.OAuthBaseURL, cfg.OAuthClientID, tok, cfg.OAuthRefreshToken, cfg.OAuthScopes,
	)
	a := &OAuthAuthenticator{
		cfg:      oc,
		mode:     ModeOAuthEnv,
		cached:   cached,
		OnRotate: cfg.OnRotate,
	}
	// The initial refresh already rotated the token; surface it now so the
	// caller can persist it before the process exits.
	if cfg.OnRotate != nil {
		if err := cfg.OnRotate(cached); err != nil {
			slog.Warn("oauth-env: failed to persist initial rotated refresh token",
				"error", err)
		}
	}
	return a, nil
}
