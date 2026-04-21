package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github/appleboy/go-jira/pkg/markdown"

	jira "github.com/andygrunwald/go-jira"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/yassinebenaid/godump"
)

var (
	Version string
	Commit  string
)

// Flag names shared between registration (newRootCmd) and lookup (loadConfig).
// Keeping them here avoids stringly-typed typo risk across files.
const (
	flagEnvFile      = "env-file"
	flagBaseURL      = "base-url"
	flagInsecure     = "insecure"
	flagUsername     = "username"
	flagPassword     = "password"
	flagToken        = "token"
	flagRef          = "ref"
	flagIssueFormat  = "issue-format"
	flagToTransition = "to-transition"
	flagResolution   = "resolution"
	flagComment      = "comment"
	flagAssignee     = "assignee"
	flagMarkdown     = "markdown"
	flagDebug        = "debug"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		slog.Error("execution failed", "error", err)
		os.Exit(1)
	}
}

// newRootCmd builds a fresh cobra command on every call so tests get a clean
// flag state and production code is unaffected by test leakage.
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "go-jira",
		Short:        "Transition, comment on, or assign Jira issues referenced in text",
		SilenceUsage: true,
		Version:      fmt.Sprintf("%s Commit: %s", Version, Commit),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd)
		},
	}

	cmd.SetVersionTemplate("Version: {{.Version}}\n")

	cmd.Flags().String(flagEnvFile, ".env", "Read in a file of environment variables")

	// Flags mirror the env-var driven config keys. Defaults are empty so that
	// cmd.Flags().Changed(name) correctly distinguishes "user set explicitly"
	// from "fall back to util.GetGlobalValue()".
	//
	// Note: --insecure is a String (not Bool) because the existing code path in
	// client.go does a strict config.insecure == "true" check. Keeping String
	// preserves exact behaviour (e.g. "1" does NOT enable insecure mode) for
	// both the flag and the env var. Pass as --insecure=true to enable.
	cmd.Flags().String(flagBaseURL, "", "Jira base URL (env: BASE_URL / INPUT_BASE_URL)")
	cmd.Flags().
		String(flagInsecure, "", "Skip TLS verification; pass \"true\" to enable (env: INSECURE / INPUT_INSECURE)")
	cmd.Flags().String(flagUsername, "", "Jira username (env: USERNAME / INPUT_USERNAME)")
	cmd.Flags().String(flagPassword, "", "Jira password (env: PASSWORD / INPUT_PASSWORD)")
	cmd.Flags().String(flagToken, "", "Jira API token (env: TOKEN / INPUT_TOKEN)")
	cmd.Flags().
		String(flagRef, "", "Commit message or text containing issue keys (env: REF / INPUT_REF)")
	cmd.Flags().
		String(flagIssueFormat, "", "Regex used to extract issue keys (env: ISSUE_FORMAT / INPUT_ISSUE_FORMAT)")
	cmd.Flags().
		String(flagToTransition, "", "Target transition name (env: TRANSITION / INPUT_TRANSITION)")
	cmd.Flags().
		String(flagResolution, "", "Resolution name to set (env: RESOLUTION / INPUT_RESOLUTION)")
	cmd.Flags().
		String(flagComment, "", "Comment body to add to matched issues (env: COMMENT / INPUT_COMMENT)")
	cmd.Flags().
		String(flagAssignee, "", "Username to assign the issues to (env: ASSIGNEE / INPUT_ASSIGNEE)")
	cmd.Flags().
		Bool(flagMarkdown, false, "Convert comment from Markdown to Jira syntax (env: MARKDOWN / INPUT_MARKDOWN)")
	cmd.Flags().Bool(flagDebug, false, "Dump resolved configuration (env: DEBUG / INPUT_DEBUG)")

	return cmd
}

func run(cmd *cobra.Command) error {
	envfile := ".env"
	if cmd != nil {
		envfile, _ = cmd.Flags().GetString(flagEnvFile)
	}
	_ = godotenv.Load(envfile)

	config := loadConfig(cmd)
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
		slog.Warn("no issues found, skipping further processing")
		return nil
	}

	if config.resolution != "" {
		config.resolution, err = getResolutionID(ctx, jiraClient, config.resolution)
		if err != nil {
			return fmt.Errorf("error getting resolution: %w", err)
		}
	}

	if config.toTransition != "" {
		if err := processTransitions(
			ctx,
			jiraClient,
			config.toTransition,
			config.resolution,
			issues,
		); err != nil {
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
