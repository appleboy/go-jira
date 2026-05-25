package main

import (
	"fmt"
	"os"
	"time"

	jira "github.com/andygrunwald/go-jira"
	"github.com/spf13/cobra"
)

// newGetCmd builds the `get` subcommand: fetch summary and status for one
// issue. Equivalent to the Python `get` subcommand.
func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get",
		Short:   "Get the summary and status of a single issue",
		GroupID: groupIssues,
		Example: `  # Fetch one issue as JSON
  go-jira get --key GAIA-123 --output json

  # Human-readable summary and status
  go-jira get --key GAIA-123 --output text`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runGet(cmd)
		},
	}
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	addAuthFlags(cmd)
	addOutputFlag(cmd)
	cmd.Flags().String(flagKey, "", "Issue key, e.g. GAIA-123 (required)")
	_ = cmd.MarkFlagRequired(flagKey)
	return cmd
}

func runGet(cmd *cobra.Command) error {
	config, err := loadDataConfig(cmd)
	if err != nil {
		return err
	}
	key, _ := cmd.Flags().GetString(flagKey)

	ctx, cancel := cmdContextWithTimeout(cmd, time.Minute)
	defer cancel()

	jiraClient, err := resolveJiraClient(ctx, config)
	if err != nil {
		return err
	}

	issue, resp, err := jiraClient.Issue.GetWithContext(ctx, key, &jira.GetQueryOptions{
		Fields: "summary,status",
	})
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("error getting issue %s: %w", key, err)
	}

	return emitResult(config, issue, func() {
		status := ""
		if issue.Fields != nil && issue.Fields.Status != nil {
			status = issue.Fields.Status.Name
		}
		summary := ""
		if issue.Fields != nil {
			summary = issue.Fields.Summary
		}
		fmt.Fprintf(os.Stdout, "%s\t%s\t%s\n", issue.Key, status, summary)
	})
}
