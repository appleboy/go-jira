package main

import (
	"errors"
	"fmt"
	"sync"

	jira "github.com/andygrunwald/go-jira"
)

// forEachIssueConcurrent runs fn for every issue in parallel. fn is responsible
// for its own logging; any error it returns is counted. When one or more issues
// fail, a single summarizing error is returned, using noun (e.g. "adding
// comments") to describe the operation.
func forEachIssueConcurrent(issues []*jira.Issue, noun string, fn func(*jira.Issue) error) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(issues))

	for _, issue := range issues {
		wg.Add(1)
		go func(iss *jira.Issue) {
			defer wg.Done()
			if err := fn(iss); err != nil {
				errChan <- err
			}
		}(issue)
	}

	wg.Wait()
	close(errChan)

	// Collect the actual errors, not just a count, so the real cause (HTTP
	// status, message) survives into the returned/serialized error instead of
	// only reaching the per-issue logs.
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("encountered %d errors while %s: %w",
			len(errs), noun, errors.Join(errs...))
	}
	return nil
}
