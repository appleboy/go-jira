package auth

import (
	"context"
	"errors"
	"fmt"
	"github/appleboy/go-jira/pkg/oauth"
	"github/appleboy/go-jira/pkg/storage"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// refreshThreshold is how long before expiry a token is proactively refreshed,
// so a request never goes out with a token about to expire mid-flight.
const refreshThreshold = 60 * time.Second

// OAuthAuthenticator authenticates requests with an OAuth access token,
// refreshing it as needed. It serves two modes:
//
//   - "oauth-storage": token loaded from local storage (interactive login).
//     Rotated tokens are written back to the Store.
//   - "oauth-env":     refresh token injected via env var (CI/CD). There is no
//     Store; rotated tokens are surfaced through OnRotate instead.
type OAuthAuthenticator struct {
	cfg      *oauth.Config
	store    storage.Store // nil in oauth-env mode
	storeKey string        // empty in oauth-env mode
	mode     string        // "oauth-storage" | "oauth-env"

	mu     sync.Mutex
	cached *storage.StoredToken

	// OnRotate, if set, is called whenever a refresh produces a new token.
	// It must not block; failures are logged, not fatal to the request.
	OnRotate func(newToken *storage.StoredToken) error
}

// Mode reports the configured OAuth mode.
func (a *OAuthAuthenticator) Mode() string { return a.mode }

// Validate ensures the authenticator is ready to make requests.
func (a *OAuthAuthenticator) Validate() error {
	if a.cfg == nil {
		return errors.New("oauth: nil config")
	}
	if a.cached == nil {
		return errors.New("oauth: no token loaded")
	}
	return nil
}

// Transport wraps base with a RoundTripper that injects (and refreshes) the
// access token.
func (a *OAuthAuthenticator) Transport(base http.RoundTripper) http.RoundTripper {
	return &oauthRoundTripper{auth: a, base: base}
}

// ensureFresh returns a valid access token, refreshing if it is at or near
// expiry. The caller must NOT hold a.mu.
func (a *OAuthAuthenticator) ensureFresh(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cached == nil {
		return "", errors.New("oauth: no cached token")
	}
	if time.Until(a.cached.ExpiresAt) > refreshThreshold {
		return a.cached.AccessToken, nil
	}
	return a.refreshLocked(ctx)
}

// forceRefresh refreshes regardless of expiry; used after a 401 response.
func (a *OAuthAuthenticator) forceRefresh(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.refreshLocked(ctx)
}

// refreshLocked performs the refresh, persists rotation, and updates the
// cache. a.mu must be held.
func (a *OAuthAuthenticator) refreshLocked(ctx context.Context) (string, error) {
	tok, err := a.cfg.Refresh(ctx, a.cached.RefreshToken)
	if err != nil {
		if errors.Is(err, oauth.ErrInvalidGrant) {
			return "", fmt.Errorf(
				"oauth: refresh token expired or revoked; run `go-jira login` again: %w",
				err,
			)
		}
		return "", err
	}

	refreshToken := tok.RefreshToken
	if refreshToken == "" {
		// Defensive: if the provider omits a new refresh token, keep the old.
		refreshToken = a.cached.RefreshToken
	}
	newTok := &storage.StoredToken{
		BaseURL:      a.cached.BaseURL,
		ClientID:     a.cached.ClientID,
		AccessToken:  tok.AccessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    tok.Expiry,
		ObtainedAt:   time.Now().UTC(),
		Scopes:       a.cached.Scopes,
	}

	if a.store != nil && a.storeKey != "" {
		if err := a.store.Save(a.storeKey, newTok); err != nil {
			return "", fmt.Errorf("oauth: persist rotated token: %w", err)
		}
	}
	if a.OnRotate != nil {
		if err := a.OnRotate(newTok); err != nil {
			slog.Warn("oauth: OnRotate hook failed", "error", err)
		}
	}

	a.cached = newTok
	return newTok.AccessToken, nil
}

// oauthRoundTripper injects the bearer token and retries once on a 401 after a
// forced refresh.
type oauthRoundTripper struct {
	auth *OAuthAuthenticator
	base http.RoundTripper
}

func (rt *oauthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	accessToken, err := rt.auth.ensureFresh(req.Context())
	if err != nil {
		return nil, err
	}

	// Clone so the caller's request is left unmodified (RoundTripper contract).
	req2 := req.Clone(req.Context())
	req2.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := rt.base.RoundTrip(req2)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	// 401: drain the body, force one refresh, and retry once.
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	newToken, err := rt.auth.forceRefresh(req.Context())
	if err != nil {
		return nil, fmt.Errorf("oauth: forced refresh after 401: %w", err)
	}
	req3 := req.Clone(req.Context())
	req3.Header.Set("Authorization", "Bearer "+newToken)
	return rt.base.RoundTrip(req3)
}
