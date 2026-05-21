package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var (
	Version string
	Commit  string

	// DefaultOAuthClientID and DefaultOAuthClientSecret are injected at build
	// time via -ldflags (see .goreleaser.yaml / Makefile). They are the
	// company-wide OAuth client baked into the binary; PKCE protects the actual
	// flow, so the secret is treated as a soft secret. Runtime resolution order
	// is env var > CLI flag > these embedded defaults.
	DefaultOAuthClientID     string
	DefaultOAuthClientSecret string
)

// Flag names shared between registration and lookup. Keeping them here avoids
// stringly-typed typo risk across files.
const (
	flagEnvFile      = "env-file"
	flagBaseURL      = "base-url"
	flagInsecure     = "insecure"
	flagUsername     = "username"
	flagPassword     = "password"
	flagToken        = "token"
	flagRef          = "ref"
	flagIssueFormat  = "issue-format"
	flagToTransition = "to-transition"
	flagResolution   = "resolution"
	flagComment      = "comment"
	flagAssignee     = "assignee"
	flagMarkdown     = "markdown"
	flagDebug        = "debug"

	// OAuth-related flags.
	flagClientID     = "client-id"
	flagClientSecret = "client-secret"
	flagCallbackPort = "callback-port"
	flagScope        = "scope"
	flagTimeout      = "timeout"
	flagConfirm      = "confirm"
)

// OAuth environment variables. Unlike the action config (which uses the
// INPUT_<KEY>/<KEY> GitHub Actions convention via util.GetGlobalValue), these
// use fixed JIRA_-prefixed names matching the documented CI/CD contract.
const (
	envOAuthClientID           = "JIRA_OAUTH_CLIENT_ID"
	envOAuthClientSecret       = "JIRA_OAUTH_CLIENT_SECRET"        //nolint:gosec // env var name, not a secret
	envOAuthRefreshToken       = "JIRA_OAUTH_REFRESH_TOKEN"        //nolint:gosec // env var name, not a secret
	envOAuthRefreshTokenOutput = "JIRA_OAUTH_REFRESH_TOKEN_OUTPUT" //nolint:gosec // env var name, not a secret
	envMasterPassword          = "JIRA_MASTER_PASSWORD"            //nolint:gosec // env var name, not a secret

	// JIRA_-prefixed aliases for the core auth/config fields, matching the env
	// naming used throughout the docs and the auth-resolver error message. The
	// action config still resolves these via the INPUT_<KEY>/<KEY> convention;
	// these are additional fallbacks (lowest precedence) so the documented
	// JIRA_* examples work as written.
	envBaseURL  = "JIRA_BASE_URL"
	envUsername = "JIRA_USERNAME"
	envPassword = "JIRA_PASSWORD" //nolint:gosec // env var name, not a secret
	envToken    = "JIRA_TOKEN"    //nolint:gosec // env var name, not a secret
	envInsecure = "JIRA_INSECURE"
)

const (
	defaultCallbackPort = 8765
	defaultScope        = "WRITE"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		slog.Error("execution failed", "error", err)
		os.Exit(1)
	}
}

// newRootCmd builds the root command and registers every subcommand. A fresh
// command is built on each call so tests get clean flag state.
//
// Running go-jira with no subcommand prints the help page. As of v1.0 (breaking
// change) the previous bare-command action behavior now lives under
// `go-jira run`.
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "go-jira",
		Short:         "Jira integration CLI with OAuth, Basic, and Bearer auth",
		SilenceUsage:  true,
		SilenceErrors: false,
		Version:       fmt.Sprintf("%s Commit: %s", Version, Commit),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.SetVersionTemplate("Version: {{.Version}}\n")

	cmd.AddCommand(
		newRunCmd(),
		newLoginCmd(),
		newLogoutCmd(),
		newWhoamiCmd(),
		newTokenCmd(),
		newConfigCmd(),
	)
	return cmd
}

// addCommonFlags registers flags shared by all subcommands that talk to Jira.
func addCommonFlags(cmd *cobra.Command) {
	cmd.Flags().String(flagEnvFile, ".env", "Read in a file of environment variables")
	cmd.Flags().String(flagBaseURL, "", "Jira base URL (env: BASE_URL / INPUT_BASE_URL)")
	cmd.Flags().
		Bool(flagInsecure, false, "Skip TLS verification (env: INSECURE / INPUT_INSECURE)")
	cmd.Flags().Bool(flagDebug, false, "Dump resolved configuration (env: DEBUG / INPUT_DEBUG)")
}

// addOAuthFlags registers the OAuth client flags shared by login and run.
func addOAuthFlags(cmd *cobra.Command) {
	cmd.Flags().String(flagClientID, "", "OAuth client ID (env: "+envOAuthClientID+")")
	cmd.Flags().
		String(flagClientSecret, "", "OAuth client secret (env: "+envOAuthClientSecret+")")
	cmd.Flags().Int(flagCallbackPort, defaultCallbackPort, "Local OAuth callback port")
	cmd.Flags().String(flagScope, defaultScope, "OAuth scope to request")
}

// loadEnvFile resolves and loads an env file, logging the absolute path that
// was loaded. An explicitly-passed --env-file that is missing is a hard error;
// the default .env is silently skipped when absent.
func loadEnvFile(envfile string, explicit bool) error {
	if envfile == "" {
		return nil
	}
	abs, err := filepath.Abs(envfile)
	if err != nil {
		return fmt.Errorf("resolve env file path: %w", err)
	}
	info, statErr := os.Stat(abs)
	if statErr != nil || info.IsDir() {
		if explicit {
			return fmt.Errorf("env file not found: %s", abs)
		}
		return nil
	}
	if err := godotenv.Load(abs); err != nil {
		return fmt.Errorf("load env file %s: %w", abs, err)
	}
	slog.Info("loaded env file", "path", abs)
	return nil
}

// loadEnvFromCmd loads the env file referenced by cmd's --env-file flag.
func loadEnvFromCmd(cmd *cobra.Command) error {
	envfile := ".env"
	explicit := false
	if cmd != nil {
		if cmd.Flags().Lookup(flagEnvFile) != nil {
			envfile, _ = cmd.Flags().GetString(flagEnvFile)
			explicit = cmd.Flags().Changed(flagEnvFile)
		}
	}
	return loadEnvFile(envfile, explicit)
}
