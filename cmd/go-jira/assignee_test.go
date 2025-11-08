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

func TestProcessAssignee(t *testing.T) {
	tests := []struct {
		name        string
		issues      []*jira.Issue
		assignee    *jira.User
		setupServer func(t *testing.T) *httptest.Server
		wantErr     bool
	}{
		{
			name: "successful assignee update for single issue",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue",
					},
				},
			},
			assignee: &jira.User{
				Name:         "john.doe",
				DisplayName:  "John Doe",
				EmailAddress: "john.doe@example.com",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if !strings.Contains(r.URL.Path, "/assignee") {
							t.Errorf("unexpected path: %s", r.URL.Path)
						}
						if r.Method != http.MethodPut {
							t.Errorf("expected PUT method, got %s", r.Method)
						}
						// Verify assignee name in request body
						body, err := io.ReadAll(r.Body)
						if err != nil {
							t.Errorf("failed to read body: %v", err)
						}
						var user jira.User
						if err := json.Unmarshal(body, &user); err != nil {
							t.Errorf("failed to unmarshal body: %v", err)
						}
						if user.Name != "john.doe" {
							t.Errorf("expected assignee name 'john.doe', got %s", user.Name)
						}
						w.WriteHeader(http.StatusNoContent)
					}),
				)
			},
			wantErr: false,
		},
		{
			name: "successful assignee update for multiple issues",
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
			assignee: &jira.User{
				Name:         "jane.smith",
				DisplayName:  "Jane Smith",
				EmailAddress: "jane.smith@example.com",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				var mu sync.Mutex
				updateCount := 0
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mu.Lock()
						updateCount++
						mu.Unlock()
						w.WriteHeader(http.StatusNoContent)
					}),
				)
			},
			wantErr: false,
		},
		{
			name: "api error during assignee update",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue",
					},
				},
			},
			assignee: &jira.User{
				Name: "invalid.user",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusBadRequest)
						w.Write([]byte(`{"errorMessages":["User not found"]}`))
					}),
				)
			},
			wantErr: true,
		},
		{
			name: "unexpected status code",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue",
					},
				},
			},
			assignee: &jira.User{
				Name: "john.doe",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK) // Should be 204
						w.Write([]byte(`{"status":"updated"}`))
					}),
				)
			},
			wantErr: true,
		},
		{
			name: "partial failure with multiple issues",
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
			assignee: &jira.User{
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
						// Fail the second request
						if count == 2 {
							w.WriteHeader(http.StatusForbidden)
							w.Write([]byte(`{"errorMessages":["Permission denied"]}`))
						} else {
							w.WriteHeader(http.StatusNoContent)
						}
					}),
				)
			},
			wantErr: true, // Should error because at least one failed
		},
		{
			name: "concurrent processing of many issues",
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
			assignee: &jira.User{
				Name: "john.doe",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				var mu sync.Mutex
				updateCount := 0
				maxConcurrent := 0
				currentConcurrent := 0
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mu.Lock()
						updateCount++
						currentConcurrent++
						if currentConcurrent > maxConcurrent {
							maxConcurrent = currentConcurrent
						}
						mu.Unlock()

						// Simulate some processing time
						// time.Sleep(10 * time.Millisecond)

						mu.Lock()
						currentConcurrent--
						mu.Unlock()

						w.WriteHeader(http.StatusNoContent)
					}),
				)
			},
			wantErr: false,
		},
		{
			name:   "empty issues list",
			issues: []*jira.Issue{},
			assignee: &jira.User{
				Name: "john.doe",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						t.Error("should not make any requests with empty issues list")
						w.WriteHeader(http.StatusNoContent)
					}),
				)
			},
			wantErr: false,
		},
		{
			name: "server error 500",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue",
					},
				},
			},
			assignee: &jira.User{
				Name: "john.doe",
			},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusInternalServerError)
						w.Write([]byte(`{"errorMessages":["Internal server error"]}`))
					}),
				)
			},
			wantErr: true,
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
			err = processAssignee(ctx, jiraClient, tt.issues, tt.assignee)

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
