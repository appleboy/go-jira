package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	jira "github.com/andygrunwald/go-jira"
)

//nolint:gocyclo
func TestAddComments(t *testing.T) {
	tests := []struct {
		name        string
		comment     string
		issues      []*jira.Issue
		user        *jira.User
		setupServer func(t *testing.T) *httptest.Server
		wantErr     bool
	}{
		{
			name:    "successful comment addition to single issue",
			comment: "Test comment",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue",
					},
				},
			},
			user: &jira.User{
				Name:         "john.doe",
				DisplayName:  "John Doe",
				EmailAddress: "john.doe@example.com",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if !strings.Contains(r.URL.Path, "/comment") {
							t.Errorf("unexpected path: %s", r.URL.Path)
						}
						if r.Method != http.MethodPost {
							t.Errorf("expected POST method, got %s", r.Method)
						}
						// Verify comment in request body
						body, err := io.ReadAll(r.Body)
						if err != nil {
							t.Errorf("failed to read body: %v", err)
						}
						var comment jira.Comment
						if err := json.Unmarshal(body, &comment); err != nil {
							t.Errorf("failed to unmarshal body: %v", err)
						}
						if comment.Body != "Test comment" {
							t.Errorf("expected comment body 'Test comment', got %s", comment.Body)
						}
						if comment.Name != "john.doe" {
							t.Errorf("expected comment author 'john.doe', got %s", comment.Name)
						}
						w.WriteHeader(http.StatusCreated)
						returnComment := jira.Comment{
							ID:   "12345",
							Body: "Test comment",
							Author: jira.User{
								Name: "john.doe",
							},
						}
						if err := json.NewEncoder(w).Encode(returnComment); err != nil {
							t.Errorf("failed to encode response: %v", err)
						}
					}),
				)
			},
			wantErr: false,
		},
		{
			name:    "successful comment addition to multiple issues",
			comment: "Closing this issue",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue 1",
					},
				},
				{
					Key: "DEF-456",
					Fields: &jira.IssueFields{
						Summary: "Test issue 2",
					},
				},
				{
					Key: "GHI-789",
					Fields: &jira.IssueFields{
						Summary: "Test issue 3",
					},
				},
			},
			user: &jira.User{
				Name: "jane.smith",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				var mu sync.Mutex
				commentCount := 0
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mu.Lock()
						commentCount++
						mu.Unlock()
						w.WriteHeader(http.StatusCreated)
						returnComment := jira.Comment{
							ID:   "12345",
							Body: "Closing this issue",
						}
						if err := json.NewEncoder(w).Encode(returnComment); err != nil {
							t.Errorf("failed to encode response: %v", err)
						}
					}),
				)
			},
			wantErr: false,
		},
		{
			name:    "api error during comment addition",
			comment: "Test comment",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue",
					},
				},
			},
			user: &jira.User{
				Name: "john.doe",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusBadRequest)
						if _, err := w.Write([]byte(`{"errorMessages":["Invalid comment"]}`)); err != nil {
							t.Errorf("failed to write response: %v", err)
						}
					}),
				)
			},
			wantErr: true,
		},
		{
			name:    "unexpected status code",
			comment: "Test comment",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue",
					},
				},
			},
			user: &jira.User{
				Name: "john.doe",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK) // Should be 201
						if _, err := w.Write([]byte(`{"id":"12345"}`)); err != nil {
							t.Errorf("failed to write response: %v", err)
						}
					}),
				)
			},
			wantErr: true,
		},
		{
			name:    "partial failure with multiple issues",
			comment: "Test comment",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue 1",
					},
				},
				{
					Key: "DEF-456",
					Fields: &jira.IssueFields{
						Summary: "Test issue 2",
					},
				},
				{
					Key: "GHI-789",
					Fields: &jira.IssueFields{
						Summary: "Test issue 3",
					},
				},
			},
			user: &jira.User{
				Name: "john.doe",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				var mu sync.Mutex
				requestCount := 0
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mu.Lock()
						requestCount++
						count := requestCount
						mu.Unlock()
						// Fail the first and third requests
						if count == 1 || count == 3 {
							w.WriteHeader(http.StatusForbidden)
							if _, err := w.Write([]byte(`{"errorMessages":["Permission denied"]}`)); err != nil {
								t.Errorf("failed to write response: %v", err)
							}
						} else {
							w.WriteHeader(http.StatusCreated)
							returnComment := jira.Comment{
								ID:   "12345",
								Body: "Test comment",
							}
							if err := json.NewEncoder(w).Encode(returnComment); err != nil {
								t.Errorf("failed to encode response: %v", err)
							}
						}
					}),
				)
			},
			wantErr: true, // Should error because at least one failed
		},
		{
			name:    "long comment with markdown",
			comment: "# Test Comment\n\nThis is a **bold** comment with *italics* and:\n- List item 1\n- List item 2\n\n```go\nfunc main() {}\n```",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue",
					},
				},
			},
			user: &jira.User{
				Name: "john.doe",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, err := io.ReadAll(r.Body)
						if err != nil {
							t.Errorf("failed to read body: %v", err)
						}
						var comment jira.Comment
						if err := json.Unmarshal(body, &comment); err != nil {
							t.Errorf("failed to unmarshal body: %v", err)
						}
						// Verify the comment body contains the expected content
						if !strings.Contains(comment.Body, "Test Comment") {
							t.Errorf("comment body should contain 'Test Comment'")
						}
						w.WriteHeader(http.StatusCreated)
						returnComment := jira.Comment{
							ID:   "12345",
							Body: comment.Body,
						}
						if err := json.NewEncoder(w).Encode(returnComment); err != nil {
							t.Errorf("failed to encode response: %v", err)
						}
					}),
				)
			},
			wantErr: false,
		},
		{
			name:    "concurrent processing of many issues",
			comment: "Test comment",
			issues: func() []*jira.Issue {
				issues := make([]*jira.Issue, 20)
				for i := 0; i < 20; i++ {
					issues[i] = &jira.Issue{
						Key: "TEST-" + string(rune(i+100)),
						Fields: &jira.IssueFields{
							Summary: "Test issue",
						},
					}
				}
				return issues
			}(),
			user: &jira.User{
				Name: "john.doe",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				var mu sync.Mutex
				commentCount := 0
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mu.Lock()
						commentCount++
						mu.Unlock()
						w.WriteHeader(http.StatusCreated)
						returnComment := jira.Comment{
							ID:   "12345",
							Body: "Test comment",
						}
						if err := json.NewEncoder(w).Encode(returnComment); err != nil {
							t.Errorf("failed to encode response: %v", err)
						}
					}),
				)
			},
			wantErr: false,
		},
		{
			name:    "empty issues list",
			comment: "Test comment",
			issues:  []*jira.Issue{},
			user: &jira.User{
				Name: "john.doe",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						t.Error("should not make any requests with empty issues list")
						w.WriteHeader(http.StatusCreated)
					}),
				)
			},
			wantErr: false,
		},
		{
			name:    "server error 500",
			comment: "Test comment",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue",
					},
				},
			},
			user: &jira.User{
				Name: "john.doe",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusInternalServerError)
						if _, err := w.Write([]byte(`{"errorMessages":["Internal server error"]}`)); err != nil {
							t.Errorf("failed to write response: %v", err)
						}
					}),
				)
			},
			wantErr: true,
		},
		{
			name:    "empty comment text",
			comment: "",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue",
					},
				},
			},
			user: &jira.User{
				Name: "john.doe",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, err := io.ReadAll(r.Body)
						if err != nil {
							t.Errorf("failed to read body: %v", err)
						}
						var comment jira.Comment
						if err := json.Unmarshal(body, &comment); err != nil {
							t.Errorf("failed to unmarshal body: %v", err)
						}
						if comment.Body != "" {
							t.Errorf("expected empty comment body, got %s", comment.Body)
						}
						w.WriteHeader(http.StatusCreated)
						returnComment := jira.Comment{
							ID:   "12345",
							Body: "",
						}
						if err := json.NewEncoder(w).Encode(returnComment); err != nil {
							t.Errorf("failed to encode response: %v", err)
						}
					}),
				)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer(t)
			defer server.Close()

			jiraClient, err := jira.NewClient(nil, server.URL)
			if err != nil {
				t.Fatalf("failed to create jira client: %v", err)
			}

			ctx := context.Background()
			err = addComments(ctx, jiraClient, tt.comment, tt.issues, tt.user)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
