package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"

	jira "github.com/andygrunwald/go-jira"
)

// addComments adds comments to issues concurrently
func addComments(ctx context.Context, jiraClient *jira.Client, comment string, issues []*jira.Issue, user *jira.User) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(issues))

	for _, issue := range issues {
		wg.Add(1)
		go func(iss *jira.Issue) {
			defer wg.Done()
			item, resp, err := jiraClient.Issue.AddCommentWithContext(
				ctx,
				iss.Key,
				&jira.Comment{
					Name: user.Name,
					Body: comment,
				},
			)
			if resp != nil && resp.Body != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				slog.Error("error adding comment", "issue", iss.Key, "error", err)
				errChan <- err
				return
			}

			if resp.StatusCode != http.StatusCreated {
				body, _ := io.ReadAll(resp.Body)
				err := fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
				slog.Error(
					"error adding comment",
					"issue",
					iss.Key,
					"status",
					resp.StatusCode,
					"body",
					string(body),
				)
				errChan <- err
				return
			}
			slog.Info("added comment to issue",
				"issue", iss.Key,
				"comment", item.Body,
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
		return fmt.Errorf("encountered %d errors while adding comments", len(errs))
	}

	return nil
}
