package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	jira "github.com/andygrunwald/go-jira"
)

// createHTTPClient creates an HTTP client with optional TLS configuration and authentication
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

// getSelf retrieves the current authenticated user
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

// getUser retrieves a user by username
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

// getResolutionID retrieves the resolution ID by name
func getResolutionID(
	ctx context.Context,
	jiraClient *jira.Client,
	resolution string,
) (string, error) {
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
