package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sync"

	jira "github.com/andygrunwald/go-jira"
)

// issueAlphanumericPattern matches string that references to an alphanumeric issue, e.g. ABC-1234
var issueAlphanumericPattern = regexp.MustCompile(`([A-Z]{1,10}-[1-9][0-9]*)`)

// processIssues retrieves issues from JIRA concurrently
func processIssues(ctx context.Context, jiraClient *jira.Client, config Config) ([]*jira.Issue, error) {
	issueKeys := getIssueKeys(config.ref, config.issuePattern)
	if len(issueKeys) == 0 {
		return nil, errors.New("no issue keys found in ref")
	}

	type result struct {
		issue *jira.Issue
		err   error
		key   string
	}

	results := make(chan result, len(issueKeys))
	var wg sync.WaitGroup

	// Process issues concurrently
	for _, issueKey := range issueKeys {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			issue, resp, err := jiraClient.Issue.GetWithContext(
				ctx,
				key,
				&jira.GetQueryOptions{
					Expand: "transitions",
				},
			)
			if resp != nil && resp.Body != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				results <- result{err: err, key: key}
				return
			}
			if resp.StatusCode != http.StatusOK {
				results <- result{err: fmt.Errorf("unexpected status: %s", resp.Status), key: key}
				return
			}
			results <- result{issue: issue, key: key}
		}(issueKey)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	issues := []*jira.Issue{}
	for r := range results {
		if r.err != nil {
			slog.Error("error getting issue", "issue", r.key, "error", r.err)
			continue
		}
		issues = append(issues, r.issue)
	}

	return issues, nil
}

// getIssueKeys extracts issue keys from a reference string using a pattern
func getIssueKeys(ref, issuePattern string) []string {
	pattern := issueAlphanumericPattern
	issueKeys := []string{}
	if issuePattern != "" {
		pattern = regexp.MustCompile(issuePattern)
	}

	matches := pattern.FindAllString(ref, -1)
	// Deduplicate issue keys
	issueKeySet := make(map[string]struct{})
	for _, match := range matches {
		if _, ok := issueKeySet[match]; ok {
			continue
		}
		issueKeySet[match] = struct{}{}
		issueKeys = append(issueKeys, match)
	}
	return issueKeys
}
