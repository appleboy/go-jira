package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"github/appleboy/go-jira/pkg/oauth"
	"github/appleboy/go-jira/pkg/storage"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// memStore is an in-memory Store for tests.
type memStore struct {
	mu sync.Mutex
	m  map[string]*storage.StoredToken
}

func newMemStore() *memStore { return &memStore{m: map[string]*storage.StoredToken{}} }

func (s *memStore) Save(key string, t *storage.StoredToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = t
	return nil
}

func (s *memStore) Load(key string) (*storage.StoredToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.m[key]; ok {
		return t, nil
	}
	return nil, storage.ErrTokenNotFound
}

func (s *memStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

func (s *memStore) Backend() string { return "mem" }

// oauthMockServer is a mock Jira instance: a rotating token endpoint plus an
// API endpoint that accepts only the current access token.
type oauthMockServer struct {
	*httptest.Server
	refreshes atomic.Int32
	tokenErr  func(w http.ResponseWriter) bool // optional override; returns true if it handled the response
}

func newOAuthMockServer(t *testing.T) *oauthMockServer {
	t.Helper()
	s := &oauthMockServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/oauth2/latest/token", func(w http.ResponseWriter, _ *http.Request) {
		if s.tokenErr != nil && s.tokenErr(w) {
			return
		}
		n := s.refreshes.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  fmt.Sprintf("access-%d", n),
			"token_type":    "bearer",
			"expires_in":    7200,
			"refresh_token": fmt.Sprintf("refresh-%d", n),
		})
	})
	mux.HandleFunc("/rest/api/2/myself", func(w http.ResponseWriter, r *http.Request) {
		// Accept any access-N token; this endpoint is only used by the 401 test
		// via a custom handler, so default to OK.
		_ = r
		w.WriteHeader(http.StatusOK)
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func (s *oauthMockServer) config() *oauth.Config {
	return &oauth.Config{
		BaseURL:      s.URL,
		ClientID:     "client-abc",
		ClientSecret: "secret-xyz",
		RedirectURI:  "http://127.0.0.1:8765/callback",
		Scopes:       []string{"WRITE"},
	}
}

func storedToken(baseURL, access, refresh string, expiresAt time.Time) *storage.StoredToken {
	return &storage.StoredToken{
		BaseURL:      baseURL,
		ClientID:     "client-abc",
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresAt:    expiresAt,
		ObtainedAt:   time.Now().UTC(),
		Scopes:       []string{"WRITE"},
	}
}

func TestEnsureFreshRefreshesWhenExpired(t *testing.T) {
	srv := newOAuthMockServer(t)
	store := newMemStore()
	key := storage.MakeKey(srv.URL, "client-abc")
	store.m[key] = storedToken(srv.URL, "stale", "refresh-0", time.Now().Add(-time.Minute))

	a := &OAuthAuthenticator{
		cfg: srv.config(), store: store, storeKey: key,
		mode: "oauth-storage", cached: store.m[key],
	}

	got, err := a.ensureFresh(context.Background())
	if err != nil {
		t.Fatalf("ensureFresh: %v", err)
	}
	if got != "access-1" {
		t.Errorf("access token = %q, want access-1", got)
	}
	// Rotation must be written back to the store.
	saved, _ := store.Load(key)
	if saved.RefreshToken != "refresh-1" {
		t.Errorf("stored refresh token = %q, want refresh-1", saved.RefreshToken)
	}
	if srv.refreshes.Load() != 1 {
		t.Errorf("refresh count = %d, want 1", srv.refreshes.Load())
	}
}

func TestEnsureFreshSkipsWhenValid(t *testing.T) {
	srv := newOAuthMockServer(t)
	a := &OAuthAuthenticator{
		cfg:    srv.config(),
		mode:   "oauth-storage",
		cached: storedToken(srv.URL, "still-good", "refresh-0", time.Now().Add(time.Hour)),
	}
	got, err := a.ensureFresh(context.Background())
	if err != nil {
		t.Fatalf("ensureFresh: %v", err)
	}
	if got != "still-good" {
		t.Errorf("access token = %q, want still-good", got)
	}
	if srv.refreshes.Load() != 0 {
		t.Errorf("refresh count = %d, want 0", srv.refreshes.Load())
	}
}

func TestEnsureFreshConcurrentRefreshesOnce(t *testing.T) {
	srv := newOAuthMockServer(t)
	a := &OAuthAuthenticator{
		cfg:    srv.config(),
		mode:   "oauth-storage",
		cached: storedToken(srv.URL, "stale", "refresh-0", time.Now().Add(-time.Minute)),
	}

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.ensureFresh(context.Background()); err != nil {
				t.Errorf("ensureFresh: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := srv.refreshes.Load(); got != 1 {
		t.Errorf("refresh count = %d, want exactly 1", got)
	}
}

func TestRoundTripRetriesOn401(t *testing.T) {
	store := newMemStore()
	var refreshes atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/rest/oauth2/latest/token", func(w http.ResponseWriter, _ *http.Request) {
		refreshes.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-access", "token_type": "bearer",
			"expires_in": 7200, "refresh_token": "new-refresh",
		})
	})
	mux.HandleFunc("/rest/api/2/myself", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer new-access" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	key := storage.MakeKey(srv.URL, "client-abc")
	cfg := &oauth.Config{
		BaseURL: srv.URL, ClientID: "client-abc", ClientSecret: "secret-xyz",
		RedirectURI: "http://127.0.0.1:8765/callback", Scopes: []string{"WRITE"},
	}
	// Token is unexpired, so the 401 (not expiry) is what triggers the refresh.
	a := &OAuthAuthenticator{
		cfg: cfg, store: store, storeKey: key, mode: "oauth-storage",
		cached: storedToken(srv.URL, "old-access", "old-refresh", time.Now().Add(time.Hour)),
	}

	client := &http.Client{Transport: a.Transport(http.DefaultTransport)}
	req, _ := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		srv.URL+"/rest/api/2/myself",
		nil,
	)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 after forced refresh", resp.StatusCode)
	}
	if refreshes.Load() != 1 {
		t.Errorf("refresh count = %d, want 1", refreshes.Load())
	}
}

// TestRoundTripRetryPreservesBody guards against the bug where the 401 retry
// replays a request whose body was already consumed by the first attempt,
// sending an empty payload on the (write) retry.
func TestRoundTripRetryPreservesBody(t *testing.T) {
	var gotBody atomic.Value // string seen on the successful (retried) request

	mux := http.NewServeMux()
	mux.HandleFunc("/rest/oauth2/latest/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-access", "token_type": "bearer",
			"expires_in": 7200, "refresh_token": "new-refresh",
		})
	})
	mux.HandleFunc("/rest/api/2/issue/ABC-1/comment", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer new-access" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		gotBody.Store(string(body))
		w.WriteHeader(http.StatusCreated)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := &oauth.Config{
		BaseURL: srv.URL, ClientID: "client-abc", ClientSecret: "secret-xyz",
		RedirectURI: "http://127.0.0.1:8765/callback", Scopes: []string{"WRITE"},
	}
	a := &OAuthAuthenticator{
		cfg: cfg, store: newMemStore(), storeKey: storage.MakeKey(srv.URL, "client-abc"),
		mode:   ModeOAuthStorage,
		cached: storedToken(srv.URL, "old-access", "old-refresh", time.Now().Add(time.Hour)),
	}

	const payload = `{"body":"a comment"}`
	client := &http.Client{Transport: a.Transport(http.DefaultTransport)}
	req, _ := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		srv.URL+"/rest/api/2/issue/ABC-1/comment",
		strings.NewReader(payload),
	)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	if got := gotBody.Load(); got != payload {
		t.Errorf("retried request body = %q, want %q (body was not rewound)", got, payload)
	}
}

func TestRefreshInvalidGrantGuidesRelogin(t *testing.T) {
	srv := newOAuthMockServer(t)
	srv.tokenErr = func(w http.ResponseWriter) bool {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
		return true
	}
	a := &OAuthAuthenticator{
		cfg:    srv.config(),
		mode:   "oauth-storage",
		cached: storedToken(srv.URL, "stale", "dead-refresh", time.Now().Add(-time.Minute)),
	}
	_, err := a.ensureFresh(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "go-jira login") {
		t.Errorf("error %q should guide the user to run `go-jira login`", err)
	}
}
