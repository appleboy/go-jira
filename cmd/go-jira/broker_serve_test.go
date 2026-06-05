package main

import (
	"strings"
	"testing"
)

// TestBrokerServeFailFast verifies `broker serve` refuses to start when required
// configuration is missing, so a misconfigured broker never silently fails every
// refresh. In particular the client secret must be present (and is read only from
// the environment).
func TestBrokerServeFailFast(t *testing.T) {
	t.Run("missing client secret", func(t *testing.T) {
		clearInputEnv(t)
		t.Setenv(envBaseURL, "https://jira.example.com")
		t.Setenv(envOAuthClientID, "client-abc")
		t.Setenv(envOAuthClientSecret, "")

		cmd := newBrokerServeCmd()
		if err := cmd.ParseFlags(nil); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		err := runBrokerServe(cmd)
		if err == nil || !strings.Contains(err.Error(), "client secret required") {
			t.Fatalf("error = %v, want it to require the client secret", err)
		}
	})

	t.Run("missing client id", func(t *testing.T) {
		clearInputEnv(t)
		t.Setenv(envBaseURL, "https://jira.example.com")
		t.Setenv(envOAuthClientID, "")
		t.Setenv(envOAuthClientSecret, "s3cr3t")

		cmd := newBrokerServeCmd()
		if err := cmd.ParseFlags(nil); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		err := runBrokerServe(cmd)
		if err == nil || !strings.Contains(err.Error(), "client ID required") {
			t.Fatalf("error = %v, want it to require the client ID", err)
		}
	})
}
