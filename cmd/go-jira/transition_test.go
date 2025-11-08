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

func TestProcessTransitions(t *testing.T) {
	tests := []struct {
		name         string
		toTransition string
		resolution   string
		issues       []*jira.Issue
		setupServer  func(t *testing.T) *httptest.Server
		wantErr      bool
	}{
		{
			name:         "successful transition without resolution",
			toTransition: "Done",
			resolution:   "",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue 1",
						Status: &jira.Status{
							Name: "In Progress",
						},
					},
					Transitions: []jira.Transition{
						{ID: "1", Name: "Done"},
						{ID: "2", Name: "In Review"},
					},
				},
				{
					Key: "DEF-456",
					Fields: &jira.IssueFields{
						Summary: "Test issue 2",
						Status: &jira.Status{
							Name: "Open",
						},
					},
					Transitions: []jira.Transition{
						{ID: "1", Name: "Done"},
						{ID: "3", Name: "In Progress"},
					},
				},
			},
			setupServer: func(t *testing.T) *httptest.Server {
				var mu sync.Mutex
				transitionCount := 0
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if !strings.Contains(r.URL.Path, "/transitions") {
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
					mu.Lock()
					transitionCount++
					mu.Unlock()
					w.WriteHeader(http.StatusNoContent)
				}))
			},
			wantErr: false,
		},
		{
			name:         "successful transition with resolution",
			toTransition: "Done",
			resolution:   "10", // Resolution ID
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue",
						Status: &jira.Status{
							Name: "In Progress",
						},
					},
					Transitions: []jira.Transition{
						{ID: "1", Name: "Done"},
					},
				},
			},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Verify resolution is sent
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("failed to read body: %v", err)
					}
					var payload map[string]interface{}
					if err := json.Unmarshal(body, &payload); err != nil {
						t.Errorf("failed to unmarshal body: %v", err)
					}
					if fields, ok := payload["fields"].(map[string]interface{}); ok {
						if resolution, ok := fields["resolution"].(map[string]interface{}); ok {
							if id, ok := resolution["id"].(string); ok && id != "10" {
								t.Errorf("expected resolution ID 10, got %s", id)
							}
						}
					}
					w.WriteHeader(http.StatusNoContent)
				}))
			},
			wantErr: false,
		},
		{
			name:         "transition not found for issue",
			toTransition: "NonExistent",
			resolution:   "",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue",
						Status: &jira.Status{
							Name: "Open",
						},
					},
					Transitions: []jira.Transition{
						{ID: "1", Name: "Done"},
						{ID: "2", Name: "In Progress"},
					},
				},
			},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Error("should not make any requests when transition not found")
					w.WriteHeader(http.StatusNoContent)
				}))
			},
			wantErr: false, // Not an error, just a warning
		},
		{
			name:         "case insensitive transition matching",
			toTransition: "done",
			resolution:   "",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue",
						Status: &jira.Status{
							Name: "Open",
						},
					},
					Transitions: []jira.Transition{
						{ID: "1", Name: "Done"},
					},
				},
			},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				}))
			},
			wantErr: false,
		},
		{
			name:         "api error during transition",
			toTransition: "Done",
			resolution:   "",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue",
						Status: &jira.Status{
							Name: "Open",
						},
					},
					Transitions: []jira.Transition{
						{ID: "1", Name: "Done"},
					},
				},
			},
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"errorMessages":["Invalid transition"]}`))
				}))
			},
			wantErr: true,
		},
		{
			name:         "partial failure with multiple issues",
			toTransition: "Done",
			resolution:   "",
			issues: []*jira.Issue{
				{
					Key: "ABC-123",
					Fields: &jira.IssueFields{
						Summary: "Test issue 1",
						Status: &jira.Status{
							Name: "Open",
						},
					},
					Transitions: []jira.Transition{
						{ID: "1", Name: "Done"},
					},
				},
				{
					Key: "DEF-456",
					Fields: &jira.IssueFields{
						Summary: "Test issue 2",
						Status: &jira.Status{
							Name: "Open",
						},
					},
					Transitions: []jira.Transition{
						{ID: "1", Name: "Done"},
					},
				},
			},
			setupServer: func(t *testing.T) *httptest.Server {
				var mu sync.Mutex
				requestCount := 0
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					requestCount++
					count := requestCount
					mu.Unlock()
					// Fail the first request, succeed the second
					if count == 1 {
						w.WriteHeader(http.StatusInternalServerError)
					} else {
						w.WriteHeader(http.StatusNoContent)
					}
				}))
			},
			wantErr: true, // Should error because at least one failed
		},
		{
			name:         "concurrent processing of many issues",
			toTransition: "Done",
			resolution:   "",
			issues:       createManyIssues(10),
			setupServer: func(t *testing.T) *httptest.Server {
				var mu sync.Mutex
				transitionCount := 0
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					transitionCount++
					mu.Unlock()
					w.WriteHeader(http.StatusNoContent)
				}))
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
			err = processTransitions(ctx, jiraClient, tt.toTransition, tt.resolution, tt.issues)

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

// Helper function to create many test issues
func createManyIssues(count int) []*jira.Issue {
	issues := make([]*jira.Issue, count)
	for i := 0; i < count; i++ {
		issues[i] = &jira.Issue{
			Key: "TEST-" + string(rune(i+100)),
			Fields: &jira.IssueFields{
				Summary: "Test issue",
				Status: &jira.Status{
					Name: "Open",
				},
			},
			Transitions: []jira.Transition{
				{ID: "1", Name: "Done"},
			},
		}
	}
	return issues
}
