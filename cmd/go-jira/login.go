package main

import (
	"fmt"
	"os"
	"time"

	"github.com/appleboy/go-jira/pkg/storage"

	"github.com/spf13/cobra"
)

// newLoginCmd builds the `login` subcommand: an interactive Authorization Code
// + PKCE flow that stores the resulting token locally.
func newLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "login",
		Short:        "Authenticate via OAuth 2.0 (Authorization Code + PKCE) and store the token",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogin(cmd)
		},
	}
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	cmd.Flags().Duration(flagTimeout, 5*time.Minute, "How long to wait for browser authorization")
	return cmd
}

func runLogin(cmd *cobra.Command) error {
	config, err := loadOAuthConfig(cmd)
	if err != nil {
		return err
	}

	timeout := 5 * time.Minute
	if cmd.Flags().Changed(flagTimeout) {
		timeout, _ = cmd.Flags().GetDuration(flagTimeout)
	}

	oc := oauthConfigFromConfig(config)
	res, err := oauthLogin(cmdContext(cmd), oc, config.callbackPort, timeout)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	store, err := resolveStore()
	if err != nil {
		return err
	}
	key := storage.MakeKey(config.baseURL, config.oauthClientID)
	stored := storage.NewStoredToken(
		config.baseURL, config.oauthClientID, res.Token, "", res.Scopes,
	)
	if err := store.Save(key, stored); err != nil {
		return fmt.Errorf("save token: %w", err)
	}

	printLoginSummary(config, stored, store.Backend())
	return nil
}

// printLoginSummary writes a human-friendly confirmation to stderr.
func printLoginSummary(config Config, tok *storage.StoredToken, backend string) {
	fmt.Fprintln(os.Stderr, "\n✅ Logged in")
	fmt.Fprintf(os.Stderr, "   Base URL:   %s\n", config.baseURL)
	fmt.Fprintf(os.Stderr, "   Scopes:     %v\n", tok.Scopes)
	fmt.Fprintf(os.Stderr, "   Expires at: %s\n", tok.ExpiresAt.Format(time.RFC3339))
	fmt.Fprintf(os.Stderr, "   Storage:    %s\n", backend)
}
