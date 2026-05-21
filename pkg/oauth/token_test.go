package oauth

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"golang.org/x/oauth2"
)

func TestExchangeCodeSuccess(t *testing.T) {
	var gotForm map[string][]string
	srv := tokenServer(t, func(_ *testing.T, w http.ResponseWriter, form map[string][]string) {
		gotForm = form
		writeToken(w, oauth2.Token{AccessToken: "access-1"}, "refresh-1")
	})
	defer srv.Close()

	tok, err := testConfig(srv.URL).ExchangeCode(context.Background(), "the-code", "the-verifier")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if tok.AccessToken != "access-1" {
		t.Errorf("access token = %q, want access-1", tok.AccessToken)
	}
	if tok.RefreshToken != "refresh-1" {
		t.Errorf("refresh token = %q, want refresh-1", tok.RefreshToken)
	}

	// Jira DC accepts standard form-body token requests; verify the params
	// arrived in the POST body with the right grant + PKCE verifier.
	if got := gotForm["grant_type"]; len(got) != 1 || got[0] != "authorization_code" {
		t.Errorf("grant_type = %v, want [authorization_code]", got)
	}
	if got := gotForm["code"]; len(got) != 1 || got[0] != "the-code" {
		t.Errorf("code = %v, want [the-code]", got)
	}
	if got := gotForm["code_verifier"]; len(got) != 1 || got[0] != "the-verifier" {
		t.Errorf("code_verifier = %v, want [the-verifier]", got)
	}
	if got := gotForm["client_id"]; len(got) != 1 || got[0] != "client-abc" {
		t.Errorf("client_id = %v, want [client-abc]", got)
	}
}

func TestExchangeCodeErrors(t *testing.T) {
	tests := []struct {
		name      string
		handler   func(*testing.T, http.ResponseWriter, map[string][]string)
		wantIs    error
		wantErr   bool
		cancelCtx bool
	}{
		{
			name: "invalid_grant maps to sentinel",
			handler: func(_ *testing.T, w http.ResponseWriter, _ map[string][]string) {
				writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "code expired")
			},
			wantIs: ErrInvalidGrant,
		},
		{
			name: "invalid_client maps to sentinel",
			handler: func(_ *testing.T, w http.ResponseWriter, _ map[string][]string) {
				writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "bad secret")
			},
			wantIs: ErrInvalidClient,
		},
		{
			name: "server error maps to sentinel",
			handler: func(_ *testing.T, w http.ResponseWriter, _ map[string][]string) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`boom`))
			},
			wantIs: ErrServerError,
		},
		{
			name: "non-json success body errors",
			handler: func(_ *testing.T, w http.ResponseWriter, _ map[string][]string) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(`<html>not a token</html>`))
			},
			wantErr: true,
		},
		{
			name: "context cancel errors",
			handler: func(_ *testing.T, w http.ResponseWriter, _ map[string][]string) {
				writeToken(w, oauth2.Token{AccessToken: "x"}, "y")
			},
			wantErr:   true,
			cancelCtx: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tokenServer(t, tt.handler)
			defer srv.Close()

			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			_, err := testConfig(srv.URL).ExchangeCode(ctx, "c", "v")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("error = %v, want errors.Is %v", err, tt.wantIs)
			}
		})
	}
}
