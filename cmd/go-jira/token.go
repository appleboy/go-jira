package main

import (
	"context"
	"errors"
	"fmt"
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

// loadStoredToken loads the token for the configured base URL and client,
// returning the store so callers can write back.
func loadStoredToken(cmd *cobra.Command) (Config, storage.Store, *storage.StoredToken, error) {
	if err := loadEnvFromCmd(cmd); err != nil {
		return Config{}, nil, nil, err
	}
	config := loadConfig(cmd)
	if err := requireBaseURL(config); err != nil {
		return Config{}, nil, nil, err
	}
	if config.oauthClientID == "" {
		return Config{}, nil, nil, errors.New("OAuth client ID required: set " +
			envOAuthClientID + " or pass --client-id")
	}
	store, err := resolveStore()
	if err != nil {
		return Config{}, nil, nil, err
	}
	key := storage.MakeKey(config.baseURL, config.oauthClientID)
	tok, err := store.Load(key)
	if err != nil {
		if errors.Is(err, storage.ErrTokenNotFound) {
			return Config{}, nil, nil, errors.New("no stored token; run `go-jira login` first")
		}
		return Config{}, nil, nil, fmt.Errorf("load token: %w", err)
	}
	return config, store, tok, nil
}

func runTokenPrint(cmd *cobra.Command) error {
	_, _, tok, err := loadStoredToken(cmd)
	if err != nil {
		return err
	}
	if !cmd.Flags().Changed(flagConfirm) {
		return errors.New(
			"this command prints a sensitive token; re-run with --confirm to acknowledge")
	}
	fmt.Println(tok.AccessToken)
	return nil
}

func runTokenStatus(cmd *cobra.Command) error {
	_, store, tok, err := loadStoredToken(cmd)
	if err != nil {
		return err
	}
	remaining := time.Until(tok.ExpiresAt).Round(time.Second)
	fmt.Fprintf(os.Stderr, "Mode:      oauth-storage\n")
	fmt.Fprintf(os.Stderr, "Expires:   %s (in %s)\n", tok.ExpiresAt.Format(time.RFC3339), remaining)
	fmt.Fprintf(os.Stderr, "Scopes:    %v\n", tok.Scopes)
	fmt.Fprintf(os.Stderr, "Storage:   %s\n", store.Backend())
	return nil
}

func runTokenRefresh(cmd *cobra.Command) error {
	config, store, tok, err := loadStoredToken(cmd)
	if err != nil {
		return err
	}

	oc := oauthConfigFromConfig(config)
	newTok, err := oc.Refresh(context.Background(), tok.RefreshToken)
	if err != nil {
		return fmt.Errorf("refresh failed: %w", err)
	}

	refreshToken := newTok.RefreshToken
	if refreshToken == "" {
		refreshToken = tok.RefreshToken
	}
	updated := &storage.StoredToken{
		BaseURL:      tok.BaseURL,
		ClientID:     tok.ClientID,
		AccessToken:  newTok.AccessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    newTok.Expiry,
		ObtainedAt:   time.Now().UTC(),
		Scopes:       tok.Scopes,
	}
	key := storage.MakeKey(config.baseURL, config.oauthClientID)
	if err := store.Save(key, updated); err != nil {
		return fmt.Errorf("save refreshed token: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Refreshed. New expiry: %s\n", updated.ExpiresAt.Format(time.RFC3339))
	return nil
}
