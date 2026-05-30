package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	jira "github.com/andygrunwald/go-jira"
)

// addComments adds comments to issues concurrently
func addComments(
	ctx context.Context,
	jiraClient *jira.Client,
	comment string,
	issues []*jira.Issue,
	user *jira.User,
) error {
	return forEachIssueConcurrent(issues, "adding comments", func(iss *jira.Issue) error {
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
			return err
		}

		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			slog.Error(
				"error adding comment",
				"issue",
				iss.Key,
				statusKey,
				resp.StatusCode,
				"body",
				string(body),
			)
			return fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
		}
		slog.Info("added comment to issue",
			"issue", iss.Key,
			"comment", item.Body,
		)
		return nil
	})
}
