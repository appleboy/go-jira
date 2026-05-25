package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// newUpdateCmd builds the `update` subcommand: apply a partial edit to an
// existing issue. It accepts the same editable fields as create (summary,
// assignee, description, components, labels, epic, sprint); only the flags the
// caller explicitly sets are sent, so unspecified fields are left untouched.
func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "update",
		Short:        "Update fields of an existing issue",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpdate(cmd)
		},
	}
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	addAuthFlags(cmd)
	addOutputFlag(cmd)
	addCustomFieldFlags(cmd)
	addEditableIssueFlags(cmd)
	cmd.Flags().String(flagKey, "", "Issue key, e.g. GAIA-123 (required)")
	_ = cmd.MarkFlagRequired(flagKey)
	return cmd
}

func runUpdate(cmd *cobra.Command) error {
	config, err := loadDataConfig(cmd)
	if err != nil {
		return err
	}
	key, _ := cmd.Flags().GetString(flagKey)

	fields := editableFieldsMap(cmd, config)
	if len(fields) == 0 {
		return errors.New(
			"nothing to update — supply at least one of " +
				"--summary/--description/--assignee/--components/--labels/--epic/--sprint",
		)
	}

	ctx, cancel := context.WithTimeout(cmdContext(cmd), time.Minute)
	defer cancel()

	jiraClient, err := resolveJiraClient(ctx, config)
	if err != nil {
		return err
	}

	resp, err := jiraClient.Issue.UpdateIssueWithContext(ctx, key, map[string]any{"fields": fields})
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("error updating issue %s: %w", key, err)
	}

	result := map[string]string{statusKey: "updated", flagKey: key}
	return emitResult(config, result, func() {
		fmt.Fprintf(os.Stdout, "updated %s\n", key)
	})
}

// editableFieldsMap builds a Jira REST "fields" payload from the editable issue
// flags (see addEditableIssueFlags), including only the flags the caller set.
// Values use the raw JSON shapes the REST API expects so the same builder works
// for partial updates and for any future map-based create path. The epic and
// sprint custom field IDs come from config (configurable per instance).
func editableFieldsMap(cmd *cobra.Command, config Config) map[string]any {
	fields := map[string]any{}
	if cmd.Flags().Changed(flagSummary) {
		v, _ := cmd.Flags().GetString(flagSummary)
		fields["summary"] = v
	}
	if cmd.Flags().Changed(flagDescription) {
		v, _ := cmd.Flags().GetString(flagDescription)
		fields["description"] = v
	}
	if cmd.Flags().Changed(flagAssignee) {
		v, _ := cmd.Flags().GetString(flagAssignee)
		fields["assignee"] = map[string]string{nameKey: v}
	}
	if cmd.Flags().Changed(flagComponents) {
		v, _ := cmd.Flags().GetString(flagComponents)
		fields["components"] = componentRefs(v)
	}
	if cmd.Flags().Changed(flagLabels) {
		v, _ := cmd.Flags().GetString(flagLabels)
		fields["labels"] = splitCSV(v)
	}
	if cmd.Flags().Changed(flagEpic) {
		v, _ := cmd.Flags().GetString(flagEpic)
		fields[config.epicField] = v
	}
	if cmd.Flags().Changed(flagSprint) {
		v, _ := cmd.Flags().GetInt(flagSprint)
		fields[config.sprintField] = v
	}
	return fields
}

// componentRefs converts a comma-separated component-name list into the
// {"name": ...} reference objects the REST API expects.
func componentRefs(s string) []map[string]string {
	names := splitCSV(s)
	out := make([]map[string]string, 0, len(names))
	for _, n := range names {
		out = append(out, map[string]string{nameKey: n})
	}
	return out
}
