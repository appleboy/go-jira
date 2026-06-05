package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/appleboy/go-jira/pkg/broker"
	"github.com/appleboy/go-jira/pkg/oauth"
)

// jiraTokenPath is the Jira DC OAuth token endpoint path (mirrors the unexported
// constant in pkg/oauth).
const jiraTokenPath = "/rest/oauth2/latest/token"

// brokerSecret is the confidential client secret the stub broker is configured
// with; the stub Jira asserts the broker actually forwards it.
const brokerSecret = "s3cr3t"

// stubJira mimics a Jira DC confidential-client token endpoint: it rejects a
// refresh that arrives without client_secret (as the real DC does for go-jira's
// app), so a successful refresh proves the broker added the secret.
type stubJira struct {
	upstreamCalls atomic.Int64
	invalidGrant  bool
	block         chan struct{} // when non-nil, the handler blocks until closed
	started       chan struct{} // receives one value when the first request lands

	mu          sync.Mutex
	lastSecret  string
	lastRefresh string
}

func newStubJira(t *testing.T, j *stubJira) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(jiraTokenPath, func(w http.ResponseWriter, r *http.Request) {
		j.upstreamCalls.Add(1)
		if j.started != nil {
			select {
			case j.started <- struct{}{}:
			default:
			}
		}
		if j.block != nil {
			<-j.block
		}
		_ = r.ParseForm()
		j.mu.Lock()
		j.lastSecret = r.PostForm.Get("client_secret")
		j.lastRefresh = r.PostForm.Get("refresh_token")
		j.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if j.invalidGrant {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"revoked"}`))
			return
		}
		if r.PostForm.Get("client_secret") == "" {
			// Confidential client: refresh without the secret is invalid_client.
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-new",
			"token_type":    "bearer",
			"expires_in":    7200,
			"refresh_token": "refresh-new",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newBrokerFor stands up a real broker server (using the production
// brokerRefreshFunc wiring) pointed at jiraURL, holding the secret.
func newBrokerFor(t *testing.T, jiraURL, callerToken string) (*httptest.Server, *broker.Server) {
	t.Helper()
	oc := &oauth.Config{
		BaseURL:      jiraURL,
		ClientID:     "client-abc",
		ClientSecret: brokerSecret,
	}
	srv, err := broker.NewServer(broker.Options{
		Refresh:     brokerRefreshFunc(oc),
		ClientID:    "client-abc",
		CallerToken: callerToken,
	})
	if err != nil {
		t.Fatalf("broker.NewServer: %v", err)
	}
	bs := httptest.NewServer(srv.Handler())
	t.Cleanup(bs.Close)
	return bs, srv
}

// clientConfig builds the client-side oauth.Config through the production
// oauthConfigFromConfig wiring, so the test also proves the client never sets
// ClientSecret. brokerURL routes refresh through the broker.
func clientConfig(t *testing.T, brokerURL, brokerToken string) *oauth.Config {
	t.Helper()
	oc := oauthConfigFromConfig(Config{
		baseURL:       "https://jira.example.com",
		oauthClientID: "client-abc",
		scope:         defaultScope,
		callbackHTTPS: true,
		brokerURL:     brokerURL,
		brokerToken:   brokerToken,
	})
	if oc.ClientSecret != "" {
		t.Fatal("client oauth.Config must never carry a ClientSecret")
	}
	return oc
}

// TestBrokerE2EHappyPath: a client configured with a broker refreshes through it;
// the broker adds the secret and forwards the client's refresh token; the client
// receives the rotated pair with the correct expiry, via exactly one upstream call.
func TestBrokerE2EHappyPath(t *testing.T) {
	j := &stubJira{}
	jira := newStubJira(t, j)
	brokerSrv, srv := newBrokerFor(t, jira.URL, "")
	oc := clientConfig(t, brokerSrv.URL, "")

	tok, err := oc.Refresh(context.Background(), "refresh-old")
	if err != nil {
		t.Fatalf("refresh via broker: %v", err)
	}
	if tok.AccessToken != "access-new" || tok.RefreshToken != "refresh-new" {
		t.Errorf("token = %+v, want rotated pair (access-new/refresh-new)", tok)
	}
	if rem := time.Until(tok.Expiry); rem < 119*time.Minute || rem > 121*time.Minute {
		t.Errorf("expiry remaining = %v, want ~120m", rem)
	}
	if j.upstreamCalls.Load() != 1 {
		t.Errorf("upstream calls = %d, want 1", j.upstreamCalls.Load())
	}
	if j.lastSecret != brokerSecret {
		t.Errorf("Jira saw client_secret %q, want the broker to add %q", j.lastSecret, brokerSecret)
	}
	if j.lastRefresh != "refresh-old" {
		t.Errorf("Jira saw refresh_token %q, want the client's refresh-old", j.lastRefresh)
	}
	if m := srv.Metrics(); m.RefreshTotal["success"] != 1 || m.UpstreamCalls != 1 {
		t.Errorf("broker metrics = %+v, want 1 success / 1 upstream", m)
	}
}

// TestBrokerE2EConcurrentSameToken: N concurrent refreshes of the SAME refresh
// token collapse to one upstream call (singleflight) and all callers receive the
// same rotated pair with no invalid_grant.
func TestBrokerE2EConcurrentSameToken(t *testing.T) {
	j := &stubJira{block: make(chan struct{}), started: make(chan struct{}, 1)}
	jira := newStubJira(t, j)
	brokerSrv, srv := newBrokerFor(t, jira.URL, "")
	oc := clientConfig(t, brokerSrv.URL, "")

	const n = 12
	var wg sync.WaitGroup
	accessTokens := make([]string, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tok, err := oc.Refresh(context.Background(), "refresh-old")
			if err != nil {
				errs[i] = err
				return
			}
			accessTokens[i] = tok.AccessToken
		}(i)
	}

	<-j.started                        // the single executor has reached Jira
	time.Sleep(100 * time.Millisecond) // let the rest coalesce as inflight waiters
	close(j.block)
	wg.Wait()

	for i := range n {
		if errs[i] != nil {
			t.Errorf("caller %d errored: %v", i, errs[i])
		}
		if accessTokens[i] != "access-new" {
			t.Errorf("caller %d got %q, want access-new", i, accessTokens[i])
		}
	}
	if j.upstreamCalls.Load() != 1 {
		t.Errorf("upstream_call_count = %d, want 1", j.upstreamCalls.Load())
	}
	if m := srv.Metrics(); m.UpstreamCalls != 1 || m.CacheHits != n-1 {
		t.Errorf("broker metrics = %+v, want 1 upstream / %d cache hits", m, n-1)
	}
}

// TestBrokerE2EInvalidGrant: an expired refresh token surfaces as
// oauth.ErrInvalidGrant on the client, which the CLI classifies as an auth
// failure and hints to run `go-jira login` again.
func TestBrokerE2EInvalidGrant(t *testing.T) {
	j := &stubJira{invalidGrant: true}
	jira := newStubJira(t, j)
	brokerSrv, _ := newBrokerFor(t, jira.URL, "")
	oc := clientConfig(t, brokerSrv.URL, "")

	_, err := oc.Refresh(context.Background(), "dead-token")
	if !errors.Is(err, oauth.ErrInvalidGrant) {
		t.Fatalf("error = %v, want errors.Is oauth.ErrInvalidGrant", err)
	}

	// Mirror runTokenRefresh's wrapping and verify the CLI recovery hint.
	ce := classify(fmt.Errorf("refresh failed: %w", err), nil)
	if ce.code != exitAuth {
		t.Errorf("exit code = %d, want %d (auth)", ce.code, exitAuth)
	}
	addHint(ce, newRootCmd())
	if !strings.Contains(strings.ToLower(ce.hint), "login") {
		t.Errorf("hint = %q, want it to mention `login`", ce.hint)
	}
}

// TestBrokerE2ECallerAuth: when the broker requires a caller token, a request
// without it is rejected with no upstream call; the correct token succeeds.
func TestBrokerE2ECallerAuth(t *testing.T) {
	j := &stubJira{}
	jira := newStubJira(t, j)
	brokerSrv, srv := newBrokerFor(t, jira.URL, "broker-secret")

	// Missing caller token: rejected, broker never calls Jira.
	noToken := clientConfig(t, brokerSrv.URL, "")
	if _, err := noToken.Refresh(context.Background(), "refresh-old"); err == nil {
		t.Fatal("expected an error when broker requires a caller token")
	}
	if j.upstreamCalls.Load() != 0 {
		t.Errorf("upstream calls = %d on auth failure, want 0", j.upstreamCalls.Load())
	}
	if m := srv.Metrics(); m.RefreshTotal["unauthorized"] != 1 {
		t.Errorf("unauthorized metric = %d, want 1", m.RefreshTotal["unauthorized"])
	}

	// Correct caller token: succeeds.
	withToken := clientConfig(t, brokerSrv.URL, "broker-secret")
	tok, err := withToken.Refresh(context.Background(), "refresh-old")
	if err != nil {
		t.Fatalf("refresh with correct caller token: %v", err)
	}
	if tok.AccessToken != "access-new" {
		t.Errorf("access token = %q, want access-new", tok.AccessToken)
	}
}
