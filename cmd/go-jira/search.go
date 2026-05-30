package main

import (
	"fmt"
	"os"
	"time"

	jira "github.com/andygrunwald/go-jira"
	"github.com/spf13/cobra"
)

// defaultSearchBaseFields is the static part of the default field selection.
// These are Jira REST field names, kept as literals and intentionally
// independent of the CLI flag / log-key constants that happen to share the same
// text — renaming a flag must not silently change the fields requested from
// Jira. The configurable epic and sprint custom fields are appended at runtime
// in searchFields so --epic-field / --sprint-field overrides apply.
var defaultSearchBaseFields = []string{
	"summary",  //nolint:goconst // Jira REST field name, not the flagSummary constant
	"status",   //nolint:goconst // Jira REST field name, not the statusKey log constant
	"assignee", //nolint:goconst // Jira REST field name, not the flagAssignee constant
	"labels",
	"components",
}

// newSearchCmd builds the `search` subcommand: run a JQL query and print the
// matching issues. Equivalent to the Python `search` subcommand.
func newSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "search",
		Short:   "Search Jira issues with a JQL query",
		GroupID: groupIssues,
		Example: `  # Search with JQL and print JSON
  go-jira search --jql 'project = GAIA AND status = "In Progress"' --output json

  # Limit results and choose returned fields
  go-jira search --jql 'assignee = currentUser()' --fields summary,status --limit 5

  # Read the JQL from stdin
  echo 'project = GAIA ORDER BY created DESC' | go-jira search --jql -`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSearch(cmd)
		},
	}
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	addAuthFlags(cmd)
	addOutputFlag(cmd)
	addCustomFieldFlags(cmd)
	cmd.Flags().String(flagJQL, "", `JQL expression; pass "-" to read from stdin (required)`)
	cmd.Flags().String(flagFields, "",
		"Comma-separated fields to return (default: summary,status,assignee,labels,components + epic/sprint fields)")
	cmd.Flags().Int(flagLimit, 20, "Maximum number of results")
	_ = cmd.MarkFlagRequired(flagJQL)
	return cmd
}

func runSearch(cmd *cobra.Command) error {
	config, err := loadDataConfig(cmd)
	if err != nil {
		return err
	}
	jql, _ := cmd.Flags().GetString(flagJQL)
	if jql, err = resolveStdin(jql); err != nil {
		return err
	}
	fieldsArg, _ := cmd.Flags().GetString(flagFields)
	limit, _ := cmd.Flags().GetInt(flagLimit)

	ctx, cancel := cmdContextWithTimeout(cmd, time.Minute)
	defer cancel()

	jiraClient, err := resolveJiraClient(ctx, config)
	if err != nil {
		return err
	}

	fields := searchFields(fieldsArg, config)
	issues, resp, err := jiraClient.Issue.SearchWithContext(ctx, jql, &jira.SearchOptions{
		Fields:     fields,
		MaxResults: limit,
	})
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("error searching issues: %w", err)
	}

	return emitResult(config, issues, func() {
		for _, issue := range issues {
			status := ""
			if issue.Fields != nil && issue.Fields.Status != nil {
				status = issue.Fields.Status.Name
			}
			summary := ""
			if issue.Fields != nil {
				summary = issue.Fields.Summary
			}
			fmt.Fprintf(os.Stdout, "%s\t%s\t%s\n", issue.Key, status, summary)
		}
	})
}

// searchFields returns the field list for the query. An explicit --fields value
// wins; otherwise the default set is used with the configured epic/sprint
// custom field IDs appended.
func searchFields(fieldsArg string, config Config) []string {
	if fieldsArg != "" {
		return splitCSV(fieldsArg)
	}
	fields := make([]string, 0, len(defaultSearchBaseFields)+2)
	fields = append(fields, defaultSearchBaseFields...)
	fields = append(fields, config.epicField, config.sprintField)
	return fields
}
