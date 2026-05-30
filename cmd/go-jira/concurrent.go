package main

import (
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

	if n := len(errChan); n > 0 {
		return fmt.Errorf("encountered %d errors while %s", n, noun)
	}
	return nil
}
