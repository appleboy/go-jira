package main

import (
	"context"
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

	// Accept the JIRA_-prefixed env vars as aliases (lowest precedence: flag >
	// INPUT_<KEY>/<KEY> > JIRA_<KEY>), so the JIRA_* examples in the docs and
	// the auth-resolver error message work as written.
	if cfg.baseURL == "" {
		cfg.baseURL = os.Getenv(envBaseURL)
	}
	if cfg.username == "" {
		cfg.username = os.Getenv(envUsername)
	}
	if cfg.password == "" {
		cfg.password = os.Getenv(envPassword)
	}
	if cfg.token == "" {
		cfg.token = os.Getenv(envToken)
	}
	if !cfg.insecure && !flagChanged(cmd, flagInsecure) &&
		util.GetGlobalValue("insecure") == "" {
		cfg.insecure = util.ToBool(os.Getenv(envInsecure))
	}

	// OAuth fields use fixed JIRA_-prefixed env vars (see main.go), not the
	// INPUT_/bare scheme.
	cfg.oauthClientID = resolveWithEnv(
		envOAuthClientID, flagStringValue(cmd, flagClientID), DefaultOAuthClientID,
	)
	cfg.oauthClientSecret = resolveWithEnv(
		envOAuthClientSecret, flagStringValue(cmd, flagClientSecret), DefaultOAuthClientSecret,
	)
	cfg.oauthRefreshToken = os.Getenv(envOAuthRefreshToken)
	cfg.oauthRefreshTokenOutput = os.Getenv(envOAuthRefreshTokenOutput)
	cfg.scope = defaultScope
	if v := flagStringValue(cmd, flagScope); v != "" {
		cfg.scope = v
	}
	// Gate on whether the flag was changed (not on a non-zero value) so an
	// explicit --callback-port=0 is preserved and later rejected by oauth.Login
	// rather than being silently replaced with the default.
	cfg.callbackPort = defaultCallbackPort
	if flagChanged(cmd, flagCallbackPort) {
		cfg.callbackPort = flagIntValue(cmd, flagCallbackPort)
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

// flagChanged reports whether the command defines the flag and the user
// explicitly set it.
func flagChanged(cmd *cobra.Command, name string) bool {
	return cmd != nil && cmd.Flags().Lookup(name) != nil && cmd.Flags().Changed(name)
}

// cmdContext returns the command's context so cancellation/deadlines (Ctrl-C,
// parent context) propagate to outbound calls, falling back to a fresh
// background context for the cmd == nil test path (or a command that has not
// been executed and so has no context set).
func cmdContext(cmd *cobra.Command) context.Context {
	if cmd != nil {
		if ctx := cmd.Context(); ctx != nil {
			return ctx
		}
	}
	return context.Background()
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

// resolveWithEnv applies the env > flag > embedded-default precedence used for
// the OAuth client ID and secret.
func resolveWithEnv(envKey, flagVal, embedded string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	if flagVal != "" {
		return flagVal
	}
	return embedded
}

// redirectURI builds the loopback callback URL for the configured port.
func (c Config) redirectURI() string {
	return fmt.Sprintf("http://127.0.0.1:%d/callback", c.callbackPort)
}

// validateBaseURL enforces the base URL rules shared by every subcommand: it
// must be present, parse as a URL with a host, and use https (or http only
// when --insecure is set). Extracted so non-run commands (login/logout/whoami/
// token/config show) reject invalid or insecure URLs up front with the same
// actionable errors as run.
func validateBaseURL(config Config) error {
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
	return nil
}

// validateConfig validates the run-action configuration. Authentication
// selection (including OAuth) is handled by auth.Resolve; this only enforces
// the base URL, ref, and the basic-auth pairing rule.
func validateConfig(config Config) error {
	if err := validateBaseURL(config); err != nil {
		return err
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
