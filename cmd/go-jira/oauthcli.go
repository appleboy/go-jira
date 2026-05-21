package main

import (
	"errors"
	"fmt"
	"github/appleboy/go-jira/pkg/oauth"
	"github/appleboy/go-jira/pkg/storage"
	"log/slog"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

// oauthLogin indirects oauth.Login so tests can stub the interactive flow.
var oauthLogin = oauth.Login

// requireBaseURL validates the base URL for non-run commands (which, unlike
// run, do not need a ref).
func requireBaseURL(config Config) error {
	if config.baseURL == "" {
		return errors.New("base_url is required")
	}
	return nil
}

// loadOAuthConfig runs the common preamble for OAuth subcommands: load the env
// file, resolve config, and require a base URL and an OAuth client ID.
func loadOAuthConfig(cmd *cobra.Command) (Config, error) {
	if err := loadEnvFromCmd(cmd); err != nil {
		return Config{}, err
	}
	config := loadConfig(cmd)
	if err := requireBaseURL(config); err != nil {
		return Config{}, err
	}
	if config.oauthClientID == "" {
		return Config{}, errors.New("OAuth client ID required: set " +
			envOAuthClientID + " or pass --client-id")
	}
	return config, nil
}

// rotationWriter returns an auth.OnRotate callback that writes the rotated
// refresh token to path (oauth-env / CI mode). When path is empty it warns
// that the rotated token will be lost — the CI secret then goes stale and the
// next run fails with invalid_grant — and returns nil (no callback).
func rotationWriter(path string) func(*storage.StoredToken) error {
	if path == "" {
		slog.Warn("oauth-env: " + envOAuthRefreshTokenOutput + " not set; the rotated " +
			"refresh token will be lost on exit and subsequent runs will fail until " +
			"you re-login locally and update the secret")
		return nil
	}
	return func(t *storage.StoredToken) error {
		return os.WriteFile(path, []byte(t.RefreshToken), 0o600)
	}
}

// resolveStoreQuiet resolves a token Store best-effort, returning nil (and
// logging at debug level) when no backend is available. Callers that merely
// want to attempt an oauth-storage lookup should not fail when, e.g., a
// headless CI box has no keyring and no master password.
func resolveStoreQuiet() storage.Store {
	store, err := storage.Resolve(storage.ResolveOptions{
		Password: os.Getenv(envMasterPassword),
	})
	if err != nil {
		slog.Debug("token storage unavailable", "error", err)
		return nil
	}
	return store
}

// resolveStore resolves a token Store and fails loudly. Used by commands that
// require storage (login/logout/token).
func resolveStore() (storage.Store, error) {
	store, err := storage.Resolve(storage.ResolveOptions{
		Password: os.Getenv(envMasterPassword),
	})
	if err != nil {
		return nil, fmt.Errorf("token storage: %w", err)
	}
	return store, nil
}

// oauthConfigFromConfig builds the protocol-layer oauth.Config from the CLI
// config, applying the insecure HTTP client when requested.
func oauthConfigFromConfig(config Config) *oauth.Config {
	return &oauth.Config{
		BaseURL:      config.baseURL,
		ClientID:     config.oauthClientID,
		ClientSecret: config.oauthClientSecret,
		RedirectURI:  config.redirectURI(),
		Scopes:       []string{config.scope},
		HTTPClient:   oauthHTTPClient(config),
	}
}

// oauthHTTPClient returns an HTTP client for OAuth token requests that honours
// the --insecure flag; nil lets oauth.Config use its default.
func oauthHTTPClient(config Config) *http.Client {
	if !config.insecure {
		return nil
	}
	return createHTTPClient(config, nil)
}
