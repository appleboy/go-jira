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

	baseURL := getGlobalValue("base_url")         // jira base url
	insecure := getGlobalValue("insecure")        // skip verify ssl certificate
	username := getGlobalValue("username")        // jira username
	password := getGlobalValue("password")        // use token instead of username and password
	token := getGlobalValue("token")              // token for authentication
	ref := getGlobalValue("ref")                  // git tag or branch name
	issueFormat := getGlobalValue("issue_format") // issue regular expression pattern
	toTransition := getGlobalValue("transition")  // move issue to a specific status
	resolution := getGlobalValue("resolution")    // set resolution when moving issue to a specific status
	comment := getGlobalValue("comment")          // add comment when moving issue to a specific status
	author := getGlobalValue("author")            // author of the comment
	debug := getGlobalValue("debug")              // enable debug mode

	if debug == "true" {
		_ = godump.Dump(ref)
		_ = godump.Dump(issueFormat)
		_ = godump.Dump(toTransition)
		_ = godump.Dump(resolution)
	}

	var httpTransport *http.Transport = nil
	var httpClient *http.Client = nil

	if insecure == "true" {
		slog.Warn("Skipping SSL certificate verification is insecure and not recommended")
		httpTransport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec
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

	var authorUser *jira.User
	if author != "" {
		authorUser, _, err = jiraClient.User.GetByUsernameWithContext(context.Background(), author)
		if err != nil {
			slog.Error("error getting author", "error", err)
			return
		}
		if authorUser != nil {
			slog.Info("author account",
				"displayName", authorUser.DisplayName,
				"email", authorUser.EmailAddress,
				"username", authorUser.Name,
			)
		}
	}

	// get resolution id
	if resolution != "" {
		resolutions, resp, err := jiraClient.Resolution.GetListWithContext(context.Background())
		if err != nil {
			slog.Error("error getting resolution", "error", err)
			return
		}
		if resp.StatusCode != http.StatusOK {
			slog.Error("error getting resolution", "status", resp.Status)
			return
		}
		for _, r := range resolutions {
			if strings.EqualFold(r.Name, resolution) {
				resolution = r.ID
				break
			}
		}
	}

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

	if comment != "" {
		currentUser := user
		if authorUser != nil {
			currentUser = authorUser
		}

		issueKeys := getIssueKeys(ref, issueFormat)
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
					Body: fmt.Sprintf(
						"%s\n\ncomment from [~%s]",
						comment,
						currentUser.Name,
					),
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
