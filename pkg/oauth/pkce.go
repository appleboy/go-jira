package oauth

import "golang.org/x/oauth2"

// NewVerifier generates a fresh PKCE code verifier (RFC 7636, 32 octets of
// randomness). Pass it to AuthorizeURL when starting a flow and to
// ExchangeCode when redeeming the code. A new verifier must be generated for
// each authorization.
func NewVerifier() string {
	return oauth2.GenerateVerifier()
}
