package oauth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestRefreshSuccessRotatesToken(t *testing.T) {
	var gotForm map[string][]string
	srv := tokenServer(t, func(_ *testing.T, w http.ResponseWriter, form map[string][]string) {
		gotForm = form
		// Jira DC rotates the refresh token on every refresh.
		writeToken(w, oauth2.Token{AccessToken: "access-2"}, "refresh-2-rotated")
	})
	defer srv.Close()

	tok, err := testConfig(srv.URL).Refresh(context.Background(), "refresh-1-old")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if tok.AccessToken != "access-2" {
		t.Errorf("access token = %q, want access-2", tok.AccessToken)
	}
	if tok.RefreshToken != "refresh-2-rotated" {
		t.Errorf("rotated refresh token = %q, want refresh-2-rotated", tok.RefreshToken)
	}

	if got := gotForm["grant_type"]; len(got) != 1 || got[0] != "refresh_token" {
		t.Errorf("grant_type = %v, want [refresh_token]", got)
	}
	if got := gotForm["refresh_token"]; len(got) != 1 || got[0] != "refresh-1-old" {
		t.Errorf("refresh_token = %v, want [refresh-1-old]", got)
	}
	// The refresh path also hits the token endpoint: as a public PKCE client it
	// must never send a client_secret, even as an empty value.
	if got, ok := gotForm["client_secret"]; ok {
		t.Errorf("client_secret present in refresh request = %v, want absent", got)
	}
}

func TestRefreshInvalidGrant(t *testing.T) {
	srv := tokenServer(t, func(_ *testing.T, w http.ResponseWriter, _ map[string][]string) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "refresh token revoked")
	})
	defer srv.Close()

	_, err := testConfig(srv.URL).Refresh(context.Background(), "dead-token")
	if !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("error = %v, want errors.Is ErrInvalidGrant", err)
	}
}

// When the provider returns invalid_grant with no error_description, the mapped
// error must still match ErrInvalidGrant and must not end with a dangling ": ".
func TestRefreshInvalidGrantNoDescription(t *testing.T) {
	srv := tokenServer(t, func(_ *testing.T, w http.ResponseWriter, _ map[string][]string) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "")
	})
	defer srv.Close()

	_, err := testConfig(srv.URL).Refresh(context.Background(), "dead-token")
	if !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("error = %v, want errors.Is ErrInvalidGrant", err)
	}
	if strings.HasSuffix(err.Error(), ": ") {
		t.Errorf("error %q ends with a dangling %q", err.Error(), ": ")
	}
}
