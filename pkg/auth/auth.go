// Package auth provides a strategy abstraction over the different ways
// go-jira can authenticate against a Jira Data Center instance.
//
// Each authentication scheme implements [Authenticator], which knows how to
// wrap an http.RoundTripper so that outgoing requests carry the right
// credentials. [Resolve] inspects a [Config] and picks the appropriate
// implementation, encoding the priority order used across the CLI.
package auth

import "net/http"

// Auth mode identifiers returned by Authenticator.Mode and used for logging,
// resolution priority, and `config show`.
const (
	ModeBasic        = "basic"
	ModeBearer       = "bearer"
	ModeOAuthStorage = "oauth-storage"
	ModeOAuthEnv     = "oauth-env"
)

// Authenticator wraps an http.RoundTripper to inject auth credentials.
type Authenticator interface {
	// Transport returns a RoundTripper that adds auth on top of base.
	// Implementations must not mutate base.
	Transport(base http.RoundTripper) http.RoundTripper

	// Validate returns an error if the authenticator is not properly
	// configured. It is called before any network request.
	Validate() error

	// Mode returns a stable identifier (one of the Mode* constants).
	// It is used for logging and `config show`.
	Mode() string
}
