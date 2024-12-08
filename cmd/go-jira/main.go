package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	jira "github.com/andygrunwald/go-jira"
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

	baseURL := getGlobalValue("base_url")
	insecure := getGlobalValue("insecure")
	username := getGlobalValue("username")
	password := getGlobalValue("password")
	ref := getGlobalValue("ref")                  // git tag or branch name
	issueFormat := getGlobalValue("issue_format") // issue regular expression pattern
	toTransition := getGlobalValue("transition")  // move issue to a specific status
	token := getGlobalValue("token")
	debug := getGlobalValue("debug")

	if debug == "true" {
		godump.Dump(ref)
		godump.Dump(issueFormat)
		godump.Dump(toTransition)
	}

	var httpTransport *http.Transport = nil
	var httpClient *http.Client = nil

	if insecure == "true" {
		httpTransport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	if username != "" && password != "" {
		auth := jira.BasicAuthTransport{
			Username:  username,
			Password:  password,
			Transport: httpTransport,
		}
		httpClient = auth.Client()
	}

	if token != "" {
		auth := jira.BearerAuthTransport{
			Token:     token,
			Transport: httpTransport,
		}
		httpClient = auth.Client()
	}

	jiraClient, err := jira.NewClient(httpClient, baseURL)
	if err != nil {
		slog.Error("error creating jira client", "error", err)
		return
	}

	user, _, err := jiraClient.User.GetSelfWithContext(context.Background())
	if err != nil {
		slog.Error("error getting self", "error", err)
		return
	}

	slog.Info("login account",
		"displayName", user.DisplayName,
		"email", user.EmailAddress,
		"username", user.Name,
	)

	if ref != "" && toTransition != "" {
		issueKeys := getIssueKeys(ref, issueFormat)
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
				if !strings.EqualFold(transition.Name, toTransition) {
					continue
				}

				resp, err := jiraClient.Issue.DoTransitionWithContext(context.Background(), issueKey, transition.ID)
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
}

func getIssueKeys(ref, issueFormat string) []string {
	var issuePattern *regexp.Regexp = issueAlphanumericPattern
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
