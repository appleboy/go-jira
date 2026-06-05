package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/appleboy/go-jira/pkg/broker"
)

// brokerStub records what the broker received and replies with a canned status
// and body, standing in for a real broker so the client mapping can be tested.
type brokerStub struct {
	gotAuth     string
	gotClientID string
	gotRefresh  string
	status      int
	body        any
}

func newBrokerStub(t *testing.T, status int, body any) (*httptest.Server, *brokerStub) {
	t.Helper()
	st := &brokerStub{status: status, body: body}
	mux := http.NewServeMux()
	mux.HandleFunc(broker.RefreshPath, func(w http.ResponseWriter, r *http.Request) {
		st.gotAuth = r.Header.Get("Authorization")
		var req broker.RefreshRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		st.gotClientID = req.ClientID
		st.gotRefresh = req.RefreshToken
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(st.status)
		_ = json.NewEncoder(w).Encode(st.body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st
}

func TestRefreshViaBrokerSuccess(t *testing.T) {
	srv, st := newBrokerStub(t, http.StatusOK, broker.TokenResponse{
		AccessToken:  "access-new",
		RefreshToken: "refresh-new",
		ExpiresIn:    7200,
		TokenType:    "bearer",
	})

	cfg := &Config{
		BaseURL:     "https://jira.example.com",
		ClientID:    "client-abc",
		BrokerURL:   srv.URL,
		BrokerToken: "broker-secret",
	}
	tok, err := cfg.Refresh(context.Background(), "refresh-old")
	if err != nil {
		t.Fatalf("Refresh via broker: %v", err)
	}
	if tok.AccessToken != "access-new" || tok.RefreshToken != "refresh-new" {
		t.Errorf("token = %+v, want rotated pair", tok)
	}
	if rem := time.Until(tok.Expiry); rem < 119*time.Minute || rem > 121*time.Minute {
		t.Errorf("expiry remaining = %v, want ~120m from expires_in", rem)
	}

	// The client must forward its refresh token + client_id and the bearer token.
	if st.gotRefresh != "refresh-old" {
		t.Errorf("broker saw refresh_token %q, want refresh-old", st.gotRefresh)
	}
	if st.gotClientID != "client-abc" {
		t.Errorf("broker saw client_id %q, want client-abc", st.gotClientID)
	}
	if st.gotAuth != "Bearer broker-secret" {
		t.Errorf("broker saw Authorization %q, want Bearer broker-secret", st.gotAuth)
	}
}

func TestRefreshViaBrokerNoTokenOmitsAuth(t *testing.T) {
	srv, st := newBrokerStub(
		t,
		http.StatusOK,
		broker.TokenResponse{AccessToken: "a", RefreshToken: "r"},
	)
	cfg := &Config{ClientID: "client-abc", BrokerURL: srv.URL}
	if _, err := cfg.Refresh(context.Background(), "old"); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if st.gotAuth != "" {
		t.Errorf("Authorization = %q, want empty when no broker token set", st.gotAuth)
	}
}

func TestRefreshViaBrokerErrorMapping(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      broker.ErrorResponse
		wantIs    error  // sentinel the error must match, or nil
		wantInMsg string // substring expected when no sentinel applies
	}{
		{
			"invalid_grant",
			http.StatusBadRequest,
			broker.ErrorResponse{Error: "invalid_grant", ErrorDescription: "revoked"},
			ErrInvalidGrant,
			"",
		},
		{
			"invalid_client",
			http.StatusBadGateway,
			broker.ErrorResponse{Error: "invalid_client"},
			ErrInvalidClient,
			"",
		},
		{
			"upstream",
			http.StatusServiceUnavailable,
			broker.ErrorResponse{Error: "server_error"},
			ErrServerError,
			"",
		},
		{
			"caller auth",
			http.StatusUnauthorized,
			broker.ErrorResponse{Error: "unauthorized"},
			nil,
			"caller credential",
		},
		{
			"other bad request",
			http.StatusBadRequest,
			broker.ErrorResponse{Error: "invalid_request"},
			nil,
			"rejected",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newBrokerStub(t, tc.status, tc.body)
			cfg := &Config{ClientID: "client-abc", BrokerURL: srv.URL}
			_, err := cfg.Refresh(context.Background(), "old")
			if err == nil {
				t.Fatal("expected an error")
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("error %v, want errors.Is %v", err, tc.wantIs)
			}
			if tc.wantInMsg != "" && !strings.Contains(err.Error(), tc.wantInMsg) {
				t.Errorf("error %q, want substring %q", err.Error(), tc.wantInMsg)
			}
		})
	}
}
