package oauth

import (
	"context"

	"golang.org/x/oauth2"
)

// Refresh exchanges a refresh token for a new token pair.
//
// ⚠️ On success Jira DC invalidates BOTH the old access token and the old
// refresh token and returns a new refresh_token in the response. Callers must
// persist the returned refresh token immediately or the next run will fail.
func (c *Config) Refresh(ctx context.Context, refreshToken string) (*oauth2.Token, error) {
	src := c.oauth2Config().TokenSource(c.ctx(ctx), &oauth2.Token{RefreshToken: refreshToken})
	tok, err := src.Token()
	if err != nil {
		return nil, mapError(err)
	}
	return tok, nil
}
