package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/appleboy/go-jira/pkg/auth"
	"github.com/appleboy/go-jira/pkg/markdown"

	jira "github.com/andygrunwald/go-jira"
	"github.com/spf13/cobra"
	"github.com/yassinebenaid/godump"
)

// newRunCmd builds the `run` subcommand, which reproduces the pre-v1.0 bare
// command behavior: extract issue keys from --ref and transition / comment /
// assign them. All action flags and the GitHub Actions INPUT_* env vars work
// here exactly as before.
func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "run",
		Short:   "Transition, comment on, or assign Jira issues referenced in text",
		GroupID: groupRun,
		Example: `  # Move every issue mentioned in the latest commit to Done
  git log -1 --format=%B | go-jira run --ref - --to-transition Done

  # Add a Markdown comment to issues referenced in a string
  go-jira run --ref "Fixes GAIA-12" --comment "**Deployed** to staging" --markdown

  # Assign matched issues to a user
  go-jira run --ref "GAIA-7 GAIA-8" --assignee jdoe`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd)
		},
	}

	addCommonFlags(cmd)
	addOAuthFlags(cmd)

	cmd.Flags().String(flagUsername, "", "Jira username (env: USERNAME / INPUT_USERNAME)")
	cmd.Flags().
		String(flagPassword, "", "Jira password — INSECURE on shared hosts, prefer env: PASSWORD / INPUT_PASSWORD")
	cmd.Flags().
		String(flagToken, "", "Jira API token — INSECURE on shared hosts, prefer env: TOKEN / INPUT_TOKEN")
	cmd.Flags().
		String(flagRef, "", `Commit message or text containing issue keys; pass "-" to read from stdin (env: REF / INPUT_REF)`)
	cmd.Flags().
		String(flagIssueFormat, "", "Regex used to extract issue keys (env: ISSUE_FORMAT / INPUT_ISSUE_FORMAT)")
	cmd.Flags().
		String(flagToTransition, "", "Target transition name (env: TRANSITION / INPUT_TRANSITION)")
	cmd.Flags().
		String(flagResolution, "", "Resolution name to set (env: RESOLUTION / INPUT_RESOLUTION)")
	cmd.Flags().
		String(flagComment, "", `Comment body to add to matched issues; pass "-" to read from stdin (env: COMMENT / INPUT_COMMENT)`)
	cmd.Flags().
		String(flagAssignee, "", "Username to assign the issues to (env: ASSIGNEE / INPUT_ASSIGNEE)")
	cmd.Flags().
		Bool(flagMarkdown, false, "Convert comment from Markdown to Jira syntax (env: MARKDOWN / INPUT_MARKDOWN)")

	return cmd
}

func run(cmd *cobra.Command) error {
	if err := loadEnvFromCmd(cmd); err != nil {
		return err
	}

	config := loadConfig(cmd)
	// Allow the free-text inputs to be piped in via the "-" sentinel so run
	// composes with other tools, e.g. `git log -1 --format=%B | go-jira run --ref -`.
	var err error
	if config.ref, err = resolveStdin(config.ref); err != nil {
		return err
	}
	if config.comment, err = resolveStdin(config.comment); err != nil {
		return err
	}
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
			flagAssignee:   config.assignee,
		})
	}

	ctx, cancel := context.WithTimeout(cmdContext(cmd), 5*time.Minute)
	defer cancel()

	authenticator, err := auth.Resolve(ctx, authConfigFromRun(config))
	if err != nil {
		return fmt.Errorf("auth resolution: %w", err)
	}
	if err := authenticator.Validate(); err != nil {
		return fmt.Errorf("auth validation: %w", err)
	}
	slog.Info("authenticated", "mode", authenticator.Mode())

	httpClient := createHTTPClient(config, authenticator)
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

// authConfigFromRun maps the run Config into an auth.Config. In oauth-env mode
// it wires the rotation write-back callback. A token Store is attached only when
// OAuth storage could actually be the chosen method — see hasExplicitCred below.
func authConfigFromRun(config Config) auth.Config {
	cfg := auth.Config{
		Username:          config.username,
		Password:          config.password,
		Token:             config.token,
		OAuthRefreshToken: config.oauthRefreshToken,
		OAuthClientID:     config.oauthClientID,
		OAuthBaseURL:      config.baseURL,
		OAuthRedirectURI:  config.redirectURI(),
		OAuthScopes:       []string{config.scope},
		OAuthHTTPClient:   oauthHTTPClient(config),
	}
	if config.oauthRefreshToken != "" {
		cfg.OnRotate = rotationWriter(config.oauthRefreshTokenOutput)
	}
	// Resolving a Store probes the OS keyring (a write+delete that can trigger a
	// keychain permission prompt), so only do it when OAuth storage could win:
	// no injected refresh token, a client ID is configured, and the user has not
	// supplied an explicit bearer/basic credential. The OAuth client ID has a
	// build-time embedded default, so without the credential guard a pure
	// token/basic run would probe the keyring on every invocation.
	hasExplicitCred := config.token != "" || (config.username != "" && config.password != "")
	if config.oauthClientID != "" && config.oauthRefreshToken == "" && !hasExplicitCred {
		cfg.Store = resolveStoreQuiet()
	}
	return cfg
}
