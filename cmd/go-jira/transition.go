package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

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
	var wg sync.WaitGroup
	errChan := make(chan error, len(issues))

	for _, issue := range issues {
		wg.Add(1)
		go func(iss *jira.Issue) {
			defer wg.Done()
			slog.Info("issue info",
				"key", iss.Key,
				"summary", iss.Fields.Summary,
				"current status", iss.Fields.Status.Name,
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
					errChan <- err
					return
				}
				if resp.StatusCode != http.StatusNoContent {
					err := fmt.Errorf("unexpected status: %s", resp.Status)
					slog.Error("error moving issue", "issue", iss.Key, "status", resp.Status)
					errChan <- err
					return
				}
				slog.Info("issue moved to transition",
					"key", iss.Key,
					"summary", iss.Fields.Summary,
					"transition", transition.Name,
				)
			}

			if !transitionFound {
				slog.Warn("transition not found for issue",
					"issue", iss.Key,
					"transition", toTransition,
				)
			}
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
		return fmt.Errorf("encountered %d errors while processing transitions", len(errs))
	}

	return nil
}
