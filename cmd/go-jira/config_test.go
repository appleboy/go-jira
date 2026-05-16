package main

import (
	"os"
	"testing"
)

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
			name: "missing authentication credentials",
			config: Config{
				baseURL: "https://jira.example.com",
				ref:     "ABC-123",
			},
			wantErr: true,
			errMsg:  "authentication credentials required (username/password or token)",
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
				insecure:     "true",
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
				insecure:     "",
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

// TestLoadConfig_FlagOverridesEnv verifies the flag-wins precedence: when a
// user explicitly passes a flag, its value trumps both INPUT_* and bare env
// vars. The CI path (no flags) is unaffected.
func TestLoadConfig_FlagOverridesEnv(t *testing.T) {
	clearInputEnv(t)

	os.Setenv("INPUT_BASE_URL", "https://from-env.example.com")
	os.Setenv("INPUT_TOKEN", "env-token")
	os.Setenv("INPUT_REF", "ENV-1")
	os.Setenv("INPUT_MARKDOWN", "false")

	cmd := newRootCmd()
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

	cmd := newRootCmd()
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
