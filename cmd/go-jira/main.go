package main

import (
	"context"
	"crypto/tls"
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
	if config.debug == "true" {
		_ = godump.Dump(map[string]interface{}{
			"ref":          config.ref,
			"issueFormat":  config.issueFormat,
			"toTransition": config.toTransition,
			"resolution":   config.resolution,
			"comment":      config.comment,
			"author":       config.author,
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

	authorUser, err := getUser(jiraClient, config.author)
	if err != nil {
		slog.Error("error getting author", "error", err)
		return
	}

	assignee, err := getUser(jiraClient, config.assignee)
	if err != nil {
		slog.Error("error getting assignee", "error", err)
		return
	}

	if config.resolution != "" {
		config.resolution, err = getResolutionID(jiraClient, config.resolution)
		if err != nil {
			slog.Error("error getting resolution", "error", err)
			return
		}
	}

	if config.ref != "" && config.toTransition != "" {
		processTransitions(jiraClient, config)
	}

	if assignee != nil {
		processAssignee(jiraClient, config, assignee)
	}

	if config.comment != "" {
		addComments(jiraClient, config, user, authorUser)
	}
}

func processAssignee(jiraClient *jira.Client, config Config, assignee *jira.User) {
	issueKeys := getIssueKeys(config.ref, config.issueFormat)
	for _, issueKey := range issueKeys {
		resp, err := jiraClient.Issue.UpdateAssigneeWithContext(
			context.Background(),
			issueKey,
			&jira.User{
				Name: assignee.Name,
			},
		)
		if err != nil {
			slog.Error("error updating assignee", "issue", issueKey, "error", err)
			continue
		}
		if resp.StatusCode != http.StatusNoContent {
			slog.Error("error updating assignee", "issue", issueKey, "status", resp.Status)
			continue
		}
		slog.Info("assignee updated",
			"issue", issueKey,
			"assignee", assignee.Name,
		)
	}
}

func getIssueKeys(ref, issueFormat string) []string {
	issuePattern := issueAlphanumericPattern
	issueKeys := []string{}
	if issueFormat != "" {
		issuePattern = regexp.MustCompile(issueFormat)
	}

	matches := issuePattern.FindAllString(ref, -1)
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
	issueFormat  string
	toTransition string
	resolution   string
	comment      string
	author       string
	assignee     string
	debug        string
}

func loadConfig() Config {
	return Config{
		baseURL:      getGlobalValue("base_url"),
		insecure:     getGlobalValue("insecure"),
		username:     getGlobalValue("username"),
		password:     getGlobalValue("password"),
		token:        getGlobalValue("token"),
		ref:          getGlobalValue("ref"),
		issueFormat:  getGlobalValue("issue_format"),
		toTransition: getGlobalValue("transition"),
		resolution:   getGlobalValue("resolution"),
		comment:      getGlobalValue("comment"),
		author:       getGlobalValue("author"),
		assignee:     getGlobalValue("assignee"),
		debug:        getGlobalValue("debug"),
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

	slog.Info("login account",
		"displayName", user.DisplayName,
		"email", user.EmailAddress,
		"username", user.Name,
	)
	return user, nil
}

func getUser(jiraClient *jira.Client, author string) (*jira.User, error) {
	if author == "" {
		return nil, nil
	}

	authorUser, _, err := jiraClient.User.GetByUsernameWithContext(context.Background(), author)
	if err != nil {
		return nil, err
	}

	if authorUser != nil {
		slog.Info("author account",
			"displayName", authorUser.DisplayName,
			"email", authorUser.EmailAddress,
			"username", authorUser.Name,
		)
	}
	return authorUser, nil
}

func getResolutionID(jiraClient *jira.Client, resolution string) (string, error) {
	resolutions, resp, err := jiraClient.Resolution.GetListWithContext(context.Background())
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

func processTransitions(jiraClient *jira.Client, config Config) {
	issueKeys := getIssueKeys(config.ref, config.issueFormat)
	for _, issueKey := range issueKeys {
		issue, resp, err := jiraClient.Issue.GetWithContext(context.Background(), issueKey, &jira.GetQueryOptions{
			Expand: "transitions",
		})
		if err != nil {
			slog.Error("error getting issue", "issue", issueKey, "error", err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			slog.Error("error getting issue", "issue", issueKey, "status", resp.Status)
			continue
		}
		slog.Info("issue info",
			"key", issue.Key,
			"summary", issue.Fields.Summary,
			"current status", issue.Fields.Status.Name,
		)
		for _, transition := range issue.Transitions {
			if !strings.EqualFold(transition.Name, config.toTransition) {
				continue
			}

			input := &jira.TransitionPayloadInput{
				TicketID:     issue.Key,
				TransitionID: transition.ID,
			}
			if config.resolution != "" {
				input.ResolutionID = convert.ToPtr(config.resolution)
			}
			resp, err := jiraClient.Issue.DoTransitionPayloadWithContext(
				context.Background(),
				input,
			)
			if err != nil {
				slog.Error("error moving issue", "issue", issueKey, "error", err)
				continue
			}
			if resp.StatusCode != http.StatusNoContent {
				slog.Error("error moving issue", "issue", issueKey, "status", resp.Status)
				continue
			}
			slog.Info("issue moved",
				"key", issue.Key,
				"summary", issue.Fields.Summary,
				"transition", transition.Name,
			)
		}
	}
}

func addComments(jiraClient *jira.Client, config Config, user, authorUser *jira.User) {
	currentUser := user
	if authorUser != nil {
		currentUser = authorUser
	}

	issueKeys := getIssueKeys(config.ref, config.issueFormat)
	for _, issueKey := range issueKeys {
		slog.Info(
			"author",
			"displayName", currentUser.DisplayName,
			"email", currentUser.EmailAddress,
			"username", currentUser.Name,
		)
		comment, resp, err := jiraClient.Issue.AddCommentWithContext(
			context.Background(),
			issueKey,
			&jira.Comment{
				Name:         currentUser.Name,
				Author:       *currentUser,
				UpdateAuthor: *currentUser,
				Body:         config.comment,
			},
		)
		if err != nil {
			slog.Error("error adding comment", "issue", issueKey, "error", err)
			continue
		}

		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			slog.Error("error adding comment", "issue", issueKey, "status", resp.StatusCode, "body", string(body))
			continue
		}
		slog.Info("comment added",
			"issue", issueKey,
			"comment", comment.Body,
		)
	}
}
