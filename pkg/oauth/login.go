package oauth

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"golang.org/x/oauth2"
)

// LoginResult is the outcome of a successful interactive login.
type LoginResult struct {
	Token  *oauth2.Token
	Scopes []string
}

// Login executes the full Authorization Code + PKCE flow: it starts a local
// callback server, opens the browser to the authorize URL, waits for the
// redirect, and exchanges the returned code for tokens. The caller is
// responsible for persisting the resulting token.
//
// port is the loopback callback port (must match the Config.RedirectURI), and
// timeout bounds how long to wait for the user to complete authorization.
func Login(
	ctx context.Context,
	cfg *Config,
	port int,
	timeout time.Duration,
) (*LoginResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	state, err := NewState()
	if err != nil {
		return nil, fmt.Errorf("oauth login: new state: %w", err)
	}
	verifier := NewVerifier()

	resultCh, shutdown, err := startCallbackServer(port, state)
	if err != nil {
		return nil, fmt.Errorf("oauth login: start callback server: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = shutdown(shutdownCtx)
	}()

	authURL := cfg.AuthorizeURL(state, verifier)

	fmt.Fprintln(os.Stderr, "\n👉 Opening browser to authorize go-jira...")
	fmt.Fprintln(os.Stderr, "   If the browser does not open, visit this URL manually:")
	fmt.Fprintln(os.Stderr, "  ", authURL)
	fmt.Fprintln(os.Stderr)
	if err := OpenBrowser(authURL); err != nil {
		slog.Warn("could not open browser; open the URL above manually", "error", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case res := <-resultCh:
		if res.Err != nil {
			return nil, res.Err
		}
		tok, err := cfg.ExchangeCode(waitCtx, res.Code, verifier)
		if err != nil {
			return nil, fmt.Errorf("oauth login: exchange code: %w", err)
		}
		return &LoginResult{Token: tok, Scopes: cfg.Scopes}, nil
	case <-waitCtx.Done():
		return nil, fmt.Errorf("oauth login: timed out after %s", timeout)
	}
}
