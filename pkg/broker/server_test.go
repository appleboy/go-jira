package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// okRefresh returns a static rotated token. Used where the refresh result is not
// the thing under test.
func okRefresh(_ context.Context, _ string) (*oauth2.Token, error) {
	return &oauth2.Token{
		AccessToken:  "access-new",
		RefreshToken: "refresh-new",
		TokenType:    "bearer",
		Expiry:       time.Now().Add(time.Hour),
	}, nil
}

func newTestServer(t *testing.T, opts Options) *Server {
	t.Helper()
	if opts.Refresh == nil {
		opts.Refresh = okRefresh
	}
	s, err := NewServer(opts)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return s
}

// postRefresh issues a POST /v1/refresh against the server's handler and returns
// the recorder.
func postRefresh(s *Server, body any, authz string) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequestWithContext(
		context.Background(), http.MethodPost, RefreshPath, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func decodeErr(t *testing.T, rec *httptest.ResponseRecorder) ErrorResponse {
	t.Helper()
	var er ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &er); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return er
}

func TestNewServerRequiresRefresh(t *testing.T) {
	if _, err := NewServer(Options{}); err == nil {
		t.Fatal("NewServer with nil Refresh should error")
	}
}

func TestRefreshHappyPath(t *testing.T) {
	s := newTestServer(t, Options{})
	rec := postRefresh(s, RefreshRequest{RefreshToken: "old"}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	var tr TokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &tr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if tr.AccessToken != "access-new" || tr.RefreshToken != "refresh-new" {
		t.Errorf("token response = %+v, want rotated pair", tr)
	}
	if tr.ExpiresIn <= 0 {
		t.Errorf("expires_in = %d, want > 0", tr.ExpiresIn)
	}
	if m := s.Metrics(); m.RefreshTotal["success"] != 1 || m.UpstreamCalls != 1 {
		t.Errorf("metrics = %+v, want 1 success / 1 upstream", m)
	}
}

func TestRefreshMethodNotAllowed(t *testing.T) {
	s := newTestServer(t, Options{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, RefreshPath, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestRefreshBadRequest(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"malformed", "{not json"},
		{"missing refresh_token", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var upstream atomic.Int64
			s := newTestServer(
				t,
				Options{Refresh: func(_ context.Context, _ string) (*oauth2.Token, error) {
					upstream.Add(1)
					return okRefresh(context.Background(), "")
				}},
			)
			req := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodPost,
				RefreshPath,
				bytes.NewReader([]byte(tc.body)),
			)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
			if upstream.Load() != 0 {
				t.Errorf("upstream called %d times, want 0 on bad request", upstream.Load())
			}
		})
	}
}

func TestRefreshClientIDMismatch(t *testing.T) {
	var upstream atomic.Int64
	s := newTestServer(t, Options{
		ClientID: "client-abc",
		Refresh: func(_ context.Context, _ string) (*oauth2.Token, error) {
			upstream.Add(1)
			return okRefresh(context.Background(), "")
		},
	})
	rec := postRefresh(s, RefreshRequest{RefreshToken: "old", ClientID: "wrong"}, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if upstream.Load() != 0 {
		t.Errorf("upstream called on client_id mismatch, want 0")
	}
}

func TestRefreshCallerAuth(t *testing.T) {
	var upstream atomic.Int64
	refresh := func(_ context.Context, _ string) (*oauth2.Token, error) {
		upstream.Add(1)
		return okRefresh(context.Background(), "")
	}
	s := newTestServer(t, Options{CallerToken: "broker-secret", Refresh: refresh})

	// Missing token → 401, no upstream call.
	rec := postRefresh(s, RefreshRequest{RefreshToken: "old"}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: status = %d, want 401", rec.Code)
	}
	// Wrong token → 401.
	rec = postRefresh(s, RefreshRequest{RefreshToken: "old"}, "Bearer nope")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: status = %d, want 401", rec.Code)
	}
	if upstream.Load() != 0 {
		t.Fatalf("upstream called %d times on auth failure, want 0", upstream.Load())
	}
	if m := s.Metrics(); m.RefreshTotal["unauthorized"] != 2 {
		t.Errorf("unauthorized metric = %d, want 2", m.RefreshTotal["unauthorized"])
	}

	// Correct token → 200, one upstream call.
	rec = postRefresh(s, RefreshRequest{RefreshToken: "old"}, "Bearer broker-secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("correct token: status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if upstream.Load() != 1 {
		t.Errorf("upstream called %d times, want 1", upstream.Load())
	}
}

func TestRefreshErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			"invalid_grant",
			&APIError{Status: http.StatusBadRequest, Code: "invalid_grant"},
			http.StatusBadRequest,
			"invalid_grant",
		},
		{
			"invalid_client",
			&APIError{Status: http.StatusBadGateway, Code: "invalid_client"},
			http.StatusBadGateway,
			"invalid_client",
		},
		{
			"upstream",
			&APIError{Status: http.StatusServiceUnavailable, Code: "server_error"},
			http.StatusServiceUnavailable,
			"server_error",
		},
		{
			"unexpected non-APIError",
			io.ErrUnexpectedEOF,
			http.StatusInternalServerError,
			"server_error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(
				t,
				Options{Refresh: func(_ context.Context, _ string) (*oauth2.Token, error) {
					return nil, tc.err
				}},
			)
			rec := postRefresh(s, RefreshRequest{RefreshToken: "old"}, "")
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if er := decodeErr(t, rec); er.Error != tc.wantCode {
				t.Errorf("error code = %q, want %q", er.Error, tc.wantCode)
			}
			if m := s.Metrics(); m.RefreshTotal["error"] != 1 {
				t.Errorf("error metric = %d, want 1", m.RefreshTotal["error"])
			}
		})
	}
}

func TestHealthzReadyz(t *testing.T) {
	s := newTestServer(t, Options{Ready: func() error { return nil }})
	for _, path := range []string{"/healthz", "/readyz"} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s status = %d, want 200", path, rec.Code)
		}
	}

	// readyz reports 503 when the readiness check fails.
	notReady := newTestServer(t, Options{Ready: func() error { return io.ErrUnexpectedEOF }})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	notReady.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("readyz (not ready) status = %d, want 503", rec.Code)
	}
}

// TestResultCacheCoalesces verifies that concurrent calls for the same key
// collapse into a single upstream call and all receive the same result, with
// exactly one caller reported as having executed it.
func TestResultCacheCoalesces(t *testing.T) {
	c := newResultCache(time.Minute, time.Now)
	var calls atomic.Int64
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	fn := func() (*oauth2.Token, error) {
		calls.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return &oauth2.Token{AccessToken: "a"}, nil
	}

	const n = 10
	var wg sync.WaitGroup
	tokens := make([]*oauth2.Token, n)
	executed := make([]bool, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tok, ex, err := c.do("k", fn)
			if err != nil {
				t.Errorf("do: %v", err)
			}
			tokens[i], executed[i] = tok, ex
		}(i)
	}

	<-started
	// Give the other goroutines time to register as inflight waiters before the
	// single executor completes.
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()

	if calls.Load() != 1 {
		t.Errorf("upstream calls = %d, want 1", calls.Load())
	}
	executedCount := 0
	for i := range n {
		if executed[i] {
			executedCount++
		}
		if tokens[i] == nil || tokens[i].AccessToken != "a" {
			t.Errorf("caller %d token = %+v, want access 'a'", i, tokens[i])
		}
	}
	if executedCount != 1 {
		t.Errorf("executed count = %d, want exactly 1", executedCount)
	}
}

// TestResultCacheTTLEviction verifies a result is reused within the TTL and a
// fresh upstream call happens once it expires.
func TestResultCacheTTLEviction(t *testing.T) {
	now := time.Unix(1000, 0)
	c := newResultCache(60*time.Second, func() time.Time { return now })
	var calls atomic.Int64
	fn := func() (*oauth2.Token, error) {
		calls.Add(1)
		return &oauth2.Token{AccessToken: "a"}, nil
	}

	if _, executed, _ := c.do("k", fn); !executed {
		t.Error("first call should execute fn")
	}
	if _, executed, _ := c.do("k", fn); executed {
		t.Error("second call within TTL should be served from cache")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d within TTL, want 1", calls.Load())
	}

	now = now.Add(61 * time.Second) // past TTL
	if _, executed, _ := c.do("k", fn); !executed {
		t.Error("call after TTL should execute fn again")
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d after TTL, want 2", calls.Load())
	}
}

// TestResultCachePanicReleasesWaiters verifies that a panic in fn does not
// deadlock coalesced waiters or wedge the key: all callers receive an error and
// a subsequent call for the same key can execute again.
func TestResultCachePanicReleasesWaiters(t *testing.T) {
	c := newResultCache(time.Minute, time.Now)
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	panicFn := func() (*oauth2.Token, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		panic("boom")
	}

	const n = 5
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, errs[i] = c.do("k", panicFn)
		}(i)
	}

	<-started
	time.Sleep(100 * time.Millisecond) // let the rest coalesce as waiters
	close(release)
	wg.Wait() // must not hang

	for i := range n {
		if errs[i] == nil {
			t.Errorf("caller %d got nil error, want the recovered panic error", i)
		}
	}

	// The key must not be wedged: a fresh call executes a healthy fn.
	tok, executed, err := c.do("k", func() (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "a"}, nil
	})
	if err != nil || tok == nil || !executed {
		t.Fatalf("key wedged after panic: tok=%v executed=%v err=%v", tok, executed, err)
	}
}

// TestRefreshCoalescedFailureMetrics verifies that when concurrent same-token
// refreshes coalesce onto a single FAILED upstream call, the upstream is hit
// once, the failure is not counted as a cache hit, and every caller is counted
// as an error.
func TestRefreshCoalescedFailureMetrics(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	var upstream atomic.Int64
	s := newTestServer(t, Options{
		Refresh: func(_ context.Context, _ string) (*oauth2.Token, error) {
			upstream.Add(1)
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
			return nil, &APIError{Status: http.StatusServiceUnavailable, Code: codeServerError}
		},
	})

	const n = 6
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			postRefresh(s, RefreshRequest{RefreshToken: "old"}, "")
		}()
	}

	<-started
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()

	m := s.Metrics()
	if upstream.Load() != 1 {
		t.Errorf("upstream calls = %d, want 1 (coalesced)", upstream.Load())
	}
	if m.UpstreamCalls != 1 {
		t.Errorf("UpstreamCalls = %d, want 1", m.UpstreamCalls)
	}
	if m.CacheHits != 0 {
		t.Errorf("CacheHits = %d, want 0 (coalesced failures are not cache hits)", m.CacheHits)
	}
	if m.RefreshTotal["error"] != n {
		t.Errorf("error metric = %d, want %d", m.RefreshTotal["error"], n)
	}
}

// newResultCache is a small test constructor mirroring the inline construction
// in NewServer.
func newResultCache(ttl time.Duration, now func() time.Time) *resultCache {
	return &resultCache{
		ttl:      ttl,
		now:      now,
		results:  map[string]cachedResult{},
		inflight: map[string]*inflightCall{},
	}
}
