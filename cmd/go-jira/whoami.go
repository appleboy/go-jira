package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/appleboy/go-jira/pkg/auth"

	jira "github.com/andygrunwald/go-jira"
	"github.com/spf13/cobra"
)

// newWhoamiCmd builds the `whoami` subcommand: it resolves the active
// authenticator (by the same priority as `run`) and reports the authenticated
// user plus the auth mode in use.
func newWhoamiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "whoami",
		Short:        "Show the authenticated Jira user and the active auth mode",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWhoami(cmd)
		},
	}
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	cmd.Flags().String(flagUsername, "", "Jira username (env: USERNAME / INPUT_USERNAME)")
	cmd.Flags().String(flagPassword, "", "Jira password (prefer env: PASSWORD / INPUT_PASSWORD)")
	cmd.Flags().String(flagToken, "", "Jira API token (prefer env: TOKEN / INPUT_TOKEN)")
	return cmd
}

func runWhoami(cmd *cobra.Command) error {
	if err := loadEnvFromCmd(cmd); err != nil {
		return err
	}
	config := loadConfig(cmd)
	if err := requireBaseURL(config); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmdContext(cmd), time.Minute)
	defer cancel()

	authenticator, err := auth.Resolve(ctx, authConfigFromRun(config))
	if err != nil {
		return fmt.Errorf("auth resolution: %w", err)
	}
	if err := authenticator.Validate(); err != nil {
		return fmt.Errorf("auth validation: %w", err)
	}

	httpClient := createHTTPClient(config, authenticator)
	jiraClient, err := jira.NewClient(httpClient, config.baseURL)
	if err != nil {
		return fmt.Errorf("error creating jira client: %w", err)
	}
	user, err := getSelf(ctx, jiraClient)
	if err != nil {
		return fmt.Errorf("error getting self: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Authenticated as %s <%s> (%s)\n",
		user.DisplayName, user.EmailAddress, user.Name)
	fmt.Fprintf(os.Stderr, "Base URL:  %s\n", config.baseURL)
	fmt.Fprintf(os.Stderr, "Auth mode: %s\n", authenticator.Mode())
	return nil
}
