package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	jira "github.com/andygrunwald/go-jira"
	"github.com/appleboy/com/convert"
)

// processTransitions processes issue transitions concurrently
func processTransitions(
	ctx context.Context,
	jiraClient *jira.Client,
	toTransition string,
	resolution string,
	issues []*jira.Issue,
) error {
	return forEachIssueConcurrent(issues, "processing transitions", func(iss *jira.Issue) error {
		// Use the nil-safe accessors: a partial issue response can leave Fields
		// or Status nil, and a deref here would panic inside the worker
		// goroutine and take down the whole process.
		summary := issueSummary(iss)
		slog.Info("issue info",
			"key", iss.Key,
			"summary", summary,
			"current status", issueStatusName(iss),
		)

		transitionFound := false
		for _, transition := range iss.Transitions {
			if !strings.EqualFold(transition.Name, toTransition) {
				continue
			}
			transitionFound = true

			input := &jira.TransitionPayloadInput{
				TicketID:     iss.Key,
				TransitionID: transition.ID,
			}
			if resolution != "" {
				input.ResolutionID = convert.ToPtr(resolution)
			}
			resp, err := jiraClient.Issue.DoTransitionPayloadWithContext(
				ctx,
				input,
			)
			if resp != nil && resp.Body != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				slog.Error("error moving issue", "issue", iss.Key, "error", err)
				return err
			}
			if resp.StatusCode != http.StatusNoContent {
				slog.Error("error moving issue", "issue", iss.Key, statusKey, resp.Status)
				return fmt.Errorf("unexpected status: %s", resp.Status)
			}
			slog.Info("issue moved to transition",
				"key", iss.Key,
				"summary", summary,
				"transition", transition.Name,
			)
			// The issue has moved; stop scanning so a second transition with the
			// same name isn't attempted against the already-transitioned issue.
			break
		}

		if !transitionFound {
			slog.Warn("transition not found for issue",
				"issue", iss.Key,
				"transition", toTransition,
			)
		}
		return nil
	})
}
