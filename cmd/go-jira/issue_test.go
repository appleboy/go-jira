package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jira "github.com/andygrunwald/go-jira"
)

func TestProcessIssues(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		setupServer func() *httptest.Server
		wantErr     bool
		wantCount   int
	}{
		{
			name: "successful processing of multiple issues",
			config: Config{
				ref:          "ABC-123 DEF-456",
				issuePattern: "",
			},
			setupServer: func() *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						issueKey := r.URL.Path[len("/rest/api/2/issue/"):]
						w.WriteHeader(http.StatusOK)
						issue := jira.Issue{
							Key: issueKey,
							Fields: &jira.IssueFields{
								Summary: "Test issue " + issueKey,
								Status: &jira.Status{
									Name: "Open",
								},
							},
						}
						if err := json.NewEncoder(w).Encode(issue); err != nil {
							t.Errorf("failed to encode response: %v", err)
						}
					}),
				)
			},
			wantErr:   false,
			wantCount: 2,
		},
		{
			name: "no issue keys found",
			config: Config{
				ref:          "No issues here",
				issuePattern: "",
			},
			setupServer: func() *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK)
					}),
				)
			},
			wantErr:   true,
			wantCount: 0,
		},
		{
			name: "partial success with some failed requests",
			config: Config{
				ref:          "ABC-123 DEF-456 GHI-789",
				issuePattern: "",
			},
			setupServer: func() *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						issueKey := r.URL.Path[len("/rest/api/2/issue/"):]
						// Simulate failure for DEF-456
						if issueKey == "DEF-456" {
							w.WriteHeader(http.StatusNotFound)
							if _, err := w.Write([]byte(`{"errorMessages":["Issue not found"]}`)); err != nil {
								t.Errorf("failed to write response: %v", err)
							}
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
						}
						if err := json.NewEncoder(w).Encode(issue); err != nil {
							t.Errorf("failed to encode response: %v", err)
						}
					}),
				)
			},
			wantErr:   false,
			wantCount: 2, // Should succeed for ABC-123 and GHI-789
		},
		{
			name: "custom issue pattern",
			config: Config{
				ref:          "Check PROJ-1234 and PROJ-5678",
				issuePattern: `(PROJ-\d+)`,
			},
			setupServer: func() *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						issueKey := r.URL.Path[len("/rest/api/2/issue/"):]
						w.WriteHeader(http.StatusOK)
						issue := jira.Issue{
							Key: issueKey,
							Fields: &jira.IssueFields{
								Summary: "Test issue " + issueKey,
							},
						}
						if err := json.NewEncoder(w).Encode(issue); err != nil {
							t.Errorf("failed to encode response: %v", err)
						}
					}),
				)
			},
			wantErr:   false,
			wantCount: 2,
		},
		{
			name: "context timeout",
			config: Config{
				ref:          "ABC-123",
				issuePattern: "",
			},
			setupServer: func() *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						// Simulate slow response
						time.Sleep(200 * time.Millisecond)
						w.WriteHeader(http.StatusOK)
					}),
				)
			},
			wantErr:   false,
			wantCount: 0, // With very short timeout, we might get 0 results
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			jiraClient, err := jira.NewClient(nil, server.URL)
			if err != nil {
				t.Fatalf("failed to create jira client: %v", err)
			}

			ctx := context.Background()
			// Use a short timeout for the timeout test
			if tt.name == "context timeout" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
			}

			issues, err := processIssues(ctx, jiraClient, tt.config)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(issues) != tt.wantCount {
				t.Errorf("got %d issues, want %d", len(issues), tt.wantCount)
			}

			// Verify issue fields for non-error cases
			for _, issue := range issues {
				if issue.Key == "" {
					t.Error("issue key should not be empty")
				}
			}
		})
	}
}

func TestGetIssueKeys_ExtendedCases(t *testing.T) {
	tests := []struct {
		name         string
		ref          string
		issuePattern string
		want         []string
	}{
		{
			name:         "multiple issues with different projects",
			ref:          "Fix ABC-123, DEF-456, and XYZ-789",
			issuePattern: "",
			want:         []string{"ABC-123", "DEF-456", "XYZ-789"},
		},
		{
			name:         "issues in commit message format",
			ref:          "[ABC-123] Fix bug\n\nAlso resolves DEF-456",
			issuePattern: "",
			want:         []string{"ABC-123", "DEF-456"},
		},
		{
			name:         "single character project key",
			ref:          "A-1 B-2 C-3",
			issuePattern: "",
			want:         []string{"A-1", "B-2", "C-3"},
		},
		{
			name:         "ten character project key",
			ref:          "ABCDEFGHIJ-123",
			issuePattern: "",
			want:         []string{"ABCDEFGHIJ-123"},
		},
		{
			name:         "issue number starts with zero (invalid)",
			ref:          "ABC-0123",
			issuePattern: "",
			want:         []string{}, // Should not match numbers starting with 0
		},
		{
			name:         "large issue number",
			ref:          "ABC-999999",
			issuePattern: "",
			want:         []string{"ABC-999999"},
		},
		{
			name:         "custom pattern with specific project",
			ref:          "ABC-123 XYZ-456 DEF-789",
			issuePattern: `(ABC-[0-9]+)`,
			want:         []string{"ABC-123"},
		},
		{
			name:         "custom pattern for multiple projects",
			ref:          "ABC-123 XYZ-456 DEF-789",
			issuePattern: `(ABC-[0-9]+|XYZ-[0-9]+)`,
			want:         []string{"ABC-123", "XYZ-456"},
		},
		{
			name:         "issues with surrounding punctuation",
			ref:          "Fix: ABC-123, DEF-456. And GHI-789!",
			issuePattern: "",
			want:         []string{"ABC-123", "DEF-456", "GHI-789"},
		},
		{
			name:         "duplicate removal maintains order",
			ref:          "ABC-123 DEF-456 ABC-123 GHI-789 DEF-456",
			issuePattern: "",
			want:         []string{"ABC-123", "DEF-456", "GHI-789"},
		},
		{
			name:         "empty string",
			ref:          "",
			issuePattern: "",
			want:         []string{},
		},
		{
			name:         "string with no valid issues",
			ref:          "This has no valid issues abc-123 123-ABC",
			issuePattern: "",
			want:         []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getIssueKeys(tt.ref, tt.issuePattern)
			if len(got) != len(tt.want) {
				t.Errorf("getIssueKeys() returned %d keys, want %d. Got: %v, Want: %v",
					len(got), len(tt.want), got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("getIssueKeys()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
