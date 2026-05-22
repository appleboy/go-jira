package main

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// captureSlog redirects the default slog logger to a buffer for the duration of
// a test and returns the buffer. The previous default is restored on cleanup.
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// clearInputEnv unsets every INPUT_* and bare env var that loadConfig reads
// and returns a restore function. Tests use this to get a clean slate without
// polluting each other.
func clearInputEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"INPUT_BASE_URL", "INPUT_INSECURE", "INPUT_USERNAME", "INPUT_PASSWORD",
		"INPUT_TOKEN", "INPUT_REF", "INPUT_ISSUE_FORMAT", "INPUT_TRANSITION",
		"INPUT_RESOLUTION", "INPUT_COMMENT", "INPUT_ASSIGNEE", "INPUT_MARKDOWN",
		"INPUT_DEBUG",
		"BASE_URL", "INSECURE", "USERNAME", "PASSWORD",
		"TOKEN", "REF", "ISSUE_FORMAT", "TRANSITION",
		"RESOLUTION", "COMMENT", "ASSIGNEE", "MARKDOWN", "DEBUG",
		"JIRA_BASE_URL", "JIRA_USERNAME", "JIRA_PASSWORD",
		"JIRA_TOKEN", "JIRA_INSECURE",
	}
	saved := make(map[string]string, len(keys))
	for _, k := range keys {
		saved[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for k, v := range saved {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	})
}

func TestRedirectURI(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			"http by default",
			Config{callbackPort: 8765},
			"http://127.0.0.1:8765/callback",
		},
		{
			"https when cert and key set",
			Config{callbackPort: 9000, callbackCert: "c.pem", callbackKey: "k.pem"},
			"https://127.0.0.1:9000/callback",
		},
		{
			"http when only cert set",
			Config{callbackPort: 8765, callbackCert: "c.pem"},
			"http://127.0.0.1:8765/callback",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.redirectURI(); got != tt.want {
				t.Errorf("redirectURI() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveCallbackPort(t *testing.T) {
	tests := []struct {
		name string
		env  string // "" means unset
		args []string
		want int
	}{
		{"default", "", nil, defaultCallbackPort},
		{"flag only", "", []string{"--callback-port=9000"}, 9000},
		{"env only", "9443", nil, 9443},
		{"env beats flag", "9443", []string{"--callback-port=9000"}, 9443},
		{"explicit zero env passes through", "0", nil, 0},
		{"invalid env falls back to flag", "abc", []string{"--callback-port=9000"}, 9000},
		{"invalid env falls back to default", "abc", nil, defaultCallbackPort},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env == "" {
				os.Unsetenv(envOAuthCallbackPort)
			} else {
				t.Setenv(envOAuthCallbackPort, tt.env)
			}
			cmd := newLoginCmd()
			if err := cmd.ParseFlags(tt.args); err != nil {
				t.Fatalf("ParseFlags: %v", err)
			}
			if got := resolveCallbackPort(cmd); got != tt.want {
				t.Errorf("resolveCallbackPort() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with username and password",
			config: Config{
				baseURL:  "https://jira.example.com",
				ref:      "ABC-123",
				username: "user",
				password: "pass",
			},
			wantErr: false,
		},
		{
			name: "valid config with token",
			config: Config{
				baseURL: "https://jira.example.com",
				ref:     "ABC-123",
				token:   "token123",
			},
			wantErr: false,
		},
		{
			name: "missing base_url",
			config: Config{
				ref:      "ABC-123",
				username: "user",
				password: "pass",
			},
			wantErr: true,
			errMsg:  "base_url is required",
		},
		{
			name: "missing ref",
			config: Config{
				baseURL:  "https://jira.example.com",
				username: "user",
				password: "pass",
			},
			wantErr: true,
			errMsg:  "ref is required",
		},
		{
			// As of v1.0 the "no credentials" case is no longer rejected by
			// validateConfig: auth selection (incl. OAuth/storage) is delegated
			// to auth.Resolve, which errors at run time if nothing is available.
			name: "no credentials passes validateConfig",
			config: Config{
				baseURL: "https://jira.example.com",
				ref:     "ABC-123",
			},
			wantErr: false,
		},
		{
			name: "username provided without password",
			config: Config{
				baseURL:  "https://jira.example.com",
				ref:      "ABC-123",
				username: "user",
			},
			wantErr: true,
			errMsg:  "password is required when username is provided",
		},
		{
			name: "password provided without username",
			config: Config{
				baseURL:  "https://jira.example.com",
				ref:      "ABC-123",
				password: "pass",
			},
			wantErr: true,
			errMsg:  "username is required when password is provided",
		},
		{
			name: "valid config with all optional fields",
			config: Config{
				baseURL:      "https://jira.example.com",
				ref:          "ABC-123",
				username:     "user",
				password:     "pass",
				issuePattern: `[A-Z]+-\d+`,
				toTransition: "Done",
				resolution:   "Fixed",
				comment:      "Test comment",
				assignee:     "john.doe",
				markdown:     true,
				debug:        true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateConfig() expected error but got nil")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("validateConfig() error = %v, want %v", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("validateConfig() unexpected error = %v", err)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"INPUT_BASE_URL", "INPUT_INSECURE", "INPUT_USERNAME", "INPUT_PASSWORD",
		"INPUT_TOKEN", "INPUT_REF", "INPUT_ISSUE_FORMAT", "INPUT_TRANSITION",
		"INPUT_RESOLUTION", "INPUT_COMMENT", "INPUT_ASSIGNEE", "INPUT_MARKDOWN",
		"INPUT_DEBUG",
	}
	for _, key := range envVars {
		originalEnv[key] = os.Getenv(key)
	}

	// Cleanup function to restore environment
	cleanup := func() {
		for key, val := range originalEnv {
			if val == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, val)
			}
		}
	}
	defer cleanup()

	tests := []struct {
		name     string
		envVars  map[string]string
		expected Config
	}{
		{
			name: "load all config values",
			envVars: map[string]string{
				"INPUT_BASE_URL":     "https://jira.example.com",
				"INPUT_INSECURE":     "true",
				"INPUT_USERNAME":     "testuser",
				"INPUT_PASSWORD":     "testpass",
				"INPUT_TOKEN":        "testtoken",
				"INPUT_REF":          "ABC-123",
				"INPUT_ISSUE_FORMAT": `[A-Z]+-\d+`,
				"INPUT_TRANSITION":   "Done",
				"INPUT_RESOLUTION":   "Fixed",
				"INPUT_COMMENT":      "Test comment",
				"INPUT_ASSIGNEE":     "john.doe",
				"INPUT_MARKDOWN":     "true",
				"INPUT_DEBUG":        "true",
			},
			expected: Config{
				baseURL:      "https://jira.example.com",
				insecure:     true,
				username:     "testuser",
				password:     "testpass",
				token:        "testtoken",
				ref:          "ABC-123",
				issuePattern: `[A-Z]+-\d+`,
				toTransition: "Done",
				resolution:   "Fixed",
				comment:      "Test comment",
				assignee:     "john.doe",
				markdown:     true,
				debug:        true,
			},
		},
		{
			name: "load minimal config",
			envVars: map[string]string{
				"INPUT_BASE_URL": "https://jira.example.com",
				"INPUT_REF":      "ABC-123",
				"INPUT_TOKEN":    "testtoken",
			},
			expected: Config{
				baseURL:      "https://jira.example.com",
				ref:          "ABC-123",
				token:        "testtoken",
				insecure:     false,
				username:     "",
				password:     "",
				issuePattern: "",
				toTransition: "",
				resolution:   "",
				comment:      "",
				assignee:     "",
				markdown:     false,
				debug:        false,
			},
		},
		{
			name: "boolean conversion",
			envVars: map[string]string{
				"INPUT_BASE_URL": "https://jira.example.com",
				"INPUT_REF":      "ABC-123",
				"INPUT_TOKEN":    "testtoken",
				"INPUT_MARKDOWN": "false",
				"INPUT_DEBUG":    "0",
			},
			expected: Config{
				baseURL:  "https://jira.example.com",
				ref:      "ABC-123",
				token:    "testtoken",
				markdown: false,
				debug:    false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			for _, key := range envVars {
				os.Unsetenv(key)
			}

			// Set test environment variables
			for key, val := range tt.envVars {
				os.Setenv(key, val)
			}

			got := loadConfig(nil)

			// Compare config fields
			if got.baseURL != tt.expected.baseURL {
				t.Errorf("baseURL = %v, want %v", got.baseURL, tt.expected.baseURL)
			}
			if got.insecure != tt.expected.insecure {
				t.Errorf("insecure = %v, want %v", got.insecure, tt.expected.insecure)
			}
			if got.username != tt.expected.username {
				t.Errorf("username = %v, want %v", got.username, tt.expected.username)
			}
			if got.password != tt.expected.password {
				t.Errorf("password = %v, want %v", got.password, tt.expected.password)
			}
			if got.token != tt.expected.token {
				t.Errorf("token = %v, want %v", got.token, tt.expected.token)
			}
			if got.ref != tt.expected.ref {
				t.Errorf("ref = %v, want %v", got.ref, tt.expected.ref)
			}
			if got.issuePattern != tt.expected.issuePattern {
				t.Errorf("issuePattern = %v, want %v", got.issuePattern, tt.expected.issuePattern)
			}
			if got.toTransition != tt.expected.toTransition {
				t.Errorf("toTransition = %v, want %v", got.toTransition, tt.expected.toTransition)
			}
			if got.resolution != tt.expected.resolution {
				t.Errorf("resolution = %v, want %v", got.resolution, tt.expected.resolution)
			}
			if got.comment != tt.expected.comment {
				t.Errorf("comment = %v, want %v", got.comment, tt.expected.comment)
			}
			if got.assignee != tt.expected.assignee {
				t.Errorf("assignee = %v, want %v", got.assignee, tt.expected.assignee)
			}
			if got.markdown != tt.expected.markdown {
				t.Errorf("markdown = %v, want %v", got.markdown, tt.expected.markdown)
			}
			if got.debug != tt.expected.debug {
				t.Errorf("debug = %v, want %v", got.debug, tt.expected.debug)
			}
		})
	}
}

// TestLoadConfig_InputPrefixStillWorks is a regression guard for the
// GitHub Actions code path: when only INPUT_* env vars are present (and no
// flags are passed), loadConfig must still pick them up via
// util.GetGlobalValue. This is the exact path CI/CD relies on.
func TestLoadConfig_InputPrefixStillWorks(t *testing.T) {
	clearInputEnv(t)

	os.Setenv("INPUT_BASE_URL", "https://from-input.example.com")
	os.Setenv("INPUT_TOKEN", "input-token")
	os.Setenv("INPUT_REF", "ABC-1")
	os.Setenv("INPUT_MARKDOWN", "true")

	got := loadConfig(nil)

	if got.baseURL != "https://from-input.example.com" {
		t.Errorf("baseURL = %q, want INPUT_BASE_URL value", got.baseURL)
	}
	if got.token != "input-token" {
		t.Errorf("token = %q, want INPUT_TOKEN value", got.token)
	}
	if got.ref != "ABC-1" {
		t.Errorf("ref = %q, want INPUT_REF value", got.ref)
	}
	if !got.markdown {
		t.Errorf("markdown = false, want true from INPUT_MARKDOWN")
	}
}

// TestLoadConfig_BareEnvStillWorks confirms the fallback path
// (INPUT_* unset, only bare KEY env vars set) keeps working — this is how
// local .env files drive the tool.
func TestLoadConfig_BareEnvStillWorks(t *testing.T) {
	clearInputEnv(t)

	os.Setenv("BASE_URL", "https://from-bare.example.com")
	os.Setenv("TOKEN", "bare-token")
	os.Setenv("REF", "ABC-2")

	got := loadConfig(nil)

	if got.baseURL != "https://from-bare.example.com" {
		t.Errorf("baseURL = %q, want BASE_URL value", got.baseURL)
	}
	if got.token != "bare-token" {
		t.Errorf("token = %q, want TOKEN value", got.token)
	}
	if got.ref != "ABC-2" {
		t.Errorf("ref = %q, want REF value", got.ref)
	}
}

// TestLoadConfig_JiraAliasesWork verifies the JIRA_-prefixed env aliases
// (matching the docs and the auth-resolver error message) resolve when the
// flag and INPUT_<KEY>/<KEY> vars are unset.
func TestLoadConfig_JiraAliasesWork(t *testing.T) {
	clearInputEnv(t)

	os.Setenv("JIRA_BASE_URL", "https://jira-alias.example.com")
	os.Setenv("JIRA_USERNAME", "alias-user")
	os.Setenv("JIRA_PASSWORD", "alias-pass")
	os.Setenv("JIRA_TOKEN", "alias-token")
	os.Setenv("JIRA_INSECURE", "true")

	got := loadConfig(nil)

	if got.baseURL != "https://jira-alias.example.com" {
		t.Errorf("baseURL = %q, want JIRA_BASE_URL value", got.baseURL)
	}
	if got.username != "alias-user" {
		t.Errorf("username = %q, want JIRA_USERNAME value", got.username)
	}
	if got.password != "alias-pass" {
		t.Errorf("password = %q, want JIRA_PASSWORD value", got.password)
	}
	if got.token != "alias-token" {
		t.Errorf("token = %q, want JIRA_TOKEN value", got.token)
	}
	if !got.insecure {
		t.Errorf("insecure = false, want true (JIRA_INSECURE)")
	}
}

// TestLoadConfig_BareEnvBeatsJiraAlias verifies precedence: the existing
// INPUT_<KEY>/<KEY> convention wins over the JIRA_ alias when both are set.
func TestLoadConfig_BareEnvBeatsJiraAlias(t *testing.T) {
	clearInputEnv(t)

	os.Setenv("BASE_URL", "https://from-bare.example.com")
	os.Setenv("JIRA_BASE_URL", "https://from-jira.example.com")
	os.Setenv("TOKEN", "bare-token")
	os.Setenv("JIRA_TOKEN", "jira-token")

	got := loadConfig(nil)

	if got.baseURL != "https://from-bare.example.com" {
		t.Errorf("baseURL = %q, want BASE_URL to win over JIRA_BASE_URL", got.baseURL)
	}
	if got.token != "bare-token" {
		t.Errorf("token = %q, want TOKEN to win over JIRA_TOKEN", got.token)
	}
}

// TestLoadConfig_FlagOverridesEnv verifies the flag-wins precedence: when a
// user explicitly passes a flag, its value trumps both INPUT_* and bare env
// vars. The CI path (no flags) is unaffected.
func TestLoadConfig_FlagOverridesEnv(t *testing.T) {
	clearInputEnv(t)

	os.Setenv("INPUT_BASE_URL", "https://from-env.example.com")
	os.Setenv("INPUT_TOKEN", "env-token")
	os.Setenv("INPUT_REF", "ENV-1")
	os.Setenv("INPUT_MARKDOWN", "false")

	cmd := newRunCmd()
	if err := cmd.ParseFlags([]string{
		"--base-url=https://from-flag.example.com",
		"--token=flag-token",
		"--markdown",
	}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}

	got := loadConfig(cmd)

	// Flags were set → flag values win.
	if got.baseURL != "https://from-flag.example.com" {
		t.Errorf("baseURL = %q, want flag value", got.baseURL)
	}
	if got.token != "flag-token" {
		t.Errorf("token = %q, want flag value", got.token)
	}
	if !got.markdown {
		t.Errorf("markdown = false, want true (flag set)")
	}

	// ref flag NOT set → must fall back to INPUT_REF.
	if got.ref != "ENV-1" {
		t.Errorf("ref = %q, want INPUT_REF value (flag not set)", got.ref)
	}
}

// TestLoadConfig_UnsetFlagFallsBackToEnv guards against a subtle regression:
// registering a flag must NOT make loadConfig treat the flag's default as
// "set". Only Changed() should trigger flag-wins behaviour.
func TestLoadConfig_UnsetFlagFallsBackToEnv(t *testing.T) {
	clearInputEnv(t)

	os.Setenv("INPUT_BASE_URL", "https://from-env.example.com")
	os.Setenv("INPUT_TOKEN", "env-token")
	os.Setenv("INPUT_REF", "ENV-1")

	cmd := newRunCmd()
	// Parse empty args — no flag is Changed().
	if err := cmd.ParseFlags([]string{}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}

	got := loadConfig(cmd)

	if got.baseURL != "https://from-env.example.com" {
		t.Errorf("baseURL = %q, want env value", got.baseURL)
	}
	if got.token != "env-token" {
		t.Errorf("token = %q, want env value", got.token)
	}
	if got.ref != "ENV-1" {
		t.Errorf("ref = %q, want env value", got.ref)
	}
}

// TestLoadConfig_WarnsOnSecretFlags verifies that --password / --token via CLI
// flag emit a warning, while the same values via env vars do not.
func TestLoadConfig_WarnsOnSecretFlags(t *testing.T) {
	t.Run("flag emits warning", func(t *testing.T) {
		clearInputEnv(t)
		buf := captureSlog(t)

		cmd := newRunCmd()
		if err := cmd.ParseFlags([]string{
			"--password=hunter2",
			"--token=t0k3n",
		}); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		_ = loadConfig(cmd)

		out := buf.String()
		if !strings.Contains(out, "--password") {
			t.Errorf("expected warning for --password, got: %s", out)
		}
		if !strings.Contains(out, "--token") {
			t.Errorf("expected warning for --token, got: %s", out)
		}
	})

	t.Run("env vars do not warn", func(t *testing.T) {
		clearInputEnv(t)
		os.Setenv("INPUT_PASSWORD", "hunter2")
		os.Setenv("INPUT_TOKEN", "t0k3n")
		buf := captureSlog(t)

		cmd := newRunCmd()
		if err := cmd.ParseFlags([]string{}); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		_ = loadConfig(cmd)

		if strings.Contains(buf.String(), "unsafe") {
			t.Errorf("expected no warning when secrets come from env, got: %s", buf.String())
		}
	})
}
