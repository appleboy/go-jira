package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github/appleboy/go-jira/pkg/markdown"
	"log/slog"
	"os"
	"time"

	jira "github.com/andygrunwald/go-jira"
	"github.com/joho/godotenv"
	"github.com/yassinebenaid/godump"
)

var (
	Version     string
	Commit      string
	showVersion bool
)

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
		_ = godump.Dump(map[string]any{
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
