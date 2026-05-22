package main

import (
	"fmt"
	"github/appleboy/go-jira/pkg/auth"
	"github/appleboy/go-jira/pkg/storage"
	"github/appleboy/go-jira/pkg/util"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// sourceEnv is the SOURCE column value reported when a config value was
// resolved from an environment variable.
const sourceEnv = "env"

// newConfigCmd builds the `config` command group.
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "config",
		Short:        "Inspect resolved configuration",
		SilenceUsage: true,
	}
	cmd.AddCommand(newConfigShowCmd())
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "show",
		Short:        "Show the resolved configuration and where each value came from",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigShow(cmd)
		},
	}
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	cmd.Flags().String(flagToken, "", "Jira API token")
	cmd.Flags().String(flagUsername, "", "Jira username")
	cmd.Flags().String(flagPassword, "", "Jira password")
	return cmd
}

func runConfigShow(cmd *cobra.Command) error {
	if err := loadEnvFromCmd(cmd); err != nil {
		return err
	}
	config := loadConfig(cmd)

	w := tabwriter.NewWriter(os.Stderr, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "FIELD\tVALUE\tSOURCE")

	row := func(field, value, flagName, envKey, aliasEnv string) {
		fmt.Fprintf(w, "%s\t%s\t%s\n", field, redactIfSecret(field, value),
			configSource(cmd, flagName, envKey, aliasEnv))
	}

	row("base_url", config.baseURL, flagBaseURL, "base_url", envBaseURL)
	row("insecure", fmt.Sprintf("%t", config.insecure), flagInsecure, "insecure", envInsecure)
	row("token", config.token, flagToken, "token", envToken)
	row("username", config.username, flagUsername, "username", envUsername)
	row("password", config.password, flagPassword, "password", envPassword)
	fmt.Fprintf(w, "oauth_client_id\t%s\t%s\n",
		redactIfSecret("oauth_client_id", config.oauthClientID),
		oauthValueSource(cmd, flagClientID, envOAuthClientID, config.oauthClientID,
			DefaultOAuthClientID))
	fmt.Fprintf(w, "oauth_client_secret\t%s\t%s\n",
		redactIfSecret("oauth_client_secret", config.oauthClientSecret),
		oauthValueSource(cmd, flagClientSecret, envOAuthClientSecret,
			config.oauthClientSecret, DefaultOAuthClientSecret))
	row("scope", config.scope, flagScope, "", "")
	// oauth-env (CI) inputs: show presence/source without leaking the token.
	fmt.Fprintf(w, "oauth_refresh_token\t%s\t%s\n",
		redactIfSecret("oauth_refresh_token", config.oauthRefreshToken),
		configSource(cmd, "", "", envOAuthRefreshToken))
	fmt.Fprintf(w, "oauth_refresh_token_output\t%s\t%s\n",
		redactIfSecret("oauth_refresh_token_output", config.oauthRefreshTokenOutput),
		configSource(cmd, "", "", envOAuthRefreshTokenOutput))
	fmt.Fprintf(w, "auth_mode\t%s\t%s\n", detectAuthMode(config), "resolved")

	return w.Flush()
}

// storedTokenExists reports whether an OAuth token is persisted for this base
// URL/client, used to mirror the resolver's oauth-storage decision without any
// network I/O.
func storedTokenExists(config Config) bool {
	// Both the base URL and client ID form the storage key; without either there
	// is nothing to look up, so skip the keyring/file access entirely and avoid
	// misleading auth_mode output.
	if config.oauthClientID == "" || config.baseURL == "" {
		return false
	}
	store := resolveStoreQuiet()
	if store == nil {
		return false
	}
	_, err := store.Load(storage.MakeKey(config.baseURL, config.oauthClientID))
	return err == nil
}

// oauthValueSource reports where an OAuth value came from. Unlike configSource
// it knows the fixed JIRA_OAUTH_* env vars and the build-time embedded default,
// so the SOURCE column never misreports an env-supplied or embedded value as
// "default/unset". embedded may be "" when there is no build-time default.
//
// The env check precedes the flag check to mirror resolveWithEnv's actual
// precedence (env > flag > embedded): when both an env var and a flag are set,
// the env var wins, so the reported source must say "env".
func oauthValueSource(cmd *cobra.Command, flagName, envKey, value, embedded string) string {
	if envKey != "" && os.Getenv(envKey) != "" {
		return sourceEnv
	}
	if flagChanged(cmd, flagName) {
		return "flag"
	}
	if embedded != "" && value == embedded {
		return "embedded-default"
	}
	return "default/unset"
}

// configSource reports where a value came from: flag, env, or default/unset.
// aliasEnv, when non-empty, is a JIRA_-prefixed alias (e.g. JIRA_BASE_URL)
// checked alongside the INPUT_<KEY>/<KEY> convention so the reported source
// matches loadConfig's resolution.
func configSource(cmd *cobra.Command, flagName, envKey, aliasEnv string) string {
	if cmd != nil && flagName != "" && cmd.Flags().Lookup(flagName) != nil &&
		cmd.Flags().Changed(flagName) {
		return "flag"
	}
	if envKey != "" && util.GetGlobalValue(envKey) != "" {
		return sourceEnv
	}
	if aliasEnv != "" && os.Getenv(aliasEnv) != "" {
		return sourceEnv
	}
	return "default/unset"
}

// detectAuthMode reports which auth mode run would select, mirroring
// auth.Resolve's priority (oauth-env > oauth-storage > bearer > basic) and the
// run path's gating: it only looks up stored OAuth tokens when no explicit
// bearer/basic credential is set. That lookup can probe the OS keyring (a
// write+delete, not strictly read-only) but performs no network I/O.
func detectAuthMode(config Config) string {
	hasExplicitCred := config.token != "" || (config.username != "" && config.password != "")
	switch {
	case config.oauthRefreshToken != "":
		return auth.ModeOAuthEnv
	case !hasExplicitCred && storedTokenExists(config):
		return auth.ModeOAuthStorage
	case config.token != "":
		return auth.ModeBearer
	case config.username != "" && config.password != "":
		return auth.ModeBasic
	default:
		return "none"
	}
}

// redactIfSecret masks values for fields that should never be printed in full.
func redactIfSecret(field, value string) string {
	if value == "" {
		return "(unset)"
	}
	switch field {
	case flagToken, flagPassword, "client_secret",
		"oauth_client_secret", "oauth_refresh_token":
		return "(set, redacted)"
	default:
		return value
	}
}
