package oauth

import (
	"context"

	"golang.org/x/oauth2"
)

// Refresh exchanges a refresh token for a new token pair.
//
// When BrokerURL is set the refresh is routed through the token refresh broker
// (which holds the confidential client_secret); otherwise it calls Jira DC
// directly as before. Either way the rotated pair is returned with the well-known
// failures mapped to this package's sentinel errors.
//
// ⚠️ On success Jira DC invalidates BOTH the old access token and the old
// refresh token and returns a new refresh_token in the response. Callers must
// persist the returned refresh token immediately or the next run will fail.
func (c *Config) Refresh(ctx context.Context, refreshToken string) (*oauth2.Token, error) {
	if c.BrokerURL != "" {
		return c.refreshViaBroker(ctx, refreshToken)
	}
	src := c.oauth2Config().TokenSource(c.ctx(ctx), &oauth2.Token{RefreshToken: refreshToken})
	tok, err := src.Token()
	if err != nil {
		return nil, mapError(err)
	}
	return tok, nil
}
