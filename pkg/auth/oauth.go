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

	// OnRotate, if set, is called (outside a.mu) whenever a refresh produces a
	// new token. In oauth-storage mode the token is already saved to the Store,
	// so a hook failure is logged and non-fatal. In oauth-env mode the hook is
	// the only persistence for the rotated refresh token, so a failure is fatal
	// and propagated to the in-flight request (see notifyRotation).
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
	if a.cached == nil {
		a.mu.Unlock()
		return "", errors.New("oauth: no cached token")
	}
	if time.Until(a.cached.ExpiresAt) > refreshThreshold {
		tok := a.cached.AccessToken
		a.mu.Unlock()
		return tok, nil
	}
	tok, rotated, err := a.refreshLocked(ctx)
	a.mu.Unlock()
	if err != nil {
		return "", err
	}
	if err := a.notifyRotation(rotated); err != nil {
		return "", err
	}
	return tok, nil
}

// forceRefresh refreshes after a 401, unless another goroutine already rotated
// the token — i.e. the access token that hit the 401 is no longer the cached
// one — in which case the burst of concurrent 401s collapses to one refresh.
func (a *OAuthAuthenticator) forceRefresh(ctx context.Context, usedToken string) (string, error) {
	a.mu.Lock()
	if a.cached != nil && a.cached.AccessToken != usedToken {
		tok := a.cached.AccessToken
		a.mu.Unlock()
		return tok, nil
	}
	tok, rotated, err := a.refreshLocked(ctx)
	a.mu.Unlock()
	if err != nil {
		return "", err
	}
	if err := a.notifyRotation(rotated); err != nil {
		return "", err
	}
	return tok, nil
}

// refreshLocked performs the refresh, persists rotation to the Store, and
// updates the cache. a.mu must be held. It returns the rotated token (non-nil
// on success) so the caller can invoke OnRotate AFTER releasing the lock — the
// hook may block on I/O (e.g. writing a CI output file) and must never stall
// concurrent requests or risk re-entrant deadlock while the mutex is held.
func (a *OAuthAuthenticator) refreshLocked(
	ctx context.Context,
) (string, *storage.StoredToken, error) {
	tok, err := a.cfg.Refresh(ctx, a.cached.RefreshToken)
	if err != nil {
		if errors.Is(err, oauth.ErrInvalidGrant) {
			return "", nil, fmt.Errorf(
				"oauth: refresh token expired or revoked; run `go-jira login` again: %w",
				err,
			)
		}
		return "", nil, err
	}

	newTok := storage.NewStoredToken(
		a.cached.BaseURL, a.cached.ClientID, tok, a.cached.RefreshToken, a.cached.Scopes,
	)
	if a.store != nil && a.storeKey != "" {
		if err := a.store.Save(a.storeKey, newTok); err != nil {
			return "", nil, fmt.Errorf("oauth: persist rotated token: %w", err)
		}
	}
	a.cached = newTok
	return newTok.AccessToken, newTok, nil
}

// notifyRotation invokes the OnRotate hook (if any) for a rotated token. It is
// called outside a.mu.
//
// In oauth-env (CI) mode OnRotate is the ONLY persistence mechanism for the
// rotated refresh token, so a failure means the injected secret is now stale
// and the next run will fail with invalid_grant; the error is propagated so the
// pipeline fails fast (mirroring resolveOAuthEnv's initial-rotation handling).
// In oauth-storage mode the token was already saved to the Store under the
// lock, so an OnRotate failure is supplementary and only logged.
func (a *OAuthAuthenticator) notifyRotation(rotated *storage.StoredToken) error {
	if rotated == nil || a.OnRotate == nil {
		return nil
	}
	if err := a.OnRotate(rotated); err != nil {
		if a.mode == ModeOAuthEnv {
			return fmt.Errorf(
				"oauth-env: persist rotated refresh token (secret is now stale): %w", err)
		}
		slog.Warn("oauth: OnRotate hook failed", "error", err)
	}
	return nil
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

	newToken, err := rt.auth.forceRefresh(req.Context(), accessToken)
	if err != nil {
		return nil, fmt.Errorf("oauth: forced refresh after 401: %w", err)
	}

	req3 := req.Clone(req.Context())
	// The first attempt consumed req.Body; rewind it for the retry, or bail if
	// it cannot be replayed (otherwise the retry would send an empty payload).
	if req.Body != nil {
		if req.GetBody == nil {
			return nil, errors.New(
				"oauth: got 401 but request body cannot be rewound to retry")
		}
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("oauth: rewind request body for retry: %w", err)
		}
		req3.Body = body
	}
	req3.Header.Set("Authorization", "Bearer "+newToken)
	return rt.base.RoundTrip(req3)
}
