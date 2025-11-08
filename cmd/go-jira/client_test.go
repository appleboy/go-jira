package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	jira "github.com/andygrunwald/go-jira"
)

func TestCreateHTTPClient(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		verify func(t *testing.T, client *http.Client)
	}{
		{
			name: "basic client without auth",
			config: Config{
				baseURL: "https://jira.example.com",
			},
			verify: func(t *testing.T, client *http.Client) {
				if client == nil {
					t.Error("expected non-nil client")
					return
				}
				// Default transport or nil is acceptable
			},
		},
		{
			name: "client with basic auth",
			config: Config{
				baseURL:  "https://jira.example.com",
				username: "testuser",
				password: "testpass",
			},
			verify: func(t *testing.T, client *http.Client) {
				if client == nil {
					t.Error("expected non-nil client")
					return
				}
				// Transport should be set with BasicAuth
				if client.Transport == nil {
					t.Error("expected non-nil transport for basic auth")
				}
			},
		},
		{
			name: "client with bearer token",
			config: Config{
				baseURL: "https://jira.example.com",
				token:   "testtoken123",
			},
			verify: func(t *testing.T, client *http.Client) {
				if client == nil {
					t.Error("expected non-nil client")
					return
				}
				// Transport should be set with BearerAuth
				if client.Transport == nil {
					t.Error("expected non-nil transport for bearer auth")
				}
			},
		},
		{
			name: "client with insecure mode",
			config: Config{
				baseURL:  "https://jira.example.com",
				insecure: "true",
				username: "testuser",
				password: "testpass",
			},
			verify: func(t *testing.T, client *http.Client) {
				if client == nil {
					t.Error("expected non-nil client")
					return
				}
				if client.Transport == nil {
					t.Error("expected non-nil transport for insecure mode")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := createHTTPClient(tt.config)
			tt.verify(t, client)
		})
	}
}

func TestGetSelf(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func() *httptest.Server
		wantErr     bool
		wantUser    *jira.User
	}{
		{
			name: "successful get self",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path != "/rest/api/2/myself" {
							t.Errorf("unexpected path: %s", r.URL.Path)
						}
						w.WriteHeader(http.StatusOK)
						user := jira.User{
							Name:         "testuser",
							DisplayName:  "Test User",
							EmailAddress: "test@example.com",
						}
						if err := json.NewEncoder(w).Encode(user); err != nil {
							t.Errorf("failed to encode response: %v", err)
						}
					}),
				)
			},
			wantErr: false,
			wantUser: &jira.User{
				Name:         "testuser",
				DisplayName:  "Test User",
				EmailAddress: "test@example.com",
			},
		},
		{
			name: "api error",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusUnauthorized)
						if _, err := w.Write([]byte(`{"errorMessages":["Unauthorized"]}`)); err != nil {
							t.Errorf("failed to write response: %v", err)
						}
					}),
				)
			},
			wantErr:  true,
			wantUser: nil,
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
			user, err := getSelf(ctx, jiraClient)

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

			if user.Name != tt.wantUser.Name {
				t.Errorf("user name = %v, want %v", user.Name, tt.wantUser.Name)
			}
			if user.DisplayName != tt.wantUser.DisplayName {
				t.Errorf(
					"user display name = %v, want %v",
					user.DisplayName,
					tt.wantUser.DisplayName,
				)
			}
			if user.EmailAddress != tt.wantUser.EmailAddress {
				t.Errorf("user email = %v, want %v", user.EmailAddress, tt.wantUser.EmailAddress)
			}
		})
	}
}

func TestGetUser(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		setupServer func() *httptest.Server
		wantErr     bool
		wantUser    *jira.User
	}{
		{
			name:     "successful get user",
			username: "john.doe",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path != "/rest/api/2/user" {
							t.Errorf("unexpected path: %s", r.URL.Path)
						}
						username := r.URL.Query().Get("username")
						if username != "john.doe" {
							t.Errorf("unexpected username: %s", username)
						}
						w.WriteHeader(http.StatusOK)
						user := jira.User{
							Name:         "john.doe",
							DisplayName:  "John Doe",
							EmailAddress: "john.doe@example.com",
						}
						if err := json.NewEncoder(w).Encode(user); err != nil {
							t.Errorf("failed to encode response: %v", err)
						}
					}),
				)
			},
			wantErr: false,
			wantUser: &jira.User{
				Name:         "john.doe",
				DisplayName:  "John Doe",
				EmailAddress: "john.doe@example.com",
			},
		},
		{
			name:        "empty username returns nil",
			username:    "",
			setupServer: func() *httptest.Server { return nil },
			wantErr:     false,
			wantUser:    nil,
		},
		{
			name:     "user not found",
			username: "nonexistent",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusNotFound)
						if _, err := w.Write([]byte(`{"errorMessages":["User not found"]}`)); err != nil {
							t.Errorf("failed to write response: %v", err)
						}
					}),
				)
			},
			wantErr:  true,
			wantUser: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.username == "" {
				user, err := getUser(context.Background(), nil, tt.username)
				if err != nil {
					t.Errorf("unexpected error for empty username: %v", err)
				}
				if user != nil {
					t.Error("expected nil user for empty username")
				}
				return
			}

			server := tt.setupServer()
			defer server.Close()

			jiraClient, err := jira.NewClient(nil, server.URL)
			if err != nil {
				t.Fatalf("failed to create jira client: %v", err)
			}

			ctx := context.Background()
			user, err := getUser(ctx, jiraClient, tt.username)

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

			if tt.wantUser != nil {
				if user.Name != tt.wantUser.Name {
					t.Errorf("user name = %v, want %v", user.Name, tt.wantUser.Name)
				}
				if user.DisplayName != tt.wantUser.DisplayName {
					t.Errorf(
						"user display name = %v, want %v",
						user.DisplayName,
						tt.wantUser.DisplayName,
					)
				}
			}
		})
	}
}

func TestGetResolutionID(t *testing.T) {
	tests := []struct {
		name        string
		resolution  string
		setupServer func() *httptest.Server
		wantErr     bool
		wantID      string
	}{
		{
			name:       "successful resolution lookup",
			resolution: "Fixed",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path != "/rest/api/2/resolution" {
							t.Errorf("unexpected path: %s", r.URL.Path)
						}
						w.WriteHeader(http.StatusOK)
						resolutions := []jira.Resolution{
							{ID: "1", Name: "Fixed"},
							{ID: "2", Name: "Won't Fix"},
							{ID: "3", Name: "Duplicate"},
						}
						if err := json.NewEncoder(w).Encode(resolutions); err != nil {
							t.Errorf("failed to encode response: %v", err)
						}
					}),
				)
			},
			wantErr: false,
			wantID:  "1",
		},
		{
			name:       "case insensitive match",
			resolution: "fixed",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK)
						resolutions := []jira.Resolution{
							{ID: "1", Name: "Fixed"},
							{ID: "2", Name: "Won't Fix"},
						}
						if err := json.NewEncoder(w).Encode(resolutions); err != nil {
							t.Errorf("failed to encode response: %v", err)
						}
					}),
				)
			},
			wantErr: false,
			wantID:  "1",
		},
		{
			name:       "resolution not found",
			resolution: "NonExistent",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK)
						resolutions := []jira.Resolution{
							{ID: "1", Name: "Fixed"},
							{ID: "2", Name: "Won't Fix"},
						}
						if err := json.NewEncoder(w).Encode(resolutions); err != nil {
							t.Errorf("failed to encode response: %v", err)
						}
					}),
				)
			},
			wantErr: false,
			wantID:  "",
		},
		{
			name:       "api error",
			resolution: "Fixed",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusInternalServerError)
						if _, err := w.Write([]byte(`{"errorMessages":["Internal error"]}`)); err != nil {
							t.Errorf("failed to write response: %v", err)
						}
					}),
				)
			},
			wantErr: true,
			wantID:  "",
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
			id, err := getResolutionID(ctx, jiraClient, tt.resolution)

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

			if id != tt.wantID {
				t.Errorf("resolution ID = %v, want %v", id, tt.wantID)
			}
		})
	}
}
