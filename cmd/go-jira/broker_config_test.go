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

// TestConfigShowBrokerSourceEnvWins verifies that when BOTH the env var and the
// flag are set, `config show` reports SOURCE=env for the broker rows — matching
// resolveWithEnv's env > flag precedence. A flag-first source label would print
// the env value while claiming it came from the flag.
func TestConfigShowBrokerSourceEnvWins(t *testing.T) {
	clearInputEnv(t)
	t.Setenv(envBrokerURL, "https://broker.fromenv")
	t.Setenv(envBrokerToken, "token-fromenv")

	out := captureStderr(t, func() {
		cmd := newConfigShowCmd()
		if err := cmd.ParseFlags([]string{
			"--base-url=https://jira.example.com",
			"--broker-url=https://broker.fromflag",
			"--broker-token=token-fromflag",
		}); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		if err := runConfigShow(cmd); err != nil {
			t.Fatalf("runConfigShow: %v", err)
		}
	})

	// The env value wins, so the broker_url row must report SOURCE=env and show
	// the env URL, never the overridden flag value.
	if src := sourceForField(t, out, "broker_url"); src != sourceEnv {
		t.Errorf("broker_url SOURCE = %q, want %q\n%s", src, sourceEnv, out)
	}
	if !strings.Contains(out, "https://broker.fromenv") {
		t.Errorf("broker_url should show the env value (env wins):\n%s", out)
	}
	if strings.Contains(out, "https://broker.fromflag") {
		t.Errorf("broker_url should NOT show the overridden flag value:\n%s", out)
	}
	if src := sourceForField(t, out, "broker_token"); src != sourceEnv {
		t.Errorf("broker_token SOURCE = %q, want %q\n%s", src, sourceEnv, out)
	}
}

// sourceForField returns the SOURCE column (the last whitespace-separated field)
// of the row whose FIELD column equals field, from a `config show` table.
func sourceForField(t *testing.T, table, field string) string {
	t.Helper()
	for line := range strings.SplitSeq(table, "\n") {
		cols := strings.Fields(line)
		if len(cols) >= 2 && cols[0] == field {
			return cols[len(cols)-1]
		}
	}
	t.Fatalf("field %q not found in table:\n%s", field, table)
	return ""
}
