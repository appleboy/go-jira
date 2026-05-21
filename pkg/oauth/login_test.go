package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// stubBrowserHittingCallback replaces browserCommand so that "opening the
// browser" instead extracts the state from the authorize URL and fires the
// OAuth redirect at the local callback server, simulating a user who approves.
// It returns an error so OpenBrowser never spawns a real process.
func stubBrowserHittingCallback(t *testing.T, port int, code string) {
	t.Helper()
	orig := browserCommand
	t.Cleanup(func() { browserCommand = orig })
	browserCommand = func(rawURL string) (string, []string, error) {
		u, err := url.Parse(rawURL)
		if err != nil {
			t.Errorf("parse authorize url: %v", err)
		}
		state := u.Query().Get("state")
		go getCallback(t, port, fmt.Sprintf("code=%s&state=%s", code, url.QueryEscape(state)))
		return "", nil, errors.New("browser stubbed")
	}
}

func TestLoginEndToEnd(t *testing.T) {
	srv := tokenServer(t, func(_ *testing.T, w http.ResponseWriter, form map[string][]string) {
		if got := form["grant_type"]; len(got) != 1 || got[0] != "authorization_code" {
			t.Errorf("grant_type = %v", got)
		}
		if got := form["code"]; len(got) != 1 || got[0] != "browser-code" {
			t.Errorf("code = %v, want [browser-code]", got)
		}
		writeToken(w, oauth2.Token{AccessToken: "access-final"}, "refresh-final")
	})
	defer srv.Close()

	port := freePort(t)
	cfg := testConfig(srv.URL)
	cfg.RedirectURI = fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	stubBrowserHittingCallback(t, port, "browser-code")

	res, err := Login(context.Background(), cfg, port, 5*time.Second)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.Token.AccessToken != "access-final" {
		t.Errorf("access token = %q, want access-final", res.Token.AccessToken)
	}
	if res.Token.RefreshToken != "refresh-final" {
		t.Errorf("refresh token = %q, want refresh-final", res.Token.RefreshToken)
	}
}

func TestLoginTimeout(t *testing.T) {
	port := freePort(t)
	cfg := testConfig("https://jira.example.com")
	cfg.RedirectURI = fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	// Browser stub that never triggers a callback.
	orig := browserCommand
	t.Cleanup(func() { browserCommand = orig })
	browserCommand = func(string) (string, []string, error) {
		return "", nil, errors.New("browser stubbed")
	}

	_, err := Login(context.Background(), cfg, port, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestLoginInvalidConfig(t *testing.T) {
	_, err := Login(context.Background(), &Config{}, 8765, time.Second)
	if err == nil {
		t.Fatal("expected validation error for empty config")
	}
}
