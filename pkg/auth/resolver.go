package auth

import "errors"

// Config carries everything Resolve needs to choose an Authenticator.
//
// The OAuth fields are reserved for Phase 4; in Phase 0 they are unused and
// safe to leave at their zero values.
type Config struct {
	// Basic / Bearer
	Username string
	Password string
	Token    string

	// OAuth (reserved for Phase 4; ignored for now).
	OAuthRefreshToken string
	OAuthBaseURL      string
	OAuthClientID     string
}

// Resolve picks the right Authenticator based on cfg.
//
// Priority (Phase 0): bearer > basic > error.
// Phase 4 will prepend oauth-env > oauth-storage.
func Resolve(cfg Config) (Authenticator, error) {
	if cfg.Token != "" {
		return &BearerAuth{Token: cfg.Token}, nil
	}
	if cfg.Username != "" && cfg.Password != "" {
		return &BasicAuth{Username: cfg.Username, Password: cfg.Password}, nil
	}
	return nil, errors.New("no authentication configured: set JIRA_TOKEN, " +
		"JIRA_USERNAME/JIRA_PASSWORD, or run `go-jira login`")
}
