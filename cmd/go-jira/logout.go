package main

import (
	"errors"
	"fmt"
	"github/appleboy/go-jira/pkg/storage"
	"os"

	"github.com/spf13/cobra"
)

// newLogoutCmd builds the `logout` subcommand, which removes the locally stored
// token for the given base URL and client.
func newLogoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "logout",
		Short:        "Remove the locally stored OAuth token for a Jira site",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogout(cmd)
		},
	}
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	return cmd
}

func runLogout(cmd *cobra.Command) error {
	if err := loadEnvFromCmd(cmd); err != nil {
		return err
	}
	config := loadConfig(cmd)
	if err := requireBaseURL(config); err != nil {
		return err
	}
	if config.oauthClientID == "" {
		return errors.New("OAuth client ID required: set " + envOAuthClientID +
			" or pass --client-id")
	}

	store, err := resolveStore()
	if err != nil {
		return err
	}
	key := storage.MakeKey(config.baseURL, config.oauthClientID)
	if err := store.Delete(key); err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Logged out from %s\n", config.baseURL)
	return nil
}
