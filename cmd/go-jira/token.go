package main

import (
	"context"
	"errors"
	"fmt"
	"github/appleboy/go-jira/pkg/auth"
	"github/appleboy/go-jira/pkg/storage"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// newTokenCmd builds the `token` command group: print / status / refresh,
// operating on the locally stored OAuth token.
func newTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "token",
		Short:        "Inspect and manage the locally stored OAuth token",
		SilenceUsage: true,
	}
	cmd.AddCommand(newTokenPrintCmd(), newTokenStatusCmd(), newTokenRefreshCmd())
	return cmd
}

func newTokenPrintCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "print",
		Short:        "Print the current access token (requires --confirm)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTokenPrint(cmd)
		},
	}
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	cmd.Flags().Bool(flagConfirm, false, "Acknowledge that this prints a sensitive token")
	return cmd
}

func newTokenStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "status",
		Short:        "Show token mode, time remaining, scopes, and storage backend",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTokenStatus(cmd)
		},
	}
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	return cmd
}

func newTokenRefreshCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "refresh",
		Short:        "Force a token refresh and print the new expiry",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTokenRefresh(cmd)
		},
	}
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	return cmd
}

// loadedToken bundles a stored token with the config and store it came from.
type loadedToken struct {
	config Config
	store  storage.Store
	token  *storage.StoredToken
}

// loadStoredToken loads the token for the configured base URL and client.
func loadStoredToken(cmd *cobra.Command) (*loadedToken, error) {
	config, err := loadOAuthConfig(cmd)
	if err != nil {
		return nil, err
	}
	store, err := resolveStore()
	if err != nil {
		return nil, err
	}
	key := storage.MakeKey(config.baseURL, config.oauthClientID)
	tok, err := store.Load(key)
	if err != nil {
		if errors.Is(err, storage.ErrTokenNotFound) {
			return nil, errors.New("no stored token; run `go-jira login` first")
		}
		return nil, fmt.Errorf("load token: %w", err)
	}
	return &loadedToken{config: config, store: store, token: tok}, nil
}

func runTokenPrint(cmd *cobra.Command) error {
	loaded, err := loadStoredToken(cmd)
	if err != nil {
		return err
	}
	if confirmed, _ := cmd.Flags().GetBool(flagConfirm); !confirmed {
		return errors.New(
			"this command prints a sensitive token; re-run with --confirm to acknowledge")
	}
	fmt.Println(loaded.token.AccessToken)
	return nil
}

func runTokenStatus(cmd *cobra.Command) error {
	loaded, err := loadStoredToken(cmd)
	if err != nil {
		return err
	}
	tok := loaded.token
	remaining := time.Until(tok.ExpiresAt).Round(time.Second)
	fmt.Fprintf(os.Stderr, "Mode:      %s\n", auth.ModeOAuthStorage)
	fmt.Fprintf(os.Stderr, "Expires:   %s (in %s)\n", tok.ExpiresAt.Format(time.RFC3339), remaining)
	fmt.Fprintf(os.Stderr, "Scopes:    %v\n", tok.Scopes)
	fmt.Fprintf(os.Stderr, "Storage:   %s\n", loaded.store.Backend())
	return nil
}

func runTokenRefresh(cmd *cobra.Command) error {
	loaded, err := loadStoredToken(cmd)
	if err != nil {
		return err
	}

	oc := oauthConfigFromConfig(loaded.config)
	newTok, err := oc.Refresh(context.Background(), loaded.token.RefreshToken)
	if err != nil {
		return fmt.Errorf("refresh failed: %w", err)
	}

	updated := storage.NewStoredToken(
		loaded.token.BaseURL, loaded.token.ClientID, newTok,
		loaded.token.RefreshToken, loaded.token.Scopes,
	)
	key := storage.MakeKey(loaded.config.baseURL, loaded.config.oauthClientID)
	if err := loaded.store.Save(key, updated); err != nil {
		return fmt.Errorf("save refreshed token: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Refreshed. New expiry: %s\n", updated.ExpiresAt.Format(time.RFC3339))
	return nil
}
