package main

import (
	"fmt"
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
	row("oauth_client_id", config.oauthClientID, flagClientID, "")
	row("scope", config.scope, flagScope, "")
	fmt.Fprintf(w, "auth_mode\t%s\t%s\n", detectAuthMode(config), "resolved")

	return w.Flush()
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

// detectAuthMode reports which auth mode run would select, without performing
// any network or storage I/O.
func detectAuthMode(config Config) string {
	switch {
	case config.oauthRefreshToken != "":
		return "oauth-env"
	case config.token != "":
		return "bearer"
	case config.username != "" && config.password != "":
		return "basic"
	case config.oauthClientID != "":
		return "oauth-storage (if a token is stored)"
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
	case "token", "password", "client_secret":
		return "(set, redacted)"
	default:
		return value
	}
}
