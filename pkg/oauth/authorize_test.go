package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"testing"
)

func TestAuthorizeURL(t *testing.T) {
	c := testConfig("https://jira.example.com")
	c.Scopes = []string{"READ", "WRITE"}
	verifier := NewVerifier()
	state := "state-123"

	raw := c.AuthorizeURL(state, verifier)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse authorize url: %v", err)
	}

	if got := u.Scheme + "://" + u.Host + u.Path; got != "https://jira.example.com"+authorizePath {
		t.Errorf("authorize endpoint = %q", got)
	}

	q := u.Query()
	checks := map[string]string{
		"client_id":             "client-abc",
		"redirect_uri":          "http://127.0.0.1:8765/callback",
		"response_type":         "code",
		"state":                 state,
		"scope":                 "READ WRITE", // space-separated, as Jira DC requires
		"code_challenge_method": "S256",
	}
	for k, want := range checks {
		if got := q.Get(k); got != want {
			t.Errorf("query %q = %q, want %q", k, got, want)
		}
	}

	// The challenge must be the base64url-no-pad SHA-256 of the verifier.
	sum := sha256.Sum256([]byte(verifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if got := q.Get("code_challenge"); got != wantChallenge {
		t.Errorf("code_challenge = %q, want %q", got, wantChallenge)
	}
}
