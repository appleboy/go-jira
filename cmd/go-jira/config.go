package main

import (
	"errors"
	"github/appleboy/go-jira/pkg/util"
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

// loadConfig loads configuration from environment variables
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
