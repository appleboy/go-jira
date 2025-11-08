package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github/appleboy/go-jira/pkg/markdown"
	"github/appleboy/go-jira/pkg/util"

	jira "github.com/andygrunwald/go-jira"
	"github.com/appleboy/com/convert"
	"github.com/joho/godotenv"
	"github.com/yassinebenaid/godump"
)

var (
	Version     string
	Commit      string
	showVersion bool
)

// issueAlphanumericPattern matches string that references to an alphanumeric issue, e.g. ABC-1234
var issueAlphanumericPattern = regexp.MustCompile(`([A-Z]{1,10}-[1-9][0-9]*)`)

func main() {
	var envfile string
	flag.StringVar(&envfile, "env-file", ".env", "Read in a file of environment variables")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.Parse()

	if showVersion {
		fmt.Printf("Version: %s Commit: %s\n", Version, Commit)
		return
	}

	if err := run(envfile); err != nil {
		slog.Error("execution failed", "error", err)
		os.Exit(1)
	}
}

func run(envfile string) error {
	_ = godotenv.Load(envfile)

	config := loadConfig()
	if err := validateConfig(config); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	if config.debug {
		_ = godump.Dump(map[string]interface{}{
			"ref":          config.ref,
			"issuePattern": config.issuePattern,
			"toTransition": config.toTransition,
			"resolution":   config.resolution,
			"comment":      config.comment,
			"assignee":     config.assignee,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	httpClient := createHTTPClient(config)
	jiraClient, err := jira.NewClient(httpClient, config.baseURL)
	if err != nil {
		return fmt.Errorf("error creating jira client: %w", err)
	}

	user, err := getSelf(ctx, jiraClient)
	if err != nil {
		return fmt.Errorf("error getting self: %w", err)
	}
	slog.Info("user account",
		"displayName", user.DisplayName,
		"email", user.EmailAddress,
		"username", user.Name,
	)

	var assignee *jira.User
	if config.assignee != "" {
		assignee, err = getUser(ctx, jiraClient, config.assignee)
		if err != nil {
			return fmt.Errorf("error getting assignee: %w", err)
		}
		slog.Info("assignee account",
			"displayName", assignee.DisplayName,
			"email", assignee.EmailAddress,
			"username", assignee.Name,
		)
	}

	// Get issue lists from ref
	issues, err := processIssues(ctx, jiraClient, config)
	if err != nil {
		return fmt.Errorf("error processing issues: %w", err)
	}
	if len(issues) == 0 {
		return errors.New("no issues found")
	}

	if config.resolution != "" {
		config.resolution, err = getResolutionID(ctx, jiraClient, config.resolution)
		if err != nil {
			return fmt.Errorf("error getting resolution: %w", err)
		}
	}

	if config.toTransition != "" {
		if err := processTransitions(ctx, jiraClient, config.toTransition, config.resolution, issues); err != nil {
			return fmt.Errorf("error processing transitions: %w", err)
		}
	}

	if assignee != nil {
		if err := processAssignee(ctx, jiraClient, issues, assignee); err != nil {
			return fmt.Errorf("error processing assignee: %w", err)
		}
	}

	if config.comment != "" {
		if config.markdown {
			config.comment = markdown.ToJira(config.comment)
		}
		if err := addComments(ctx, jiraClient, config.comment, issues, user); err != nil {
			return fmt.Errorf("error adding comments: %w", err)
		}
	}

	return nil
}

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

type Config struct {
	baseURL      string
	insecure     string
	username     string
	password     string
	token        string
	ref          string
	issuePattern string
	toTransition string
	resolution   string
	comment      string
	assignee     string
	markdown     bool
	debug        bool
}

func loadConfig() Config {
	return Config{
		baseURL:      util.GetGlobalValue("base_url"),
		insecure:     util.GetGlobalValue("insecure"),
		username:     util.GetGlobalValue("username"),
		password:     util.GetGlobalValue("password"),
		token:        util.GetGlobalValue("token"),
		ref:          util.GetGlobalValue("ref"),
		issuePattern: util.GetGlobalValue("issue_format"),
		toTransition: util.GetGlobalValue("transition"),
		resolution:   util.GetGlobalValue("resolution"),
		comment:      util.GetGlobalValue("comment"),
		assignee:     util.GetGlobalValue("assignee"),
		markdown:     util.ToBool(util.GetGlobalValue("markdown")),
		debug:        util.ToBool(util.GetGlobalValue("debug")),
	}
}

func validateConfig(config Config) error {
	if config.baseURL == "" {
		return errors.New("base_url is required")
	}
	if config.ref == "" {
		return errors.New("ref is required")
	}
	if config.username == "" && config.password == "" && config.token == "" {
		return errors.New("authentication credentials required (username/password or token)")
	}
	if config.username != "" && config.password == "" {
		return errors.New("password is required when username is provided")
	}
	if config.password != "" && config.username == "" {
		return errors.New("username is required when password is provided")
	}
	return nil
}

func createHTTPClient(config Config) *http.Client {
	var httpTransport *http.Transport

	if config.insecure == "true" {
		slog.Warn("Skipping SSL certificate verification is insecure and not recommended")
		httpTransport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec
		}
	}

	if config.username != "" && config.password != "" {
		auth := jira.BasicAuthTransport{
			Username:  config.username,
			Password:  config.password,
			Transport: httpTransport,
		}
		return auth.Client()
	}

	if config.token != "" {
		auth := jira.BearerAuthTransport{
			Token:     config.token,
			Transport: httpTransport,
		}
		return auth.Client()
	}

	return &http.Client{Transport: httpTransport}
}

func getSelf(ctx context.Context, jiraClient *jira.Client) (*jira.User, error) {
	user, resp, err := jiraClient.User.GetSelfWithContext(ctx)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	return user, nil
}

func getUser(ctx context.Context, jiraClient *jira.Client, username string) (*jira.User, error) {
	if username == "" {
		return nil, nil
	}

	user, resp, err := jiraClient.User.GetByUsernameWithContext(ctx, username)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	return user, nil
}

func getResolutionID(ctx context.Context, jiraClient *jira.Client, resolution string) (string, error) {
	resolutions, resp, err := jiraClient.Resolution.GetListWithContext(ctx)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error getting resolution: %s", resp.Status)
	}
	for _, r := range resolutions {
		if strings.EqualFold(r.Name, resolution) {
			return r.ID, nil
		}
	}
	return "", nil
}

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
