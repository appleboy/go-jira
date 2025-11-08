package main

import (
	"os"
	"testing"
)

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

			got := loadConfig()

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
