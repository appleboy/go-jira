package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/appleboy/go-jira/pkg/auth"

	jira "github.com/andygrunwald/go-jira"
)

// createHTTPClient creates an HTTP client with optional TLS configuration and
// authentication. It clones http.DefaultTransport so all standard-library
// defaults (proxy, connection pool, timeouts, HTTP/2) are preserved, only
// overriding TLSClientConfig when --insecure is set, and layers the
// authenticator's credentials on top via its RoundTripper.
func createHTTPClient(config Config, authenticator auth.Authenticator) *http.Client {
	httpTransport := http.DefaultTransport.(*http.Transport).Clone()

	if config.insecure {
		slog.Warn("Skipping SSL certificate verification is insecure and not recommended")
		httpTransport.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, // #nosec G402 -- opt-in via flag
		}
	}

	if authenticator == nil {
		return &http.Client{Transport: httpTransport}
	}
	return &http.Client{Transport: authenticator.Transport(httpTransport)}
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
