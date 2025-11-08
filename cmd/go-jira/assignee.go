package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	jira "github.com/andygrunwald/go-jira"
)

// processAssignee updates assignee for issues concurrently
func processAssignee(ctx context.Context, jiraClient *jira.Client, issues []*jira.Issue, assignee *jira.User) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(issues))

	for _, issue := range issues {
		wg.Add(1)
		go func(iss *jira.Issue) {
			defer wg.Done()
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
				errChan <- err
				return
			}
			if resp.StatusCode != http.StatusNoContent {
				err := fmt.Errorf("unexpected status: %s", resp.Status)
				slog.Error("error updating assignee", "issue", iss.Key, "status", resp.Status)
				errChan <- err
				return
			}
			slog.Info("assignee updated",
				"issue", iss.Key,
				"assignee", assignee.Name,
			)
		}(issue)
	}

	wg.Wait()
	close(errChan)

	// Collect any errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("encountered %d errors while updating assignees", len(errs))
	}

	return nil
}
