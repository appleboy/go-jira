package oauth

import (
	"context"

	"golang.org/x/oauth2"
)

// ExchangeCode exchanges an authorization code for an access/refresh token
// pair, supplying the PKCE verifier that matches the challenge sent to
// AuthorizeURL. Well-known OAuth errors are mapped to sentinel errors.
func (c *Config) ExchangeCode(
	ctx context.Context,
	code, verifier string,
) (*oauth2.Token, error) {
	tok, err := c.oauth2Config().Exchange(
		c.ctx(ctx),
		code,
		oauth2.VerifierOption(verifier),
	)
	if err != nil {
		return nil, mapError(err)
	}
	return tok, nil
}
