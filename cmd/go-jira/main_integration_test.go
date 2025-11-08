package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	jira "github.com/andygrunwald/go-jira"
)

// setupTestServer creates a comprehensive mock Jira server for integration tests
func setupTestServer(options testServerOptions) *httptest.Server {
	var mu sync.Mutex
	requestLog := make([]string, 0)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestLog = append(requestLog, r.Method+" "+r.URL.Path)
		mu.Unlock()

		// Handle /rest/api/2/myself endpoint
		if r.URL.Path == "/rest/api/2/myself" {
			if options.selfError {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"errorMessages":["Unauthorized"]}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			user := jira.User{
				Name:         "testuser",
				DisplayName:  "Test User",
				EmailAddress: "test@example.com",
			}
			json.NewEncoder(w).Encode(user)
			return
		}

		// Handle /rest/api/2/user endpoint (get user by username)
		if r.URL.Path == "/rest/api/2/user" {
			if options.assigneeError {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"errorMessages":["User not found"]}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			user := jira.User{
				Name:         "assignee",
				DisplayName:  "Assignee User",
				EmailAddress: "assignee@example.com",
			}
			json.NewEncoder(w).Encode(user)
			return
		}

		// Handle /rest/api/2/resolution endpoint
		if r.URL.Path == "/rest/api/2/resolution" {
			if options.resolutionError {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"errorMessages":["Internal error"]}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			resolutions := []jira.Resolution{
				{ID: "1", Name: "Fixed"},
				{ID: "2", Name: "Done"},
			}
			json.NewEncoder(w).Encode(resolutions)
			return
		}

		// Handle issue retrieval /rest/api/2/issue/{issueKey}
		if strings.HasPrefix(r.URL.Path, "/rest/api/2/issue/") && r.Method == http.MethodGet {
			issueKey := r.URL.Path[len("/rest/api/2/issue/"):]

			// Check if it's a comment or assignee endpoint
			if strings.Contains(issueKey, "/comment") || strings.Contains(issueKey, "/assignee") ||
				strings.Contains(issueKey, "/transitions") {
				goto handleOtherEndpoints
			}

			if options.issueError {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"errorMessages":["Issue not found"]}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			issue := jira.Issue{
				Key: issueKey,
				Fields: &jira.IssueFields{
					Summary: "Test issue " + issueKey,
					Status: &jira.Status{
						Name: "Open",
					},
				},
				Transitions: []jira.Transition{
					{ID: "1", Name: "Done"},
				},
			}
			json.NewEncoder(w).Encode(issue)
			return
		}

	handleOtherEndpoints:
		// Handle transitions /rest/api/2/issue/{issueKey}/transitions
		if strings.Contains(r.URL.Path, "/transitions") {
			if r.Method == http.MethodPost {
				if options.transitionError {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"errorMessages":["Invalid transition"]}`))
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		// Handle assignee updates /rest/api/2/issue/{issueKey}/assignee
		if strings.Contains(r.URL.Path, "/assignee") {
			if options.assigneeUpdateError {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"errorMessages":["Permission denied"]}`))
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Handle comments /rest/api/2/issue/{issueKey}/comment
		if strings.Contains(r.URL.Path, "/comment") {
			if options.commentError {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"errorMessages":["Invalid comment"]}`))
				return
			}
			w.WriteHeader(http.StatusCreated)
			comment := jira.Comment{
				ID:   "12345",
				Body: "Test comment",
			}
			json.NewEncoder(w).Encode(comment)
			return
		}

		// Default response
		w.WriteHeader(http.StatusNotFound)
	}))
}

type testServerOptions struct {
	selfError           bool
	assigneeError       bool
	issueError          bool
	resolutionError     bool
	transitionError     bool
	assigneeUpdateError bool
	commentError        bool
}

func TestRun(t *testing.T) {
	tests := []struct {
		name          string
		envVars       map[string]string
		serverOptions testServerOptions
		wantErr       bool
		errContains   string
	}{
		{
			name: "successful complete workflow",
			envVars: map[string]string{
				"INPUT_REF":        "ABC-123",
				"INPUT_TOKEN":      "testtoken",
				"INPUT_INSECURE":   "true",
				"INPUT_TRANSITION": "Done",
				"INPUT_COMMENT":    "Test comment",
			},
			serverOptions: testServerOptions{},
			wantErr:       false,
		},
		{
			name: "successful with assignee",
			envVars: map[string]string{
				"INPUT_REF":      "ABC-123",
				"INPUT_TOKEN":    "testtoken",
				"INPUT_INSECURE": "true",
				"INPUT_ASSIGNEE": "assignee",
			},
			serverOptions: testServerOptions{},
			wantErr:       false,
		},
		{
			name: "successful with resolution",
			envVars: map[string]string{
				"INPUT_REF":        "ABC-123",
				"INPUT_TOKEN":      "testtoken",
				"INPUT_INSECURE":   "true",
				"INPUT_TRANSITION": "Done",
				"INPUT_RESOLUTION": "Fixed",
			},
			serverOptions: testServerOptions{},
			wantErr:       false,
		},
		{
			name: "successful with markdown comment",
			envVars: map[string]string{
				"INPUT_REF":      "ABC-123",
				"INPUT_TOKEN":    "testtoken",
				"INPUT_INSECURE": "true",
				"INPUT_COMMENT":  "# Test\n**bold**",
				"INPUT_MARKDOWN": "true",
			},
			serverOptions: testServerOptions{},
			wantErr:       false,
		},
		{
			name: "missing base_url",
			envVars: map[string]string{
				"INPUT_BASE_URL": "", // Empty to trigger validation error
				"INPUT_REF":      "ABC-123",
				"INPUT_TOKEN":    "testtoken",
			},
			serverOptions: testServerOptions{},
			wantErr:       true,
			errContains:   "base_url is required",
		},
		{
			name: "missing ref",
			envVars: map[string]string{
				"INPUT_BASE_URL": "https://jira.example.com",
				"INPUT_REF":      "", // Empty to trigger validation error
				"INPUT_TOKEN":    "testtoken",
			},
			serverOptions: testServerOptions{},
			wantErr:       true,
			errContains:   "ref is required",
		},
		{
			name: "missing authentication",
			envVars: map[string]string{
				"INPUT_BASE_URL": "https://jira.example.com",
				"INPUT_REF":      "ABC-123",
			},
			serverOptions: testServerOptions{},
			wantErr:       true,
			errContains:   "authentication credentials required",
		},
		{
			name: "get self error",
			envVars: map[string]string{
				"INPUT_REF":      "ABC-123",
				"INPUT_TOKEN":    "testtoken",
				"INPUT_INSECURE": "true",
			},
			serverOptions: testServerOptions{
				selfError: true,
			},
			wantErr:     true,
			errContains: "error getting self",
		},
		{
			name: "get assignee error",
			envVars: map[string]string{
				"INPUT_REF":      "ABC-123",
				"INPUT_TOKEN":    "testtoken",
				"INPUT_INSECURE": "true",
				"INPUT_ASSIGNEE": "invalid",
			},
			serverOptions: testServerOptions{
				assigneeError: true,
			},
			wantErr:     true,
			errContains: "error getting assignee",
		},
		{
			name: "no issues found",
			envVars: map[string]string{
				"INPUT_REF":      "No issues here",
				"INPUT_TOKEN":    "testtoken",
				"INPUT_INSECURE": "true",
			},
			serverOptions: testServerOptions{},
			wantErr:       true,
			errContains:   "no issue keys found",
		},
		{
			name: "issue retrieval error",
			envVars: map[string]string{
				"INPUT_REF":      "ABC-123",
				"INPUT_TOKEN":    "testtoken",
				"INPUT_INSECURE": "true",
			},
			serverOptions: testServerOptions{
				issueError: true,
			},
			wantErr:     true,
			errContains: "no issues found",
		},
		{
			name: "resolution retrieval error",
			envVars: map[string]string{
				"INPUT_REF":        "ABC-123",
				"INPUT_TOKEN":      "testtoken",
				"INPUT_INSECURE":   "true",
				"INPUT_TRANSITION": "Done",
				"INPUT_RESOLUTION": "Fixed",
			},
			serverOptions: testServerOptions{
				resolutionError: true,
			},
			wantErr:     true,
			errContains: "error getting resolution",
		},
		{
			name: "transition error",
			envVars: map[string]string{
				"INPUT_REF":        "ABC-123",
				"INPUT_TOKEN":      "testtoken",
				"INPUT_INSECURE":   "true",
				"INPUT_TRANSITION": "Done",
			},
			serverOptions: testServerOptions{
				transitionError: true,
			},
			wantErr:     true,
			errContains: "error processing transitions",
		},
		{
			name: "assignee update error",
			envVars: map[string]string{
				"INPUT_REF":      "ABC-123",
				"INPUT_TOKEN":    "testtoken",
				"INPUT_INSECURE": "true",
				"INPUT_ASSIGNEE": "assignee",
			},
			serverOptions: testServerOptions{
				assigneeUpdateError: true,
			},
			wantErr:     true,
			errContains: "error processing assignee",
		},
		{
			name: "comment error",
			envVars: map[string]string{
				"INPUT_REF":      "ABC-123",
				"INPUT_TOKEN":    "testtoken",
				"INPUT_INSECURE": "true",
				"INPUT_COMMENT":  "Test comment",
			},
			serverOptions: testServerOptions{
				commentError: true,
			},
			wantErr:     true,
			errContains: "error adding comments",
		},
		{
			name: "multiple issues workflow",
			envVars: map[string]string{
				"INPUT_REF":        "ABC-123 DEF-456 GHI-789",
				"INPUT_TOKEN":      "testtoken",
				"INPUT_INSECURE":   "true",
				"INPUT_TRANSITION": "Done",
				"INPUT_COMMENT":    "Closing all issues",
				"INPUT_ASSIGNEE":   "assignee",
			},
			serverOptions: testServerOptions{},
			wantErr:       false,
		},
		{
			name: "custom issue pattern",
			envVars: map[string]string{
				"INPUT_REF":          "Check PROJ-123 and PROJ-456",
				"INPUT_TOKEN":        "testtoken",
				"INPUT_INSECURE":     "true",
				"INPUT_ISSUE_FORMAT": `(PROJ-\d+)`,
			},
			serverOptions: testServerOptions{},
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and clear environment
			originalEnv := make(map[string]string)
			envVars := []string{
				"INPUT_BASE_URL", "INPUT_INSECURE", "INPUT_USERNAME", "INPUT_PASSWORD",
				"INPUT_TOKEN", "INPUT_REF", "INPUT_ISSUE_FORMAT", "INPUT_TRANSITION",
				"INPUT_RESOLUTION", "INPUT_COMMENT", "INPUT_ASSIGNEE", "INPUT_MARKDOWN",
				"INPUT_DEBUG",
			}
			for _, key := range envVars {
				originalEnv[key] = os.Getenv(key)
				os.Unsetenv(key)
			}
			defer func() {
				for key, val := range originalEnv {
					if val == "" {
						os.Unsetenv(key)
					} else {
						os.Setenv(key, val)
					}
				}
			}()

			// Setup test server only if BASE_URL is not explicitly tested to be missing
			if _, hasBaseURL := tt.envVars["INPUT_BASE_URL"]; !hasBaseURL {
				server := setupTestServer(tt.serverOptions)
				defer server.Close()
				tt.envVars["INPUT_BASE_URL"] = server.URL
			}

			// Set environment variables for test
			for key, val := range tt.envVars {
				os.Setenv(key, val)
			}

			// Run the function
			err := run("")

			// Check results
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want error containing %v", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRunWithEnvFile(t *testing.T) {
	// Create a temporary .env file
	envContent := `INPUT_BASE_URL=https://jira.example.com
INPUT_TOKEN=testtoken
INPUT_REF=ABC-123
`
	tmpfile, err := os.CreateTemp("", "test.env")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(envContent)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Setup test server
	server := setupTestServer(testServerOptions{})
	defer server.Close()

	// Override base URL and insecure with test server
	os.Setenv("INPUT_BASE_URL", server.URL)
	os.Setenv("INPUT_INSECURE", "true")
	defer func() {
		os.Unsetenv("INPUT_BASE_URL")
		os.Unsetenv("INPUT_INSECURE")
	}()

	// Run with env file
	err = run(tmpfile.Name())
	if err != nil {
		t.Errorf("unexpected error with env file: %v", err)
	}
}
