package broker

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/oauth2"
)

// defaultCacheTTL bounds how long a rotated token pair is reused for the same
// refresh_token. It only needs to cover the brief window in which concurrent
// callers race on one (about-to-be-invalidated) refresh_token; it is NOT a
// general token cache. Kept short so a revoked token is not honoured for long.
const defaultCacheTTL = 60 * time.Second

// maxRequestBody caps the refresh request body to defend against oversized
// payloads; a refresh_token is small.
const maxRequestBody = 1 << 16 // 64 KiB

// codeServerError is the OAuth2-style error code returned for an unexpected
// (non-*APIError) failure.
const codeServerError = "server_error"

// RefreshFunc performs the secret-bearing refresh against Jira DC. The cmd layer
// wires oauth.Config.Refresh into it and translates oauth's sentinel errors into
// *APIError so the handler can pick the right HTTP status without this package
// importing pkg/oauth (which would create an import cycle).
type RefreshFunc func(ctx context.Context, refreshToken string) (*oauth2.Token, error)

// APIError lets the RefreshFunc dictate the HTTP status and OAuth2 error code the
// broker returns. A non-*APIError failure is reported as 500 server_error.
type APIError struct {
	Status int    // HTTP status to return
	Code   string // OAuth2-style error code for the JSON body
	Err    error  // underlying cause, for logging only (never sent to the client)
}

func (e *APIError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("broker: %s (status %d): %v", e.Code, e.Status, e.Err)
	}
	return fmt.Sprintf("broker: %s (status %d)", e.Code, e.Status)
}

func (e *APIError) Unwrap() error { return e.Err }

// Metrics is a snapshot of the broker's counters, exposed for tests and
// observability. Counters are cumulative since process start.
type Metrics struct {
	RefreshTotal  map[string]int64 // keyed by result: "success" | "error" | "unauthorized" | "bad_request"
	UpstreamCalls int64            // actual refresh calls forwarded to Jira DC
	CacheHits     int64            // requests served without an upstream call (TTL hit or coalesced)
}

// Server is the broker HTTP service. The zero value is not usable; construct it
// with NewServer.
type Server struct {
	refresh     RefreshFunc
	clientID    string // when set, an incoming request's client_id must match it
	callerToken string // optional caller bearer token; enforced only when non-empty
	// callerTokenSum is sha256(callerToken), precomputed once so the per-request
	// constant-time compare does not re-hash the (immutable) configured token.
	callerTokenSum [32]byte
	ready          func() error
	cache          *resultCache

	refreshSuccess      atomic.Int64
	refreshError        atomic.Int64
	refreshUnauthorized atomic.Int64
	refreshBadRequest   atomic.Int64
	upstreamCalls       atomic.Int64
	cacheHits           atomic.Int64
}

// Options configures a Server.
type Options struct {
	// Refresh performs the upstream refresh (required).
	Refresh RefreshFunc
	// ClientID, when set, is matched against a request's optional client_id; a
	// mismatch is rejected with 400. Empty disables the check.
	ClientID string
	// CallerToken, when set, is the bearer token callers must present. Empty
	// disables caller-token auth entirely (network-layer controls are the
	// primary defence — see the package and plan docs).
	CallerToken string
	// Ready, when set, is called by /readyz; a non-nil error reports not-ready.
	Ready func() error
}

// NewServer builds a Server from opts. Refresh must be non-nil. Logging goes to
// the slog default (configured by the CLI's setupLogging).
func NewServer(opts Options) (*Server, error) {
	if opts.Refresh == nil {
		return nil, errors.New("broker: Refresh function is required")
	}
	return &Server{
		refresh:        opts.Refresh,
		clientID:       opts.ClientID,
		callerToken:    opts.CallerToken,
		callerTokenSum: sha256.Sum256([]byte(opts.CallerToken)),
		ready:          opts.Ready,
		cache: &resultCache{
			ttl:      defaultCacheTTL,
			now:      time.Now,
			results:  map[string]cachedResult{},
			inflight: map[string]*inflightCall{},
		},
	}, nil
}

// Handler returns the broker's HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(RefreshPath, s.handleRefresh)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	return mux
}

// Metrics returns a snapshot of the broker's counters.
func (s *Server) Metrics() Metrics {
	return Metrics{
		RefreshTotal: map[string]int64{
			"success":      s.refreshSuccess.Load(),
			"error":        s.refreshError.Load(),
			"unauthorized": s.refreshUnauthorized.Load(),
			"bad_request":  s.refreshBadRequest.Load(),
		},
		UpstreamCalls: s.upstreamCalls.Load(),
		CacheHits:     s.cacheHits.Load(),
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	// Liveness must never touch the secret or upstream; always 200 if the
	// process is serving.
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok\n")
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	if s.ready != nil {
		if err := s.ready(); err != nil {
			slog.Warn("broker not ready", "error", err)
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ready\n")
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "")
		return
	}

	// Caller authentication (defence in depth; primary control is network-layer).
	// Enforced only when a caller token is configured. Constant-time compare.
	if !s.callerAuthOK(r) {
		s.refreshUnauthorized.Add(1)
		slog.Warn("broker refresh rejected: caller auth failed",
			"latency_ms", latencyMS(start))
		s.writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	var req RefreshRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxRequestBody)).Decode(&req); err != nil {
		s.refreshBadRequest.Add(1)
		s.writeError(w, http.StatusBadRequest, "invalid_request", "malformed JSON body")
		return
	}
	if req.RefreshToken == "" {
		s.refreshBadRequest.Add(1)
		s.writeError(w, http.StatusBadRequest, "invalid_request", "refresh_token is required")
		return
	}
	// Optional client_id check: reject a client pointed at the wrong broker.
	if req.ClientID != "" && s.clientID != "" && req.ClientID != s.clientID {
		s.refreshBadRequest.Add(1)
		s.writeError(w, http.StatusBadRequest, "invalid_client_id",
			"client_id does not match this broker")
		return
	}

	// Hash the token once: the full hash is the cache/coalescing key, and a short
	// prefix correlates logs without revealing the token.
	key := keyFor(req.RefreshToken)
	keyPrefix := key[:12]
	// The shared upstream call is detached from this request's cancellation: when
	// callers coalesce, the executor's disconnect must not abort the one upstream
	// refresh that every waiter depends on. The HTTP client's own timeout still
	// bounds it. Request values (if any) are preserved.
	refreshCtx := context.WithoutCancel(r.Context())
	tok, executed, err := s.cache.do(key, func() (*oauth2.Token, error) {
		return s.refresh(refreshCtx, req.RefreshToken)
	})
	switch {
	case executed:
		s.upstreamCalls.Add(1)
	case err == nil:
		// Served without an upstream call (TTL hit or coalesced success). A
		// coalesced *failure* is counted only as an error below, not a cache hit.
		s.cacheHits.Add(1)
	}

	if err != nil {
		s.refreshError.Add(1)
		status, code := classifyRefreshError(err)
		slog.Warn("broker refresh failed",
			"key", keyPrefix, "executed", executed, "status", status,
			"code", code, "latency_ms", latencyMS(start), "error", err)
		s.writeError(w, status, code, "")
		return
	}
	// A RefreshFunc must return a non-nil token on success. Guard the nil/nil case
	// defensively so a misbehaving Refresh reports a server error instead of
	// panicking tokenResponse here — which, via coalescing, would also take down
	// every waiter sharing this key.
	if tok == nil {
		s.refreshError.Add(1)
		slog.Warn("broker refresh returned no token and no error",
			"key", keyPrefix, "executed", executed, "latency_ms", latencyMS(start))
		s.writeError(w, http.StatusInternalServerError, codeServerError, "")
		return
	}

	resp := tokenResponse(tok)
	s.refreshSuccess.Add(1)
	slog.Info("broker refresh ok",
		"key", keyPrefix, "executed", executed, "status", http.StatusOK,
		"latency_ms", latencyMS(start))
	s.writeJSON(w, http.StatusOK, resp)
}

// callerAuthOK reports whether the request carries the configured caller token.
// When no caller token is configured the check is disabled and always passes
// (the primary control is the network layer; see the package docs).
func (s *Server) callerAuthOK(r *http.Request) bool {
	if s.callerToken == "" {
		return true
	}
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return false
	}
	got := strings.TrimPrefix(h, prefix)
	// Hash the presented token to a fixed length: subtle.ConstantTimeCompare
	// short-circuits when the byte-slice lengths differ, which would otherwise
	// leak the token length via timing. SHA-256 makes both operands 32 bytes so
	// the compare is genuinely length-independent. The want side (callerTokenSum)
	// is precomputed at construction since the configured token never changes.
	gotSum := sha256.Sum256([]byte(got))
	return subtle.ConstantTimeCompare(gotSum[:], s.callerTokenSum[:]) == 1
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	// Refresh responses carry OAuth token material; per RFC 6749 §5.1 the token
	// endpoint MUST forbid caching so no intermediary (reverse proxy, shared
	// cache) retains a token-bearing body. writeJSON serves every /v1/refresh
	// response (success and error), so setting it here covers them all.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Warn("broker: encode response", "error", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, code, desc string) {
	s.writeJSON(w, status, ErrorResponse{Error: code, ErrorDescription: desc})
}

// classifyRefreshError extracts the HTTP status and OAuth2 error code from an
// *APIError, defaulting to 500 server_error for any other failure.
func classifyRefreshError(err error) (int, string) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status, apiErr.Code
	}
	return http.StatusInternalServerError, codeServerError
}

// tokenResponse builds the wire response from a token, recomputing expires_in
// from the token's absolute expiry so a cache-served token reports its true
// remaining lifetime rather than its original (now-stale) one.
func tokenResponse(tok *oauth2.Token) TokenResponse {
	resp := TokenResponse{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
	}
	if !tok.Expiry.IsZero() {
		if rem := int64(time.Until(tok.Expiry).Seconds()); rem > 0 {
			resp.ExpiresIn = rem
		}
	}
	if scope, ok := tok.Extra("scope").(string); ok {
		resp.Scope = scope
	}
	return resp
}

func latencyMS(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}

// keyFor derives the cache/coalescing key from the refresh token. The full
// sha256 is the key; the plaintext token is never stored or logged.
func keyFor(refreshToken string) string {
	sum := sha256.Sum256([]byte(refreshToken))
	return hex.EncodeToString(sum[:])
}

// --- result cache + per-key request coalescing ("singleflight") ---

type cachedResult struct {
	tok       *oauth2.Token
	fetchedAt time.Time
}

type inflightCall struct {
	wg  sync.WaitGroup
	tok *oauth2.Token
	err error
}

// resultCache coalesces concurrent refreshes of the same key into one upstream
// call and reuses a successful result for a short TTL. It is purely in-memory,
// never persists, and runs no background goroutine. Expired entries are reclaimed
// two ways: a per-key freshness check on access (an expired hit is dropped and
// treated as a miss) and an amortized full sweep that runs at most once per TTL
// window. Live memory is bounded by the distinct tokens seen within ~one TTL
// window (it can grow during a burst of distinct tokens, but every entry is
// reclaimed within ~2×TTL).
type resultCache struct {
	ttl time.Duration
	now func() time.Time

	// onCoalesce, when non-nil, is invoked each time a caller joins an existing
	// in-flight call (just before it waits). It is a test seam used to gate the
	// executor until all waiters have provably coalesced, so concurrency tests are
	// deterministic without a timing-based sleep; it is nil (and free) in
	// production.
	onCoalesce func()

	mu        sync.Mutex
	results   map[string]cachedResult
	inflight  map[string]*inflightCall
	lastSwept time.Time // time of the last full eviction sweep; gates evictExpiredLocked
}

// do returns a token for key, calling fn at most once across concurrent callers
// of the same key and reusing a fresh cached result. executed is true only for
// the caller that actually invoked fn (i.e. made the upstream call).
func (c *resultCache) do(
	key string,
	fn func() (*oauth2.Token, error),
) (tok *oauth2.Token, executed bool, err error) {
	c.mu.Lock()
	c.evictExpiredLocked()
	if r, ok := c.results[key]; ok {
		// Per-key freshness check: the sweep above is amortized (runs at most
		// once per TTL), so an entry may still be present past its TTL — never
		// serve a stale result; drop it and fall through to a fresh refresh.
		if c.now().Sub(r.fetchedAt) < c.ttl {
			c.mu.Unlock()
			return r.tok, false, nil
		}
		delete(c.results, key)
	}
	if call, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		if c.onCoalesce != nil {
			c.onCoalesce()
		}
		call.wg.Wait()
		return call.tok, false, call.err
	}
	call := &inflightCall{}
	call.wg.Add(1)
	c.inflight[key] = call
	c.mu.Unlock()

	executed = true
	// The cleanup runs via defer so the inflight entry is always cleared and
	// waiters are always released — even if fn panics. Otherwise a single panic
	// would wedge this key forever and leak every coalesced (and future) waiter.
	// A recovered panic becomes call.err so waiters get a real error, not a nil
	// token that would nil-deref downstream.
	defer func() {
		if r := recover(); r != nil && call.err == nil {
			call.err = fmt.Errorf("broker: refresh panicked: %v", r)
		}
		c.mu.Lock()
		delete(c.inflight, key)
		// Only successful results are cached; a failure must not be served to the
		// next caller (e.g. a transient upstream error should be retried).
		if call.err == nil && call.tok != nil {
			c.results[key] = cachedResult{tok: call.tok, fetchedAt: c.now()}
		}
		c.mu.Unlock()
		call.wg.Done()
		// Propagate the result to this (executor) caller's named returns, covering
		// the panic path where the explicit return below never ran.
		tok, err = call.tok, call.err
	}()

	// Upstream call runs OUTSIDE the lock so other keys are not blocked; callers
	// of the SAME key wait on call.wg above.
	call.tok, call.err = fn()
	return call.tok, true, call.err
}

// evictExpiredLocked reclaims results past their TTL. c.mu must be held.
//
// The sweep is amortized: it walks the whole map at most once per TTL window
// (gated by lastSwept) rather than on every call, so the per-request hot path
// stays O(1) instead of O(len(results)) under the lock — otherwise a burst of
// distinct tokens would make every later request (even cache hits for unrelated
// keys) pay a full-map scan while holding the global lock. Correctness does not
// depend on the sweep cadence: do()'s per-key freshness check never serves an
// expired entry. Only reclamation of never-re-accessed keys relies on the sweep.
func (c *resultCache) evictExpiredLocked() {
	now := c.now()
	if now.Sub(c.lastSwept) < c.ttl {
		return
	}
	c.lastSwept = now
	for k, r := range c.results {
		if now.Sub(r.fetchedAt) >= c.ttl {
			delete(c.results, k)
		}
	}
}
