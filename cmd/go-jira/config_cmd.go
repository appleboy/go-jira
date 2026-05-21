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

	row := func(field, value, flagName, envKey string) {
		fmt.Fprintf(w, "%s\t%s\t%s\n", field, redactIfSecret(field, value),
			configSource(cmd, flagName, envKey))
	}

	row("base_url", config.baseURL, flagBaseURL, "base_url")
	row("insecure", fmt.Sprintf("%t", config.insecure), flagInsecure, "insecure")
	row("token", config.token, flagToken, "token")
	row("username", config.username, flagUsername, "username")
	fmt.Fprintf(w, "oauth_client_id\t%s\t%s\n",
		redactIfSecret("oauth_client_id", config.oauthClientID),
		oauthClientIDSource(cmd, config.oauthClientID))
	row("scope", config.scope, flagScope, "")
	fmt.Fprintf(w, "auth_mode\t%s\t%s\n", detectAuthMode(config), "resolved")

	return w.Flush()
}

// storedTokenExists reports whether an OAuth token is persisted for this base
// URL/client, used to mirror the resolver's oauth-storage decision without any
// network I/O.
func storedTokenExists(config Config) bool {
	if config.oauthClientID == "" {
		return false
	}
	store := resolveStoreQuiet()
	if store == nil {
		return false
	}
	_, err := store.Load(storage.MakeKey(config.baseURL, config.oauthClientID))
	return err == nil
}

// oauthClientIDSource reports where the OAuth client ID came from. Unlike
// configSource it knows the fixed JIRA_OAUTH_CLIENT_ID env var and the
// build-time embedded default, so the SOURCE column never misreports an
// env-supplied or embedded value as "default/unset".
func oauthClientIDSource(cmd *cobra.Command, value string) string {
	if cmd != nil && cmd.Flags().Lookup(flagClientID) != nil &&
		cmd.Flags().Changed(flagClientID) {
		return "flag"
	}
	if os.Getenv(envOAuthClientID) != "" {
		return "env"
	}
	if value != "" && value == DefaultOAuthClientID {
		return "embedded-default"
	}
	return "default/unset"
}

// configSource reports where a value came from: flag, env, or default/unset.
func configSource(cmd *cobra.Command, flagName, envKey string) string {
	if cmd != nil && flagName != "" && cmd.Flags().Lookup(flagName) != nil &&
		cmd.Flags().Changed(flagName) {
		return "flag"
	}
	if envKey != "" && util.GetGlobalValue(envKey) != "" {
		return "env"
	}
	return "default/unset"
}

// detectAuthMode reports which auth mode run would select, mirroring
// auth.Resolve's priority (oauth-env > oauth-storage > bearer > basic). It does
// a read-only storage lookup but no network I/O.
func detectAuthMode(config Config) string {
	switch {
	case config.oauthRefreshToken != "":
		return auth.ModeOAuthEnv
	case storedTokenExists(config):
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
	case flagToken, flagPassword, "client_secret":
		return "(set, redacted)"
	default:
		return value
	}
}
