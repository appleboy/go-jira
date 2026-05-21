package auth

import (
	"errors"
	"net/http"

	jira "github.com/andygrunwald/go-jira"
)

// BasicAuth authenticates requests with HTTP Basic Authentication using a
// username and password.
type BasicAuth struct {
	Username string
	Password string
}

// Transport returns a RoundTripper that adds Basic Auth on top of base.
// It delegates to the fork's jira.BasicAuthTransport so any extra handling
// there is preserved, and never mutates base.
func (a *BasicAuth) Transport(base http.RoundTripper) http.RoundTripper {
	return &jira.BasicAuthTransport{
		Username:  a.Username,
		Password:  a.Password,
		Transport: base,
	}
}

// Validate ensures both credentials are present.
func (a *BasicAuth) Validate() error {
	if a.Username == "" {
		return errors.New("basic auth: username is required")
	}
	if a.Password == "" {
		return errors.New("basic auth: password is required")
	}
	return nil
}

// Mode reports the stable identifier "basic".
func (a *BasicAuth) Mode() string { return "basic" }
