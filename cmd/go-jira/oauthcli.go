package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/appleboy/go-jira/pkg/oauth"
	"github.com/appleboy/go-jira/pkg/storage"

	"github.com/spf13/cobra"
)

// oauthLogin indirects oauth.Login so tests can stub the interactive flow.
var oauthLogin = oauth.Login

// oauthRequestTimeout bounds OAuth token/refresh requests made through the
// insecure HTTP client, matching pkg/oauth's default for the nil-client case.
const oauthRequestTimeout = 30 * time.Second

// requireBaseURL validates the base URL for non-run commands (which, unlike
// run, do not need a ref). It applies the same parse + scheme/--insecure rules
// as run via the shared validateBaseURL, so every subcommand rejects invalid
// or insecure URLs consistently.
func requireBaseURL(config Config) error {
	return validateBaseURL(config)
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
		return atomicWriteFile(path, []byte(t.RefreshToken))
	}
}

// atomicWriteFile writes data to path: it ensures the parent directory exists,
// writes to a temp file in the same directory, flushes it to disk (Sync), then
// renames it into place. The temp+rename is atomic (no torn write) and the Sync
// flushes the bytes before the rename, so an interrupted write can't leave a
// truncated/empty rotated refresh token (which would fail the next CI run with
// invalid_grant). The parent directory entry is not itself fsynced, so this is
// an atomic, data-flushed replace rather than a full power-loss guarantee.
//
// The file is created 0o600 since it always holds a secret (the rotated OAuth
// refresh token).
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".rotate-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	// os.Rename replaces an existing destination on every supported platform
	// (on Windows it maps to MoveFileEx with MOVEFILE_REPLACE_EXISTING), so a
	// repeated rotation overwrites the previous token file atomically.
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
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
		BaseURL:         config.baseURL,
		ClientID:        config.oauthClientID,
		RedirectURI:     config.redirectURI(),
		Scopes:          []string{config.scope},
		TLSCertFile:     config.callbackCert,
		TLSKeyFile:      config.callbackKey,
		GenerateTLSCert: config.callbackHTTPS,
		HTTPClient:      oauthHTTPClient(config),
		// Client-side broker routing for refresh. ClientSecret is deliberately
		// never set here: only the broker process holds the secret.
		BrokerURL:   config.brokerURL,
		BrokerToken: config.brokerToken,
	}
}

// oauthHTTPClient returns an HTTP client for OAuth token requests that honours
// the --insecure flag; nil lets oauth.Config use its default.
func oauthHTTPClient(config Config) *http.Client {
	if !config.insecure {
		return nil
	}
	client := createHTTPClient(config, nil)
	// createHTTPClient leaves Timeout unset (the main Jira client relies on the
	// command context). Injecting it into oauth.Config would otherwise bypass
	// pkg/oauth's default token-request timeout, so set the same 30s here.
	client.Timeout = oauthRequestTimeout
	return client
}
