package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	jira "github.com/andygrunwald/go-jira"
	"github.com/spf13/cobra"
	"github.com/trivago/tgo/tcontainer"
)

// newCreateCmd builds the `create` subcommand: create a Task issue. Equivalent
// to the Python `create` subcommand, including epic-link and sprint custom
// fields (configurable via --epic-field / --sprint-field).
func newCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "create",
		Short:        "Create a Task issue",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCreate(cmd)
		},
	}
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	addAuthFlags(cmd)
	addOutputFlag(cmd)
	addCustomFieldFlags(cmd)
	addEditableIssueFlags(cmd)
	cmd.Flags().String(flagProject, "", "Project key, e.g. GAIA (required)")
	_ = cmd.MarkFlagRequired(flagProject)
	_ = cmd.MarkFlagRequired(flagSummary)
	return cmd
}

func runCreate(cmd *cobra.Command) error {
	config, err := loadDataConfig(cmd)
	if err != nil {
		return err
	}

	project, _ := cmd.Flags().GetString(flagProject)
	summary, _ := cmd.Flags().GetString(flagSummary)
	assignee, _ := cmd.Flags().GetString(flagAssignee)
	description, _ := cmd.Flags().GetString(flagDescription)
	if description, err = resolveStdin(description); err != nil {
		return err
	}
	components, _ := cmd.Flags().GetString(flagComponents)
	labels, _ := cmd.Flags().GetString(flagLabels)
	epic, _ := cmd.Flags().GetString(flagEpic)
	sprint, _ := cmd.Flags().GetInt(flagSprint)
	sprintSet := cmd.Flags().Changed(flagSprint)

	fields := &jira.IssueFields{
		Project:  jira.Project{Key: project},
		Type:     jira.IssueType{Name: "Task"},
		Summary:  summary,
		Unknowns: tcontainer.NewMarshalMap(),
	}
	if description != "" {
		fields.Description = description
	}
	if assignee != "" {
		fields.Assignee = &jira.User{Name: assignee}
	}
	if components != "" {
		fields.Components = splitComponents(components)
	}
	if labels != "" {
		fields.Labels = splitCSV(labels)
	}
	if epic != "" {
		fields.Unknowns[config.epicField] = epic
	}
	if sprintSet {
		fields.Unknowns[config.sprintField] = sprint
	}

	ctx, cancel := context.WithTimeout(cmdContext(cmd), time.Minute)
	defer cancel()

	jiraClient, err := resolveJiraClient(ctx, config)
	if err != nil {
		return err
	}

	created, resp, err := jiraClient.Issue.CreateWithContext(ctx, &jira.Issue{Fields: fields})
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("error creating issue: %w", err)
	}

	return emitResult(config, created, func() {
		fmt.Fprintf(os.Stdout, "created %s\n", created.Key)
	})
}

// splitCSV splits a comma-separated string into trimmed, non-empty values.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// splitComponents converts a comma-separated component-name list into the
// library's component slice.
func splitComponents(s string) []*jira.Component {
	names := splitCSV(s)
	out := make([]*jira.Component, 0, len(names))
	for _, n := range names {
		out = append(out, &jira.Component{Name: n})
	}
	return out
}
