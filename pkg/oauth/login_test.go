package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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

// TestLoginInvalidPort verifies Login rejects an out-of-range callback port
// (e.g. an explicit --callback-port=0) instead of trying to bind :0.
func TestLoginInvalidPort(t *testing.T) {
	cfg := testConfig("https://jira.example.com")
	cfg.RedirectURI = "http://127.0.0.1:0/callback"

	_, err := Login(context.Background(), cfg, 0, time.Second)
	if err == nil {
		t.Fatal("expected error for out-of-range callback port")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Errorf("error = %q, want an out-of-range message", err.Error())
	}
}

// TestLoginRedirectHostMismatch verifies Login rejects a redirect URI whose
// host is not the loopback address the callback server binds.
func TestLoginRedirectHostMismatch(t *testing.T) {
	cfg := testConfig("https://jira.example.com")
	cfg.RedirectURI = "http://localhost:8765/callback"

	_, err := Login(context.Background(), cfg, 8765, time.Second)
	if err == nil {
		t.Fatal("expected error when redirect URI host is not 127.0.0.1")
	}
	if !strings.Contains(err.Error(), "host must be 127.0.0.1") {
		t.Errorf("error = %q, want a host-mismatch message", err.Error())
	}
}

// TestLoginRedirectPathMismatch verifies Login rejects a redirect URI whose
// path is not the one the callback server serves.
func TestLoginRedirectPathMismatch(t *testing.T) {
	cfg := testConfig("https://jira.example.com")
	cfg.RedirectURI = "http://127.0.0.1:8765/wrong"

	_, err := Login(context.Background(), cfg, 8765, time.Second)
	if err == nil {
		t.Fatal("expected error when redirect URI path is not /callback")
	}
	if !strings.Contains(err.Error(), "path must be /callback") {
		t.Errorf("error = %q, want a path-mismatch message", err.Error())
	}
}

// TestLoginRedirectPortMismatch verifies Login fails fast (rather than hanging
// until timeout) when the callback port disagrees with the RedirectURI port.
func TestLoginRedirectPortMismatch(t *testing.T) {
	cfg := testConfig("https://jira.example.com")
	cfg.RedirectURI = "http://127.0.0.1:9999/callback"

	_, err := Login(context.Background(), cfg, 8765, time.Second)
	if err == nil {
		t.Fatal("expected error when redirect URI port does not match callback port")
	}
	if !strings.Contains(err.Error(), "does not match callback port") {
		t.Errorf("error = %q, want a port-mismatch message", err.Error())
	}
}
