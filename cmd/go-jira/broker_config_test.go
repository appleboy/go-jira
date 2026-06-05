package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStderr redirects os.Stderr for the duration of fn and returns what was
// written. runConfigShow renders its table to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = orig }()
	defer r.Close()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(out)
}

// TestLoadConfig_BrokerEnvAndFlag verifies the client-side broker fields resolve
// with env > flag precedence, matching the other OAuth settings.
func TestLoadConfig_BrokerEnvAndFlag(t *testing.T) {
	t.Run("env wins over flag", func(t *testing.T) {
		clearInputEnv(t)
		t.Setenv(envBrokerURL, "https://broker.env")
		t.Setenv(envBrokerToken, "token-env")

		cmd := newRunCmd()
		if err := cmd.ParseFlags([]string{
			"--broker-url=https://broker.flag",
			"--broker-token=token-flag",
		}); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		got := loadConfig(cmd)
		if got.brokerURL != "https://broker.env" {
			t.Errorf("brokerURL = %q, want env value", got.brokerURL)
		}
		if got.brokerToken != "token-env" {
			t.Errorf("brokerToken = %q, want env value", got.brokerToken)
		}
	})

	t.Run("flag used when env unset", func(t *testing.T) {
		clearInputEnv(t)
		os.Unsetenv(envBrokerURL)
		os.Unsetenv(envBrokerToken)

		cmd := newRunCmd()
		if err := cmd.ParseFlags([]string{"--broker-url=https://broker.flag"}); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		got := loadConfig(cmd)
		if got.brokerURL != "https://broker.flag" {
			t.Errorf("brokerURL = %q, want flag value", got.brokerURL)
		}
		if got.brokerToken != "" {
			t.Errorf("brokerToken = %q, want empty", got.brokerToken)
		}
	})

	t.Run("unset by default", func(t *testing.T) {
		clearInputEnv(t)
		os.Unsetenv(envBrokerURL)
		os.Unsetenv(envBrokerToken)
		got := loadConfig(nil)
		if got.brokerURL != "" || got.brokerToken != "" {
			t.Errorf("broker fields = (%q, %q), want both empty by default",
				got.brokerURL, got.brokerToken)
		}
	})
}

// TestConfigShowBroker verifies `config show` reports the broker URL with its
// source and redacts the broker token rather than printing it.
func TestConfigShowBroker(t *testing.T) {
	clearInputEnv(t)
	os.Unsetenv(envBrokerURL)
	os.Unsetenv(envBrokerToken)
	t.Setenv(envBrokerURL, "https://broker.internal")
	t.Setenv(envBrokerToken, "super-secret-token")

	out := captureStderr(t, func() {
		cmd := newConfigShowCmd()
		if err := cmd.ParseFlags([]string{"--base-url=https://jira.example.com"}); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		if err := runConfigShow(cmd); err != nil {
			t.Fatalf("runConfigShow: %v", err)
		}
	})

	if !strings.Contains(out, "broker_url") || !strings.Contains(out, "https://broker.internal") {
		t.Errorf("config show output missing broker_url value:\n%s", out)
	}
	if strings.Contains(out, "super-secret-token") {
		t.Errorf("config show leaked the broker token:\n%s", out)
	}
	if !strings.Contains(out, "broker_token") || !strings.Contains(out, "(set, redacted)") {
		t.Errorf("config show should mark broker_token as redacted:\n%s", out)
	}
}
