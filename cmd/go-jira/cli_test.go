package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/appleboy/go-jira/pkg/oauth"
	"github.com/appleboy/go-jira/pkg/storage"

	jira "github.com/andygrunwald/go-jira"
	keyring "github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

// TestBareCommandShowsHelp verifies that invoking go-jira with no subcommand
// prints the help page (listing available commands) and exits without error.
func TestBareCommandShowsHelp(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected no error when no subcommand is given, got %v", err)
	}
	// Commands are organized into named groups (see newRootCmd's AddGroup),
	// so cobra emits the group titles instead of the generic "Available
	// Commands" heading. Assert on a grouped command listing instead.
	if !strings.Contains(out.String(), "whoami") {
		t.Errorf("expected help output to list available commands, got:\n%s", out.String())
	}
}

func TestSubcommandsRegistered(t *testing.T) {
	cmd := newRootCmd()
	want := map[string]bool{
		"run": false, "login": false, "logout": false,
		"whoami": false, "token": false, "config": false,
	}
	for _, c := range cmd.Commands() {
		want[c.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("subcommand %q not registered", name)
		}
	}
}

func TestLoginStoresToken(t *testing.T) {
	keyring.MockInit()
	t.Setenv(envOAuthClientID, "client-abc")

	orig := oauthLogin
	t.Cleanup(func() { oauthLogin = orig })
	oauthLogin = func(_ context.Context, _ *oauth.Config, _ int, _ time.Duration) (*oauth.LoginResult, error) {
		return &oauth.LoginResult{
			Token: &oauth2.Token{
				AccessToken:  "access-1",
				RefreshToken: "refresh-1",
				Expiry:       time.Now().Add(time.Hour),
			},
			Scopes: []string{"WRITE"},
		}, nil
	}

	cmd := newLoginCmd()
	if err := cmd.ParseFlags([]string{"--base-url=https://jira.example.com"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if err := runLogin(cmd); err != nil {
		t.Fatalf("runLogin: %v", err)
	}

	store := &storage.KeyringStore{}
	key := storage.MakeKey("https://jira.example.com", "client-abc")
	tok, err := store.Load(key)
	if err != nil {
		t.Fatalf("load stored token: %v", err)
	}
	if tok.AccessToken != "access-1" || tok.RefreshToken != "refresh-1" {
		t.Errorf("stored token = %+v, want access-1/refresh-1", tok)
	}
}

func TestLogoutDeletesToken(t *testing.T) {
	keyring.MockInit()
	t.Setenv(envOAuthClientID, "client-abc")

	store := &storage.KeyringStore{}
	key := storage.MakeKey("https://jira.example.com", "client-abc")
	if err := store.Save(key, &storage.StoredToken{AccessToken: "x"}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	cmd := newLogoutCmd()
	if err := cmd.ParseFlags([]string{"--base-url=https://jira.example.com"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if err := runLogout(cmd); err != nil {
		t.Fatalf("runLogout: %v", err)
	}
	if _, err := store.Load(key); err != storage.ErrTokenNotFound {
		t.Errorf("token still present after logout: %v", err)
	}
}

func TestWhoamiBearer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/2/myself" {
			_ = json.NewEncoder(w).Encode(jira.User{
				Name: "tester", DisplayName: "Tester", EmailAddress: "t@example.com",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	t.Setenv("INPUT_BASE_URL", srv.URL)
	t.Setenv("INPUT_TOKEN", "pat-123")
	t.Setenv("INPUT_INSECURE", "true") // httptest serves http://

	cmd := newWhoamiCmd()
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if err := runWhoami(cmd); err != nil {
		t.Fatalf("runWhoami: %v", err)
	}
}
