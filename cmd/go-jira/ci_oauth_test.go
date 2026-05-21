package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jira "github.com/andygrunwald/go-jira"
)

// oauthEnvServer mocks a Jira DC instance for the CI (oauth-env) path: the
// token endpoint (refresh) plus the API endpoints `run` touches.
func oauthEnvServer(t *testing.T, refreshErr string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/oauth2/latest/token", func(w http.ResponseWriter, _ *http.Request) {
		if refreshErr != "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"` + refreshErr + `"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-ci", "token_type": "bearer",
			"expires_in": 7200, "refresh_token": "refresh-ci-rotated",
		})
	})
	mux.HandleFunc("/rest/api/2/myself", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jira.User{Name: "ci", DisplayName: "CI Bot"})
	})
	mux.HandleFunc("/rest/api/2/issue/", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/rest/api/2/issue/")
		_ = json.NewEncoder(w).Encode(jira.Issue{
			Key:    key,
			Fields: &jira.IssueFields{Summary: "s", Status: &jira.Status{Name: "Open"}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRunOAuthEnvMode(t *testing.T) {
	srv := oauthEnvServer(t, "")
	out := filepath.Join(t.TempDir(), "rotated.txt")

	t.Setenv("INPUT_BASE_URL", srv.URL)
	t.Setenv("INPUT_INSECURE", "true")
	t.Setenv("INPUT_REF", "ABC-123")
	t.Setenv(envOAuthClientID, "client-abc")
	t.Setenv(envOAuthClientSecret, "secret-xyz")
	t.Setenv(envOAuthRefreshToken, "injected-refresh")
	t.Setenv(envOAuthRefreshTokenOutput, out)

	if err := run(nil); err != nil {
		t.Fatalf("run in oauth-env mode: %v", err)
	}

	// The initial refresh rotates the token; it must be written to the output.
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read rotation output: %v", err)
	}
	if string(got) != "refresh-ci-rotated" {
		t.Errorf("rotation output = %q, want refresh-ci-rotated", got)
	}
}

func TestRunOAuthEnvInvalidGrant(t *testing.T) {
	srv := oauthEnvServer(t, "invalid_grant")

	t.Setenv("INPUT_BASE_URL", srv.URL)
	t.Setenv("INPUT_INSECURE", "true")
	t.Setenv("INPUT_REF", "ABC-123")
	t.Setenv(envOAuthClientID, "client-abc")
	t.Setenv(envOAuthClientSecret, "secret-xyz")
	t.Setenv(envOAuthRefreshToken, "dead-refresh")

	err := run(nil)
	if err == nil {
		t.Fatal("expected error for invalid_grant refresh token")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error %q should mention invalid_grant", err)
	}
}
