package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	jira "github.com/andygrunwald/go-jira"
	"github.com/spf13/cobra"
)

// captureStdout redirects os.Stdout for the duration of fn and returns whatever
// was written. emitResult writes the command result to os.Stdout, so this lets
// the command tests assert on the rendered JSON/text.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(out)
}

// runDataCmd executes a freshly-built data subcommand against the test server,
// returning its stdout and error. It injects the base URL, an explicit bearer
// token (so auth resolves without touching the OS keyring), and --insecure (the
// httptest server is plain http). JIRA_OAUTH_REFRESH_TOKEN is neutralized so a
// developer's real env can't change the resolved auth mode.
func runDataCmd(
	t *testing.T,
	cmd *cobra.Command,
	serverURL string,
	extraArgs ...string,
) (string, error) {
	t.Helper()
	t.Setenv("JIRA_OAUTH_REFRESH_TOKEN", "")
	args := append([]string{
		"--env-file", "",
		"--base-url", serverURL,
		"--insecure",
		"--token", "test-token",
	}, extraArgs...)
	cmd.SetArgs(args)

	var runErr error
	out := captureStdout(t, func() {
		runErr = cmd.Execute()
	})
	return out, runErr
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b ,c ", []string{"a", "b", "c"}},
		{"a,,b", []string{"a", "b"}},
		{"", []string{}},
	}
	for _, tt := range tests {
		got := splitCSV(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("splitCSV(%q) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitCSV(%q)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
			}
		}
	}
}

func TestSearchFields(t *testing.T) {
	config := Config{epicField: "customfield_10101", sprintField: "customfield_10100"}

	// Explicit --fields wins verbatim.
	got := searchFields("summary, status ,labels", config)
	want := []string{"summary", "status", "labels"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("searchFields explicit = %v, want %v", got, want)
	}

	// Default set appends the configured custom field IDs.
	got = searchFields("", config)
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "customfield_10101") ||
		!strings.Contains(joined, "customfield_10100") {
		t.Errorf("searchFields default missing custom fields: %v", got)
	}
	if !strings.Contains(joined, "summary") || !strings.Contains(joined, "status") {
		t.Errorf("searchFields default missing base fields: %v", got)
	}
}

func TestSearchCmd(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			result := map[string]any{
				"startAt":    0,
				"maxResults": 20,
				"total":      2,
				"issues": []jira.Issue{
					{Key: "GAIA-1", Fields: &jira.IssueFields{Summary: "first"}},
					{Key: "GAIA-2", Fields: &jira.IssueFields{Summary: "second"}},
				},
			}
			_ = json.NewEncoder(w).Encode(result)
		}),
	)
	defer server.Close()

	// Happy path: JSON output contains both issues.
	out, err := runDataCmd(t, newSearchCmd(), server.URL, "--jql", "project=GAIA")
	if err != nil {
		t.Fatalf("search returned error: %v", err)
	}
	if !strings.Contains(out, "GAIA-1") || !strings.Contains(out, "GAIA-2") {
		t.Errorf("search output missing issues: %s", out)
	}

	// Text output: one tab-separated line per issue.
	out, err = runDataCmd(t, newSearchCmd(), server.URL,
		"--jql", "project=GAIA", "--output", "text")
	if err != nil {
		t.Fatalf("search text returned error: %v", err)
	}
	if lines := strings.Count(strings.TrimSpace(out), "\n"); lines != 1 {
		t.Errorf("expected 2 text lines, got %d: %q", lines+1, out)
	}
}

func TestSearchCmdMissingJQL(t *testing.T) {
	// --jql is required; cobra should reject before any request.
	_, err := runDataCmd(t, newSearchCmd(), "https://example.invalid")
	if err == nil {
		t.Fatal("expected error for missing --jql")
	}
}

func TestSearchCmdServerError(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errorMessages":["unauthorized"]}`))
		}),
	)
	defer server.Close()

	_, err := runDataCmd(t, newSearchCmd(), server.URL, "--jql", "project=GAIA")
	if err == nil {
		t.Fatal("expected error for HTTP 401")
	}
}

func TestGetCmd(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := strings.TrimPrefix(r.URL.Path, "/rest/api/2/issue/")
			_ = json.NewEncoder(w).Encode(jira.Issue{
				Key: key,
				Fields: &jira.IssueFields{
					Summary: "the summary",
					Status:  &jira.Status{Name: "Open"},
				},
			})
		}),
	)
	defer server.Close()

	out, err := runDataCmd(t, newGetCmd(), server.URL, "--key", "GAIA-123")
	if err != nil {
		t.Fatalf("get returned error: %v", err)
	}
	if !strings.Contains(out, "GAIA-123") || !strings.Contains(out, "the summary") {
		t.Errorf("get output missing fields: %s", out)
	}
}

func TestCreateCmdCustomFields(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "1", "key": "GAIA-9"})
		}),
	)
	defer server.Close()

	out, err := runDataCmd(t, newCreateCmd(), server.URL,
		"--project", "GAIA", "--summary", "do the thing",
		"--epic", "GAIA-42", "--sprint", "55",
		"--components", "api,web", "--labels", "x,y")
	if err != nil {
		t.Fatalf("create returned error: %v", err)
	}
	if !strings.Contains(out, "GAIA-9") {
		t.Errorf("create output missing key: %s", out)
	}

	fields, ok := gotBody["fields"].(map[string]any)
	if !ok {
		t.Fatalf("request body missing fields: %v", gotBody)
	}
	if fields["customfield_10101"] != "GAIA-42" {
		t.Errorf("epic custom field = %v, want GAIA-42", fields["customfield_10101"])
	}
	// JSON numbers decode as float64.
	if sprint, _ := fields["customfield_10100"].(float64); sprint != 55 {
		t.Errorf("sprint custom field = %v, want 55", fields["customfield_10100"])
	}
	if comps, ok := fields["components"].([]any); !ok || len(comps) != 2 {
		t.Errorf("components = %v, want 2 entries", fields["components"])
	}
}

func TestCreateCmdCustomFieldOverride(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"key": "GAIA-9"})
		}),
	)
	defer server.Close()

	_, err := runDataCmd(t, newCreateCmd(), server.URL,
		"--project", "GAIA", "--summary", "s",
		"--epic", "GAIA-1", "--epic-field", "customfield_99999")
	if err != nil {
		t.Fatalf("create returned error: %v", err)
	}
	fields := gotBody["fields"].(map[string]any)
	if fields["customfield_99999"] != "GAIA-1" {
		t.Errorf("override epic field not used: %v", fields)
	}
	if _, exists := fields["customfield_10101"]; exists {
		t.Errorf("default epic field should not be set when overridden: %v", fields)
	}
}

func TestUpdateCmdNothingToUpdate(t *testing.T) {
	_, err := runDataCmd(t, newUpdateCmd(), "https://example.invalid", "--key", "GAIA-1")
	if err == nil || !strings.Contains(err.Error(), "nothing to update") {
		t.Fatalf("expected 'nothing to update' error, got %v", err)
	}
}

func TestUpdateCmd(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)
	defer server.Close()

	out, err := runDataCmd(t, newUpdateCmd(), server.URL,
		"--key", "GAIA-1", "--description", "new body")
	if err != nil {
		t.Fatalf("update returned error: %v", err)
	}
	if !strings.Contains(out, "updated") || !strings.Contains(out, "GAIA-1") {
		t.Errorf("update output unexpected: %s", out)
	}
}

func TestUpdateCmdAllFields(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusNoContent)
		}),
	)
	defer server.Close()

	_, err := runDataCmd(t, newUpdateCmd(), server.URL,
		"--key", "GAIA-1",
		"--summary", "new summary",
		"--assignee", "jdoe",
		"--labels", "a,b",
		"--components", "api,web",
		"--epic", "GAIA-42",
		"--sprint", "55")
	if err != nil {
		t.Fatalf("update returned error: %v", err)
	}

	fields, ok := gotBody["fields"].(map[string]any)
	if !ok {
		t.Fatalf("request body missing fields: %v", gotBody)
	}
	if fields["summary"] != "new summary" {
		t.Errorf("summary = %v", fields["summary"])
	}
	if a, _ := fields["assignee"].(map[string]any); a == nil || a["name"] != "jdoe" {
		t.Errorf("assignee = %v", fields["assignee"])
	}
	if labels, ok := fields["labels"].([]any); !ok || len(labels) != 2 {
		t.Errorf("labels = %v", fields["labels"])
	}
	if comps, ok := fields["components"].([]any); !ok || len(comps) != 2 {
		t.Errorf("components = %v", fields["components"])
	}
	if fields["customfield_10101"] != "GAIA-42" {
		t.Errorf("epic field = %v", fields["customfield_10101"])
	}
	if sprint, _ := fields["customfield_10100"].(float64); sprint != 55 {
		t.Errorf("sprint field = %v", fields["customfield_10100"])
	}
	// description was not passed, so it must not be in the partial update.
	if _, exists := fields["description"]; exists {
		t.Errorf("description should be absent in partial update: %v", fields)
	}
}

func TestSprintsCmd(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(jira.SprintsList{
				Values: []jira.Sprint{
					{ID: 1, Name: "Sprint 1", State: "active"},
				},
			})
		}),
	)
	defer server.Close()

	out, err := runDataCmd(t, newSprintsCmd(), server.URL, "--board-id", "10381")
	if err != nil {
		t.Fatalf("sprints returned error: %v", err)
	}
	if !strings.Contains(out, "Sprint 1") {
		t.Errorf("sprints output missing sprint: %s", out)
	}
}

func TestBoardsCmd(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(jira.BoardsList{
				Values: []jira.Board{
					{ID: 7, Name: "GAIA board", Type: "scrum"},
				},
			})
		}),
	)
	defer server.Close()

	out, err := runDataCmd(t, newBoardsCmd(), server.URL, "--project", "GAIA")
	if err != nil {
		t.Fatalf("boards returned error: %v", err)
	}
	if !strings.Contains(out, "GAIA board") {
		t.Errorf("boards output missing board: %s", out)
	}
}

func TestLinkCmd(t *testing.T) {
	var gotBody jira.IssueLink
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusCreated)
		}),
	)
	defer server.Close()

	out, err := runDataCmd(t, newLinkCmd(), server.URL,
		"--from", "GAIA-1", "--to", "GAIA-2", "--link-type", "Blocks")
	if err != nil {
		t.Fatalf("link returned error: %v", err)
	}
	if !strings.Contains(out, "linked") {
		t.Errorf("link output unexpected: %s", out)
	}
	if gotBody.Type.Name != "Blocks" ||
		gotBody.InwardIssue == nil || gotBody.InwardIssue.Key != "GAIA-1" ||
		gotBody.OutwardIssue == nil || gotBody.OutwardIssue.Key != "GAIA-2" {
		t.Errorf("link request body unexpected: %+v", gotBody)
	}
}
