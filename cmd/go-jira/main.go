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
	"regexp"
	"strings"

	jira "github.com/andygrunwald/go-jira"
	"github.com/appleboy/com/convert"
	"github.com/joho/godotenv"
	"github.com/yassinebenaid/godump"
	"github/appleboy/go-jira/pkg/markdown"
	"github/appleboy/go-jira/pkg/util"
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

	_ = godotenv.Load(envfile)

	config := loadConfig()
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

	httpClient := createHTTPClient(config)
	jiraClient, err := jira.NewClient(httpClient, config.baseURL)
	if err != nil {
		slog.Error("error creating jira client", "error", err)
		return
	}

	user, err := getSelf(jiraClient)
	if err != nil {
		slog.Error("error getting self", "error", err)
		return
	}
	slog.Info("user account",
		"displayName", user.DisplayName,
		"email", user.EmailAddress,
		"username", user.Name,
	)

	assignee, err := getUser(jiraClient, config.assignee)
	if err != nil {
		slog.Error("error getting assignee", "error", err)
		return
	}
	if assignee != nil {
		slog.Info("assignee account",
			"displayName", assignee.DisplayName,
			"email", assignee.EmailAddress,
			"username", assignee.Name,
		)
	}

	// Get issue lists from ref
	issues := processIssues(jiraClient, config)
	if len(issues) == 0 {
		slog.Error("no issues found")
		return
	}

	if config.resolution != "" {
		config.resolution, err = getResolutionID(jiraClient, config.resolution)
		if err != nil {
			slog.Error("error getting resolution", "error", err)
			return
		}
	}

	if config.toTransition != "" {
		processTransitions(jiraClient, config.toTransition, config.resolution, issues)
	}

	if assignee != nil {
		processAssignee(jiraClient, issues, assignee)
	}

	if config.comment != "" {
		if config.markdown {
			config.comment = markdown.ToJira(config.comment)
		}
		addComments(jiraClient, config.comment, issues, user)
	}
}

func processIssues(jiraClient *jira.Client, config Config) []*jira.Issue {
	issueKeys := getIssueKeys(config.ref, config.issuePattern)
	issues := []*jira.Issue{}
	for _, issueKey := range issueKeys {
		issue, resp, err := jiraClient.Issue.GetWithContext(
			context.Background(),
			issueKey,
			&jira.GetQueryOptions{
				Expand: "transitions",
			},
		)
		if err != nil {
			slog.Error("error getting issue", "issue", issueKey, "error", err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			slog.Error("error getting issue", "issue", issueKey, "status", resp.Status)
			continue
		}
		issues = append(issues, issue)
	}
	return issues
}

func processAssignee(jiraClient *jira.Client, issues []*jira.Issue, assignee *jira.User) {
	for _, issue := range issues {
		resp, err := jiraClient.Issue.UpdateAssigneeWithContext(
			context.Background(),
			issue.Key,
			&jira.User{
				Name: assignee.Name,
			},
		)
		if err != nil {
			slog.Error("error updating assignee", "issue", issue.Key, "error", err)
			continue
		}
		if resp.StatusCode != http.StatusNoContent {
			slog.Error("error updating assignee", "issue", issue.Key, "status", resp.Status)
			continue
		}
		slog.Info("assignee updated",
			"issue", issue.Key,
			"assignee", assignee.Name,
		)
	}
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

func createHTTPClient(config Config) *http.Client {
	var httpTransport *http.Transport = nil

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

func getSelf(jiraClient *jira.Client) (*jira.User, error) {
	user, _, err := jiraClient.User.GetSelfWithContext(context.Background())
	if err != nil {
		return nil, err
	}

	return user, nil
}

func getUser(jiraClient *jira.Client, username string) (*jira.User, error) {
	if username == "" {
		return nil, nil
	}

	user, _, err := jiraClient.User.GetByUsernameWithContext(context.Background(), username)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func getResolutionID(jiraClient *jira.Client, resolution string) (string, error) {
	resolutions, resp, err := jiraClient.Resolution.GetListWithContext(context.Background())
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("error getting resolution: " + resp.Status)
	}
	for _, r := range resolutions {
		if strings.EqualFold(r.Name, resolution) {
			return r.ID, nil
		}
	}
	return "", nil
}

func processTransitions(
	jiraClient *jira.Client,
	toTransition string,
	resolution string,
	issues []*jira.Issue,
) {
	for _, issue := range issues {
		slog.Info("issue info",
			"key", issue.Key,
			"summary", issue.Fields.Summary,
			"current status", issue.Fields.Status.Name,
		)
		for _, transition := range issue.Transitions {
			if !strings.EqualFold(transition.Name, toTransition) {
				continue
			}

			input := &jira.TransitionPayloadInput{
				TicketID:     issue.Key,
				TransitionID: transition.ID,
			}
			if resolution != "" {
				input.ResolutionID = convert.ToPtr(resolution)
			}
			resp, err := jiraClient.Issue.DoTransitionPayloadWithContext(
				context.Background(),
				input,
			)
			if err != nil {
				slog.Error("error moving issue", "issue", issue.Key, "error", err)
				continue
			}
			if resp.StatusCode != http.StatusNoContent {
				slog.Error("error moving issue", "issue", issue.Key, "status", resp.Status)
				continue
			}
			slog.Info("issue moved to transition",
				"key", issue.Key,
				"summary", issue.Fields.Summary,
				"transition", transition.Name,
			)
		}
	}
}

func addComments(jiraClient *jira.Client, comment string, issues []*jira.Issue, user *jira.User) {
	currentUser := user

	for _, issue := range issues {
		item, resp, err := jiraClient.Issue.AddCommentWithContext(
			context.Background(),
			issue.Key,
			&jira.Comment{
				Name: currentUser.Name,
				Body: comment,
			},
		)
		if err != nil {
			slog.Error("error adding comment", "issue", issue.Key, "error", err)
			continue
		}

		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			slog.Error(
				"error adding comment",
				"issue",
				issue.Key,
				"status",
				resp.StatusCode,
				"body",
				string(body),
			)
			continue
		}
		slog.Info("added comment to issue",
			"issue", issue.Key,
			"comment", item.Body,
		)
	}
}
