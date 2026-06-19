package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete",
		Short:   "Delete a Jira issue",
		GroupID: groupIssues,
		Example: `  # Delete an issue (confirmation required)
  go-jira delete --key GAIA-123 --confirm

  # Delete an issue and its subtasks
  go-jira delete --key GAIA-123 --confirm --delete-subtasks`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDelete(cmd)
		},
	}
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	addAuthFlags(cmd)
	addOutputFlag(cmd)
	cmd.Flags().String(flagKey, "", "Issue key to delete, e.g. GAIA-123 (required)")
	_ = cmd.MarkFlagRequired(flagKey)
	cmd.Flags().Bool(flagConfirm, false, "Confirm the deletion (required)")
	cmd.Flags().Bool(flagDeleteSubtasks, false, "Also delete all subtasks of the issue")
	return cmd
}

func runDelete(cmd *cobra.Command) error {
	confirm, _ := cmd.Flags().GetBool(flagConfirm)
	if !confirm {
		return fmt.Errorf("use --confirm to confirm deletion")
	}

	config, err := loadDataConfig(cmd)
	if err != nil {
		return err
	}
	key, _ := cmd.Flags().GetString(flagKey)
	deleteSubtasks, _ := cmd.Flags().GetBool(flagDeleteSubtasks)

	ctx, cancel := cmdContextWithTimeout(cmd, time.Minute)
	defer cancel()

	jiraClient, err := resolveJiraClient(ctx, config)
	if err != nil {
		return err
	}

	u := &url.URL{Path: "rest/api/2/issue/" + key}
	if deleteSubtasks {
		u.RawQuery = "deleteSubtasks=true"
	}
	apiPath := u.String()

	req, err := jiraClient.NewRequestWithContext(ctx, http.MethodDelete, apiPath, nil)
	if err != nil {
		return fmt.Errorf("error building delete request for %s: %w", key, err)
	}
	resp, err := jiraClient.Do(req, nil)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("error deleting issue %s: %w", key, err)
	}

	result := map[string]string{statusKey: "deleted", flagKey: key}
	return emitResult(config, result, func() {
		fmt.Fprintf(os.Stdout, "deleted %s\n", key)
	})
}
