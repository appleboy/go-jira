package oauth

import (
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
)

// RFC 6749 error codes this package recognizes, shared by the direct-path error
// mapping (mapError) and the broker-path mapping (mapBrokerError).
const (
	codeInvalidGrant  = "invalid_grant"
	codeInvalidClient = "invalid_client"
)

// Sentinel errors for the well-known OAuth failure modes that callers act on.
//
// ErrInvalidGrant is the important one: it means the refresh token has expired
// or been revoked, so the user must run `go-jira login` again (or, in CI, the
// stored secret must be rotated).
var (
	ErrInvalidGrant  = errors.New("oauth: invalid_grant (refresh token expired or revoked)")
	ErrInvalidClient = errors.New(
		"oauth: invalid_client (check client_id; if the app is a confidential " +
			"client that requires a secret on refresh, route refresh through a token " +
			"refresh broker by setting JIRA_TOKEN_BROKER_URL)")
	ErrServerError = errors.New("oauth: server error")
	// ErrBrokerUnauthorized means the token refresh broker rejected the caller's
	// own credential (the JIRA_BROKER_TOKEN bearer was missing or wrong). It is a
	// 401-class auth failure — distinct from a Jira-side auth failure — so the CLI
	// classifies it as an auth error (exit 3) on every refresh path by identity,
	// rather than relying on the wrapping message.
	ErrBrokerUnauthorized = errors.New("oauth: broker rejected the caller credential")
)

// wrapWithDesc wraps an OAuth sentinel with the provider's error_description, or
// returns the bare sentinel when no description is supplied so the message never
// ends with a dangling ": ". Shared by the direct (mapError) and broker
// (mapBrokerError) paths so both produce identical, clean CLI output.
func wrapWithDesc(sentinel error, desc string) error {
	if desc == "" {
		return sentinel
	}
	return fmt.Errorf("%w: %s", sentinel, desc)
}

// mapError translates x/oauth2's *oauth2.RetrieveError into our sentinel
// errors where the RFC 6749 'error' code is one we act on, and surfaces 5xx
// responses as ErrServerError. Other errors pass through unchanged.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	var re *oauth2.RetrieveError
	if !errors.As(err, &re) {
		return err
	}
	switch re.ErrorCode {
	case codeInvalidGrant:
		return wrapWithDesc(ErrInvalidGrant, re.ErrorDescription)
	case codeInvalidClient:
		return wrapWithDesc(ErrInvalidClient, re.ErrorDescription)
	}
	if re.Response != nil && re.Response.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("%w: %d", ErrServerError, re.Response.StatusCode)
	}
	return err
}
