package main

import (
	"context"
	"fmt"
	"github.com/appleboy/go-jira/pkg/auth"

	jira "github.com/andygrunwald/go-jira"
	"github.com/spf13/cobra"
)

// loadDataConfig runs the common preamble for the data subcommands (search,
// create, update, get, sprints, boards, link): load the env file, resolve the
// config, and require a valid base URL. These commands have no --ref, so they
// use requireBaseURL rather than the run-specific validateConfig.
func loadDataConfig(cmd *cobra.Command) (Config, error) {
	if err := loadEnvFromCmd(cmd); err != nil {
		return Config{}, err
	}
	config := loadConfig(cmd)
	if err := requireBaseURL(config); err != nil {
		return Config{}, err
	}
	if config.output != outputJSON && config.output != outputText {
		return Config{}, fmt.Errorf(
			"invalid output format %q (from --output or OUTPUT/INPUT_OUTPUT): must be %q or %q",
			config.output, outputJSON, outputText,
		)
	}
	return config, nil
}

// resolveJiraClient runs the auth + HTTP client + jira.Client preamble shared by
// every Jira-talking subcommand (search, create, update, get, sprints, boards,
// link). It mirrors the block in runWhoami: resolve the authenticator by the
// usual priority, validate it, then build an authenticated jira.Client against
// config.baseURL. Callers are expected to have already validated the base URL
// (via requireBaseURL) and loaded the env file.
func resolveJiraClient(ctx context.Context, config Config) (*jira.Client, error) {
	authenticator, err := auth.Resolve(ctx, authConfigFromRun(config))
	if err != nil {
		return nil, fmt.Errorf("auth resolution: %w", err)
	}
	if err := authenticator.Validate(); err != nil {
		return nil, fmt.Errorf("auth validation: %w", err)
	}

	httpClient := createHTTPClient(config, authenticator)
	jiraClient, err := jira.NewClient(httpClient, config.baseURL)
	if err != nil {
		return nil, fmt.Errorf("error creating jira client: %w", err)
	}
	return jiraClient, nil
}
