package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/appleboy/go-jira/pkg/oauth"

	"github.com/spf13/cobra"
)

// Exit codes let callers (CI jobs, agents) classify a failure without parsing
// stderr. The taxonomy is intentionally small and stable; it is documented in
// the root command help and the README.
const (
	exitError     = 1 // generic runtime error
	exitUsage     = 2 // bad flags/arguments — the invocation itself is wrong
	exitAuth      = 3 // authentication / authorization failure (401/403)
	exitRateLimit = 4 // rate limited (HTTP 429); see retry_after in the error
)

// Stable machine-readable error kinds emitted in the structured stderr payload.
const (
	kindError     = "error"
	kindUsage     = "usage"
	kindAuth      = "auth"
	kindRateLimit = "rate_limit"
)

// cliError carries a classified failure: the process exit code, a stable
// machine-readable kind, and optional HTTP diagnostics surfaced for rate-limit
// and auth failures. It is what main turns into a structured stderr payload.
type cliError struct {
	code       int
	kind       string // stable token: error|usage|auth|rate_limit
	message    string
	hint       string // actionable next step surfaced to the caller
	statusCode int    // HTTP status when known (0 otherwise)
	retryAfter string // value of the Retry-After header when present
	err        error  // wrapped cause, for errors.Is/As
}

func (e *cliError) Error() string { return e.message }
func (e *cliError) Unwrap() error { return e.err }

// requestDiag records the most recent error-status HTTP response seen by the
// diagTransport during a single CLI invocation. The failing call is the last
// one made (errors propagate immediately), so the last recorded value is the
// one that explains the exit. Guarded by a mutex because http.Transport may use
// multiple goroutines.
type requestDiag struct {
	mu         sync.Mutex
	statusCode int
	retryAfter string
}

func (d *requestDiag) record(statusCode int, retryAfter string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.statusCode = statusCode
	d.retryAfter = retryAfter
}

func (d *requestDiag) snapshot() (statusCode int, retryAfter string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.statusCode, d.retryAfter
}

type diagCtxKey struct{}

// withDiag attaches a requestDiag to ctx so the HTTP transport can record the
// failing response out-of-band, where the command's returned error cannot reach.
func withDiag(ctx context.Context, d *requestDiag) context.Context {
	return context.WithValue(ctx, diagCtxKey{}, d)
}

func diagFrom(ctx context.Context) *requestDiag {
	if ctx == nil {
		return nil
	}
	d, _ := ctx.Value(diagCtxKey{}).(*requestDiag)
	return d
}

// diagTransport is a RoundTripper that records the status and Retry-After of any
// 4xx/5xx response into the requestDiag carried by the request context. It only
// reads headers (never the body), so it is safe to layer beneath the Jira
// client, which still consumes the body to build its own error.
type diagTransport struct {
	base http.RoundTripper
}

func (t *diagTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if resp != nil && resp.StatusCode >= http.StatusBadRequest {
		if d := diagFrom(req.Context()); d != nil {
			d.record(resp.StatusCode, resp.Header.Get("Retry-After"))
		}
	}
	return resp, err
}

// classify maps a command error into a cliError with an exit code. An error that
// already carries a code (a *cliError) wins; otherwise the HTTP diagnostics
// recorded by diagTransport are consulted, then the message is inspected for
// usage and auth signals. Everything else is a generic error (exit 1).
func classify(err error, diag *requestDiag) *cliError {
	if err == nil {
		return nil
	}

	var ce *cliError
	if errors.As(err, &ce) {
		return ce
	}

	msg := err.Error()

	if diag != nil {
		if sc, retryAfter := diag.snapshot(); sc != 0 {
			switch sc {
			case http.StatusTooManyRequests:
				return &cliError{
					code:       exitRateLimit,
					kind:       kindRateLimit,
					message:    msg,
					statusCode: sc,
					retryAfter: retryAfter,
					err:        err,
				}
			case http.StatusUnauthorized, http.StatusForbidden:
				return &cliError{
					code:       exitAuth,
					kind:       kindAuth,
					message:    msg,
					statusCode: sc,
					err:        err,
				}
			}
		}
	}

	if isUsageError(msg) {
		return &cliError{code: exitUsage, kind: kindUsage, message: msg, err: err}
	}
	// oauth.ErrInvalidGrant (refresh token expired or revoked) surfaces from a
	// direct refresh call with no HTTP diagnostics and no "auth ..." message
	// prefix, so match it by error identity to keep all auth classification here.
	if isAuthError(msg) || errors.Is(err, oauth.ErrInvalidGrant) {
		return &cliError{code: exitAuth, kind: kindAuth, message: msg, err: err}
	}
	return &cliError{code: exitError, kind: kindError, message: msg, err: err}
}

// usageErrorPrefixes are the leading strings Cobra uses for invocation errors
// (bad flags, missing required flags, unknown commands). Cobra does not type
// these, so prefix matching is the established way to detect them.
var usageErrorPrefixes = []string{
	"required flag",
	"unknown command",
	"unknown flag",
	"unknown shorthand flag",
	"invalid argument",
	"flag needs an argument",
	"accepts ",
	"invalid output format",
}

func isUsageError(msg string) bool {
	for _, p := range usageErrorPrefixes {
		if strings.HasPrefix(msg, p) {
			return true
		}
	}
	return false
}

func isAuthError(msg string) bool {
	return strings.Contains(msg, "auth resolution") ||
		strings.Contains(msg, "auth validation")
}

// errorEnvelope is the structured failure payload written to stderr. The
// optional HTTP fields are omitted unless populated (rate-limit / auth cases).
type errorEnvelope struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Kind       string `json:"kind"`
	Message    string `json:"message"`
	Hint       string `json:"hint,omitempty"`
	ExitCode   int    `json:"exit_code"`
	StatusCode int    `json:"status_code,omitempty"`
	RetryAfter string `json:"retry_after,omitempty"`
}

// emitError writes a single structured JSON error object to stderr so agents can
// classify a failure without scraping log lines. Rate-limit and auth failures
// include the HTTP status and any Retry-After hint.
func emitError(ce *cliError) {
	enc := json.NewEncoder(os.Stderr)
	enc.SetIndent("", "  ")
	_ = enc.Encode(errorEnvelope{Error: errorPayload{
		Kind:       ce.kind,
		Message:    ce.message,
		Hint:       ce.hint,
		ExitCode:   ce.code,
		StatusCode: ce.statusCode,
		RetryAfter: ce.retryAfter,
	}})
}

// addHint populates ce.hint with an actionable next step so callers (and
// agents) get a suggested fix alongside the failure. For an unknown command we
// surface cobra's nearest matches ("Did you mean ..."); other kinds get a
// stable, kind-specific instruction. An already-set hint is left untouched.
func addHint(ce *cliError, root *cobra.Command) {
	if ce == nil || ce.hint != "" {
		return
	}
	switch ce.kind {
	case kindUsage:
		if name := unknownCommandName(ce.message); name != "" && root != nil {
			// SuggestionsFor uses SuggestionsMinimumDistance, which cobra only
			// defaults to 2 as a side effect of Execute. Set it here so hint
			// generation does not depend on call ordering.
			if root.SuggestionsMinimumDistance <= 0 {
				root.SuggestionsMinimumDistance = 2
			}
			if s := root.SuggestionsFor(name); len(s) > 0 {
				ce.hint = fmt.Sprintf("Did you mean %q? Run %q to list all commands.",
					s[0], root.Name()+" --help")
				return
			}
		}
		ce.hint = fmt.Sprintf("Run %q for usage and examples.", rootName(root)+" --help")
	case kindAuth:
		name := rootName(root)
		if errors.Is(ce.err, oauth.ErrInvalidGrant) {
			// The refresh token itself is dead, so suggesting `token refresh`
			// would be self-referential (it is what just failed) and loop an
			// agent; the only recovery is a full re-login.
			ce.hint = fmt.Sprintf(
				"The refresh token has expired or been revoked; run %q to re-authenticate.",
				name+" login",
			)
			return
		}
		ce.hint = fmt.Sprintf(
			"OAuth access token may be expired: run %q to renew it from the saved "+
				"refresh token, then retry. If that fails (refresh token expired or "+
				"revoked) or no token is stored, run %q to re-authenticate. For "+
				"--token or basic auth, verify the base URL and credentials instead.",
			name+" token refresh", name+" login",
		)
	case kindRateLimit:
		ce.hint = "Wait for the duration in retry_after before retrying; requests are not retried automatically."
	default:
		ce.hint = fmt.Sprintf(
			"Run %q for the command's flags and examples.",
			rootName(root)+" <command> --help",
		)
	}
}

func rootName(root *cobra.Command) string {
	if root != nil {
		return root.Name()
	}
	return "go-jira"
}

// unknownCommandName extracts X from cobra's `unknown command "X" for "..."`
// message so we can compute spelling suggestions. Returns "" for other errors.
func unknownCommandName(msg string) string {
	const prefix = `unknown command "`
	if !strings.HasPrefix(msg, prefix) {
		return ""
	}
	rest := msg[len(prefix):]
	if name, _, ok := strings.Cut(rest, `"`); ok {
		return name
	}
	return ""
}
