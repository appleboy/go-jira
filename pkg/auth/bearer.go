package auth

import (
	"errors"
	"net/http"

	jira "github.com/andygrunwald/go-jira"
)

// BearerAuth authenticates requests with a bearer token, e.g. a Jira
// Personal Access Token (PAT).
type BearerAuth struct {
	Token string
}

// Transport returns a RoundTripper that adds the bearer token on top of base.
// It delegates to the fork's jira.BearerAuthTransport so any extra handling
// there is preserved, and never mutates base.
func (a *BearerAuth) Transport(base http.RoundTripper) http.RoundTripper {
	return &jira.BearerAuthTransport{
		Token:     a.Token,
		Transport: base,
	}
}

// Validate ensures the token is present.
func (a *BearerAuth) Validate() error {
	if a.Token == "" {
		return errors.New("bearer auth: token is required")
	}
	return nil
}

// Mode reports the stable identifier "bearer".
func (a *BearerAuth) Mode() string { return "bearer" }
