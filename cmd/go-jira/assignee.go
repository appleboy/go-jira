package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	jira "github.com/andygrunwald/go-jira"
)

// processAssignee updates assignee for issues concurrently
func processAssignee(
	ctx context.Context,
	jiraClient *jira.Client,
	issues []*jira.Issue,
	assignee *jira.User,
) error {
	return forEachIssueConcurrent(issues, "updating assignees", func(iss *jira.Issue) error {
		resp, err := jiraClient.Issue.UpdateAssigneeWithContext(
			ctx,
			iss.Key,
			&jira.User{
				Name: assignee.Name,
			},
		)
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}
		if err != nil {
			slog.Error("error updating assignee", "issue", iss.Key, "error", err)
			return err
		}
		if resp.StatusCode != http.StatusNoContent {
			slog.Error("error updating assignee", "issue", iss.Key, statusKey, resp.Status)
			return fmt.Errorf("unexpected status: %s", resp.Status)
		}
		slog.Info("assignee updated",
			"issue", iss.Key,
			"assignee", assignee.Name,
		)
		return nil
	})
}
