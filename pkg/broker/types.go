// Package broker implements the server side of the OAuth token refresh broker.
//
// Some Jira DC OAuth applications are confidential clients: their token endpoint
// requires a client_secret on the grant_type=refresh_token step. go-jira ships
// as a public PKCE client and must never embed that secret (a published binary
// would expose it). The broker is a small server-side service that holds the
// client_secret and performs only the secret-bearing refresh step on a client's
// behalf: the CLI sends its refresh_token, the broker adds the client_secret,
// forwards to Jira DC, and returns the rotated token pair.
//
// The broker does not persist tokens at rest. It keeps only a short-TTL
// in-memory result cache (with per-key request coalescing) to absorb the
// refresh-token rotation race Jira DC creates — a successful refresh
// invalidates the old refresh_token, so concurrent refreshes of the same token
// must collapse to a single upstream call and share the one rotated pair.
//
// This package deliberately does NOT import pkg/oauth. The refresh itself is
// supplied as a RefreshFunc so the only component that holds the secret and
// talks to Jira (pkg/oauth via the cmd wiring) stays in one place and there is
// no import cycle with pkg/oauth's client-side broker caller.
package broker

// RefreshPath is the broker's refresh endpoint path. The client appends it to
// the configured broker base URL; the server registers it on its mux.
const RefreshPath = "/v1/refresh"

// RefreshRequest is the JSON body of POST /v1/refresh.
//
// The broker uses its OWN configured base URL / client_id / client_secret; it
// never accepts an upstream endpoint from the caller. ClientID is optional: when
// present the broker verifies it matches its own client and rejects a mismatch,
// catching a client pointed at the wrong broker.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id,omitempty"`
}

// TokenResponse is the success body of POST /v1/refresh. It mirrors the standard
// OAuth2 token response shape so the client maps it onto an *oauth2.Token with
// minimal effort. ExpiresIn is recomputed per response from the cached token's
// absolute expiry, so a cache-served token reports its true remaining lifetime.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// ErrorResponse is the JSON error body, using OAuth2-style error codes so the
// client can map them back to its existing sentinel errors (e.g. invalid_grant
// → "run `go-jira login` again").
type ErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}
