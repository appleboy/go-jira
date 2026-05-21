package oauth

import "golang.org/x/oauth2"

// AuthorizeURL builds the URL the user's browser is sent to in order to grant
// access. The S256 PKCE challenge is derived from verifier; state binds the
// request to its callback. Scopes are space-separated by x/oauth2, as Jira DC
// requires.
func (c *Config) AuthorizeURL(state, verifier string) string {
	return c.oauth2Config().AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}
