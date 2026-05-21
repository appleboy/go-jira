package main

import (
	"errors"
	"fmt"
	"github/appleboy/go-jira/pkg/util"
	"log/slog"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

// Config holds the application configuration.
type Config struct {
	baseURL      string
	username     string
	password     string
	token        string
	ref          string
	issuePattern string
	toTransition string
	resolution   string
	comment      string
	assignee     string
	insecure     bool
	markdown     bool
	debug        bool

	// OAuth
	oauthClientID           string
	oauthClientSecret       string
	oauthRefreshToken       string
	oauthRefreshTokenOutput string
	scope                   string
	callbackPort            int
}

// loadConfig resolves configuration from CLI flags (when explicitly set)
// falling back to environment variables via util.GetGlobalValue.
//
// The INPUT_<KEY> → <KEY> lookup order inside util.GetGlobalValue is preserved
// verbatim, so GitHub Actions (which sets INPUT_*) and local .env usage both
// keep working. Passing cmd == nil sends every action lookup straight to the
// environment, keeping tests and non-cobra callers working unchanged.
func loadConfig(cmd *cobra.Command) Config {
	getString := func(flagName, envKey string) string {
		if cmd != nil && cmd.Flags().Lookup(flagName) != nil && cmd.Flags().Changed(flagName) {
			v, _ := cmd.Flags().GetString(flagName)
			return v
		}
		return util.GetGlobalValue(envKey)
	}
	getBool := func(flagName, envKey string) bool {
		if cmd != nil && cmd.Flags().Lookup(flagName) != nil && cmd.Flags().Changed(flagName) {
			v, _ := cmd.Flags().GetBool(flagName)
			return v
		}
		return util.ToBool(util.GetGlobalValue(envKey))
	}

	cfg := Config{
		baseURL:      getString(flagBaseURL, "base_url"),
		insecure:     getBool(flagInsecure, "insecure"),
		username:     getString(flagUsername, "username"),
		password:     getString(flagPassword, "password"),
		token:        getString(flagToken, "token"),
		ref:          getString(flagRef, "ref"),
		issuePattern: getString(flagIssueFormat, "issue_format"),
		toTransition: getString(flagToTransition, "transition"),
		resolution:   getString(flagResolution, "resolution"),
		comment:      getString(flagComment, "comment"),
		assignee:     getString(flagAssignee, "assignee"),
		markdown:     getBool(flagMarkdown, "markdown"),
		debug:        getBool(flagDebug, "debug"),
	}

	// OAuth fields use fixed JIRA_-prefixed env vars (see main.go), not the
	// INPUT_/bare scheme.
	cfg.oauthClientID = resolveOAuthClientID(flagStringValue(cmd, flagClientID))
	cfg.oauthClientSecret = resolveOAuthClientSecret(flagStringValue(cmd, flagClientSecret))
	cfg.oauthRefreshToken = os.Getenv(envOAuthRefreshToken)
	cfg.oauthRefreshTokenOutput = os.Getenv(envOAuthRefreshTokenOutput)
	cfg.scope = defaultScope
	if v := flagStringValue(cmd, flagScope); v != "" {
		cfg.scope = v
	}
	cfg.callbackPort = defaultCallbackPort
	if v := flagIntValue(cmd, flagCallbackPort); v != 0 {
		cfg.callbackPort = v
	}

	warnOnSecretFlags(cmd)
	return cfg
}

// warnOnSecretFlags warns when secrets arrive via CLI flag — they leak into ps
// / /proc/<pid>/cmdline / shell history. Env vars and .env files don't.
func warnOnSecretFlags(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	for _, name := range []string{flagPassword, flagToken, flagClientSecret} {
		if cmd.Flags().Lookup(name) != nil && cmd.Flags().Changed(name) {
			slog.Warn(
				"passing secrets via CLI flag is unsafe on shared hosts; prefer env vars or .env",
				"flag", "--"+name,
			)
		}
	}
}

// flagStringValue returns a string flag's value when the command defines it and
// the user changed it, otherwise "".
func flagStringValue(cmd *cobra.Command, name string) string {
	if cmd == nil || cmd.Flags().Lookup(name) == nil || !cmd.Flags().Changed(name) {
		return ""
	}
	v, _ := cmd.Flags().GetString(name)
	return v
}

// flagIntValue returns an int flag's value when the command defines it,
// otherwise 0.
func flagIntValue(cmd *cobra.Command, name string) int {
	if cmd == nil || cmd.Flags().Lookup(name) == nil {
		return 0
	}
	v, _ := cmd.Flags().GetInt(name)
	return v
}

// resolveOAuthClientID resolves the client ID with precedence env > flag >
// embedded default (decision 3.2).
func resolveOAuthClientID(flagVal string) string {
	if v := os.Getenv(envOAuthClientID); v != "" {
		return v
	}
	if flagVal != "" {
		return flagVal
	}
	return DefaultOAuthClientID
}

// resolveOAuthClientSecret mirrors resolveOAuthClientID for the client secret.
func resolveOAuthClientSecret(flagVal string) string {
	if v := os.Getenv(envOAuthClientSecret); v != "" {
		return v
	}
	if flagVal != "" {
		return flagVal
	}
	return DefaultOAuthClientSecret
}

// redirectURI builds the loopback callback URL for the configured port.
func (c Config) redirectURI() string {
	return fmt.Sprintf("http://127.0.0.1:%d/callback", c.callbackPort)
}

// validateConfig validates the run-action configuration. Authentication
// selection (including OAuth) is handled by auth.Resolve; this only enforces
// the base URL, ref, and the basic-auth pairing rule.
func validateConfig(config Config) error {
	if config.baseURL == "" {
		return errors.New("base_url is required")
	}
	u, err := url.Parse(config.baseURL)
	if err != nil || u.Host == "" {
		return errors.New("base_url must be a valid URL")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !config.insecure {
			return errors.New("base_url must use https; pass --insecure=true to allow http")
		}
	default:
		return errors.New("base_url must use http or https scheme")
	}
	if config.ref == "" {
		return errors.New("ref is required")
	}
	if config.username != "" && config.password == "" {
		return errors.New("password is required when username is provided")
	}
	if config.password != "" && config.username == "" {
		return errors.New("username is required when password is provided")
	}
	return nil
}
