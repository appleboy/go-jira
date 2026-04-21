package main

import (
	"errors"

	"github/appleboy/go-jira/pkg/util"

	"github.com/spf13/cobra"
)

// Config holds the application configuration
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

// loadConfig resolves configuration from CLI flags (when explicitly set)
// falling back to environment variables via util.GetGlobalValue.
//
// The INPUT_<KEY> → <KEY> lookup order inside util.GetGlobalValue is preserved
// verbatim, so GitHub Actions (which sets INPUT_*) and local .env usage both
// keep working without any change.
//
// Passing cmd == nil is supported and makes every lookup go straight to the
// environment — this keeps existing tests and any caller that doesn't construct
// a cobra command working unchanged.
func loadConfig(cmd *cobra.Command) Config {
	getString := func(flagName, envKey string) string {
		if cmd != nil && cmd.Flags().Changed(flagName) {
			v, _ := cmd.Flags().GetString(flagName)
			return v
		}
		return util.GetGlobalValue(envKey)
	}
	getBool := func(flagName, envKey string) bool {
		if cmd != nil && cmd.Flags().Changed(flagName) {
			v, _ := cmd.Flags().GetBool(flagName)
			return v
		}
		return util.ToBool(util.GetGlobalValue(envKey))
	}

	return Config{
		baseURL:      getString(flagBaseURL, "base_url"),
		insecure:     getString(flagInsecure, "insecure"),
		username:     getString(flagUsername, "username"),
		password:     getString(flagPassword, "password"),
		token:        getString(flagToken, "token"),
		ref:          getString(flagRef, "ref"),
		issuePattern: getString(flagIssueFormat, "issue_format"),
		toTransition: getString(flagToTransition, "transition"),
		resolution:   getString(flagResolution, "resolution"),
		comment:      getString(flagComment, "comment"),
		assignee:     getString(flagAssignee, "assignee"),
		markdown:     getBool(flagMarkdown, "markdown"),
		debug:        getBool(flagDebug, "debug"),
	}
}

// validateConfig validates the configuration
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
