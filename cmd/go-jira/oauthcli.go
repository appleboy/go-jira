package main

import (
	"fmt"
	"github/appleboy/go-jira/pkg/oauth"
	"github/appleboy/go-jira/pkg/storage"
	"log/slog"
	"net/http"
	"os"
)

// oauthLogin indirects oauth.Login so tests can stub the interactive flow.
var oauthLogin = oauth.Login

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
