package oauth

import (
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
)

// Sentinel errors for the well-known OAuth failure modes that callers act on.
//
// ErrInvalidGrant is the important one: it means the refresh token has expired
// or been revoked, so the user must run `go-jira login` again (or, in CI, the
// stored secret must be rotated).
var (
	ErrInvalidGrant  = errors.New("oauth: invalid_grant (refresh token expired or revoked)")
	ErrInvalidClient = errors.New(
		"oauth: invalid_client (check client_id, or the app may be a confidential " +
			"client that requires a secret — go-jira only supports public PKCE clients)")
	ErrServerError = errors.New("oauth: server error")
)

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
	case "invalid_grant":
		return fmt.Errorf("%w: %s", ErrInvalidGrant, re.ErrorDescription)
	case "invalid_client":
		return fmt.Errorf("%w: %s", ErrInvalidClient, re.ErrorDescription)
	}
	if re.Response != nil && re.Response.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("%w: %d", ErrServerError, re.Response.StatusCode)
	}
	return err
}
